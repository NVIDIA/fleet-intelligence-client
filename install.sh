#!/usr/bin/env bash

# SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

readonly REPOSITORY="NVIDIA/fleet-intelligence-client"
readonly DEFAULT_BASE_URL="https://github.com/${REPOSITORY}/releases/download"

version="${NVFLEETINT_VERSION:-latest}"
install_dir="${NVFLEETINT_INSTALL_DIR:-${HOME}/.local/bin}"

# Download resilience. Every fetch is bounded by a connect and a total timeout
# and a fixed number of attempts, so a dropped or throttled network fails the
# install instead of hanging a provisioning pipeline indefinitely.
connect_timeout="${NVFLEETINT_CONNECT_TIMEOUT:-10}"
max_time="${NVFLEETINT_MAX_TIME:-120}"
retry_attempts="${NVFLEETINT_RETRY_ATTEMPTS:-4}"
retry_delay="${NVFLEETINT_RETRY_DELAY:-2}"
retry_max_delay="${NVFLEETINT_RETRY_MAX_DELAY:-30}"

# Fallback sources. base_url replaces the default download root; fallback_url is
# tried only after the primary is exhausted; cache_dir is consulted before the
# network and populated after a successful checksum verification.
base_url="${NVFLEETINT_BASE_URL:-$DEFAULT_BASE_URL}"
fallback_url="${NVFLEETINT_FALLBACK_BASE_URL:-}"
cache_dir="${NVFLEETINT_CACHE_DIR:-}"

usage() {
  cat <<'EOF'
Install nvfleetint for macOS or Linux.

Usage: install.sh [--version <version>] [--install-dir <directory>]

Environment variables:
  NVFLEETINT_VERSION      Release version, for example v1.2.3 (default: latest)
  NVFLEETINT_INSTALL_DIR  Installation directory (default: $HOME/.local/bin)

Download resilience:
  NVFLEETINT_CONNECT_TIMEOUT  Per-connection timeout in seconds (default: 10)
  NVFLEETINT_MAX_TIME         Per-request timeout in seconds (default: 120)
  NVFLEETINT_RETRY_ATTEMPTS   Attempts per source (default: 4)
  NVFLEETINT_RETRY_DELAY      Initial backoff in seconds, doubling (default: 2)
  NVFLEETINT_RETRY_MAX_DELAY  Maximum backoff in seconds (default: 30)

Fallback sources:
  NVFLEETINT_BASE_URL           Download root, must be https (default: GitHub releases).
                                Assets are read from <root>/<tag>/<asset>.
  NVFLEETINT_FALLBACK_BASE_URL  Mirror tried after the primary root is exhausted
  NVFLEETINT_CACHE_DIR          Local artifact cache, read before the network and
                                populated after checksum verification
EOF
}

# Rejects a non-numeric or non-positive tunable before it reaches curl or sleep
require_positive_int() {
  local name=$1 value=$2
  case "$value" in
    ''|*[!0-9]*)
      echo "Error: ${name} must be a positive integer, got: ${value}" >&2
      exit 1
      ;;
  esac
  [[ "$value" -gt 0 ]] || {
    echo "Error: ${name} must be a positive integer, got: ${value}" >&2
    exit 1
  }
}

# Keeps a caller-supplied mirror from downgrading the transport to plaintext.
# Plain http is accepted only for loopback, matching the rule the SDK applies to
# its own base URL (nvfleetint/baseurl.go) so local mock servers keep working.
require_secure_url() {
  local name=$1 value=$2
  case "$value" in
    https://*) return 0 ;;
    http://127.0.0.1|http://127.0.0.1[:/]*) return 0 ;;
    http://localhost|http://localhost[:/]*) return 0 ;;
    http://\[::1\]|http://\[::1\][:/]*) return 0 ;;
  esac

  echo "Error: ${name} must be an https:// URL (plain http is allowed only for localhost), got: ${value}" >&2
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      [[ $# -ge 2 ]] || { echo "--version requires a value" >&2; exit 1; }
      version="$2"
      shift 2
      ;;
    --install-dir)
      [[ $# -ge 2 ]] || { echo "--install-dir requires a value" >&2; exit 1; }
      install_dir="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

require_positive_int NVFLEETINT_CONNECT_TIMEOUT "$connect_timeout"
require_positive_int NVFLEETINT_MAX_TIME "$max_time"
require_positive_int NVFLEETINT_RETRY_ATTEMPTS "$retry_attempts"
require_positive_int NVFLEETINT_RETRY_DELAY "$retry_delay"
require_positive_int NVFLEETINT_RETRY_MAX_DELAY "$retry_max_delay"

require_secure_url NVFLEETINT_BASE_URL "$base_url"
base_url="${base_url%/}"
if [[ -n "$fallback_url" ]]; then
  require_secure_url NVFLEETINT_FALLBACK_BASE_URL "$fallback_url"
  fallback_url="${fallback_url%/}"
fi

# Reports whether a curl exit status is a transient transport failure.
# 6 DNS, 7 connect, 18 partial transfer, 28 timeout, 35 TLS handshake,
# 52 empty reply, 55 send error, 56 receive error.
is_retryable_curl_status() {
  case "$1" in
    6|7|18|28|35|52|55|56) return 0 ;;
    *) return 1 ;;
  esac
}

# Reports whether an HTTP status is worth another attempt. A 404 means the
# release or asset does not exist, so retrying only delays a certain failure.
is_retryable_http_code() {
  case "$1" in
    408|425|429|500|502|503|504) return 0 ;;
    *) return 1 ;;
  esac
}

# Fetches url into dest with bounded retries and exponential backoff, echoing
# the value of write_out on success. Every attempt carries an explicit connect
# and total timeout. Returns non-zero once the attempts are exhausted or the
# failure is deterministic, always after logging why.
fetch_with_retry() {
  local description=$1 url=$2 dest=$3 write_out=$4
  local attempt=1 delay="$retry_delay"
  local result status http_code payload reason

  while :; do
    status=0
    # The status code is appended last and is always three digits, so the
    # caller's write_out value can be recovered by trimming it.
    result="$(curl -sSL \
      --connect-timeout "$connect_timeout" \
      --max-time "$max_time" \
      -w "${write_out}%{http_code}" \
      -o "$dest" \
      "$url")" || status=$?

    http_code="${result: -3}"
    payload="${result%???}"

    if [[ $status -eq 0 && "$http_code" == 2?? ]]; then
      printf '%s' "$payload"
      return 0
    fi

    if [[ $status -ne 0 ]]; then
      reason="curl exit status ${status}"
      is_retryable_curl_status "$status" || {
        echo "Error: ${description} failed (${reason}); not retryable." >&2
        return 1
      }
    else
      reason="HTTP ${http_code}"
      is_retryable_http_code "$http_code" || {
        echo "Error: ${description} failed (${reason}); not retryable." >&2
        return 1
      }
    fi

    if [[ "$attempt" -ge "$retry_attempts" ]]; then
      echo "Error: ${description} failed after ${retry_attempts} attempts (${reason})." >&2
      return 1
    fi

    echo "Warning: ${description} failed (${reason}); retrying in ${delay}s (attempt $((attempt + 1))/${retry_attempts})." >&2
    sleep "$delay"
    attempt=$((attempt + 1))
    delay=$((delay * 2))
    if [[ "$delay" -gt "$retry_max_delay" ]]; then
      delay="$retry_max_delay"
    fi
  done
}

for command in awk curl find install mkdir uname; do
  command -v "$command" >/dev/null 2>&1 || {
    echo "Required command not found: $command" >&2
    exit 1
  }
done

case "$(uname -s)" in
  Darwin)
    os="darwin"
    extension="zip"
    checksum_file="checksums.txt"
    command -v unzip >/dev/null 2>&1 || { echo "Required command not found: unzip" >&2; exit 1; }
    ;;
  Linux)
    os="linux"
    extension="tar.gz"
    checksum_file="checksums.txt"
    command -v tar >/dev/null 2>&1 || { echo "Required command not found: tar" >&2; exit 1; }
    ;;
  *)
    echo "Unsupported operating system: $(uname -s)" >&2
    exit 1
    ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *)
    echo "Unsupported architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

if [[ "$version" == "latest" ]]; then
  latest_url="$(fetch_with_retry "latest release lookup" \
    "https://github.com/${REPOSITORY}/releases/latest" /dev/null '%{url_effective}')" || exit 1
  version="${latest_url##*/}"
  [[ "$version" == v* ]] || {
    echo "Could not determine the latest release version" >&2
    exit 1
  }
fi

tag="$version"
[[ "$tag" == v* ]] || tag="v${tag}"
[[ "$tag" =~ ^v[0-9][0-9A-Za-z.+-]*$ ]] || {
  echo "Invalid release version: $tag" >&2
  exit 1
}
release_version="${tag#v}"
asset="nvfleetint_${release_version}_${os}_${arch}.${extension}"
work_dir="$(mktemp -d "${TMPDIR:-/tmp}/nvfleetint-install.XXXXXX")"

cleanup() {
  case "$work_dir" in
    "${TMPDIR:-/tmp}"/nvfleetint-install.*) rm -rf "$work_dir" ;;
    *) echo "Refusing to remove unexpected temporary directory: $work_dir" >&2 ;;
  esac
}
trap cleanup EXIT

# Resolves one release file into work_dir: the cache first, then each configured
# download root in turn. Every source is fully retried before the next is tried.
obtain_file() {
  local name=$1
  local dest="${work_dir}/${name}"
  local root

  if [[ -n "$cache_dir" && -f "${cache_dir}/${tag}/${name}" ]]; then
    echo "Using cached ${name} from ${cache_dir}/${tag}"
    cp "${cache_dir}/${tag}/${name}" "$dest"
    return 0
  fi

  for root in "$base_url" ${fallback_url:+"$fallback_url"}; do
    if fetch_with_retry "download of ${name} from ${root}" \
      "${root}/${tag}/${name}" "$dest" ''; then
      return 0
    fi
    echo "Warning: giving up on ${root} for ${name}." >&2
  done

  echo "Error: could not obtain ${name} from any configured source." >&2
  return 1
}

# Stores a verified file in the cache. Only called after checksum verification,
# so a later run never reuses an artifact this run could not vouch for.
cache_store() {
  local name=$1
  [[ -n "$cache_dir" ]] || return 0
  mkdir -p "${cache_dir}/${tag}"
  cp "${work_dir}/${name}" "${cache_dir}/${tag}/${name}"
}

echo "Downloading nvfleetint ${tag} for ${os}/${arch}"
obtain_file "$asset" || exit 1
obtain_file "$checksum_file" || exit 1

expected_checksum="$(awk -v name="$asset" '$2 == name || $2 == "*" name { print $1; exit }' \
  "${work_dir}/${checksum_file}")"
[[ "$expected_checksum" =~ ^[0-9a-fA-F]{64}$ ]] || {
  echo "No valid SHA-256 checksum found for ${asset}" >&2
  exit 1
}

if command -v sha256sum >/dev/null 2>&1; then
  actual_checksum="$(sha256sum "${work_dir}/${asset}" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  actual_checksum="$(shasum -a 256 "${work_dir}/${asset}" | awk '{print $1}')"
else
  echo "A SHA-256 tool is required (sha256sum or shasum)" >&2
  exit 1
fi

[[ "$actual_checksum" == "$expected_checksum" ]] || {
  echo "Checksum verification failed for ${asset}" >&2
  exit 1
}

cache_store "$asset"
cache_store "$checksum_file"

extract_dir="${work_dir}/extract"
mkdir -p "$extract_dir"
if [[ "$extension" == "zip" ]]; then
  unzip -q "${work_dir}/${asset}" -d "$extract_dir"
else
  tar -xzf "${work_dir}/${asset}" -C "$extract_dir"
fi

binary="$(find "$extract_dir" -type f -name nvfleetint -print -quit)"
[[ -n "$binary" ]] || { echo "nvfleetint was not found in ${asset}" >&2; exit 1; }

if [[ "$os" == "darwin" ]]; then
  codesign --verify --strict --verbose=2 "$binary"
  codesign -vvvv -R="notarized" --check-notarization "$binary"
fi

mkdir -p "$install_dir"
install -m 0755 "$binary" "${install_dir}/nvfleetint"

echo "Installed nvfleetint to ${install_dir}/nvfleetint"
case ":${PATH}:" in
  *":${install_dir}:"*) ;;
  *) echo "Add ${install_dir} to PATH to run nvfleetint without its full path." ;;
esac
"${install_dir}/nvfleetint" version
