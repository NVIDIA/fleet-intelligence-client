# CLI guide

Use `nvfleetint` to inspect fleet health, inventory, alerts, events, and
reports. Run `nvfleetint <command> --help` for every available flag.

## Authentication

Create an NGC [personal API key](https://docs.nvidia.com/ngc/latest/ngc-user-guide.html#generating-a-personal-api-key)
or [service key](https://org.ngc.nvidia.com/identity-access/service-keys), then
store it:

```bash
nvfleetint auth add
nvfleetint auth status
```

`auth add` asks for the key and the API URL on stdin.

```
$ nvfleetint auth add
API key:
API URL [production: https://api.fleet-intelligence.nvidia.com]:
Profile "default" added.
```

Prompts and their warnings are written to stderr, so stdout stays parseable. An
answer that is rejected — an empty key, a malformed URL — is asked for again, up
to three times.

With no name, that stores the key in a profile called `default`. If you only
work against one tenant, that is the whole of it — the rest of this section is
about running against several.

On Linux and macOS, credentials are stored in
`~/.config/nvfleetint/config.yaml` with file mode `0600`. On Windows, they are
stored in `%USERPROFILE%\.config\nvfleetint\config.yaml`; access follows Windows
ACLs, and the Go file mode controls only whether the file is writable.

### Profiles

A profile pairs an API key with an API URL, so one installation can work
against several tenants or endpoints:

```bash
nvfleetint auth add prod            # prompts for the key, keeps the production URL
nvfleetint auth add dev             # prompts for the key, answer the URL prompt with your endpoint
nvfleetint auth list
nvfleetint auth use prod            # pick the default
nvfleetint auth add dev             # existing name: rotate the key
nvfleetint auth remove dev
```

Note the two different roles a profile name plays. On `auth add/remove/use` the
profile is what the command acts on, so it is a positional `<name>`. On every
command that calls the API, `--profile` instead chooses the credentials for that
one invocation:

```bash
nvfleetint node list --profile dev
nvfleetint auth status --profile dev
```

Without `--profile`, commands use the current profile — the one marked `*` by
`nvfleetint auth list`. API keys are never printed: `auth list` and
`auth status` only report whether a key is configured.

`auth add` creates a profile or changes an existing one — there is no separate
update command, so re-running it with the same name is how a key is rotated. On
an existing name the change is partial: the key prompt then offers to keep the
stored key and the URL prompt offers the stored URL, so pressing Enter at either
one leaves that value untouched. Rotating a key preserves a custom API URL and
vice versa, and keeping both is reported as `unchanged` rather than treated as an
error. The output otherwise says `added` or `updated`, which is how you notice a
name collision; `auth list` shows what is already taken. A new profile still
requires a key, because there is nothing stored to keep.

The name is optional only on `auth add`, where omitting it means the `default`
profile. `auth remove` and `auth use` always require one: defaulting a deletion
or a switch would act on a profile you never named.

An empty answer always means "keep what is stored", never "clear this field", so
piping an unset `$KEY` leaves the stored key alone instead of wiping it.

Replacing a stored API key destroys a value that cannot be recovered, so
`auth add` on a profile that already has one prints a warning above the key
prompt. At a terminal, typing a new key is itself the answer to that warning —
there is no second confirmation. Pressing Enter keeps the stored key without ever
displaying it.

Replacing a stored key this way requires `--yes`; without it the command fails
and tells you so rather than overwriting a key nobody confirmed. `--yes` is not
needed to create a profile, to change only its API URL, or to supply the first
key for a profile that has none — none of those take anything away.

`auth remove` deletes an API key that cannot be recovered, so it asks for
confirmation. The prompt is written to stderr (stdout stays parseable) and
defaults to No. Pass `--yes` to skip it; in a script or CI job, where stdin is
not a terminal, the command refuses to prompt and tells you to use `--yes`
rather than hanging.

Removing the current profile always clears the selection — no other profile is
promoted in its place. Pick the next one explicitly with `auth use <name>`.
Removing any other profile leaves the current selection untouched. The command
prints the resulting current profile either way.

## Common commands

```bash
# Fleet summary and inventory
nvfleetint overview
nvfleetint computezone list
nvfleetint nodegroup list
nvfleetint node list
nvfleetint node describe <node-uuid>

# Health, alerts, and events
nvfleetint node health <node-uuid> \
  --start 2026-05-01T00:00:00Z --end 2026-05-08T00:00:00Z
nvfleetint alert summary
nvfleetint alert node <node-uuid>
nvfleetint alert list --severity Critical
nvfleetint alert describe <alert-uuid> --node <node-uuid>
nvfleetint alert options --view historical
nvfleetint event list --window 24h
nvfleetint event buckets --window 24h

# Tags and reports
nvfleetint tag list --prefix gpu
nvfleetint report inventory
nvfleetint report error --window 24h
```

List commands support shared flags including `--all`, `--page`, `--page-size`,
`--timeout`, and `--output json`.

The investigative alert workflow is `summary → node → describe`: start with
impacted-node counts, inspect one node's alerts, then retrieve one alert's
complete event timeline. `alert list` separately provides the fleet-wide flat
alert records. `alert summary`, `alert node`, and `alert options` default to the
active view; pass `--view historical` for history.

## CSV reports

Write an inventory report to a file:

```bash
nvfleetint report inventory --format csv > inventory.csv
```

Download and verify a signed inventory bundle:

```bash
nvfleetint report inventory --format csv --signed \
  --output-path inventory-report.zip
unzip -l inventory-report.zip
unzip inventory-report.zip

# Use the extracted names shown by unzip -l.
nvfleetint report verify \
  --csv <extracted-csv-path> \
  --bundle <extracted-sig-bundle-path>
```

Use `--key signing-key.pub` with `report verify` to supply a local public key.

## Version and updates

`nvfleetint version` prints the binary version, git commit, and build date. It
also asks GitHub for the newest published release and prints an upgrade notice
when the running build is behind:

```text
nvfleetint 1.0.0
commit: e9941e1
built: 2026-08-14

Update available: v1.1.0 (current v1.0.0)
Release notes: https://github.com/NVIDIA/fleet-intelligence-client/releases/tag/v1.1.0
Upgrade: curl -fsSL https://github.com/NVIDIA/fleet-intelligence-client/releases/latest/download/install.sh | bash
```

The notice goes to stderr, so piping `version` output stays unaffected. With
`-o json` the check appears as an `updateCheck` object instead:

```json
{
  "name": "nvfleetint",
  "version": "1.0.0",
  "commit": "e9941e1",
  "buildDate": "2026-08-14",
  "updateCheck": {
    "latestVersion": "v1.1.0",
    "releaseUrl": "https://github.com/NVIDIA/fleet-intelligence-client/releases/tag/v1.1.0",
    "updateAvailable": true
  }
}
```

The lookup is best effort and bounded by a short timeout: it never fails the
command, and it is skipped entirely for a locally built binary, which has no
release version to compare. Turn it off with `--check-update=false` for a single
run, or by exporting `NVFLEETINT_NO_UPDATE_CHECK=1`. Prereleases are ignored, so
a stable install is never pointed at a release candidate.

## Automation

For scripts and CI jobs, authenticate without writing a configuration file:

```bash
export NVFLEETINT_API_KEY="<ngc-api-key>"
export NVFLEETINT_API_URL="https://api.fleet-intelligence.nvidia.com"
```

`NVFLEETINT_API_URL` is optional when using the production API. To pick a stored
profile instead, pass `--profile <name>`.

Credentials resolve in this order, highest first:

1. `--profile <name>` — the profile's key and URL are used exactly as stored.
2. The current profile, with `NVFLEETINT_API_KEY` and `NVFLEETINT_API_URL`
   overlaid on top of it. With neither a profile nor those variables set,
   commands fail and tell you to run `nvfleetint auth add`.

Selecting a profile explicitly with `--profile` deliberately ignores
`NVFLEETINT_API_KEY` and `NVFLEETINT_API_URL`: with several tenants
configured, a stale variable would otherwise send one tenant's key to another
tenant's endpoint. `nvfleetint auth status` prints the source of each value and
notes when environment credentials were set but ignored.

An explicitly named profile that is not stored is an error — you named it, so
using something else instead would be wrong. A *current* profile that is no
longer stored is not: the environment overlay still applies, and
`nvfleetint auth status` reports the stale selection as a warning. The same
holds for a configuration file that cannot be read: if the environment supplies
a key, commands run and `auth status` warns rather than reporting no profiles.

`NVFLEETINT_SERVICE_KEY` was renamed to `NVFLEETINT_API_KEY` and is no longer
read. When it is set and `NVFLEETINT_API_KEY` is not, the "no credentials"
error and `auth status` both say so.

Use `--output json` or `-o json` for machine-readable output. API-backed
commands preserve the API response shape for a single page. With `--all`, list
commands return a normalized object:

```json
{
  "items": [],
  "pagination": {
    "page": 1,
    "pageSize": 100,
    "total": 0,
    "hasMore": false,
    "pagesFetched": 1
  }
}
```

Successful JSON is written to stdout. When JSON output is active, failures are
written to stderr in this form:

```json
{
  "error": {
    "code": "command_error",
    "message": "--csv is required"
  }
}
```

Command validation and local failures use `error.code: "command_error"`. API
failures use `error.code: "api_error"` and may also include `statusCode`,
`status`, and `details`.

Exit code `0` means success, `1` means a general failure, and `77` means a 401
or 403 API error reached command failure handling. `auth status` is diagnostic:
it reports those responses as `connection: "unauthorized"` and exits `0`.
Prefer `error.code` over the exit code when handling JSON errors.

Commands that stream CSV do not accept `--output`. The profile-mutating
commands (`auth add`, `auth remove`, `auth use`) do not provide
JSON output; `auth list` and `auth status` do.
