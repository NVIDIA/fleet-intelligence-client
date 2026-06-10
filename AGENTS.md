# Repository Guidelines

## Project Structure & Module Organization

Fleet Intelligence ships a Go SDK and the `nvfleetctl` CLI. Keep Cobra command wiring in `cmd/nvfleetctl/`. Public SDK code belongs in `pkg/fleetintelligence/`. CLI-only details belong under `internal/`, including `internal/config/` and generated client code in `internal/generated/`. The public API contract is in `api/openapi/`; design notes live in `docs/`.

Maintain the intended dependency direction:

```text
cmd/nvfleetctl -> pkg/fleetintelligence -> internal/generated
```

CLI handlers should call SDK APIs instead of making direct HTTP calls.

## Build, Test, and Development Commands

- `make build`: builds `bin/nvfleetctl` with version ldflags.
- `make test`: runs all Go unit tests with `go test ./...`.
- `make lint`: checks `gofmt` output and runs `go vet ./...`.
- `make check`: runs lint, tests, and build; this is the CI validation path.
- `make fmt`: formats tracked Go files with `gofmt -w`.
- `go run ./cmd/nvfleetctl --help`: runs the CLI locally without building.
- `make setup-git-hooks`: enables local commit and secret-scan hooks.

## Coding Style & Naming Conventions

Use standard Go formatting: tabs from `gofmt`, idiomatic package names, and small focused files. Prefer clear exported names in `pkg/fleetintelligence` because that package is the public SDK surface. Keep `internal/` packages private to CLI or generated implementation concerns. Avoid adding dependencies unless they simplify maintained code or support the public API boundary.

## Comment Conventions

Start comments with a capital letter and omit trailing periods. Add a comment for every exported and unexported function describing what it does. Add a comment for every struct describing what it represents. Use inline comments only when they clarify non-obvious behavior.

## Testing Guidelines

Tests use Go’s built-in `testing` package and live next to implementation files as `*_test.go` files. Name tests with descriptive `Test...` functions and use `go test ./path -run TestName` for focused runs. Add coverage for CLI behavior, config parsing, and SDK request/response handling.

## Commit & Pull Request Guidelines

Commit subjects follow the enforced conventional-commit format:

```text
<type>(<scope>): <subject> [(GPUHEALTH-####)]
```

Allowed types are `feat`, `fix`, `perf`, `refactor`, `docs`, `chore`, `test`, and `revert`. Example: `feat(auth): add status command (GPUHEALTH-2284)`. Pull requests should describe the change, link Jira, call out API or CLI behavior changes, and include the `make check` result.

## Security & Configuration Tips

Install `trufflehog` before enabling hooks: `brew install trufflehog`. Do not commit credentials, generated secrets, or local config files. CLI config is expected at `~/.config/nvfleetctl/config.yaml` with restrictive permissions.
