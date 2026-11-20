# Architecture

This repository owns two customer-facing entry points for Fleet Intelligence:

- `cmd/nvfleetint`, the terminal-native CLI binary.
- `nvfleetint`, the public Go SDK.
- `nvfleetint/verify`, an opt-in public Go package for signed report verification.

## Dependency Direction

```text
cmd/nvfleetint -> internal/cmd/<command> -> internal/cmdutil -> nvfleetint -> internal/generated
                         |                      |
                         |                      -> internal/output
                         -> nvfleetint

internal/cmd/report -> nvfleetint/verify
```

The CLI should call the SDK. Command handlers should not make direct HTTP calls
to the Fleet Intelligence API.

The SDK should wrap the generated OpenAPI client and expose a smaller,
handwritten, stable Go API.

Each layer has one job:

- `cmd/nvfleetint` is the binary. It receives the link-time build values and
  assembles the command tree, and holds nothing else.
- `internal/cmd/<command>` is one package per top-level command. Packages here
  do not import each other; anything two of them need belongs in `cmdutil`.
  Command packages may import the SDK directly because they own resource
  behavior and rendering decisions.
- `internal/cmdutil` is the shared cobra/SDK glue: common flags, the client
  those flags build, parsing, pagination, and error rendering.
- `internal/output` is pure formatting. It takes plain values, never SDK
  models, so a formatter that needs one lives in `cmdutil` instead.
- `nvfleetint` is the main public SDK surface. Rules about what the API accepts
  live here, so the CLI states them nowhere.
- `nvfleetint/verify` is separate so the sigstore, rekor, and TUF dependency
  tree stays out of callers that never verify a signed report.

## Repository Boundary

Event and XID burst lists use separate page and exact-count APIs. The SDK
combines their responses to preserve existing list totals and raw JSON fields;
`SkipTotal` opts out of the count query and leaves Total unqueried. Relative
windows are resolved to fixed absolute bounds before paired requests. The CLI's
`--all` traversal freezes bounds across every page, follows `hasMore`, and asks
for the exact count only once after traversal. Separate requests are not a
database snapshot. Count failures fail the command without emitting partial
successful output, and there is no fallback to restricted legacy endpoints.

`event count` and `xidburst count` call only the count APIs. `event buckets`
remains a histogram operation: a scalar count is not a compatible replacement.
Its retirement and the legacy SDK method require a separate compatibility
decision before the corresponding service-key restriction can be published.

The backend service remains the source of truth for API implementation and
service behavior. This repository should depend only on the public customer API
contract copied into `api/openapi/`. Do not import private backend packages.

## CLI Conventions

- Use resource-first commands: `nvfleetint <resource> <verb> [args] [flags]`.
- Prefer table output for homogeneous lists.
- Support JSON output for scripts and automation workflows.
- Keep destructive operations behind confirmation prompts and `--dry-run`.
