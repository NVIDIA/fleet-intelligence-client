# OpenAPI Contract

This directory contains the curated public Fleet Intelligence customer API
contract used by the Go SDK and `nvfleetint` CLI.

The backend service remains the source of truth for API implementation and
service behavior. This repository should only contain customer-facing API
surfaces needed by the SDK and CLI.

Examples of customer-facing surfaces include:

- `/v1/computezones`
- `/v1/nodegroups`
- `/v1/nodes`
- `/v1/alerts`
- `/v1/alert_timeline`
- `/v1/reports/inventory`
- `/v1/reports/error`

Generated Go client code is checked in under `internal/generated/fleetapi/`.
After changing `openapi.yaml` or `oapi-codegen.yaml`, regenerate it with:

```bash
make generate
```

Keep the generated client private to this repository. Public SDK types and
methods should be exposed through `nvfleetint`.
