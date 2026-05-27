# Roadmap

## Milestone 1: Repo and Client Foundation

- Curate public customer API spec.
- Configure OpenAPI client generation.
- Add generated low-level client under `internal/generated`.
- Add public SDK constructor and shared request configuration.
- Keep CI passing.

## Milestone 2: Auth and Core CLI UX

- Implement `nvfleetctl auth login/logout/status`.
- Store config at `~/.config/nvfleetctl/config.yaml` with mode `0600`.
- Add common flags for output and pagination.
- Add consistent table and JSON output helpers.

## Milestone 3: Read-Only Inventory

- Implement `computezone list`.
- Implement `nodegroup list`.
- Implement `node list`.
- Implement `node describe`.

## Milestone 4: Alerts and Reports

- Implement alert list and timeline commands.
- Implement inventory and error reports.
- Add agent skills for fleet health reports and node RCA/RCCA.

## Milestone 5: Write Operations

- Add compute zone and node group create/update/delete commands.
- Add node soft-delete.
- Add node tagging.
- Add `--dry-run` and `--yes` behavior for write commands.
