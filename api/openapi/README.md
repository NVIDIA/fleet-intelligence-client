# OpenAPI Contract

Place the curated public Fleet Intelligence customer API contract in this
directory.

The backend repository currently publishes generated Swagger docs. Before using
that spec here, curate it so this repository contains only customer-facing API
surfaces needed by the SDK and CLI.

Initial target surfaces:

- `/v1/computezones`
- `/v1/nodegroups`
- `/v1/nodes`
- `/v1/alerts`
- `/v1/alert_timeline`
- `/v1/reports/inventory`
- `/v1/reports/error`

The first implementation task should choose the generator and wire it into
`make generate`.
