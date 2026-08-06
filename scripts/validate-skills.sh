#!/usr/bin/env bash
# SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

fail() {
  echo "skills validation failed: $*" >&2
  exit 1
}

repo_root=$(cd -P -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)

found=0
for skill_md in skills/*/SKILL.md; do
  [ -f "$skill_md" ] || continue
  found=1

  skill_dir=${skill_md%/SKILL.md}
  folder_name=${skill_dir##*/}
  [ "$(sed -n '1p' "$skill_md")" = "---" ] \
    || fail "$skill_md must start with YAML frontmatter"
  closing_line=$(awk 'NR > 1 && $0 == "---" { print NR; exit }' "$skill_md")
  [ -n "$closing_line" ] || fail "$skill_md has no closing frontmatter delimiter"

  # Support only the portable YAML subset used here: two one-line plain scalars.
  frontmatter=$(sed -n "2,$((closing_line - 1))p" "$skill_md")
  fields=$(printf '%s\n' "$frontmatter" | sed '/^[[:space:]]*$/d; /^[[:space:]]*#/d')
  [ "$(printf '%s\n' "$fields" | wc -l | tr -d ' ')" -eq 2 ] \
    || fail "$skill_md frontmatter must contain only name and description"
  printf '%s\n' "$fields" | grep -Eq '^name: [a-z0-9]+(-[a-z0-9]+)*$' \
    || fail "$skill_md has an invalid name"
  printf '%s\n' "$fields" | grep -Eq '^description: [A-Za-z0-9][A-Za-z0-9 .,;/()_-]*$' \
    || fail "$skill_md description must use the supported one-line plain-text format"

  name=$(printf '%s\n' "$fields" | sed -n 's/^name: //p')
  description=$(printf '%s\n' "$fields" | sed -n 's/^description: //p')
  [ "$name" = "$folder_name" ] \
    || fail "$skill_md name '$name' must match folder '$folder_name'"
  [ "${#name}" -le 64 ] || fail "$skill_md name exceeds 64 characters"
  [ "${#description}" -le 1024 ] || fail "$skill_md description exceeds 1024 characters"
  [ "$(wc -l < "$skill_md" | tr -d ' ')" -le 500 ] \
    || fail "$skill_md exceeds 500 lines"

  openai_yaml="$skill_dir/agents/openai.yaml"
  [ -f "$openai_yaml" ] || fail "$skill_dir is missing agents/openai.yaml"
  [ "$(grep -Ec '^interface:$' "$openai_yaml")" -eq 1 ] \
    || fail "$openai_yaml must contain one interface mapping"
  invalid_openai=$(sed -nE \
    '/^[[:space:]]*($|#)/d; /^interface:$/d; /^  (display_name|short_description|default_prompt): "[^"\\]+"$/d; p' \
    "$openai_yaml")
  [ -z "$invalid_openai" ] \
    || fail "$openai_yaml must contain interface and three simple quoted string fields"
  [ "$(grep -Ec '^  (display_name|short_description|default_prompt):' "$openai_yaml")" -eq 3 ] \
    || fail "$openai_yaml is missing required interface fields"
  short_description=$(sed -nE 's/^  short_description: "([^"\\]+)"$/\1/p' "$openai_yaml")
  [ "${#short_description}" -ge 25 ] && [ "${#short_description}" -le 64 ] \
    || fail "$openai_yaml short_description must contain 25-64 characters"
  default_prompt=$(sed -nE 's/^  default_prompt: "([^"\\]+)"$/\1/p' "$openai_yaml")
  printf '%s' "$default_prompt" | grep -Fq "\$$name" \
    || fail "$openai_yaml default prompt must mention \$$name"

  while IFS= read -r reference; do
    [ -n "$reference" ] || continue
    case "$reference" in
      http://*|https://*|mailto:*) continue ;;
      /*|[A-Za-z]:/*|[A-Za-z]:\\*) fail "$skill_md contains absolute reference $reference" ;;
      *\\*) fail "$skill_md contains non-portable reference $reference" ;;
    esac
    case "/$reference/" in
      */../*) fail "$skill_md contains escaping reference $reference" ;;
    esac
    case "$reference" in
      references/*) ;;
      *) continue ;;
    esac

    candidate="$skill_dir/$reference"
    [ -f "$candidate" ] || fail "$skill_md references missing file $reference"
    [ ! -L "$candidate" ] || fail "$skill_md reference must not be a symlink: $reference"
    skill_root=$(cd -P -- "$skill_dir" && pwd -P)
    references_root=$(cd -P -- "$skill_dir/references" && pwd -P)
    candidate_dir=$(cd -P -- "${candidate%/*}" && pwd -P)
    case "$references_root/" in "$skill_root/"*) ;; *) fail "$skill_md references directory escapes skill directory" ;; esac
    case "$candidate_dir/" in "$references_root/"*) ;; *) fail "$skill_md reference escapes references directory: $reference" ;; esac
  done < <(grep -Eo '\]\([^#)[:space:]]+' "$skill_md" | sed 's/^](//' | sort -u || true)
done

[ "$found" -eq 1 ] || fail "no skills/*/SKILL.md files found"

canonical_contract="$repo_root/skills/nvfleetint/references/cli-contract.md"
contracts=(
  "$repo_root/skills/fleet-health-report/references/cli-contract.md"
  "$repo_root/skills/node-rca-rcca/references/cli-contract.md"
)
for contract in "${contracts[@]}"; do
  if ! cmp -s "$canonical_contract" "$contract"; then
    fail "$contract differs from the canonical CLI contract; run scripts/sync-skill-cli-contract.sh"
  fi
done

if grep -R -n --include='*.md' -- '--timeout 60s' "$repo_root/skills" >/dev/null; then
  fail "skills contain the obsolete shortened --timeout 60s example"
fi
if grep -R -nE --include='*.md' 'nvfleetctl|pkg/fleetintelligence' "$repo_root/skills" >/dev/null; then
  fail "skills contain a removed client or package name"
fi
if grep -R -nE --include='*.md' 'node list.*--sort-by health([[:space:]]|$)' "$repo_root/skills" >/dev/null; then
  fail "skills use invalid node sort key health; use healthStatus"
fi
if grep -R -nE --include='*.md' 'rsync.*--delete.*(\.agents|skills/)' "$repo_root/skills" >/dev/null; then
  fail "skills contain a broad destructive install command"
fi

fleet_skill="$repo_root/skills/fleet-health-report/SKILL.md"
required_fleet_rules=(
  'Probe before `--all`'
  'Never ask the user for IDs'
  'Use **5,000 alerts** as the fixed threshold'
  'alert list --severity Critical --all'
  'alert list --severity Warning --all'
  'Give alert collection at most 10 minutes'
  'Critical.count + Warning.count'
  'date -u -d "@$now"'
  'date -u -r "$now" -v-24H'
  'date -u -r "$now" -v-48H'
)
for required in "${required_fleet_rules[@]}"; do
  if ! grep -Fq "$required" "$fleet_skill"; then
    fail "$fleet_skill is missing required large-fleet rule: $required"
  fi
done

nvfleet_skill="$repo_root/skills/nvfleetint/SKILL.md"
required_nvfleet_rules=(
  '--compute-zone-names'
  '--nodegroup-names'
  'supports sorting only by `hostname` or `nodeUUID`'
  'alert timeline --active'
  'It does not support'
)
for required in "${required_nvfleet_rules[@]}"; do
  if ! grep -Fq -- "$required" "$nvfleet_skill"; then
    fail "$nvfleet_skill is missing required CLI guidance: $required"
  fi
done

for required in healthNodeCount '--view basic' '--firmware-check'; do
  if ! grep -Fq -- "$required" "$canonical_contract"; then
    fail "$canonical_contract is missing required shared guidance: $required"
  fi
done

node_skill="$repo_root/skills/node-rca-rcca/SKILL.md"
for required in 'date -u -d "@$now"' 'date -u -r "$now" -v-7d' 'now - 604800'; do
  if ! grep -Fq "$required" "$node_skill"; then
    fail "$node_skill is missing required cross-platform date rule: $required"
  fi
done
for command in 'event list' 'event buckets'; do
  if ! grep -E "$command.*--start .*--end " "$node_skill" >/dev/null; then
    fail "$node_skill must use pinned --start/--end for $command"
  fi
  if grep -E "$command.*--window" "$node_skill" >/dev/null; then
    fail "$node_skill must not use a drifting relative window for $command"
  fi
done
node_rca_dir="$repo_root/skills/node-rca-rcca"
if grep -R -nE --include='*.md' 'event (list|buckets).*--window' "$node_rca_dir" >/dev/null; then
  fail "node-rca-rcca contains a drifting relative event window"
fi

fleet_template="$repo_root/skills/fleet-health-report/references/html-report-template.md"
node_template="$repo_root/skills/node-rca-rcca/references/html-report-template.md"
extract_root_tokens() {
  awk '
    /^    :root \{$/ { active = 1 }
    active { print }
    active && /^    \}$/ { exit }
  ' "$1"
}
fleet_root_tokens=$(extract_root_tokens "$fleet_template")
node_root_tokens=$(extract_root_tokens "$node_template")
if [ "$fleet_root_tokens" != "$node_root_tokens" ]; then
  fail "HTML template :root token blocks differ"
fi
for template in "$fleet_template" "$node_template"; do
  if ! grep -Fq 'Unverified' "$template"; then
    fail "$template must map Unverified to a status style"
  fi
done
if ! grep -Fq '[backend_health_badge]' "$fleet_template"; then
  fail "$fleet_template must make the backend-health badge conditional"
fi
if grep -Fq '[backend_health_percentage]' "$fleet_template"; then
  fail "$fleet_template contains the obsolete unconditional backend-health placeholder"
fi
for section_id in at-a-glance distribution trend concentration attention; do
  if ! grep -Fq "$section_id" "$fleet_skill"; then
    fail "$fleet_skill is missing required section check $section_id"
  fi
done

report_writing="$repo_root/skills/node-rca-rcca/references/report-writing.md"
for section_id in summary node-details impact timeline root-cause contributing-factors actions validation evidence unknowns; do
  if ! grep -Fq "$section_id" "$report_writing"; then
    fail "$report_writing is missing required section id $section_id"
  fi
done

echo "All agent skills are valid."
