# Fleet Intelligence Client

Go SDK and `nvfleetctl` CLI for the NVIDIA Fleet Intelligence customer API.
Inspect GPU fleet health, inventory, alerts, and reports from a terminal or Go
program.

## Install

### Linux and macOS

Install `nvfleetctl`:

```bash
curl -fsSL https://github.com/NVIDIA/fleet-intelligence-client/releases/latest/download/install.sh | bash
```

The installer verifies the release checksum and, on macOS, the Developer ID
signature and Apple notarization ticket before installing the executable.
For publisher-authenticated verification using the signed checksum manifest,
see [Verify release artifacts](docs/VERIFY_RELEASES.md).

### Windows

Install `nvfleetctl`:

```powershell
Invoke-WebRequest https://github.com/NVIDIA/fleet-intelligence-client/releases/latest/download/install.ps1 -OutFile install.ps1
.\install.ps1
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

## AI Skills

Install the bundled Fleet Intelligence skills for Claude Code, Cursor, and
Codex:

```bash
npx skills add NVIDIA/fleet-intelligence-client \
  --skill '*' \
  --agent claude-code --agent cursor --agent codex
```

Install and authenticate `nvfleetctl` first. See
[Agent Skills](docs/SKILLS.md) for available skills and usage guidance.

## Documentation

- [CLI guide](docs/CLI.md)
- [Go SDK](docs/SDK.md)
- [OpenAPI contract](api/openapi/openapi.yaml)
- [Contributing](CONTRIBUTING.md)
- [Security policy](SECURITY.md)

This project is available under the [Apache License 2.0](LICENSE). Open a
[GitHub issue](https://github.com/NVIDIA/fleet-intelligence-client/issues) for
support, and report vulnerabilities through the [security policy](SECURITY.md).
