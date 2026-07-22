#!/usr/bin/env bash

# SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "Usage: $0 <version> <output-directory>" >&2
  exit 1
fi

readonly REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly VERSION="${1#v}"
readonly OUTPUT_DIR="$(mkdir -p "$2" && cd "$2" && pwd)"
readonly APPLE_TEAM_ID="${APPLE_TEAM_ID:-6KR3T733EC}"
readonly NVSEC_SSA_SCOPE="${NVSEC_SSA_SCOPE:-SIGNING_MACOS}"
readonly COMMIT="${COMMIT:-$(git -C "$REPO_ROOT" rev-parse --short HEAD)}"
readonly BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
readonly WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/nvfleetctl-release.XXXXXX")"

cleanup() {
  case "$WORK_DIR" in
    "${TMPDIR:-/tmp}"/nvfleetctl-release.*) rm -rf "$WORK_DIR" ;;
    *) echo "Refusing to remove unexpected work directory: $WORK_DIR" >&2 ;;
  esac
}
trap cleanup EXIT

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Required command not found: $1" >&2
    exit 1
  fi
}

require_env() {
  if [[ -z "${!1:-}" ]]; then
    echo "Required environment variable is not set: $1" >&2
    exit 1
  fi
}

for command in codesign go nvsec pkgbuild pkgutil spctl unzip xcrun zip; do
  require_command "$command"
done

require_env NVSEC_SSA_CLIENT_ID
require_env NVSEC_SSA_CLIENT_SECRET

notary_profile="${APPLE_NOTARY_PROFILE:-fleetint-notarytool}"
if [[ -n "${APPLE_ID:-}" || -n "${APPLE_APP_SPECIFIC_PASSWORD:-}" ]]; then
  require_env APPLE_ID
  require_env APPLE_APP_SPECIFIC_PASSWORD
  xcrun notarytool store-credentials "$notary_profile" \
    --apple-id "$APPLE_ID" \
    --password "$APPLE_APP_SPECIFIC_PASSWORD" \
    --team-id "$APPLE_TEAM_ID"
fi

submit_to_3s() {
  local input_file="$1"
  local description="$2"
  local result_dir="$3"
  local result_filename="$4"
  local -a args=(
    3s submit
    --job_type MACOS
    --scope "$NVSEC_SSA_SCOPE"
    --auth ssa
    --input_file "$input_file"
    --description "$description"
    --download
    --print_log
    --result_dir "$result_dir"
    --result_filename "$result_filename"
    --timeout 600
  )

  if [[ -n "${NSPECT_ID:-}" ]]; then
    args+=(--nspect_id "$NSPECT_ID")
  fi

  nvsec "${args[@]}"
}

build_and_release_arch() {
  local arch="$1"
  local arch_dir="$WORK_DIR/$arch"
  local unsigned_binary="$arch_dir/unsigned/nvfleetctl"
  local signing_input_dir="$arch_dir/signing-input"
  local binary_signing_zip="$arch_dir/nvfleetctl.command.zip"
  local signed_binary_zip="$arch_dir/nvfleetctl.command.signed.zip"
  local signed_binary_dir="$arch_dir/signed-binary"
  local signed_binary="$signed_binary_dir/nvfleetctl"
  local package_root="$arch_dir/package-root"
  local unsigned_package="$arch_dir/nvfleetctl.pkg"
  local signed_package="$arch_dir/nvfleetctl.signed.pkg"
  local expanded_package="$arch_dir/expanded-package"
  local final_package="$OUTPUT_DIR/nvfleetctl_${VERSION}_darwin_${arch}.pkg"
  local payload_list="$arch_dir/package-payload.txt"

  mkdir -p "$(dirname "$unsigned_binary")" "$signing_input_dir" "$signed_binary_dir"

  echo "Building nvfleetctl ${VERSION} for darwin/${arch}"
  (
    cd "$REPO_ROOT"
    CGO_ENABLED=0 GOOS=darwin GOARCH="$arch" go build \
      -trimpath \
      -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_DATE}" \
      -o "$unsigned_binary" \
      ./cmd/nvfleetctl
  )

  # The shared 3S MACOS job accepts a standalone executable when it is wrapped
  # in a ZIP and given a recognized command-file extension.
  cp "$unsigned_binary" "$signing_input_dir/nvfleetctl.command"
  chmod 755 "$signing_input_dir/nvfleetctl.command"
  (
    cd "$signing_input_dir"
    COPYFILE_DISABLE=1 zip -q -X "$binary_signing_zip" nvfleetctl.command
  )

  submit_to_3s \
    "$binary_signing_zip" \
    "nvfleetctl ${VERSION} darwin/${arch} executable" \
    "$arch_dir" \
    "$(basename "$signed_binary_zip")"

  COPYFILE_DISABLE=1 unzip -q "$signed_binary_zip" -d "$signed_binary_dir"
  mv "$signed_binary_dir/nvfleetctl.command" "$signed_binary"
  chmod 755 "$signed_binary"

  codesign --verify --strict --verbose=4 "$signed_binary"
  codesign -dvvv "$signed_binary" 2>&1

  mkdir -p "$package_root/usr/local/bin"
  cp "$signed_binary" "$package_root/usr/local/bin/nvfleetctl"
  chmod 755 "$package_root/usr/local/bin/nvfleetctl"

  COPYFILE_DISABLE=1 pkgbuild \
    --root "$package_root" \
    --identifier com.nvidia.fleetintelligence.nvfleetctl \
    --version "$VERSION" \
    --install-location / \
    "$unsigned_package"

  pkgutil --payload-files "$unsigned_package" >"$payload_list"
  if grep -Eq '(^|/)\._' "$payload_list"; then
    echo "Package contains unexpected AppleDouble metadata:" >&2
    grep -E '(^|/)\._' "$payload_list" >&2
    exit 1
  fi

  submit_to_3s \
    "$unsigned_package" \
    "nvfleetctl ${VERSION} darwin/${arch} installer" \
    "$arch_dir" \
    "$(basename "$signed_package")"

  pkgutil --check-signature "$signed_package"
  pkgutil --expand-full "$signed_package" "$expanded_package"
  codesign --verify --strict --verbose=4 "$expanded_package/Payload/usr/local/bin/nvfleetctl"

  xcrun notarytool submit "$signed_package" \
    --keychain-profile "$notary_profile" \
    --wait \
    --progress
  xcrun stapler staple "$signed_package"
  xcrun stapler validate "$signed_package"
  spctl --assess --verbose=4 --type install "$signed_package"

  cp "$signed_package" "$final_package"
  echo "Created $final_package"
}

build_and_release_arch amd64
build_and_release_arch arm64

(
  cd "$OUTPUT_DIR"
  shasum -a 256 nvfleetctl_"${VERSION}"_darwin_*.pkg >checksums-macos.txt
)

echo "macOS release artifacts:"
ls -lh "$OUTPUT_DIR"
