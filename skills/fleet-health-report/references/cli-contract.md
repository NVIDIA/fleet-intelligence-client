# nvfleetint CLI contract

Read this before querying live data.

## Commands and retries

- Use `--output json`; never parse tables. Confirm flags with `nvfleetint <command> [<subcommand>] --help`.
- `--timeout` is per request (default 2m). Don't lower it. Use 3m–5m for demonstrated slow node pulls above 1,000 rows and 5m–10m for demonstrated slow alert pulls; still bound the whole workflow separately.
- The client retries idempotent GET/HEAD requests up to three attempts for transient network failures and HTTP 408, 429, 500, 502, 503, and 504. During `--all`, only the failed page retries. Never retry the whole command in a loop.
- Exit 127 means not installed; use a release from <https://github.com/NVIDIA/fleet-intelligence-client/releases>.
- These workflows are read-only. `tag set` is the one command that writes to the fleet: never run it to gather data, and run it only when the user asks for that change by name, with `--yes` and the exact tag list they gave. It replaces a node's tags rather than adding to them.

## Auth and profiles

- Require `Connection: ok` from `auth status`; it exits 0 even when auth fails. Exit 77 or HTTP 401/403 is auth failure, not an empty fleet.
- Never request or expose an API key. Direct users to <https://org.ngc.nvidia.com/identity-access/service-keys> and `nvfleetint auth add`, which prompts for the key on stdin.
- If the user names a tenant/environment, use that profile. If multiple profiles exist and none is named, ask which one. Pass the same explicit `--profile <name>` to every report command.
- Explicit `--profile` ignores `NVFLEETINT_API_KEY` and `NVFLEETINT_API_URL`.

## Pagination

- Lists accept `--page`, `--page-size` (1–100), and `--all`. Non-paginated: `overview`, `node describe`, `node health`, `alert options`, `alert describe`, `node options`, `nodegroup options`, `xidburst options`, `event options`, `event buckets`, `tag list`, and `report error --view overview|graph`.
- Single pages use backend arrays (`nodes`, `nodeGroups`, `computezones`, `alerts`, `events`) plus top-level `total`, `page`, and `pageSize`. Most use `hasMore`; `alert list` uses nonempty `pageCursorNext` instead. `alert summary` uses `nodes`, `alert node` uses `alerts`, and report-error list uses `nodes`.
- `--all` normalizes every paginated list to `{items, pagination:{total,hasMore,pagesFetched}}`. Non-paginated commands keep their native shape, except default `node describe` wraps its available source views as `{inband: ..., oob: ...}`.
- Count cheaply with the same filters plus `--page-size 1`, reading `.total`. For `alert summary`, `.total` is nodes with matching alerts; use `totalCritical` and `totalWarning` for alert aggregates. `tag list` is the exception: count its non-paginated `tags`.
- `--view basic` returns identities only: node returns UUID with available hostname and BMC identity; compute zone and node group return `id`/`name`. Node basic rejects `--health`, `--agent-status`, `--verification-check`, and `--firmware-check`, and supports identity, node-group, compute-zone, GPU-type, and GPU-count sorting, plus GPU type/count filters. Compute-zone basic rejects `--include-metrics`. Node-group basic rejects `--include-metrics`, compute-zone name filters, health, gpu-type, and sorting.

## Data sources

- `--data-source` uses `inband`/`oob`: for nodes and inventory it selects the source view; for `alert summary`, `alert node`, `event list`, and `event buckets` it filters the record's producer, independently of current agent status. The latter accept comma-separated sources and default to both in one response. `alert list` has no source filter.
- Node basic list/options combine both sources when omitted, including JSON. Detail list/options and inventory tables fetch both sources separately; pagination and `--all` apply per source. Their detail JSON requires one explicit source and has no source wrapper; inventory CSV/signed downloads also require one. Inventory source-specific sorts (`hostname`, `publicIP`, `privateIP`, `nodeName`, `bmcMac`, `bmcIP`) require an explicit source; shared sorts apply to both.
- Node describe defaults to both source views; an explicit source selects one. Use `--data-source` in place of the removed node `--agent-type` flag.

- XID burst list/options default to `inband` and accept `oob` or `all`; use the same source for both. Combined results retain separate observations of a fault from each path. Describe resolves the source from the burst ID.
- `report error` selects one source (`inband` by default, or `oob`) for every view and format; OOB node rows use `nodeName` and `bmcMac`.

## Interpretation

- Alert-summary agent-status filters describe current enrollment/liveness even in historical view: OR within a flag, AND across flags, omitted means unrestricted. Missing status data is not `not_enrolled`.
- Event and XID burst identity, membership, and GPU type describe current inventory, not inventory at observation time. `event options` is a catalog, not evidence of matching events; `Others` matches names outside that catalog.

XID device-map keys are opaque labels; OOB keys may be GPU UUIDs or unresolved Redfish tokens.

## JSON names

Use backend names: `healthStatus`, `verificationCheck`, `verificationCheckReason`, `lastVerificationCheckTS`, and `location`. `overview` calls the healthy-node count `healthNodeCount`, not `healthyNodeCount`.

## Completeness

Before analysis require exit 0, nonempty valid JSON, and no top-level `error`/`api_error`. For every `--all` response require an `items` array, `hasMore == false`, `pagesFetched >= 1`, and—when `total > 0`—item count equal to total. A missing or null total is unreported; a nonempty result with explicit `total: 0` is malformed. Empty with `hasMore: false` is valid.

Filtered pulls may be composed only when each is complete and the disjoint filters cover the domain. Record each reported total or validated item count, then union. Never analyze partial pages. On malformed/incomplete data, use only the report workflow's allowed fallback or stop.

The fleet-health workflow's deliberately bounded `alert summary --page-size 10` is the sole partial-page exception: use only its server-computed top-level aggregates for fleet-wide claims, label returned rows `showing N of total`, and never infer the attributes of unseen nodes.

## Secrets

Never expose credentials, authorization headers, environment values, or raw config. Check whether auth environment variables are set; never print them.
