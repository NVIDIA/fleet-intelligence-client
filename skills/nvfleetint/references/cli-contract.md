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

- Lists accept `--page`, `--page-size` (1–100), and `--all`. Non-paginated: `overview`, `node describe`, `node health`, `alert options`, `alert describe`, `node options`, `nodegroup options`, `xidburst options`, `event buckets`, `tag list`, and `report error --view overview|graph`.
- Single pages use backend arrays (`nodes`, `nodeGroups`, `computezones`, `alerts`, `events`) plus top-level `total`, `page`, and `pageSize`. Most use `hasMore`; `alert list` uses nonempty `pageCursorNext` instead. `alert summary` uses `nodes`, `alert node` uses `alerts`, and report-error list uses `nodes`. Detail `node list --output json` requires `--data-source inband|oob` and returns one source without a wrapper.
- `--all` normalizes every paginated list to `{items, pagination:{total,hasMore,pagesFetched}}`, including detail `node list` with its required `--data-source`. Non-paginated commands keep their native shape, except default `node describe` wraps its available source views as `{inband: ..., oob: ...}`.
- Count cheaply with the same filters plus `--page-size 1`, reading `.total`. For `alert summary`, `.total` is nodes with matching alerts; use `totalCritical` and `totalWarning` for alert aggregates. `tag list` is the exception: count its non-paginated `tags`.
- `--view basic` returns identities only: node returns UUID with available hostname and BMC identity; compute zone and node group return `id`/`name`. Node basic rejects `--health`, `--agent-status`, `--verification-check`, and `--firmware-check`, and supports identity, node-group, compute-zone, GPU-type, and GPU-count sorting, plus GPU type/count filters. With no source, node basic queries and `node options --view basic` use both sources; detail node list and options tables fetch both sources into separate sections. Detail JSON for these commands requires `--data-source` and returns one source without a wrapper; basic JSON allows omission. `report inventory` tables default to both sources with independent pagination (`--all` fetches all pages of each); JSON, CSV, and signed downloads require `--data-source inband|oob`. An explicit source returns one inventory report. `--agent-type` is no longer accepted on node commands; use `--data-source`. Compute-zone basic rejects `--include-metrics`. Node-group basic rejects `--include-metrics`, compute-zone name filters, health, gpu-type, and sorting.

- Alert summary and node queries accept comma-separated `--data-source` (`inband`, `oob`); omission includes both sources in one response, with matching counts and pagination. This filters the alert-producing source independently of current agent status. Flat and node-alert rows expose `dataSource`.
- Summary alone accepts `--inband-agent-status` and `--oob-agent-status` (`online`, `offline`, `not_enrolled`): OR within each flag, AND across flags, omitted means unrestricted. These are current statuses even in historical view.
- Summary rows retain `nodeName`, `bmcHostname`, and optional `agentStatus.inband`/`agentStatus.oob` objects (`status`, `lastSeenAt`); missing is not `not_enrolled`.

## JSON names

Use backend names: `healthStatus`, `verificationCheck`, `verificationCheckReason`, `lastVerificationCheckTS`, and `location`. `overview` calls the healthy-node count `healthNodeCount`, not `healthyNodeCount`.

## Completeness

Before analysis require exit 0, nonempty valid JSON, and no top-level `error`/`api_error`. For every `--all` response require an `items` array, `hasMore == false`, `pagesFetched >= 1`, and—when `total > 0`—item count equal to total. A missing or null total is unreported; a nonempty result with explicit `total: 0` is malformed. Empty with `hasMore: false` is valid.

Filtered pulls may be composed only when each is complete and the disjoint filters cover the domain. Record each reported total or validated item count, then union. Never analyze partial pages. On malformed/incomplete data, use only the report workflow's allowed fallback or stop.

The fleet-health workflow's deliberately bounded `alert summary --page-size 10` is the sole partial-page exception: use only its server-computed top-level aggregates for fleet-wide claims, label returned rows `showing N of total`, and never infer the attributes of unseen nodes.

## Secrets

Never expose credentials, authorization headers, environment values, or raw config. Check whether auth environment variables are set; never print them.
