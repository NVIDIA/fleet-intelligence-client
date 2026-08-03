# NVIDIA Fleet Intelligence Client

## Overview

`nvfleetint` provides command-line and Go interfaces to NVIDIA Fleet
Intelligence.

- **CLI:** Use the `nvfleetint` executable to inspect fleet status, node health,
  inventory, alerts, events, and reports from a terminal or automation
  workflow.
- **Go SDK:** Import
  `github.com/NVIDIA/fleet-intelligence-client/nvfleetint` to integrate Fleet
  Intelligence data into Go applications and services.

Both interfaces support fleet summaries, compute zones, node groups, nodes,
health history, alerts, events, tags, inventory reports, and error reports.

## Install

### CLI

#### Linux and macOS

Install `nvfleetint`:

```bash
curl -fsSL https://github.com/NVIDIA/fleet-intelligence-client/releases/latest/download/install.sh | bash
```

The installer verifies the release checksum and, on macOS, the Developer ID
signature and Apple notarization ticket before installing the executable.
For publisher-authenticated verification using the signed checksum manifest,
see [Verify release artifacts](docs/VERIFY_RELEASES.md).

#### Windows

Install `nvfleetint`:

```powershell
Invoke-WebRequest https://github.com/NVIDIA/fleet-intelligence-client/releases/latest/download/install.ps1 -OutFile install.ps1
.\install.ps1
```

### Go SDK

Add the `nvfleetint` package to a Go module:

```bash
go get github.com/NVIDIA/fleet-intelligence-client/nvfleetint
```

See the [Go SDK guide](docs/SDK.md) for client setup and usage.

## Quick Start

Choose an NGC API key:

- [Personal API key](https://docs.nvidia.com/ngc/latest/ngc-user-guide.html#generating-a-personal-api-key)
  for individual use.
- [Service key](https://org.ngc.nvidia.com/identity-access/service-keys) for
  programmatic integrations.

```bash
nvfleetint auth add --profile default --key <your-ngc-api-key>
nvfleetint overview
nvfleetint node list
```

Credentials are stored in named profiles, so one installation can work against
several tenants or endpoints. Add more with `nvfleetint auth add --profile
<name>`, switch the default with `nvfleetint auth use`, and select one for a
single command with `--profile`:

```bash
nvfleetint node list --profile dev
```

Run `nvfleetint <command> --help` for command-specific flags.

## AI Skills

Install the bundled Fleet Intelligence skills for Claude Code, Cursor, and
Codex:

```bash
npx skills add NVIDIA/fleet-intelligence-client \
  --skill '*' \
  --agent claude-code --agent cursor --agent codex
```

Install and authenticate `nvfleetint` first. See
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
