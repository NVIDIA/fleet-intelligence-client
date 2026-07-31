#!/usr/bin/env bash
# SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

fail() {
  echo "skills validation failed: $*" >&2
  exit 1
}

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

  keys=$(sed -n "2,$((closing_line - 1))p" "$skill_md" \
    | sed -nE '/^[[:space:]]*($|#)/d; s/^([a-zA-Z0-9_-]+):.*/\1/p')
  unexpected=$(printf '%s\n' "$keys" | sed '/^$/d; /^name$/d; /^description$/d')
  [ -z "$unexpected" ] \
    || fail "$skill_md has non-portable frontmatter key(s): $(printf '%s' "$unexpected" | tr '\n' ' ')"

  name=$(sed -n "2,$((closing_line - 1))p" "$skill_md" \
    | sed -nE 's/^name:[[:space:]]*(.*)$/\1/p')
  description=$(sed -n "2,$((closing_line - 1))p" "$skill_md" \
    | sed -nE 's/^description:[[:space:]]*(.*)$/\1/p')

  [ -n "$name" ] || fail "$skill_md is missing name"
  [ -n "$description" ] || fail "$skill_md is missing description"
  [ "$name" = "$folder_name" ] \
    || fail "$skill_md name '$name' must match folder '$folder_name'"
  printf '%s' "$name" | grep -Eq '^[a-z0-9]+(-[a-z0-9]+)*$' \
    || fail "$skill_md name must use lowercase letters, digits, and single hyphens"
  [ "${#name}" -le 64 ] || fail "$skill_md name exceeds 64 characters"
  [ "${#description}" -le 1024 ] \
    || fail "$skill_md description exceeds 1024 characters"

  lines=$(wc -l < "$skill_md" | tr -d ' ')
  [ "$lines" -le 500 ] || fail "$skill_md exceeds 500 lines"

  openai_yaml="$skill_dir/agents/openai.yaml"
  [ -f "$openai_yaml" ] || fail "$skill_dir is missing agents/openai.yaml"
  grep -Eq '^interface:$' "$openai_yaml" \
    || fail "$openai_yaml is missing interface"
  grep -Eq '^  display_name: ".+"$' "$openai_yaml" \
    || fail "$openai_yaml is missing a quoted display_name"
  grep -Eq '^  short_description: ".+"$' "$openai_yaml" \
    || fail "$openai_yaml is missing a quoted short_description"
  short_description=$(sed -nE 's/^  short_description: "(.*)"$/\1/p' "$openai_yaml")
  [ "${#short_description}" -ge 25 ] && [ "${#short_description}" -le 64 ] \
    || fail "$openai_yaml short_description must contain 25-64 characters"
  default_prompt=$(sed -nE 's/^  default_prompt: "(.*)"$/\1/p' "$openai_yaml")
  [ -n "$default_prompt" ] || fail "$openai_yaml is missing a quoted default_prompt"
  printf '%s' "$default_prompt" | grep -Fq "\$$name" \
    || fail "$openai_yaml default prompt must mention \$$name"

  while IFS= read -r reference; do
    [ -z "$reference" ] && continue
    [ -f "$skill_dir/$reference" ] \
      || fail "$skill_md references missing file $reference"
  done < <(grep -Eo '\(references/[^)#[:space:]]+' "$skill_md" \
    | sed 's/^(//' | sort -u || true)
done

[ "$found" -eq 1 ] || fail "no skills/*/SKILL.md files found"
echo "All agent skills are valid."
