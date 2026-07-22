#!/usr/bin/env bash

# SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "Usage: $0 <version> <signed-binary-directory> <output-directory>" >&2
  exit 1
fi

version="${1#v}"
input_dir="$(cd "$2" && pwd)"
output_dir="$(mkdir -p "$3" && cd "$3" && pwd)"
work_dir="$(mktemp -d "${TMPDIR:-/tmp}/nvfleetctl-build-packages.XXXXXX")"
trap 'rm -rf "$work_dir"' EXIT

for arch in amd64 arm64; do
  binary="$input_dir/nvfleetctl_${version}_darwin_${arch}"
  package="$output_dir/nvfleetctl_${version}_darwin_${arch}.unsigned.pkg"
  package_root="$work_dir/$arch/root"
  payload_list="$work_dir/$arch/payload.txt"

  chmod 755 "$binary"
  codesign --verify --strict --verbose=4 "$binary"

  mkdir -p "$package_root/usr/local/bin"
  cp "$binary" "$package_root/usr/local/bin/nvfleetctl"
  chmod 755 "$package_root/usr/local/bin/nvfleetctl"

  COPYFILE_DISABLE=1 pkgbuild \
    --root "$package_root" \
    --identifier com.nvidia.fleetintelligence.nvfleetctl \
    --version "$version" \
    --install-location / \
    "$package"

  pkgutil --payload-files "$package" >"$payload_list"
  if grep -Eq '(^|/)\._' "$payload_list"; then
    echo "Package contains AppleDouble metadata:" >&2
    grep -E '(^|/)\._' "$payload_list" >&2
    exit 1
  fi
done
