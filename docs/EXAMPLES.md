# Examples & Usage

Worked examples for the main `nvfleetctl` workflows: authentication, inspecting
your fleet, tracking alerts, generating reports, and verifying a Sigstore-signed
inventory report. For the full flag list on any command, run
`nvfleetctl <command> --help`.

## Global flags

Most list and read commands share these flags:

- `-o, --output` — `table` (default, for humans) or `json` (for scripts and
  agents). `json` emits the raw API response.
- `--all` — fetch every page and merge the results (list commands only).
- `--page` / `--page-size` — fetch a single page (`--page` is 0-indexed).
  `--page` cannot be combined with `--all`.
- `--timeout` — per-request timeout, e.g. `30s` or `2m` (must be greater than 0).

```bash
# Human-readable table (the default)
nvfleetctl node list

# Fetch a specific page of 25 results
nvfleetctl node list --page 0 --page-size 25

# Fetch every page as JSON with a longer timeout
nvfleetctl node list --all --output json --timeout 2m

# --all wraps results in {items, pagination}; pull hostnames with jq
nvfleetctl node list --all --output json | jq -r '.items[].hostname'
```

A single-page `--output json` response is the raw API payload (results under the
resource's own key, e.g. `nodes`), whereas `--all` normalizes every page into an
`{items, pagination}` envelope.

## Authentication

`nvfleetctl` authenticates with an NGC service key. You authenticate once; the
key is stored locally and reused on every subsequent call.

Generate a service key at
<https://org.dev.ngc.nvidia.com/identity-access/service-keys>.

```bash
# Store a service key (uses the default API URL)
nvfleetctl auth login --key <your-ngc-service-key>

# Point the CLI at a non-default API endpoint
nvfleetctl auth login --key <your-ngc-service-key> \
  --api-url https://api.fleet-intelligence.nvidia.com

# Check what is configured (does not contact the API)
nvfleetctl auth status

# Remove the stored service key
nvfleetctl auth logout
```

`auth status` prints the configured API URL, whether a service key is stored,
and a connection line (always `not checked` — status does not call the API):

```text
API URL: https://api.fleet-intelligence.nvidia.com
Service key: configured
Connection: not checked
```

### Where credentials live

Credentials are written to `~/.config/nvfleetctl/config.yaml` with file mode
`0600` (readable and writable only by your user). `auth logout` clears the
service key from this file but leaves the API URL in place.

## Inspecting your fleet

The fleet is organized into compute zones, which contain node groups, which
contain nodes. Each `list` command supports two views: `detail` (default, full
columns) and `basic` (just IDs and names). Filters and sorting are only
available in the `detail` view.

### Compute zones

```bash
# List all compute zones (detail view)
nvfleetctl computezone list

# Just IDs and names
nvfleetctl computezone list --view basic

# Filter to specific zones
nvfleetctl computezone list --zone-ids zone-a,zone-b
```

### Node groups

```bash
# List all node groups
nvfleetctl nodegroup list

# Only unhealthy or degraded groups, worst health first
nvfleetctl nodegroup list --health Degraded,Unhealthy

# Filter by GPU type and sort by GPU utilization, highest first
nvfleetctl nodegroup list --gpu-type H100 --sort-by gpuUtil --order desc
```

Node groups accept `--health` (`Healthy`, `Degraded`, `Unhealthy`, `Unknown`),
`--gpu-type`, `--nodegroup-ids`, `--sort-by` (`health`, `gpuUtil`, `nodes`), and
`--order` (`asc`/`desc`); sorting defaults to `health`.

### Nodes

```bash
# List all nodes
nvfleetctl node list

# Filter by health state and hostname substring
nvfleetctl node list --health Degraded,Unhealthy --hostname gpu-node

# Nodes whose agent is offline, sorted by hostname
nvfleetctl node list --agent-status Offline --sort-by hostname --order asc

# Nodes that failed integrity or firmware checks
nvfleetctl node list --integrity-check Unverified,Degraded --firmware-check Failed

# Describe a single node by UUID (full detail, including resources and system info)
nvfleetctl node describe <node-uuid>

# Describe a node as JSON
nvfleetctl node describe <node-uuid> --output json
```

Node filters: `--health`, `--hostname` (partial match), `--agent-status`
(`Online`, `Offline`, `Unknown`), `--integrity-check` (`Verified`, `Unverified`,
`Degraded`, `Pending`, `Unsupported`, `Unknown`), `--firmware-check` (`Passed`,
`Failed`, `Unknown`), `--node-uuids`, plus `--sort-by`/`--order`.

## Alerts

```bash
# List all alerts
nvfleetctl alert list

# Only critical alerts for one node
nvfleetctl alert list --severity Critical --node <node-uuid>

# List nodes that have alert history
nvfleetctl alert timeline

# Show only nodes with currently active alerts
nvfleetctl alert timeline --active

# Show the alert timeline for a single node
nvfleetctl alert timeline --node <node-uuid>

# Describe one alert's full event timeline (requires the node UUID)
nvfleetctl alert describe <alert-uuid> --node <node-uuid>
```

`alert list` accepts `--severity` (`Critical` or `Warning`) and `--node`.
`alert timeline` with no `--node` lists nodes that have alert history; with
`--node` it lists that node's alerts. `--active` limits either to currently
active alerts.

## Error reports

The error report summarizes fleet errors over a time range. A time range is
always required: use `--window` for a relative range (units `ns`, `us`, `ms`,
`s`, `m`, `h`), or `--start` and `--end` together for an absolute RFC3339 range.

Three views are available:

- `overview` (default) — summary totals for the range.
- `list` — per-error or per-node breakdown; requires `--group-by error|node`.
- `graph` — a time series of error counts.

```bash
# Summary totals over the last 24 hours (default view)
nvfleetctl report error --window 24h

# Errors grouped by type over the last 7 days
nvfleetctl report error --view list --group-by error --window 168h

# Errors grouped by node for an absolute range
nvfleetctl report error --view list --group-by node \
  --start 2026-05-01T00:00:00Z --end 2026-05-08T00:00:00Z

# Error count time series for the last day
nvfleetctl report error --view graph --window 24h

# Export the grouped list as CSV (list view only)
nvfleetctl report error --view list --group-by error --window 24h --format csv > errors.csv
```

`--format csv` is only supported with `--view list`, and cannot be combined with
`--output` or pagination flags. `--view graph` and `--view overview` do not
paginate.

## Inventory reports

```bash
# Inventory report as a table (default)
nvfleetctl report inventory

# Every node, as JSON
nvfleetctl report inventory --all --output json

# Plain (unsigned) CSV to stdout
nvfleetctl report inventory --format csv > inventory.csv
```

For a tamper-evident CSV you can verify locally, use `--signed` — see below.

## Signed inventory report verification

The inventory report can be downloaded as a Sigstore-signed CSV bundle and
verified locally. Verification is built into `nvfleetctl` — no `cosign` or other
external tooling is required.

### 1. Download the signed bundle

```bash
# Download a signed CSV bundle into the current directory
nvfleetctl report inventory --format csv --signed

# ...or to a specific path
nvfleetctl report inventory --format csv --signed --output-path ./reports/inventory.zip
```

`--signed` requires `--format csv`. The command writes a zip (named
`inventory-report.zip` by default) and prints where it landed:

```text
Signed report written to inventory-report.zip
```

### 2. Unzip the bundle

```bash
unzip inventory-report.zip
```

The zip expands to a folder named `inventory_report_<timestamp>/` containing two
files that share the same stem:

```text
inventory_report_<timestamp>.csv         the report
inventory_report_<timestamp>.sig.bundle  its Sigstore signature
```

### 3. Verify

Pass the `.csv` to `--csv` and the `.sig.bundle` to `--bundle`. By default the
signing key is fetched from the configured API, so this requires network access
and a valid service key:

```bash
nvfleetctl report verify \
  --csv inventory_report_<timestamp>.csv \
  --bundle inventory_report_<timestamp>.sig.bundle
```

On success:

```text
Verified OK
```

### Verify offline

To verify without contacting the API, supply a previously downloaded PEM public
key with `--key`. This makes verification fully offline:

```bash
nvfleetctl report verify \
  --csv inventory_report_<timestamp>.csv \
  --bundle inventory_report_<timestamp>.sig.bundle \
  --key signing-key.pub
```

### Troubleshooting

- **`verification failed: ... does not match the signature`** — the report was
  modified, or `--csv` and `--bundle` point at mismatched files. Re-download the
  bundle and verify the matching pair.
- **`... is not a valid signature bundle`** — `--bundle` must point at the
  `.sig.bundle` file from the unzipped report, not the `.csv` or the `.zip`.
- **`... is not a valid PEM public key`** — the file passed to `--key` is not a
  PEM public key. Omit `--key` to fetch the signing key from the API instead.
