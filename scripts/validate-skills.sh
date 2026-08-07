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
  'nvfleetint auth list --output json'
  'nvfleetint auth status --profile <profile> --output json'
  'same explicit `--profile <profile>`'
  'Probe each list with the same filters, `--view basic` where supported'
  'Resolve supplied names to IDs'
  'alert options --view active'
  'alert summary <scope>'
  '--sort-by alert --order desc --page-size 10'
  '--component-type <component-types> --all'
  'agent_liveness'
  'comma-separated list of component IDs excluding exact IDs `psirt` and `agent_liveness`'
  'Alert collection invokes at most 12 `nvfleetint` commands'
  'An `--all` command may make multiple paginated requests.'
  'Do not fetch every affected node merely to count or rank them.'
  'Nodes with Active Alerts'
  'Machines Needing Immediate Attention (up to 10)'
  '1. Fleet Summary:'
  '2. Fleet Distribution:'
  '3. Active Alerts:'
  '4. Error Distribution:'
  'Fleet Health Percentage'
  'Do not show both or repeat the percentage'
  'report error --view list --group-by error'
  'without an aggregate fleet severity badge'
  'date -u -d "@$now"'
  'date -u -r "$now" -v-24H'
)
for required in "${required_fleet_rules[@]}"; do
  if ! grep -Fq -- "$required" "$fleet_skill"; then
    fail "$fleet_skill is missing required collection rule: $required"
  fi
done

collection_order=(
  'nvfleetint auth list --output json'
  'nvfleetint auth status --profile <profile> --output json'
  'nvfleetint overview --profile <profile> --output json'
  'nvfleetint computezone list'
  'nvfleetint nodegroup list'
  'nvfleetint node list <scope>'
  'nvfleetint report error'
  'nvfleetint alert options'
  'nvfleetint alert summary'
  'nvfleetint alert node'
)
previous_line=0
for command in "${collection_order[@]}"; do
  line=$(awk -v needle="$command" 'index($0, needle) { print NR; exit }' "$fleet_skill")
  if [ -z "$line" ]; then
    fail "$fleet_skill is missing ordered workflow command: $command"
  fi
  if [ "$line" -le "$previous_line" ]; then
    fail "$fleet_skill has workflow command out of order: $command"
  fi
  previous_line=$line
done

for obsolete in '5,000 alerts' 'alert list --severity Critical --all' \
  'alert list --severity Warning --all' 'Critical.count + Warning.count' \
  'alert list --all --output json' 'prev_start' 'id="trend"' 'Overall status:' \
  'alert node <node_uuid> --view active --without-psirt' 'server-ranked top 10' \
  'only the top 10' 'top-10 summary nodes' 'top-10 drill-down' \
  'top 10 machines' 'top machines needing attention'; do
  if grep -Fq "$obsolete" "$fleet_skill"; then
    fail "$fleet_skill contains obsolete fleet-report guidance: $obsolete"
  fi
done

nvfleet_skill="$repo_root/skills/nvfleetint/SKILL.md"
required_nvfleet_rules=(
  '--compute-zone-names'
  '--nodegroup-names'
  'supports sorting only by `hostname` or `nodeUUID`'
  'alert summary --output json'
  'alert node <node-uuid> --output json'
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
for required in 'nvfleetint auth list --output json' \
  'nvfleetint auth status --profile <profile> --output json' \
  'node list --hostname <hostname> --view basic --all' \
  'node describe <node_uuid> --profile <profile> --output json' \
  'Fetch current alerts and complete historical alerts' \
  'Aggregate current and historical alerts by component ID/display name and status' \
  'deduplicating the same `alertUuid` across both sets' \
  'count prior historical rows with the same component ID after excluding its own `alertUuid`' \
  'Describe every unique current alert' \
  'Use each description' \
  'timeline, messages, errors, incidents, and suggested actions' \
  'Run at most four describe calls concurrently' \
  'collapsed `<details>` breakdown for every current alert' \
  'Prefer official NVIDIA documentation' \
  'Never include hostname, node UUID, profile, tenant, or customer data in a query'; do
  if ! grep -Fq "$required" "$node_skill"; then
    fail "$node_skill is missing required RCA workflow guidance: $required"
  fi
done
for required in 'alert node <node_uuid> --without-psirt --all' \
  'alert node <node_uuid> --view historical --without-psirt --all' \
  'alert describe <alert_uuid> --node <node_uuid> --profile <profile> --output json'; do
  if ! grep -Fq "$required" "$node_skill"; then
    fail "$node_skill is missing required alert command: $required"
  fi
done

node_collection_order=(
  'nvfleetint auth list'
  'nvfleetint auth status'
  'nvfleetint node list'
  'nvfleetint node describe'
  'nvfleetint alert node <node_uuid> --without-psirt'
  'nvfleetint alert node <node_uuid> --view historical'
  'nvfleetint alert describe <alert_uuid>'
)
previous_line=0
for command in "${node_collection_order[@]}"; do
  line=$(awk -v needle="$command" 'index($0, needle) { print NR; exit }' "$node_skill")
  if [ -z "$line" ]; then
    fail "$node_skill is missing ordered workflow command: $command"
  fi
  if [ "$line" -le "$previous_line" ]; then
    fail "$node_skill has workflow command out of order: $command"
  fi
  previous_line=$line
done

if grep -R -n --include='*.md' 'nvfleetint alert timeline' "$repo_root/skills" >/dev/null; then
  fail "skills reference the removed alert timeline command"
fi
if grep -niE '\bevents?\b|nvfleetint event (list|buckets)' "$node_skill" >/dev/null; then
  fail "node-rca-rcca must use alert evidence without event API calls"
fi
if grep -niE '48-hour|172800|date -u|alert window' "$node_skill" >/dev/null; then
  fail "node-rca-rcca must use the complete historical alert response without a local time cutoff"
fi

fleet_theme="$repo_root/skills/fleet-health-report/references/html-theme.md"
node_theme="$repo_root/skills/node-rca-rcca/references/html-theme.md"
fleet_workspace="$repo_root/skills/fleet-health-report/references/workspace.md"
node_workspace="$repo_root/skills/node-rca-rcca/references/workspace.md"

for report_skill in fleet-health-report node-rca-rcca; do
  references="$repo_root/skills/$report_skill/references"
  for reference in cli-contract.md html-theme.md workspace.md; do
    if [ ! -f "$references/$reference" ]; then
      fail "$report_skill is missing shared reference $reference"
    fi
  done
  reference_count=$(find "$references" -mindepth 1 -maxdepth 1 -type f | wc -l | tr -d ' ')
  if [ "$reference_count" -ne 3 ]; then
    fail "$report_skill must contain exactly the three shared references"
  fi
done

if ! cmp -s "$fleet_theme" "$node_theme"; then
  fail "report HTML theme references differ; run scripts/sync-skill-cli-contract.sh"
fi
if ! cmp -s "$fleet_workspace" "$node_workspace"; then
  fail "report workspace references differ; run scripts/sync-skill-cli-contract.sh"
fi

for theme in "$fleet_theme" "$node_theme"; do
  for theme_rule in '--surface-canvas: #f7f7f7' 'html[data-theme="dark"]' \
    'maximum width 1180px' '16px corners' '<details>' 'Unverified' 'For print'; do
    if ! grep -Fq -- "$theme_rule" "$theme"; then
      fail "$theme is missing shared theme guidance: $theme_rule"
    fi
  done
  for report_section in at-a-glance distribution errors alerts summary node-details root-cause; do
    if grep -Fq "id=\"$report_section\"" "$theme"; then
      fail "$theme must not define report-specific section $report_section"
    fi
  done
done

for workspace in "$fleet_workspace" "$node_workspace"; do
  for workspace_rule in 'mktemp -d "/tmp/nvfleet-report.XXXXXXXX"' \
    'Write the final HTML outside the scratch directory' \
    'find "$work" -mindepth 1 -maxdepth 1' 'rmdir -- "$work"'; do
    if ! grep -Fq -- "$workspace_rule" "$workspace"; then
      fail "$workspace is missing shared workspace guidance: $workspace_rule"
    fi
  done
done

for section_id in at-a-glance distribution errors alerts; do
  if ! grep -Fq "$section_id" "$fleet_skill"; then
    fail "$fleet_skill is missing required section check $section_id"
  fi
done
fleet_section_order=(
  '1. Fleet Summary:'
  '2. Fleet Distribution:'
  '3. Active Alerts:'
  '4. Error Distribution:'
)
previous_line=0
for section in "${fleet_section_order[@]}"; do
  line=$(awk -v needle="$section" 'index($0, needle) { print NR; exit }' "$fleet_skill")
  if [ -z "$line" ] || [ "$line" -le "$previous_line" ]; then
    fail "$fleet_skill has a missing or out-of-order report section: $section"
  fi
  previous_line=$line
done
for section_id in summary node-details evidence root-cause corrective-actions references unknowns; do
  if ! grep -Fq "$section_id" "$node_skill"; then
    fail "$node_skill is missing required section id $section_id"
  fi
done
node_section_order=(
  '1. Executive Summary:'
  '2. Node Details:'
  '3. Alert Evidence:'
  '4. Root Cause Analysis:'
  '5. Corrective Action Plan:'
  '6. References:'
  '7. Assumptions and Unknowns:'
)
previous_line=0
for section in "${node_section_order[@]}"; do
  line=$(awk -v needle="$section" 'index($0, needle) { print NR; exit }' "$node_skill")
  if [ -z "$line" ] || [ "$line" -le "$previous_line" ]; then
    fail "$node_skill has a missing or out-of-order report section: $section"
  fi
  previous_line=$line
done

echo "All agent skills are valid."
