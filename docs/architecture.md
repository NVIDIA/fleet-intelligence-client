# Architecture

This repository owns two customer-facing clients for Fleet Intelligence:

- `nvfleetint`, a terminal-native CLI.
- `nvfleetint`, a public Go SDK.

## Dependency Direction

```text
cmd/nvfleetint -> nvfleetint -> internal/generated
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

- Use resource-first commands: `nvfleetint <resource> <verb> [args] [flags]`.
- Prefer table output for homogeneous lists.
- Support JSON output for scripts and automation workflows.
- Keep destructive operations behind confirmation prompts and `--dry-run`.
