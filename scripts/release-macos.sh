#!/usr/bin/env bash

# SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

usage() {
  cat >&2 <<'EOF'
Usage: release-macos.sh <phase> <version> <input-directory> <output-directory>

Phases:
  sign-binaries     Sign GoReleaser-built Darwin executables with 3S (Linux).
  build-packages    Build unsigned PKGs from signed executables (macOS).
  sign-packages     Sign the outer PKGs with 3S (Linux).
  notarize-packages Notarize and staple signed PKGs (macOS).
EOF
  exit 1
}

if [[ $# -ne 4 ]]; then
  usage
fi

readonly PHASE="$1"
readonly VERSION="${2#v}"
readonly INPUT_DIR="$(cd "$3" && pwd)"
readonly OUTPUT_DIR="$(mkdir -p "$4" && cd "$4" && pwd)"
readonly APPLE_TEAM_ID="${APPLE_TEAM_ID:-6KR3T733EC}"
readonly NVSEC_SSA_SCOPE="${NVSEC_SSA_SCOPE:-SIGNING_MACOS}"
readonly WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/nvfleetctl-release.XXXXXX")"
readonly ARCHES=(amd64 arm64)

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

require_3s() {
  require_command nvsec
  require_env NVSEC_SSA_CLIENT_ID
  require_env NVSEC_SSA_CLIENT_SECRET
}

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

find_goreleaser_binary() {
  local arch="$1"
  local binary

  binary="$(find "$INPUT_DIR" -type f -name "nvfleetctl_${arch}" -print -quit)"
  if [[ -z "$binary" ]]; then
    echo "GoReleaser output not found for darwin/${arch} under $INPUT_DIR" >&2
    exit 1
  fi
  printf '%s\n' "$binary"
}

sign_binaries() {
  require_3s
  require_command find
  require_command unzip
  require_command zip

  local arch
  for arch in "${ARCHES[@]}"; do
    local arch_dir="$WORK_DIR/$arch"
    local signing_input_dir="$arch_dir/signing-input"
    local unsigned_binary
    local binary_signing_zip="$arch_dir/nvfleetctl.command.zip"
    local signed_binary_zip="$arch_dir/nvfleetctl.command.signed.zip"
    local signed_binary_dir="$arch_dir/signed-binary"
    local output_binary="$OUTPUT_DIR/nvfleetctl_${VERSION}_darwin_${arch}"

    unsigned_binary="$(find_goreleaser_binary "$arch")"
    mkdir -p "$signing_input_dir" "$signed_binary_dir"
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
    cp "$signed_binary_dir/nvfleetctl.command" "$output_binary"
    chmod 755 "$output_binary"
    echo "Created $output_binary"
  done
}

build_packages() {
  require_command codesign
  require_command pkgbuild
  require_command pkgutil

  local arch
  for arch in "${ARCHES[@]}"; do
    local arch_dir="$WORK_DIR/$arch"
    local signed_binary="$INPUT_DIR/nvfleetctl_${VERSION}_darwin_${arch}"
    local package_root="$arch_dir/package-root"
    local unsigned_package="$OUTPUT_DIR/nvfleetctl_${VERSION}_darwin_${arch}.unsigned.pkg"
    local expanded_package="$arch_dir/expanded-package"
    local payload_list="$arch_dir/package-payload.txt"

    if [[ ! -f "$signed_binary" ]]; then
      echo "Signed executable not found: $signed_binary" >&2
      exit 1
    fi

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

    pkgutil --expand-full "$unsigned_package" "$expanded_package"
    codesign --verify --strict --verbose=4 \
      "$expanded_package/Payload/usr/local/bin/nvfleetctl"
    echo "Created $unsigned_package"
  done
}

sign_packages() {
  require_3s

  local arch
  for arch in "${ARCHES[@]}"; do
    local unsigned_package="$INPUT_DIR/nvfleetctl_${VERSION}_darwin_${arch}.unsigned.pkg"
    local output_package="nvfleetctl_${VERSION}_darwin_${arch}.pkg"

    if [[ ! -f "$unsigned_package" ]]; then
      echo "Unsigned package not found: $unsigned_package" >&2
      exit 1
    fi

    submit_to_3s \
      "$unsigned_package" \
      "nvfleetctl ${VERSION} darwin/${arch} installer" \
      "$OUTPUT_DIR" \
      "$output_package"
    echo "Created $OUTPUT_DIR/$output_package"
  done
}

configure_notary_profile() {
  require_command xcrun

  notary_profile="${APPLE_NOTARY_PROFILE:-fleetint-notarytool}"
  if [[ -n "${APPLE_ID:-}" || -n "${APPLE_APP_SPECIFIC_PASSWORD:-}" ]]; then
    require_env APPLE_ID
    require_env APPLE_APP_SPECIFIC_PASSWORD
    xcrun notarytool store-credentials "$notary_profile" \
      --apple-id "$APPLE_ID" \
      --password "$APPLE_APP_SPECIFIC_PASSWORD" \
      --team-id "$APPLE_TEAM_ID"
  fi
}

notarize_packages() {
  require_command codesign
  require_command pkgutil
  require_command shasum
  require_command spctl
  configure_notary_profile

  local arch
  for arch in "${ARCHES[@]}"; do
    local input_package="$INPUT_DIR/nvfleetctl_${VERSION}_darwin_${arch}.pkg"
    local output_package="$OUTPUT_DIR/nvfleetctl_${VERSION}_darwin_${arch}.pkg"
    local expanded_package="$WORK_DIR/$arch/expanded-package"

    if [[ ! -f "$input_package" ]]; then
      echo "Signed package not found: $input_package" >&2
      exit 1
    fi

    pkgutil --check-signature "$input_package"
    mkdir -p "$(dirname "$expanded_package")"
    pkgutil --expand-full "$input_package" "$expanded_package"
    codesign --verify --strict --verbose=4 \
      "$expanded_package/Payload/usr/local/bin/nvfleetctl"

    cp "$input_package" "$output_package"
    xcrun notarytool submit "$output_package" \
      --keychain-profile "$notary_profile" \
      --wait \
      --progress
    xcrun stapler staple "$output_package"
    xcrun stapler validate "$output_package"
    spctl --assess --verbose=4 --type install "$output_package"
    echo "Created $output_package"
  done

  (
    cd "$OUTPUT_DIR"
    shasum -a 256 nvfleetctl_"${VERSION}"_darwin_*.pkg >checksums-macos.txt
  )
}

case "$PHASE" in
  sign-binaries) sign_binaries ;;
  build-packages) build_packages ;;
  sign-packages) sign_packages ;;
  notarize-packages) notarize_packages ;;
  *) usage ;;
esac

echo "Phase ${PHASE} artifacts:"
ls -lh "$OUTPUT_DIR"
