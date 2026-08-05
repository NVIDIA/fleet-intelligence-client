# Fleet report workspace

Read before capturing full lists.

```bash
umask 077
work=$(mktemp -d "/tmp/fleet-health.XXXXXXXX") || exit 1
printf 'scratch: %s\n' "$work"
```

Preserve that exact path. Before later access or cleanup require
`[ -d "$work" ]`, `[ ! -L "$work" ]`, and `[ -O "$work" ]`. Write each
command to a distinct file (especially unfiltered, Critical, and Warning alert
captures); never append or let concurrent commands share a target.

Write the final HTML outside the scratch directory. After the report validates,
delete only direct files and then the empty directory:

```bash
find "$work" -mindepth 1 -maxdepth 1 \( -type f -o -type l \) -delete &&
  rmdir -- "$work"
```

Never use recursive deletion, wildcards, or a reconstructed path. Retain the
workspace when validation fails. On PowerShell, use a GUID under system temp,
restrict its ACL, reject reparse points, and clean exact literal files without
recurse.
