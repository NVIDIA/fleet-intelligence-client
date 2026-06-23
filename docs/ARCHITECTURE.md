# Architecture

This repository owns two customer-facing clients for Fleet Intelligence:

- `nvfleetctl`, a terminal-native CLI.
- `pkg/fleetintelligence`, a public Go SDK.

## Dependency Direction

```text
cmd/nvfleetctl -> pkg/fleetintelligence -> internal/generated
```

The CLI should call the SDK. Command handlers should not make direct HTTP calls
to the Fleet Intelligence API.

The SDK should wrap the generated OpenAPI client and expose a smaller,
handwritten, stable Go API.

## Repository Boundary

The backend service remains the source of truth for API implementation and
service behavior. This repository should depend only on the public customer API
contract copied into `api/openapi/`. Do not import private backend packages.

## CLI Conventions

- Use resource-first commands: `nvfleetctl <resource> <verb> [args] [flags]`.
- Prefer table output for homogeneous lists.
- Support JSON output for scripts and automation workflows.
- Keep destructive operations behind confirmation prompts and `--dry-run`.
