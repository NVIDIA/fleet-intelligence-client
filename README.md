# Fleet Intelligence Client

[github.com/NVIDIA/fleet-intelligence-client](https://github.com/NVIDIA/fleet-intelligence-client)

Go SDK and `nvfleetctl` CLI for the NVIDIA Fleet Intelligence customer API.
Use it to inspect GPU fleet health, inventory, alerts, and reports from a
terminal or Go program.

## What is included

- `nvfleetctl`: CLI for authentication, fleet inspection, alerts, and reports.
- `pkg/fleetintelligence`: public Go SDK over the generated OpenAPI client.
- `api/openapi`: public customer API contract used by the SDK and CLI.

## Requirements

- An NGC service key from
  <https://org.ngc.nvidia.com/identity-access/service-keys>. See the
  [Fleet Intelligence API reference](https://docs.nvidia.com/fleet-intelligence/latest/api-reference.html)
  for key-generation instructions.
- Linux, macOS, or Windows on an amd64 or arm64 platform.

## Install

### Linux and macOS

Download and inspect the installer, then run it:

```bash
NVFLEETCTL_VERSION=v1.0.0
curl -fsSLO "https://raw.githubusercontent.com/NVIDIA/fleet-intelligence-client/${NVFLEETCTL_VERSION}/install.sh"
less install.sh
bash install.sh --version "${NVFLEETCTL_VERSION}"
```

By default, this installs `nvfleetctl` to `$HOME/.local/bin`. Add that directory
to your shell profile for future sessions and to `PATH` in the current session:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Select another installation directory with:

```bash
bash install.sh --version "${NVFLEETCTL_VERSION}" --install-dir "$HOME/bin"
```

The installer verifies the release checksum and, on macOS, the Developer ID
signature and Apple notarization ticket before installing the executable.

### Windows

Download and inspect the PowerShell installer, then run it:

```powershell
Invoke-WebRequest `
  https://raw.githubusercontent.com/NVIDIA/fleet-intelligence-client/main/install.ps1 `
  -OutFile install.ps1
Get-Content .\install.ps1
.\install.ps1
```

The installer verifies the SHA-256 checksum and 3S Authenticode signature,
installs under `%LOCALAPPDATA%\Programs\nvfleetctl\bin`, and adds that directory
to the user `PATH`. Use `-NoModifyPath` to leave `PATH` unchanged.

## Quick Start

```bash
nvfleetctl auth login --key <your-ngc-service-key>
nvfleetctl overview
nvfleetctl node list
nvfleetctl alert list
nvfleetctl report inventory
```

Common output and pagination flags:

```bash
nvfleetctl node list --all --output json
nvfleetctl node list --page 1 --page-size 25
nvfleetctl node list --health Degraded,Unhealthy
```

Run `nvfleetctl <command> --help` for command-specific flags.

## Use with AI agents (Claude Code and Codex)

This repository ships Agent Skills for answering fleet questions with
`nvfleetctl` and generating fleet health reports. Install them interactively:

```bash
npx skills add NVIDIA/fleet-intelligence-client
```

To install both skills globally for Codex and Claude Code without prompts:

```bash
npx skills add NVIDIA/fleet-intelligence-client \
  --skill nvfleetctl \
  --skill fleet-health-report \
  --agent codex \
  --agent claude-code \
  --global \
  --yes
```

The skills use the installed `nvfleetctl` binary, so [install](#install) and
[authenticate](#quick-start) it first. Restart the agent after installation,
then ask it a fleet question or request a fleet health report.

## Documentation

- [Examples](docs/EXAMPLES.md)
- [Architecture](docs/ARCHITECTURE.md)
- [OpenAPI contract](api/openapi/openapi.yaml)
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
