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

if [[ "$target" != darwin_* || "${SIGN_MACOS_BINARIES:-false}" != "true" ]]; then
  exit 0
fi

: "${NVSEC_SSA_CLIENT_ID:?NVSEC_SSA_CLIENT_ID is required}"
: "${NVSEC_SSA_CLIENT_SECRET:?NVSEC_SSA_CLIENT_SECRET is required}"

work_dir="$(mktemp -d "${TMPDIR:-/tmp}/nvfleetctl-sign-macos.XXXXXX")"
cleanup() {
  case "$work_dir" in
    "${TMPDIR:-/tmp}"/nvfleetctl-sign-macos.*) rm -rf "$work_dir" ;;
    *) echo "Refusing to remove unexpected temporary directory: $work_dir" >&2 ;;
  esac
}
trap cleanup EXIT

mkdir -p "$work_dir/input" "$work_dir/output"
cp "$binary" "$work_dir/input/nvfleetctl.command"
chmod 755 "$work_dir/input/nvfleetctl.command"
(
  cd "$work_dir/input"
  zip -q -X "$work_dir/input.zip" nvfleetctl.command
)

args=(
  3s submit
  --job_type MACOS
  --scope "${NVSEC_MACOS_SSA_SCOPE:-${NVSEC_SSA_SCOPE:-SIGNING_MACOS}}"
  --auth ssa
  --input_file "$work_dir/input.zip"
  --description "nvfleetctl ${GITHUB_REF_NAME:-snapshot} ${target}"
  --download --print_log --timeout 600
  --result_dir "$work_dir" --result_filename signed.zip
)
[[ -z "${NSPECT_ID:-}" ]] || args+=(--nspect_id "$NSPECT_ID")

nvsec "${args[@]}"
unzip -q "$work_dir/signed.zip" -d "$work_dir/output"
install -m 0755 "$work_dir/output/nvfleetctl.command" "$binary"
