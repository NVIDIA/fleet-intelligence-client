# nvfleetint CLI contract

Read this before querying live data.

## Commands and retries

- Use `--output json`; never parse tables. Confirm flags with `nvfleetint <command> [<subcommand>] --help`.
- `--timeout` is per request (default 2m). Don't lower it. Use 3m–5m for demonstrated slow node pulls above 1,000 rows and 5m–10m for demonstrated slow alert pulls; still bound the whole workflow separately.
- The client retries idempotent GET/HEAD requests up to three attempts for transient network failures and HTTP 408, 429, 500, 502, 503, and 504. During `--all`, only the failed page retries. Never retry the whole command in a loop.
- Exit 127 means not installed; use a release from <https://github.com/NVIDIA/fleet-intelligence-client/releases>.

## Auth and profiles

- Require `Connection: ok` from `auth status`; it exits 0 even when auth fails. Exit 77 or HTTP 401/403 is auth failure, not an empty fleet.
- Never request or expose an API key. Direct users to <https://org.ngc.nvidia.com/identity-access/service-keys> and `nvfleetint auth add --api-key <key>`.
- If the user names a tenant/environment, use that profile. If multiple profiles exist and none is named, ask which one. Pass the same explicit `--profile <name>` to every report command.
- Explicit `--profile` ignores `NVFLEETINT_API_KEY` and `NVFLEETINT_API_URL`.

## Pagination

- Lists accept `--page`, `--page-size` (1–100), and `--all`. Non-paginated: `overview`, `node describe`, `node health`, `alert options`, `alert describe`, `event buckets`, `tag list`, and `report error --view overview|graph`.
- Single pages use backend arrays (`nodes`, `nodeGroups`, `computezones`, `alerts`, `events`) plus top-level `total`, `page`, and `pageSize`. Most use `hasMore`; `alert list` uses nonempty `pageCursorNext` instead. `alert summary` uses `nodes`, `alert node` uses `alerts`, and report-error list uses `nodes`.
- `--all` normalizes every list to `{items, pagination:{total,hasMore,pagesFetched}}`.
- Count cheaply with the same filters plus `--page-size 1`, reading `.total`. For `alert summary`, `.total` is nodes with matching alerts; use `totalCritical` and `totalWarning` for alert aggregates. `tag list` is the exception: count its non-paginated `tags`.
- `--view basic` returns identities only: node returns hostname/UUID; compute zone and node group return `id`/`name`. Node basic rejects `--health`, `--agent-status`, `--verification-check`, and `--firmware-check`, and sorts only by hostname or nodeUUID. Node-group basic rejects health, gpu-type, and sorting.

## JSON names

Use backend names: `healthStatus`, `integrityCheck`, `integrityCheckReason`, `lastIntegrityCheckTS`, and `geoLocation`. `overview` calls the healthy-node count `healthNodeCount`, not `healthyNodeCount`.

## Completeness

Before analysis require exit 0, nonempty valid JSON, and no top-level `error`/`api_error`. For every `--all` response require an `items` array, `hasMore == false`, `pagesFetched >= 1`, and—when `total > 0`—item count equal to total. A nonempty result with `total: 0` means total was unreported; label it and rely on traversal evidence. Empty with `hasMore: false` is valid.

Filtered pulls may be composed only when each is complete and the disjoint filters cover the domain. Record each reported total or validated item count, then union. Never analyze partial pages. On malformed/incomplete data, use only the report workflow's allowed fallback or stop.

The fleet-health workflow's deliberately bounded `alert summary --page-size 10` is the sole partial-page exception: use only its server-computed top-level aggregates for fleet-wide claims, label returned rows `showing N of total`, and never infer the attributes of unseen nodes.

## Secrets

Never expose credentials, authorization headers, environment values, or raw config. Check whether auth environment variables are set; never print them.
