#!/usr/bin/env bash

# SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "Usage: $0 <version> <goreleaser-dist> <output-directory>" >&2
  exit 1
fi

: "${NVSEC_SSA_CLIENT_ID:?NVSEC_SSA_CLIENT_ID is required}"
: "${NVSEC_SSA_CLIENT_SECRET:?NVSEC_SSA_CLIENT_SECRET is required}"

version="${1#v}"
input_dir="$(cd "$2" && pwd)"
output_dir="$(mkdir -p "$3" && cd "$3" && pwd)"
work_dir="$(mktemp -d "${TMPDIR:-/tmp}/nvfleetctl-sign-binaries.XXXXXX")"
trap 'rm -rf "$work_dir"' EXIT

for arch in amd64 arm64; do
  binary="$(find "$input_dir" -type f -name "nvfleetctl_${arch}" -print -quit)"
  if [[ -z "$binary" ]]; then
    echo "GoReleaser output not found for darwin/${arch}" >&2
    exit 1
  fi

  arch_dir="$work_dir/$arch"
  mkdir -p "$arch_dir/input" "$arch_dir/output"
  cp "$binary" "$arch_dir/input/nvfleetctl.command"
  chmod 755 "$arch_dir/input/nvfleetctl.command"
  (cd "$arch_dir/input" && zip -q -X "$arch_dir/input.zip" nvfleetctl.command)

  args=(
    3s submit --job_type MACOS --scope "${NVSEC_SSA_SCOPE:-SIGNING_MACOS}" --auth ssa
    --input_file "$arch_dir/input.zip"
    --description "nvfleetctl ${version} darwin/${arch} executable"
    --download --print_log --timeout 600
    --result_dir "$arch_dir" --result_filename signed.zip
  )
  [[ -z "${NSPECT_ID:-}" ]] || args+=(--nspect_id "$NSPECT_ID")
  nvsec "${args[@]}"

  unzip -q "$arch_dir/signed.zip" -d "$arch_dir/output"
  cp "$arch_dir/output/nvfleetctl.command" "$output_dir/nvfleetctl_${version}_darwin_${arch}"
  chmod 755 "$output_dir/nvfleetctl_${version}_darwin_${arch}"
done
