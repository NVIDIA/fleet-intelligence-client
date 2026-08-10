#!/usr/bin/env bash

# SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

readonly REPOSITORY="NVIDIA/fleet-intelligence-client"

# Every network call is bounded. Without these a stalled connection leaves the
# installer -- and any CI job running it -- hanging indefinitely. --retry uses
# curl's built-in exponential backoff (1s, 2s, 4s ...); --retry-max-time caps
# the total time spent retrying, and --max-time caps each individual attempt.
readonly CONNECT_TIMEOUT=10
readonly METADATA_MAX_TIME=30
readonly DOWNLOAD_MAX_TIME=300
readonly RETRY_ATTEMPTS=3
readonly RETRY_MAX_TIME=120

version="${NVFLEETINT_VERSION:-latest}"
install_dir="${NVFLEETINT_INSTALL_DIR:-${HOME}/.local/bin}"

usage() {
  cat <<'EOF'
Install nvfleetint for macOS or Linux.

Usage: install.sh [--version <version>] [--install-dir <directory>]

Environment variables:
  NVFLEETINT_VERSION      Release version, for example v1.2.3 (default: latest)
  NVFLEETINT_INSTALL_DIR  Installation directory (default: $HOME/.local/bin)
EOF
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

curl_retry_opts=(
  --connect-timeout "$CONNECT_TIMEOUT"
  --retry "$RETRY_ATTEMPTS"
  --retry-max-time "$RETRY_MAX_TIME"
)

if [[ "$version" == "latest" ]]; then
  latest_url="$(curl -fsSL "${curl_retry_opts[@]}" --max-time "$METADATA_MAX_TIME" \
    -o /dev/null -w '%{url_effective}' \
    "https://github.com/${REPOSITORY}/releases/latest")"
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
base_url="https://github.com/${REPOSITORY}/releases/download/${tag}"
work_dir="$(mktemp -d "${TMPDIR:-/tmp}/nvfleetint-install.XXXXXX")"

cleanup() {
  case "$work_dir" in
    "${TMPDIR:-/tmp}"/nvfleetint-install.*) rm -rf "$work_dir" ;;
    *) echo "Refusing to remove unexpected temporary directory: $work_dir" >&2 ;;
  esac
}
trap cleanup EXIT

echo "Downloading nvfleetint ${tag} for ${os}/${arch}"
curl -fsSL "${curl_retry_opts[@]}" --max-time "$DOWNLOAD_MAX_TIME" \
  "${base_url}/${asset}" -o "${work_dir}/${asset}"
curl -fsSL "${curl_retry_opts[@]}" --max-time "$DOWNLOAD_MAX_TIME" \
  "${base_url}/${checksum_file}" -o "${work_dir}/${checksum_file}"

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
