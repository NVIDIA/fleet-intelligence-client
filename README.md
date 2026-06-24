# Fleet Intelligence Client

Go SDK and `nvfleetctl` CLI for the NVIDIA Fleet Intelligence customer API.
Use it to inspect GPU fleet health, inventory, alerts, and reports from a
terminal or Go program.

## What is included

- `nvfleetctl`: CLI for authentication, fleet inspection, alerts, and reports.
- `pkg/fleetintelligence`: public Go SDK over the generated OpenAPI client.
- `api/openapi`: public customer API contract used by the SDK and CLI.

## Requirements

- Go 1.23+ for source builds and `go install`.
- An NGC service key from
  <https://org.ngc.nvidia.com/identity-access/service-keys>. See the
  [Fleet Intelligence API reference](https://docs.nvidia.com/fleet-intelligence/latest/api-reference.html)
  for key-generation instructions.
- Linux, macOS, or Windows on a Go-supported amd64/arm64 platform.

## Install

Use Go installer:

```bash
# While this repository is private, set GOPRIVATE first so `go install`
# fetches via git instead of the public proxy:
export GOPRIVATE=github.com/NVIDIA/fleet-intelligence-client
go install github.com/NVIDIA/fleet-intelligence-client/cmd/nvfleetctl@latest
```

Or build from source:

```bash
git clone https://github.com/NVIDIA/fleet-intelligence-client.git
cd fleet-intelligence-client
make build
./bin/nvfleetctl --help
```

## Quick Start

```bash
nvfleetctl auth login --key <your-ngc-service-key>
nvfleetctl node list
nvfleetctl alert list
nvfleetctl report inventory
```

Common output and pagination flags:

```bash
nvfleetctl node list --all --output json
nvfleetctl node list --page 0 --page-size 25
nvfleetctl node list --health Degraded,Unhealthy
```

Run `nvfleetctl <command> --help` for command-specific flags.

## Documentation

- [Examples](docs/EXAMPLES.md)
- [Architecture](docs/ARCHITECTURE.md)
- [OpenAPI contract](api/openapi/openapi.yaml)
- [Changelog](CHANGELOG.md)
- [Contributing](CONTRIBUTING.md)
- [Security policy](SECURITY.md)

## Development

```bash
make build
make test
make lint
make check
```

Commit subjects follow conventional commits:
`<type>(<scope>): <subject>`. Contributions require DCO sign-off; see
[`CONTRIBUTING.md`](CONTRIBUTING.md).

## Support

This client is experimental and under active development. Open a
[GitHub issue](https://github.com/NVIDIA/fleet-intelligence-client/issues) for
bugs, feature requests, or questions. Do not file public issues for security
vulnerabilities; follow [`SECURITY.md`](SECURITY.md).

## License

Apache License 2.0. See [LICENSE](LICENSE).
