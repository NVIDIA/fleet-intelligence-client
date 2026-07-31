# Fleet Intelligence Client

Go SDK and `nvfleetctl` CLI for the NVIDIA Fleet Intelligence customer API.
Inspect GPU fleet health, inventory, alerts, and reports from a terminal or Go
program.

## Install

### Linux and macOS

Install `nvfleetctl`:

```bash
NVFLEETCTL_VERSION=v1.0.0
curl -fsSL "https://raw.githubusercontent.com/NVIDIA/fleet-intelligence-client/${NVFLEETCTL_VERSION}/install.sh" | \
  bash -s -- --version "$NVFLEETCTL_VERSION"
```

The installer verifies the release checksum and, on macOS, the Developer ID
signature and Apple notarization ticket before installing the executable.
For publisher-authenticated verification using the signed checksum manifest,
see [Verify release artifacts](docs/VERIFY_RELEASES.md).

### Windows

Install `nvfleetctl`:

```powershell
$Version = "v1.0.0"
$Installer = "https://raw.githubusercontent.com/NVIDIA/fleet-intelligence-client/$Version/install.ps1"
Invoke-WebRequest $Installer -OutFile install.ps1
.\install.ps1 -Version $Version
```

## Quick Start

Choose an NGC API key:

- [Personal API key](https://docs.nvidia.com/ngc/latest/ngc-user-guide.html#generating-a-personal-api-key)
  for individual use.
- [Service key](https://org.ngc.nvidia.com/identity-access/service-keys) for
  programmatic integrations.

```bash
nvfleetctl auth login --key <your-ngc-api-key>
nvfleetctl overview
nvfleetctl node list
```

Run `nvfleetctl <command> --help` for command-specific flags.

## Use with AI agents

Install the portable `nvfleetctl`, fleet health report, and node RCA/RCCA skills
for Claude Code, Cursor, and Codex:

```bash
npx skills add NVIDIA/fleet-intelligence-client \
  --skill '*' \
  --agent claude-code \
  --agent cursor \
  --agent codex
```

[Install](#install) and [authenticate](#quick-start) `nvfleetctl` first, then
start a new agent session. See [Agent Skills](docs/SKILLS.md) for skill scopes,
global installation, and usage guidance.

## Learn more

- [Examples](docs/EXAMPLES.md)
- [Agent Skills](docs/SKILLS.md)
- [Architecture](docs/ARCHITECTURE.md)
- [OpenAPI contract](api/openapi/openapi.yaml)
- [Contributing](CONTRIBUTING.md)
- [Security policy](SECURITY.md)

This project is experimental and available under the
[Apache License 2.0](LICENSE). Open a
[GitHub issue](https://github.com/NVIDIA/fleet-intelligence-client/issues) for
support, and report vulnerabilities through the [security policy](SECURITY.md).
