#!/usr/bin/env bash

# SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "Usage: $0 <goreleaser-target> <binary>" >&2
  exit 1
fi

target="$1"
binary="$2"

if [[ "$target" != windows_* || "${SIGN_WINDOWS_BINARIES:-false}" != "true" ]]; then
  exit 0
fi

: "${NVSEC_SSA_CLIENT_ID:?NVSEC_SSA_CLIENT_ID is required}"
: "${NVSEC_SSA_CLIENT_SECRET:?NVSEC_SSA_CLIENT_SECRET is required}"

work_dir="$(mktemp -d "${TMPDIR:-/tmp}/nvfleetctl-sign-windows.XXXXXX")"
trap 'rm -rf "$work_dir"' EXIT

args=(
  3s submit
  --job_type "${NVSEC_WINDOWS_JOB_TYPE:-WINDOWS_AUTH}"
  --scope "${NVSEC_SSA_SCOPE:-SIGNING_WINDOWS_AUTHENTICODE}"
  --auth ssa
  --input_file "$binary"
  --description "nvfleetctl ${GITHUB_REF_NAME:-snapshot} ${target}"
  --download --print_log --timeout 600
  --result_dir "$work_dir" --result_filename nvfleetctl.exe
)
[[ -z "${NSPECT_ID:-}" ]] || args+=(--nspect_id "$NSPECT_ID")

nvsec "${args[@]}"
mv "$work_dir/nvfleetctl.exe" "$binary"
