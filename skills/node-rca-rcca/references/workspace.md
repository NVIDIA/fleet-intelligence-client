# Shared Report Workspace

Use this workflow when a report needs saved API responses.

## Capture

Create one unpredictable private directory and preserve its exact path:

```bash
umask 077
work=$(mktemp -d "/tmp/nvfleet-report.XXXXXXXX") || exit 1
printf 'scratch: %s\n' "$work"
```

Before later access or cleanup require `[ -d "$work" ]`, `[ ! -L "$work" ]`, and `[ -O "$work" ]`. Write each response to a distinct file. Fetch costly data once, then inspect and parse the saved response; do not share a target between concurrent commands.

On PowerShell, use a GUID directory under system temp, restrict its ACL, reject reparse points, and retain the exact literal path.

## Write and validate

Write the final HTML outside the scratch directory. Sanitize a generated filename to `A-Za-z0-9._-`, or honor an explicit output path. Compose the document once, then validate the required section IDs supplied by the calling skill, closing `</html>`, and absence of unresolved `[placeholder]` tokens.

## Cleanup

After successful validation, delete only direct files/links and the empty exact directory:

```bash
find "$work" -mindepth 1 -maxdepth 1 \( -type f -o -type l \) -delete &&
  rmdir -- "$work"
```

Retain evidence after validation failure. Do not recursively delete, use globs, reconstruct the path, or remove unknown subdirectories. PowerShell cleanup uses exact literal paths without recursion.
