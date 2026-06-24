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

### Use a prebuilt binary

Download the `nvfleetctl` archive for your platform from the
[release artifacts](https://github.com/NVIDIA/fleet-intelligence-client/releases).

#### Linux and macOS

Choose the matching OS and architecture:

- Linux amd64: `nvfleetctl_<version>_linux_amd64.tar.gz`
- Linux arm64: `nvfleetctl_<version>_linux_arm64.tar.gz`
- macOS Intel: `nvfleetctl_<version>_darwin_amd64.tar.gz`
- macOS Apple Silicon: `nvfleetctl_<version>_darwin_arm64.tar.gz`

Then extract and install:

```bash
tar -xzf nvfleetctl_<version>_<os>_<arch>.tar.gz
chmod +x nvfleetctl
sudo mv nvfleetctl /usr/local/bin/nvfleetctl
nvfleetctl version
```

If you do not have permission to write to `/usr/local/bin`, install to a
user-local directory:

```bash
mkdir -p "$HOME/.local/bin"
mv nvfleetctl "$HOME/.local/bin/nvfleetctl"
```

Make sure `$HOME/.local/bin` is on your `PATH`, then verify the install:

```bash
nvfleetctl version
```

#### Windows

Download the matching Windows zip:

- Windows amd64: `nvfleetctl_<version>_windows_amd64.zip`
- Windows arm64: `nvfleetctl_<version>_windows_arm64.zip`

Unzip the archive, then add the directory containing `nvfleetctl.exe` to your
`PATH`. Verify the install:

```powershell
nvfleetctl version
```

### Install with Go

Use Go installer:

```bash
# While this repository is private, set GOPRIVATE first so `go install`
# fetches via git instead of the public proxy:
export GOPRIVATE=github.com/NVIDIA/fleet-intelligence-client
go install github.com/NVIDIA/fleet-intelligence-client/cmd/nvfleetctl@latest
```

### Build from source

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
