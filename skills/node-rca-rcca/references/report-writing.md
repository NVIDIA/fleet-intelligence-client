# Report workspace and writing

Read before creating evidence files or the final report.

## Private workspace

Create one unpredictable private directory; preserve its exact path:

```bash
node_uuid="<validated-node-uuid>"
umask 077
work=$(mktemp -d "/tmp/node-rca-$node_uuid.XXXXXXXX") || exit 1
printf 'scratch: %s\n' "$work"
```

Before later access/cleanup require `[ -d "$work" ]`, `[ ! -L "$work" ]`,
and `[ -O "$work" ]`. Keep files directly inside it. On PowerShell, use a GUID
under the system temp directory, restrict its ACL, reject reparse points, and
verify ownership.

## Fetch once

Write large responses to separate files, then parse those files:

```bash
out="$work/alert-<alert_uuid>.json"
nvfleetint alert describe <alert_uuid> --node <node_uuid> --output json > "$out"
jq '<expression>' "$out"
```

Never pipe a costly fetch into `jq` or refetch after a parser error. Inspect
keys/sample and extract needed aggregates from the saved response; stop when the
reason and onset are established.

## Write and validate once

Copy the template chrome, fill the body, and emit the entire document with a
quoted heredoc:

```bash
slug=$(printf '%s' "<hostname-or-node-uuid>" | tr -c 'A-Za-z0-9._-' '-' | sed 's/^[.-]*//')
[ -n "$slug" ] || exit 1
out="node-rca-rcca-$slug.html"
node_uuid="<node_uuid>"
printf '%s' "$node_uuid" | grep -qiE '^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$' || exit 1
work="<exact-validated-work-path>"

cat > "$out" <<'NVFLEET_REPORT'
<!doctype html>
... complete template and populated sections ...
</html>
NVFLEET_REPORT

miss=0
for id in summary node-details impact timeline root-cause contributing-factors           actions validation evidence unknowns; do
  grep -q "id=\"$id\"" "$out" || { echo "missing: $id" >&2; miss=1; }
done
[ "$miss" -eq 0 ] && ! grep -qE '\[[a-z_]+\]' "$out" && grep -q '</html>' "$out"   && find "$work" -mindepth 1 -maxdepth 1 \( -type f -o -type l \) -delete   && rmdir -- "$work"   && echo "report ok: $out"
```

The ten IDs are required; `reference` is optional. Quote the heredoc delimiter
so shell metacharacters stay literal. Compose once instead of editing the
emitted report. If validation fails, retain evidence and rewrite from it.

Use the exact scratch path; never reconstruct it. Sanitize generated filenames
as above, or honor an explicit user path. Delete only validated direct files,
then the empty directory—never recursive deletion, wildcards, or unknown
subdirectories. PowerShell cleanup must use exact literal paths without recurse.
