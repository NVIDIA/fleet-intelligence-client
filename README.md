# Fleet Intelligence Client

Go SDK and `nvfleetctl` CLI for the NVIDIA Fleet Intelligence customer API — for operators and developers who need to inspect and manage GPU fleet health, alerts, and inventory from the terminal or from Go code.

## Overview

Fleet Intelligence Client is the customer-facing, open-source client boundary for NVIDIA Fleet Intelligence. It ships two ways to talk to the Fleet Intelligence customer API:

- **`nvfleetctl`** — a terminal-native CLI for inspecting and managing your GPU fleet.
- **`pkg/fleetintelligence`** — a public Go SDK that exposes a small, stable, handwritten API over the generated OpenAPI client, for embedding fleet operations in your own Go programs.

The client depends only on the public customer API contract (`api/openapi/`); the backend service remains the source of truth for the API implementation.

Key features:

- **Authenticate once** with an NGC service key (`auth login`); credentials are stored locally at `~/.config/nvfleetctl/config.yaml` (mode `0600`).
- **Inspect your fleet** — list and describe compute zones, node groups, and nodes, including filtering by health state.
- **Track alerts** — list alerts and alert timelines.
- **Generate reports** — inventory and error reports, with built-in verification of Sigstore-signed inventory reports (no external tooling required).
- **Automation-friendly** — `table` output for humans and `json` output for scripts and automation workflows.

## Getting Started

Install `nvfleetctl` with the Go toolchain, or build it from source.

```bash
# Option A: Install with the Go toolchain (requires Go 1.23+)
go install github.com/NVIDIA/fleet-intelligence-client/cmd/nvfleetctl@latest

# Option B: Build from source into ./bin (clone the repo first)
git clone https://github.com/NVIDIA/fleet-intelligence-client.git
cd fleet-intelligence-client
make build
# then run ./bin/nvfleetctl, or add it to your PATH

# Option C: Run without installing (from a cloned repo)
go run ./cmd/nvfleetctl --help

# Verify
nvfleetctl version
```

Then authenticate with an NGC service key and make your first call:

```bash
# Service keys: https://org.dev.ngc.nvidia.com/identity-access/service-keys
nvfleetctl auth login --key <your-ngc-service-key>
nvfleetctl node list
```
## Requirements

- **OS/Arch:** Any platform supported by the Go toolchain (Linux, macOS, and Windows on amd64/arm64). No GPU is required on the machine running the client.
- **Runtime/Compiler:** Go 1.23+ (needed for `go install`, `make build`, and `go run`). A prebuilt binary has no runtime dependency on Go.
- **Credentials:** An NGC service key, generated at <https://org.dev.ngc.nvidia.com/identity-access/service-keys>.

## Usage

```bash
# Authenticate once with your NGC service key
nvfleetctl auth login --key <your-ngc-service-key>

# Inspect your fleet
nvfleetctl computezone list             # list compute zones
nvfleetctl nodegroup list               # list node groups
nvfleetctl node list                    # list nodes
nvfleetctl node describe <uuid>         # describe a single node

# Track alerts and generate reports
nvfleetctl alert list                   # list alerts
nvfleetctl report inventory             # generate an inventory report

# Combine global flags: fetch all pages as JSON, filtered by health state
nvfleetctl node list --all --health Degraded,Unhealthy --output json
```

Most list and read commands accept shared flags: `-o, --output` (`table` or `json`), `--all`, `--page`/`--page-size`, and `--timeout`. Run `nvfleetctl <command> --help` to see all flags for any command.

- More examples & usage (auth, fleet inspection, alerts, reports, signed-report verification): see [`docs/EXAMPLES.md`](docs/EXAMPLES.md)
- Go SDK reference: the [`pkg/fleetintelligence`](pkg/fleetintelligence) package
- API contract: [`api/openapi/openapi.yaml`](api/openapi/openapi.yaml)

## Releases

- Changelog: [`CHANGELOG.md`](CHANGELOG.md)
- Releases & tags: [GitHub releases](https://github.com/NVIDIA/fleet-intelligence-client/releases)

## Contribution Guidelines
- Start here: [`CONTRIBUTING.md`](CONTRIBUTING.md)
- Code of Conduct: [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md)
- Development quickstart (build/test):
```bash
# Clone and build
git clone https://github.com/NVIDIA/fleet-intelligence-client.git
cd fleet-intelligence-client

make build      # build the nvfleetctl binary into ./bin
make test       # run the test suite
make lint       # run the linters
make check      # run all pre-merge checks
```

Commit subjects follow conventional commits: `<type>(<scope>): <subject>`.
Contributions also require a DCO sign-off; see [`CONTRIBUTING.md`](CONTRIBUTING.md).

## Security
- Vulnerability disclosure: see [`SECURITY.md`](SECURITY.md).
- Do not file public issues for security reports — follow the private disclosure process in `SECURITY.md`.

## Support
- Level: **Experimental** — this client is under active development and APIs may change.
- How to get help: open a [GitHub issue](https://github.com/NVIDIA/fleet-intelligence-client/issues).

## Community
- Questions, ideas, and general discussion: open a [GitHub Discussion](https://github.com/NVIDIA/fleet-intelligence-client/discussions) or [issue](https://github.com/NVIDIA/fleet-intelligence-client/issues).
- Bugs and feature requests: [GitHub Issues](https://github.com/NVIDIA/fleet-intelligence-client/issues).

## References
- [Architecture notes](docs/ARCHITECTURE.md) — CLI/SDK design and repository boundary
- [OpenAPI contract](api/openapi/openapi.yaml) — the public customer API
- [NGC service keys](https://org.dev.ngc.nvidia.com/identity-access/service-keys) — generate credentials for the CLI

## License
This project is licensed under the Apache License 2.0 — see the [LICENSE](LICENSE) file for details.
