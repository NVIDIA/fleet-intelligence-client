# CLI guide

Use `nvfleetint` to inspect fleet health, inventory, alerts, events, and
reports. Run `nvfleetint <command> --help` for every available flag.

## Authentication

Create an NGC [personal API key](https://docs.nvidia.com/ngc/latest/ngc-user-guide.html#generating-a-personal-api-key)
or [service key](https://org.ngc.nvidia.com/identity-access/service-keys), then
store it:

```bash
nvfleetint auth add --key <your-ngc-api-key>
nvfleetint auth status
```

With no name, that stores the key in a profile called `default`. If you only
work against one tenant, that is the whole of it — the rest of this section is
about running against several.

On Linux and macOS, credentials are stored in
`~/.config/nvfleetint/config.yaml` with file mode `0600`. On Windows, they are
stored in `%USERPROFILE%\.config\nvfleetint\config.yaml`; access follows Windows
ACLs, and the Go file mode controls only whether the file is writable.

### Profiles

A profile pairs a service key with an API URL, so one installation can work
against several tenants or endpoints:

```bash
nvfleetint auth add prod --key <ngc-service-key>
nvfleetint auth add dev --key <ngc-service-key> --api-url https://dev.example.com
nvfleetint auth list
nvfleetint auth use prod            # pick the default
nvfleetint auth add dev --key <rotated-key>   # existing name: rotate the key (prompts)
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
`nvfleetint auth list`. Service keys are never printed: `auth list` and
`auth status` only report whether a key is configured.

`auth add` creates a profile or changes an existing one — there is no separate
update command, so re-running it with the same name is how a key is rotated. On
an existing name the change is partial: an omitted flag leaves that value
untouched, so rotating a key preserves a custom API URL and vice versa. The
output says `added` or `updated`, which is how you notice a name collision;
`auth list` shows what is already taken. A new profile still requires `--key`,
and re-running `auth add <name>` with no flags at all is an error rather than a
reset to defaults.

The name is optional only on `auth add`, where omitting it means the `default`
profile. `auth remove` and `auth use` always require one: defaulting a deletion
or a switch would act on a profile you never named.

An empty value is rejected rather than treated as "clear this field", so
`--key "$KEY"` with `KEY` unset fails instead of silently wiping the stored key.

Because replacing a stored service key destroys a value that cannot be
recovered, `--key` on a profile that already has one asks for confirmation
before writing — the prompt names the fields being replaced, goes to stderr, and
defaults to No. Nothing else prompts: creating a profile, changing only its API
URL, and supplying the first key for a profile that has none all take nothing
away. Pass `--yes` to skip the prompt; in a script or CI job, where stdin is not
a terminal, the command refuses to prompt and tells you to use `--yes` rather
than hanging:

```bash
nvfleetint auth add prod --key <rotated-key> --yes
```

`auth remove` deletes a service key that cannot be recovered, so it asks for
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
nvfleetint alert list --severity Critical
nvfleetint alert timeline --node <node-uuid>
nvfleetint event list --window 24h
nvfleetint event buckets --window 24h

# Tags and reports
nvfleetint tag list --prefix gpu
nvfleetint report inventory
nvfleetint report error --window 24h
```

List commands support shared flags including `--all`, `--page`, `--page-size`,
`--timeout`, and `--output json`.

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

## Automation

For scripts and CI jobs, authenticate without writing a configuration file:

```bash
export NVFLEETINT_SERVICE_KEY="<ngc-service-key>"
export NVFLEETINT_API_URL="https://api.fleet-intelligence.nvidia.com"
```

`NVFLEETINT_API_URL` is optional when using the production API. To pick a stored
profile instead, set `NVFLEETINT_PROFILE=<name>`.

Credentials resolve in this order, highest first:

1. `--profile <name>` — the profile's key and URL are used exactly as stored.
2. `NVFLEETINT_PROFILE` — the same, for a whole shell session or CI job.
3. The current profile, with `NVFLEETINT_SERVICE_KEY` and `NVFLEETINT_API_URL`
   overlaid on top of it. With neither a profile nor those variables set,
   commands fail and tell you to run `nvfleetint auth add`.

Selecting a profile explicitly (either of the first two) deliberately ignores
`NVFLEETINT_SERVICE_KEY` and `NVFLEETINT_API_URL`: with several tenants
configured, a stale variable would otherwise send one tenant's key to another
tenant's endpoint. `nvfleetint auth status` prints the source of each value and
notes when environment credentials were set but ignored.

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
