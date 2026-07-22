#!/usr/bin/env bash

# SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "Usage: $0 <version> <signed-package-directory> <output-directory>" >&2
  exit 1
fi

version="${1#v}"
input_dir="$(cd "$2" && pwd)"
output_dir="$(mkdir -p "$3" && cd "$3" && pwd)"
profile="${APPLE_NOTARY_PROFILE:-fleetint-notarytool}"
team_id="${APPLE_TEAM_ID:-6KR3T733EC}"

if [[ -n "${APPLE_ID:-}" || -n "${APPLE_APP_SPECIFIC_PASSWORD:-}" ]]; then
  : "${APPLE_ID:?APPLE_ID is required}"
  : "${APPLE_APP_SPECIFIC_PASSWORD:?APPLE_APP_SPECIFIC_PASSWORD is required}"
  xcrun notarytool store-credentials "$profile" \
    --apple-id "$APPLE_ID" \
    --password "$APPLE_APP_SPECIFIC_PASSWORD" \
    --team-id "$team_id"
fi

for arch in amd64 arm64; do
  input="$input_dir/nvfleetctl_${version}_darwin_${arch}.pkg"
  output="$output_dir/nvfleetctl_${version}_darwin_${arch}.pkg"

  pkgutil --check-signature "$input"
  cp "$input" "$output"
  xcrun notarytool submit "$output" --keychain-profile "$profile" --wait --progress
  xcrun stapler staple "$output"
  xcrun stapler validate "$output"
  spctl --assess --verbose=4 --type install "$output"
done

(cd "$output_dir" && shasum -a 256 nvfleetctl_"${version}"_darwin_*.pkg >checksums-macos.txt)
