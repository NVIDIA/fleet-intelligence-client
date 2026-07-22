#!/usr/bin/env bash

# SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "Usage: $0 <version> <unsigned-package-directory> <output-directory>" >&2
  exit 1
fi

: "${NVSEC_SSA_CLIENT_ID:?NVSEC_SSA_CLIENT_ID is required}"
: "${NVSEC_SSA_CLIENT_SECRET:?NVSEC_SSA_CLIENT_SECRET is required}"

version="${1#v}"
input_dir="$(cd "$2" && pwd)"
output_dir="$(mkdir -p "$3" && cd "$3" && pwd)"

for arch in amd64 arm64; do
  input="$input_dir/nvfleetctl_${version}_darwin_${arch}.unsigned.pkg"
  output="nvfleetctl_${version}_darwin_${arch}.pkg"
  args=(
    3s submit --job_type MACOS --scope "${NVSEC_SSA_SCOPE:-SIGNING_MACOS}" --auth ssa
    --input_file "$input"
    --description "nvfleetctl ${version} darwin/${arch} installer"
    --download --print_log --timeout 600
    --result_dir "$output_dir" --result_filename "$output"
  )
  [[ -z "${NSPECT_ID:-}" ]] || args+=(--nspect_id "$NSPECT_ID")
  nvsec "${args[@]}"
done
