#!/usr/bin/env bash
# SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

repo_root=$(cd -P -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
# Portable skills cannot reference sibling skill directories. Keep nvfleetint's
# copy canonical in the repository and package byte-identical copies with the two
# report skills; validate-skills.sh rejects drift.
canonical="$repo_root/skills/nvfleetint/references/cli-contract.md"

targets=(
  "$repo_root/skills/fleet-health-report/references/cli-contract.md"
  "$repo_root/skills/node-rca-rcca/references/cli-contract.md"
)
for target in "${targets[@]}"; do
  cp -- "$canonical" "$target"
done

echo "Synchronized portable CLI contract copies."
