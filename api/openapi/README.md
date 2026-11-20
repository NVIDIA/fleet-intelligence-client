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

## Page/count migration

The event and XID page/count operations were imported from the reviewed backend
`perf/xid-api-query-performance` contract at
`7ce1e491f5fe8eccd84497b1f86bac30870c45ba`. They must be deployed before
releasing this CLI migration. They are not present on every deployed backend.

The targeted importer preserves unrelated operations and existing model
definitions. To repeat against a reviewed backend OpenAPI JSON:

```bash
python3 scripts/sync-page-count-contract.py /path/to/backend/openapi.json
make generate
```

Legacy operations remain in this SDK contract for compatibility, especially
`GetEventBuckets`, pending histogram retirement. The migrated list commands do
not request the legacy list endpoints. Do not publish the companion UAM
restrictions before the backend APIs, client rollout, and histogram decision.
