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
echo "All agent skills are valid."
