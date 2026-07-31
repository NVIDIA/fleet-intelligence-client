# Scratch workspace

Create one unpredictable, private temporary directory and keep all evidence
inside it. Preserve the exact path returned at creation; never reconstruct it
from a UUID or hostname.

## POSIX shell

```bash
node_uuid="<validated-node-uuid>"
umask 077
work=$(mktemp -d "/tmp/node-rca-$node_uuid.XXXXXXXX") || exit 1
printf 'scratch: %s\n' "$work"
```

Before each later read, write, or cleanup, set `work` to that exact printed path
and require `[ -d "$work" ]`, `[ ! -L "$work" ]`, and `[ -O "$work" ]`.
Keep files directly inside it. After the report passes validation, delete only
those files and remove the empty directory:

```bash
find "$work" -mindepth 1 -maxdepth 1 \( -type f -o -type l \) -delete
rmdir -- "$work"
```

Never use recursive deletion, a wildcard, or a reconstructed path. If an
unexpected subdirectory exists, cleanup must fail and retain the evidence.

## PowerShell

```powershell
$Work = Join-Path ([System.IO.Path]::GetTempPath()) ("node-rca-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $Work -ErrorAction Stop | Out-Null
Write-Output "scratch: $Work"
```

Restrict the directory ACL to the current user before writing evidence. For
later commands, reuse the exact printed path, reject reparse points, and verify
ownership. Delete known files with exact `-LiteralPath` values, then remove the
empty directory without `-Recurse`, `-Force`, or wildcards.
