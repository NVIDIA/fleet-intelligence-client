#!/usr/bin/env bash
# SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

repo_root=$(cd -P -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
# Portable skills cannot reference sibling skill directories. Package
# byte-identical shared references inside both report skills; validate-skills.sh
# rejects drift.
canonical="$repo_root/skills/nvfleetint/references/cli-contract.md"

targets=(
  "$repo_root/skills/fleet-health-report/references/cli-contract.md"
  "$repo_root/skills/node-rca-rcca/references/cli-contract.md"
)
for target in "${targets[@]}"; do
  cp -- "$canonical" "$target"
done

cp -- "$repo_root/skills/fleet-health-report/references/html-theme.md" \
  "$repo_root/skills/node-rca-rcca/references/html-theme.md"
cp -- "$repo_root/skills/fleet-health-report/references/workspace.md" \
  "$repo_root/skills/node-rca-rcca/references/workspace.md"

echo "Synchronized portable shared report references."
