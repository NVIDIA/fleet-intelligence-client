# fleet-intelligence-client-go

Go SDK and `nvfleetctl` CLI for the Fleet Intelligence customer API.

This repository is intended to become the customer-facing, open-source-ready
client boundary for Fleet Intelligence. The backend repository remains the
source of truth for the API implementation; this repository should depend only
on the public customer API contract.

## Repository Layout

```text
cmd/nvfleetctl/        CLI entrypoint
pkg/fleetintelligence/ Public Go SDK package
internal/generated/    Generated OpenAPI client code
internal/config/       CLI configuration helpers
internal/output/       CLI output helpers
api/openapi/           Public customer API contract
docs/                  Architecture and roadmap notes
```

## Usage

### Install

Requires Go 1.23+.

Build the `nvfleetctl` binary into `bin/`:

```bash
make build
```

Then run it from there, or put it on your `PATH`:

```bash
./bin/nvfleetctl --help
```

To install it directly into your Go bin directory (`$(go env GOPATH)/bin`):

```bash
go install gitlab-master.nvidia.com/gpu-health/fleet-intelligence-client-go/cmd/nvfleetctl@latest
```

Or run it without installing:

```bash
go run ./cmd/nvfleetctl --help
go run ./cmd/nvfleetctl version
```

### Authenticate

`nvfleetctl` talks to the Fleet Intelligence customer API using an NGC service
key. Service keys can be generated at
https://org.dev.ngc.nvidia.com/identity-access/service-keys. Store your
credentials once with `auth login`:

```bash
nvfleetctl auth login --key <your-ngc-service-key>
```

By default the CLI targets `https://api.fleet-intelligence.nvidia.com`. To point
at a different API endpoint, pass `--api-url`:

```bash
nvfleetctl auth login --key <your-ngc-service-key> --api-url https://api.example.nvidia.com
```

Credentials are written to `~/.config/nvfleetctl/config.yaml` (file mode `0600`).
Check or clear them with:

```bash
nvfleetctl auth status   # show configured API URL and key status
nvfleetctl auth logout   # remove the stored service key
```

### Run commands

Once authenticated, inspect your fleet. Common command groups:

```bash
nvfleetctl computezone list          # list compute zones
nvfleetctl nodegroup list            # list node groups
nvfleetctl node list                 # list nodes
nvfleetctl node describe <uuid>      # describe a single node
nvfleetctl alert list                # list alerts
nvfleetctl alert timeline            # list alert timelines
nvfleetctl report inventory          # generate an inventory report
nvfleetctl report error              # generate an error report
nvfleetctl report verify             # verify a signed inventory report
```

Verify a signed inventory report downloaded with
`report inventory --format csv --signed`. No external tools are required —
verification is built in.

That command downloads a zip (`inventory-report.zip` by default). Unzip it
first; it expands to a folder named `inventory_report_<timestamp>/` containing
two files that share the same stem:

| File | Contents |
| --- | --- |
| `inventory_report_<timestamp>.csv` | the report |
| `inventory_report_<timestamp>.sig.bundle` | its Sigstore signature |

Pass the `.csv` to `--csv` and the `.sig.bundle` to `--bundle`. By default the
signing key is fetched from the configured API; pass `--key` to verify fully
offline:

```bash
# Unzip the downloaded bundle
unzip inventory-report.zip
cd inventory_report_2026-06-15_00-00-00

# Verify using the automatically fetched signing key
nvfleetctl report verify \
  --csv inventory_report_2026-06-15_00-00-00.csv \
  --bundle inventory_report_2026-06-15_00-00-00.sig.bundle

# Verify offline with a previously downloaded public key
nvfleetctl report verify \
  --csv inventory_report_2026-06-15_00-00-00.csv \
  --bundle inventory_report_2026-06-15_00-00-00.sig.bundle \
  --key signing-key.pub
```

Most list and read commands accept shared flags:

- `-o, --output` — output format: `table` (default) or `json`
- `--all` — fetch all pages
- `--page`, `--page-size` — paginate results
- `--timeout` — request timeout (e.g. `30s`, `2m`)

For example, fetch every node as JSON and filter by health state:

```bash
nvfleetctl node list --all --health Degraded,Unhealthy --output json
```

Use `nvfleetctl <command> --help` to see all available flags for any command.

## Development

Requirements:

- Go 1.23+

Common commands:

```bash
make build
make test
make lint
make check
make setup-git-hooks
```

Run the scaffolded CLI:

```bash
go run ./cmd/nvfleetctl --help
go run ./cmd/nvfleetctl version
```

## Git Hooks

This repository includes git hooks for secret scanning and commit message
validation. Install `trufflehog`, then enable the hooks once per checkout:

```bash
brew install trufflehog
make setup-git-hooks
```

You can also run the hook manually:

```bash
make test-git-hooks
```

Commit subjects must use the same conventional-commit shape as
`gpu-health-backend`:

```text
<type>(<scope>): <subject> [(GPUHEALTH-####)]
```
