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
nvfleetint node list --data-source oob --bmc-mac 02:00:0a:00:00:0a
nvfleetint node list --view basic --search 02:00:0a:00:00:0a
nvfleetint node describe <node-uuid>
nvfleetint node describe <node-uuid> --data-source oob --section all

# Health, alerts, and events
nvfleetint node health <node-uuid> \
  --start 2026-05-01T00:00:00Z --end 2026-05-08T00:00:00Z
nvfleetint alert summary
nvfleetint alert node <node-uuid>
nvfleetint alert list --severity Critical
nvfleetint alert describe <alert-uuid> --node <node-uuid>
nvfleetint alert options --view historical
nvfleetint event options
nvfleetint event list --window 24h
nvfleetint event buckets --window 24h

# XID bursts
nvfleetint xidburst list --window 24h
nvfleetint xidburst list --window 168h --xid-numbers 48,94 --job-disruption
nvfleetint xidburst list --window 24h --exclude-nodegroup-ids ng-1,ng-2
nvfleetint xidburst describe <burst-id>

# Compute zone metadata
nvfleetint computezone update <zone-id> --type datacenter
nvfleetint computezone update <zone-id> --contact-email ops@example.com
nvfleetint computezone update <zone-id> \
  --location-city Baltimore --location-latitude 39.0458 --location-longitude -76.6413

# Tags and reports
nvfleetint tag list --prefix gpu
nvfleetint tag set <node-uuid> --tags gpu-health,burn_in
nvfleetint report inventory
nvfleetint report error --window 24h
```

List commands support shared flags including `--all`, `--page`, `--page-size`,
`--timeout`, and `--output json`.

Detailed node list and describe commands query both in-band and out-of-band
(OOB) views by default for table output, with labeled sections. Detail
`node list --output json` requires `--data-source inband` or `--data-source oob`,
including with `--all`, and returns one source without an `inband`/`oob` wrapper.
Default `node describe` JSON still uses top-level `inband` and `oob` objects;
select a source to request one view. OOB lists accept `--bmc-mac` and
the `bmcMac` sort key. OOB describe tables can be narrowed with
`--section managers,systems,chassis,firmware,fabrics`; `systems` includes memory
and storage, `chassis` includes power supplies, and
`fabrics` includes switches. Tables use nested status and resource summaries.
Structured inventory locations are available in JSON output. `--section all`
includes every inventory section. Both the out-of-band node list and summary table include
`NODE KIND`. JSON describe output always includes
the full inventory, including nested Redfish status and condition details.

`node list --agent-uuids` filters exact, case-sensitive in-band agent identities;
`--system-uuids` filters system identities in the selected source inventory.
Both take comma-separated values and support basic view. Without a source, basic
view matches active in-band agent identities or system identities from either
active source. Agent UUID filtering is rejected for OOB. Omitted UUID filters
leave the query unrestricted by those identities. Inventory reports do not
accept these filters.

`--agent-type` has been removed from node commands; use `--data-source`.
Detail-list filters and sorting require an explicit source. Basic lists can filter by GPU type/count and sort by identity,
node group, compute zone, GPU type, or GPU count without selecting a source.
Basic responses still contain identity fields only. Agent-status filters accept
`Online` or `Offline`; healthy liveness is online and all other liveness states
are offline.

`node options --view basic` returns the choices for combined basic queries.
Without a source, `node options` displays both detail sources in separate
In-band and Out-of-band table sections. Detail `node options --output json` requires
`--data-source inband` or `--data-source oob` and keeps the original
single-response JSON shape. For both list and options, `--view basic --output json`
allows source omission and returns one combined API response. OOB list/detail tables include GPU
type and count when available.

`report inventory` tables default to separate In-band and Out-of-band sections.
`--page` and `--page-size` apply independently to each source; `--all` fetches
all pages for each. JSON output, CSV, and signed CSV bundles require
`--data-source inband` or `--data-source oob` and contain one source.
An explicit source also selects a single table. OOB rows use node name, BMC MAC identifier, and BMC
IP, plus GPU type and count from the OOB source; shared enrollment/removal
times and verification fields remain available. Both sources support `gpuType`
and `gpuCount` sorting. In-band-only sorts include hostname and public/private
IP; OOB identity sorts are `nodeName`, `bmcMac`, and `bmcIP`.

```bash
nvfleetint node list --view basic --gpu-type NVIDIA-H100 --sort-by gpuCount
nvfleetint node options --view basic --data-source oob
nvfleetint report inventory --data-source oob --sort-by bmcMac --all
nvfleetint report inventory --data-source oob --format csv
```

The investigative alert workflow is `summary → node → describe`: start with
impacted-node counts, inspect one node's alerts, then retrieve one alert's
complete event timeline. `alert list` separately provides the fleet-wide flat
alert records. `alert summary`, `alert node`, and `alert options` default to the
active view; pass `--view historical` for history.

`alert summary` and `alert node` accept `--data-source inband|oob` or
`--data-source inband,oob`. Omission includes both sources in the same API
response. This filters the source that produced the alert, independently of
current agent status.

`alert summary` also accepts `--inband-agent-status` and `--oob-agent-status`:
`online`, `offline`, or `not_enrolled`. Values within each comma-separated
filter are ORed; when both flags are supplied, both conditions must match.
Omitting a status flag leaves that source's status unrestricted. Statuses
describe current enrollment/liveness even in historical view; missing status
data does not mean `not_enrolled`.

Use `alert options` to discover available filter values for the selected view,
and each command's `--help` for supported flags and sort keys.

```bash
nvfleetint alert summary --inband-agent-status offline,not_enrolled --oob-agent-status online
nvfleetint alert node <node-uuid> --view historical --data-source oob --all --output json
```

`event list` and `event buckets` share filters; omitting `--data-source`
includes both sources. `--component` and `--components` cannot be combined,
nor can inclusion and exclusion filters for the same group/zone dimension.
`event options` lists catalog values independently of time and inventory
filters, so an option may have no matching events. The `Others` event name
matches names outside the catalog; GPU types reflect current node inventory.

`xidburst list` requires a time range (`--window`, or `--start` and `--end`),
which applies to each burst's start time. Some of its filters and response
fields — category, subcategory, platform-attributed disruption, XID
descriptions, and DC-admin actions — are available only to cloud-provider/NCP
callers. Node groups and compute zones can be filtered inclusively
(`--nodegroup-ids`, `--compute-zone-ids`) or by exclusion
(`--exclude-nodegroup-ids`, `--exclude-compute-zone-ids`), but not both for the
same dimension.

`xidburst list` and `xidburst options` default to in-band; use the same
`--data-source inband|oob|all` for both. With `all`, the same fault observed on
both paths remains two distinct bursts. Describe needs only the burst ID.
`xidburst describe` adds the impacted device labels, per-XID
catalog details, and suggested actions.

`report error` selects one source with `--data-source inband|oob` and defaults
to in-band for tables, JSON, and CSV in every view. OOB node rows use node name
and BMC MAC identity. For example:

```bash
nvfleetint report error --data-source oob --view list --group-by node --window 24h
```

`computezone update` changes one compute zone's type, contact, and location.
Contact and location are structured values on the API, so each of their fields
has its own flag: `--contact-email`, `--contact-pic`, `--location-city`,
`--location-country`, `--location-region`, `--location-latitude`, and
`--location-longitude`. A flag that is not given leaves the stored value alone
and a flag given an empty value clears that field, so setting a contact never
disturbs a location. At least one field is required, and the command asks for
confirmation before writing, so scripts and CI runs need `--yes`. Compute zone
names are agent-managed and cannot be changed through the customer API. `--type`
accepts `datacenter` or `cloud provider`. `--dry-run` reads the compute zone,
builds the merged PUT request that would be sent, and prints it without
sending the PUT.

`tag set` replaces a node's tags rather than adding to them: a tag the node
already carries that is not listed in `--tags` is removed, and `--clear`
removes every tag. It asks for confirmation before writing, so scripts and CI
runs need `--yes`. Tags use lowercase letters, digits, hyphens, and
underscores; they must start and end with a letter or digit, cannot contain
consecutive separators, and are at most 50 characters. The names `null`,
`none`, `undefined`, `true`, and `false` are reserved. Run
`nvfleetint tag list --node <node-uuid>` to see a node's current tags before
replacing them.

## CSV reports

Write an inventory report to a file:

```bash
nvfleetint report inventory --data-source inband --format csv > inventory.csv
```

Download and verify a signed inventory bundle:

```bash
nvfleetint report inventory --data-source inband --format csv --signed \
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
Upgrade: nvfleetint upgrade
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
command. It is skipped for a locally built binary, which has no release version
to compare. When the CLI is offline, the command prints a short warning to
stderr saying it cannot check whether the version is up to date. Prereleases
are ignored, so a stable install is never pointed at a release candidate.

The check reads the redirect from the `releases/latest` permalink on
`github.com` rather than calling the GitHub REST API, so it consumes none of the
API's unauthenticated request budget — a shared allowance that matters behind a
corporate NAT or on CI runners. `install.sh` and `install.ps1` resolve the
version to download the same way.

### Upgrading in place

`nvfleetint upgrade` installs the newest release over the running binary:

```text
$ nvfleetint upgrade
nvfleetint v1.0.0 -> v1.1.0
Install dir: /home/you/.local/bin

This runs the official installer (https://github.com/.../download/v1.1.0/install.sh), which
verifies the release checksum and code signature.
Are you sure? [y/N]: y

Downloading nvfleetint v1.1.0 for linux/amd64
Installed nvfleetint to /home/you/.local/bin/nvfleetint
Upgraded v1.0.0 -> v1.1.0
```

It downloads and runs the installer published with that release rather than
replacing the binary itself, so an upgrade performs exactly the same
verification a fresh install does — the release checksum, and the Developer ID
signature and notarization ticket or the Authenticode signature. The installer
is pinned to the resolved tag, so the script and the archive it fetches always
come from one release.

The new binary is written to the directory the running one occupies, with
symlinks resolved. If that directory is not writable — a system-wide install
under `/usr/local/bin`, say — the command says so and stops before downloading
anything; re-run it with elevated privileges, or install elsewhere with
`NVFLEETINT_INSTALL_DIR`.

Prompts and installer output go to stderr, so `-o json` emits only a summary on
stdout:

```json
{"name":"nvfleetint","from":"1.0.0","to":"v1.1.0","installDir":"/home/you/.local/bin","upgraded":true}
```

`--yes` skips the confirmation, and is required in a script or CI job: where
stdin is not a terminal the command refuses to prompt and says so, rather than
upgrading a binary nobody confirmed. Unlike the passive check, `upgrade` reports
every failure — a failed lookup or a failed install is an error, because silence
would leave you believing you had been upgraded. A locally built binary cannot
be upgraded, since it has no release version to compare.

#### Installing a specific release

`--version` installs exactly the release named instead of the newest one, with
or without the leading `v`:

```bash
nvfleetint upgrade --version v1.1.0
```

An older release is a legitimate target — pinning a known-good build is the
reason to name a version — so `--version` will downgrade. Naming the version
already installed does nothing and says so.

Without `--version`, the command installs the newest release and does nothing
when the running build is already on it.

`--version` selects *which* published release to install; it cannot change
where the release comes from. The repository and host are compiled in, the
value must match a release tag (letters, digits, and `.+-`, after a leading
`v`) or it is refused locally without a network call, and the release's own
checksum and code-signature verification runs either way.

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
