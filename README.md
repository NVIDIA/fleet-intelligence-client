# fleet-intelligence-client-go

Go SDK and `fleetctl` CLI for the Fleet Intelligence customer API.

This repository is intended to become the customer-facing, open-source-ready
client boundary for Fleet Intelligence. The backend repository remains the
source of truth for the API implementation; this repository should depend only
on the public customer API contract.

## Repository Layout

```text
cmd/fleetctl/          CLI entrypoint
pkg/fleetintelligence/ Public Go SDK package
internal/generated/    Generated OpenAPI client code
internal/config/       CLI configuration helpers
internal/output/       CLI output helpers
api/openapi/           Public customer API contract
docs/                  Architecture and roadmap notes
```

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
go run ./cmd/fleetctl --help
go run ./cmd/fleetctl version
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
