# Scratch workspace

Keep every temporary evidence file in one OS-managed scratch directory derived
from the validated node UUID. This makes the path reconstructable in harnesses
that start a fresh shell for each command.

## POSIX shell

Validate the UUID before using it in any filesystem path:

```bash
node_uuid="<node_uuid>"
printf '%s' "$node_uuid" \
  | grep -qiE '^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$' \
  || { echo "refusing: malformed node UUID" >&2; exit 1; }
work="${TMPDIR:-/tmp}/node-rca-$node_uuid"
mkdir -p "$work"
```

Re-run that validation block at the start of every command that reads or writes
scratch. Name files directly inside `work`, such as
`"$work/node-describe.json"`. Delete only that validated path after the report
passes its mechanical checks.

Do not accumulate paths in a shell variable: variables may not survive between
tool calls. Do not use an unresolved glob for cleanup. Do not use a hostname,
hand-typed identifier, `/`, `..`, or an unvalidated environment variable in the
deletion target.

The fixed directory avoids incompatible GNU/BSD `mktemp` template behavior.

## PowerShell

Apply the same invariant with native operations:

```powershell
$NodeUuid = "<node_uuid>"
if ($NodeUuid -notmatch '^[0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}$') {
    throw "Refusing: malformed node UUID"
}
$Work = Join-Path ([System.IO.Path]::GetTempPath()) "node-rca-$NodeUuid"
New-Item -ItemType Directory -Path $Work -Force | Out-Null
```

Reconstruct and revalidate `$Work` in every command invocation. After all report
checks pass, remove that exact directory with
`Remove-Item -LiteralPath $Work -Recurse -Force`. Never use a wildcard cleanup
target.
