---
name: nvfleetint
description: Query NVIDIA Fleet Intelligence with the nvfleetint CLI. Use for ad hoc questions about fleets, nodes, GPUs, node groups, compute zones, alerts, agent health, firmware, verification, inventory, errors, or authentication. For a fleet-wide HTML snapshot use fleet-health-report; for a single-node RCA/RCCA use node-rca-rcca.
---

# Answering fleet questions with nvfleetint

**Fleet Intelligence** is the NVIDIA backend product for understanding a GPU fleet — its inventory, health, alerts, and reports. `nvfleetint` is the command-line client for that backend's customer API.

Treat `nvfleetint` as **your data-access tool**: it's how you read live state from the Fleet Intelligence backend. Whenever the user asks about the state of their fleet, don't answer from memory or guess — run the right `nvfleetint` command(s), parse the output, and answer their actual question in plain language. The data is live, so every answer should be grounded in a command you just ran.

## Runtime requirements

- Run `nvfleetint` through the harness's local command-execution capability.
- Require `nvfleetint` on `PATH` and authenticated network access to the Fleet
  Intelligence backend.
- Treat shell snippets as POSIX examples. On Windows, use equivalent PowerShell
  commands without changing the `nvfleetint` arguments or evidence rules.
- Use `jq` only when it is available; otherwise parse the JSON with another
  local structured-data tool rather than grepping table output.

## How to work

The user wants an *answer*, not a command dump. So:

1. **Run commands with `--output json`** (or the command's JSON `--format`). JSON parses reliably; table output is for humans and is easy to misread. You read the JSON, the user gets prose.
2. **Summarize findings in plain language.** Lead with the answer ("3 of your 48 nodes are unhealthy"), then the supporting detail. Show a small table only when the user is comparing items or explicitly wants a listing.
3. **Filter at the source.** These commands have rich filter flags (`--health`, `--severity`, `--gpu-type`, etc.). Filtering server-side is faster and more accurate than pulling everything and grepping.
4. **Keep commands small and fast.** A fleet can hold hundreds of thousands of nodes/alerts/events, so an unbounded pull is slow and can time out or blow up your context. Ask for the least data that answers the question: filter tightly, and page in small chunks (`--page-size` is 1–100, default is fine). **For a count from a paginated list, don't fetch the rows at all** — run with `--page-size 1` and read the top-level `total` (see [Counting without fetching](#counting-without-fetching)). `tag list` is the exception: it has no pagination flags and returns `tags` without a top-level `total`. Avoid `--all` unless you truly need every row, and ask for confirmation before using it unless the user explicitly requested all records, a complete export, or a fleet-wide report. Even then, prefer a tight filter first.
5. **Don't invent data.** If a field isn't in the output, say so rather than guessing. UUIDs, hostnames, and counts must come from real command output.

Before relying on output, confirm the tool is installed and authenticated — see [Setup and auth](#setup-and-auth). If `nvfleetint` isn't found (e.g. `command not found`, exit code **127**), it isn't installed — don't try to build it from source; point the user at the releases page as described in [Setup and auth](#setup-and-auth). If a command exits with code **77**, that's an auth/permission failure, not a real "no results" — handle it as described there.

## Mapping questions to commands

Use this to pick the entry point. Each command takes `--output json` and
`--timeout <dur>` (e.g. `30s`). Paginated `list` commands also take `--all`,
`--page`, and `--page-size` (1–100); `tag list` does not paginate.

| The user is asking about… | Start here |
| --- | --- |
| A one-shot fleet summary — total/healthy/unhealthy counts, top-line metrics | `overview` |
| Regions / zones / where capacity lives | `computezone list` |
| Node groups, their health %, GPU utilization | `nodegroup list` |
| Individual nodes — health, GPU type/count, agent online/offline, firmware/verification | `node list`, then `node describe <uuid>` for one node |
| How one node's health changed over a time window | `node health <uuid>` |
| Active problems, severities, what's firing now | `alert list`, `alert timeline`, `alert describe` |
| Raw event stream or an event histogram over time | `event list`, `event buckets` |
| What customer tags exist (optionally scoped to a resource) | `tag list` |
| A full inventory snapshot (export, audit, signed bundle) | `report inventory` |
| Error trends / counts over a time range | `report error` |
| Checking a previously downloaded signed report | `report verify` |

### Fleet overview

For a fast, top-line answer ("how's the fleet doing?", "how many nodes total / unhealthy?") start with `overview` — a single call returns fleet-wide counts plus summary metrics, no pagination.

```bash
nvfleetint overview --output json
nvfleetint overview --include-metrics=false --output json   # counts only, skip the metrics block
```

Use it for the headline number; drop to `node list` / `nodegroup list` when the user wants the actual nodes behind the count.

### Inspecting the fleet

```bash
# Compute zones (regions/sites and their node counts)
nvfleetint computezone list --output json
nvfleetint computezone list --zone-ids zone-1,zone-2 --output json

# Node groups — filter by health, GPU type; sort by health/nodes
nvfleetint nodegroup list --output json
nvfleetint nodegroup list --health Degraded,Unhealthy --output json
nvfleetint nodegroup list --gpu-type H100 --sort-by health --order desc --output json

# Nodes — the most filterable command
nvfleetint node list --output json
nvfleetint node list --health Degraded,Unhealthy --output json
nvfleetint node list --agent-status Offline --output json
nvfleetint node list --hostname gpu-node-7 --output json          # partial hostname match
nvfleetint node list --gpu-type H100 --sort-by hostname --order asc --output json

# Everything about one node (system info, resources, network, health, components)
nvfleetint node describe <node-uuid> --output json

# One node's health status timeline + summary over a window (both --start and --end REQUIRED, RFC3339)
nvfleetint node health <node-uuid> --start 2026-07-14T00:00:00Z --end 2026-07-21T00:00:00Z --output json
```

`node describe` is the current-state snapshot; `node health` answers "when did this node go bad / how has its health trended?" over an explicit window (there's no `--window` shortcut here — pass absolute `--start`/`--end`).

Filter vocabularies (case-sensitive, comma-separate multiple values):
- **health**: `Healthy`, `Degraded`, `Unhealthy`, `Unknown`
- **agent-status**: `Online`, `Offline`, `Unknown`
- **verification-check**: `Verified`, `Unverified`, `Degraded`, `Pending`, `Unsupported`, `Unknown`
- **firmware-check**: `Passed`, `Failed`, `Unknown`
- **node sort-by**: `hostname`, `nodeUUID`, `health`, `nodeGroup`, `computeZone`, `gpuType`, `gpuCount`, `integrityCheck`, `agentStatus` (+ `--order asc|desc`) — the `integrityCheck` sort key keeps the backend field name and corresponds to the `verification-check` filter

`--output json` returns the raw backend field names, not the display terms: verification state is `integrityCheck` (with `integrityCheckReason`, `lastIntegrityCheckTS`) and location is `geoLocation`. The `verification-check`/"location" naming applies only to the CLI flag and table output.

To get an exact **count**, use the [Counting without fetching](#counting-without-fetching) trick — `--page-size 1` and read the top-level `total`. Don't eyeball the length of one page (it's only the first page), and don't pull `--all` just to count — on a large fleet that fetches thousands of rows you'll immediately throw away.

### Counting without fetching

When the user only wants a number ("how many nodes are unhealthy?", "how many critical alerts?"), you don't need the rows — you need the `total`. For a paginated `list`, run the same filtered command with **`--page-size 1`** and read the **top-level `total`** field. This is one tiny request regardless of fleet size, and every filter still applies, so the count is exactly the filtered count. Do not apply this shortcut to `tag list`: it has no pagination flags and returns only `tags`, with no top-level `total`; count the returned tags instead.

```bash
nvfleetint node list --health Unhealthy --page-size 1 --output json      # -> read .total
nvfleetint alert list --severity Critical --page-size 1 --output json    # -> read .total
nvfleetint event list --window 24h --page-size 1 --output json           # -> read .total
```

Two different JSON shapes carry the total, depending on whether you paged or pulled everything:

- **Single page** (default / `--page-size N`, no `--all`): `total` is at the **top level**, alongside the resource-specific item array. Keys are `total`, `page`, `pageSize`, and a "more pages?" indicator; the items live under a per-resource key — `nodes` / `nodeGroups` / `computezones` (lowercase) / `alerts` / `events`. Read `.total`. The more-pages indicator is `hasMore` (a bool) on every list **except `alert list`**, which instead exposes `pageCursorNext` (a string that's non-empty when more pages exist and absent/empty otherwise) and has **no `hasMore` field** — so for alerts, check `pageCursorNext`, not `hasMore`.
- **`--all`** (merged across every page): the shape is `{"items": [...], "pagination": {"total": N, "hasMore": ..., "pagesFetched": ...}}`. Read `.pagination.total`.

So for a count, `--page-size 1` → `.total`; only reach for `--all` → `.pagination.total` when you actually want the rows too.

### Alerts

```bash
# Alerts firing now; filter by severity, state, component, and/or node
nvfleetint alert list --output json
nvfleetint alert list --severity Critical --output json
nvfleetint alert list --severity Critical --node <node-uuid> --output json
nvfleetint alert list --state Triggered --component GPU --output json

# Timeline: which nodes have alert history, or one node's history
nvfleetint alert timeline --output json                 # all nodes with history
nvfleetint alert timeline --active --output json        # only currently-active alerts
nvfleetint alert timeline --node <node-uuid> --output json

# Full event history for a single alert (note: --node is REQUIRED here)
nvfleetint alert describe <alert-uuid> --node <node-uuid> --output json
```

`alert list` filter vocabularies: **severity** = `Critical`, `Warning`; **state** = `Detected`, `Triggered`, `Resolved`; `--component` matches a component name (e.g. `GPU`).

When the user says "what's wrong right now?" prefer `alert list --severity Critical` and `node list --health Unhealthy,Degraded`. Use the timeline when they ask about history or recurrence.

### Events

Events are the raw, time-stamped fleet event stream (below the alert layer). Every event command **requires a time range** — either `--window <dur>` (relative) or `--start`/`--end` (absolute RFC3339, used together) — and can be narrowed by `--node` and `--component`.

```bash
# Individual events over a range
nvfleetint event list --window 24h --output json
nvfleetint event list --window 168h --node <node-uuid> --component GPU --output json
nvfleetint event list --start 2026-05-01T00:00:00Z --end 2026-05-08T00:00:00Z --output json

# Time-bucketed counts for a histogram (--max-buckets 1-1000, default 100)
nvfleetint event buckets --window 24h --output json
nvfleetint event buckets --window 168h --max-buckets 50 --output json
```

`event list` paginates (`--all`, `--page`, `--page-size`); `event buckets` does not — it returns the bucketed series in one call. Reach for events when the user wants the granular "what happened, and when" detail that `alert list` (current problems) and `report error` (aggregate counts) don't give.

### Tags

```bash
# All unique customer tags across the fleet
nvfleetint tag list --output json
nvfleetint tag list --prefix gpu --output json                       # case-insensitive prefix filter

# Scope to one resource (use at MOST one of --node / --nodegroup / --computezone)
nvfleetint tag list --node <node-uuid> --output json
nvfleetint tag list --computezone <zone-id> --prefix env --output json
```

`tag list` answers "what tags are in use?" — the resource filters are mutually exclusive, but `--prefix` can combine with any one of them. There's no pagination here.

### Reports

```bash
# Inventory snapshot of the whole fleet
nvfleetint report inventory --all --output json
nvfleetint report inventory --format csv > inventory.csv        # CSV for the user
nvfleetint report inventory --format csv --signed               # signed bundle (CSV + cosign signature)
nvfleetint report inventory --format csv --signed --output-path ./reports/

# Error report over a time range. Pick ONE time selector:
#   --window <dur>            relative, e.g. 24h, 168h (Go duration; no d unit)
#   --start ... --end ...     absolute RFC3339, used together
nvfleetint report error --window 24h --output json                          # overview (totals)
nvfleetint report error --view list --group-by error --window 168h --output json
nvfleetint report error --view list --group-by node \
  --start 2026-05-01T00:00:00Z --end 2026-05-08T00:00:00Z --output json
nvfleetint report error --view graph --window 24h --output json             # time series

# Verify a signed inventory bundle the user already downloaded
nvfleetint report verify --csv inventory_report_<ts>.csv --bundle inventory_report_<ts>.sig.bundle
nvfleetint report verify --csv report.csv --bundle report.sig.bundle --key signing-key.pub   # offline
```

`report error` notes: `--view list` requires `--group-by error|node`. `--format csv` is only valid with `--view list`. Default view is `overview`.

## Setup and auth

### Installation

If `nvfleetint` isn't on the user's PATH (a command fails with `command not found` / exit code **127**), it isn't installed. Don't build it from source — direct the user to download a prebuilt binary for their platform from the releases page:

<https://github.com/NVIDIA/fleet-intelligence-client/releases>

Tell them to grab the latest release asset matching their OS/architecture, extract it, and put `nvfleetint` somewhere on their PATH. Once it's installed, re-run `nvfleetint auth status` and continue.

### Auth

Credentials live in named **profiles** in `~/.config/nvfleetint/config.yaml` (mode 0600). A profile pairs an API key with an API URL, so one machine can reach several tenants or endpoints.

```bash
nvfleetint auth status                   # check before querying if unsure
nvfleetint auth list                     # which profiles exist, and which is current
nvfleetint auth add --api-key <ngc-api-key>         # no name: the "default" profile
nvfleetint auth add <name> --api-key <ngc-api-key>
nvfleetint auth add <name> --api-key <ngc-api-key> --api-url https://api.fleet-intelligence.nvidia.com
nvfleetint auth use <name>     # change the default
nvfleetint auth add <name> --api-key <rotated-key> --yes   # existing name: rotate the key
nvfleetint auth remove <name>
```

`auth add/remove/use` take the profile as a **positional** `<name>` — it is the thing being changed. Don't pass `--profile` to them; they don't accept it. On `auth add` the name is optional and means the profile called `default`; prefer that form when the user hasn't mentioned multiple tenants, rather than inventing a name for them. `auth remove` and `auth use` always require the name.

There is no `auth update`: `auth add` on an existing name changes that profile in place (partial — an omitted flag keeps the stored value), which is also the key-rotation path. Replacing a key a profile already has prompts for confirmation, and **you cannot answer that prompt** — you have no terminal, so the command fails with "cannot prompt for confirmation". Pass `--yes` only when the user has actually asked to replace that profile's key; otherwise report the prompt back and let them decide. Nothing else prompts, so the fixes the CLI suggests in its own error messages (`auth add <name> --api-key ...` for a profile with no key, `auth add <name> --api-url ...` for a rejected endpoint) are safe to run as printed. **Check `auth list` before adding**, or a mistyped name that happens to exist will overwrite a working key. The output says `added` vs `updated` — read it back to the user.

Every API-backed command, plus `auth status`, instead accepts `--profile <name>` to use one profile for a single invocation (`nvfleetint node list --profile dev`). Without it, commands use the current profile — the one marked `*` in `auth list`. If the user mentions more than one environment, tenant, or org, run `auth list` first and ask which profile they mean rather than guessing.

The API URL must be `https` (plain `http` is only allowed for `localhost`), so never suggest an `http://` endpoint.

`auth status` verifies the **effective** credentials against the backend and prints `Profile:`, `API URL:`, and `API key:` lines showing what resolved and where each value came from. Pass `--profile <name>` to check a specific profile. It's diagnostic: it exits `0` and reports a `Connection:` line rather than failing on bad credentials. Read that line — don't treat exit 0 as "authenticated." Require `Connection: ok`; treat `Connection: unauthorized` (missing, invalid, or expired key), `unauthenticated`, or `error: ...` as an auth failure and stop. The `API URL:` line tells you which endpoint was checked, which is how you spot a profile or env override pointing somewhere unexpected.

Credentials resolve highest-first: `--profile`, then the current profile with `NVFLEETINT_API_KEY` / `NVFLEETINT_API_URL` overlaid on top. **Selecting a profile explicitly ignores those two env vars entirely** — that is deliberate, so a stale variable can't send one tenant's key to another tenant's endpoint. So a bad env override only affects commands that *don't* pass `--profile`. If `auth status` (without `--profile`) reports a wrong `API URL:` or an `API key:` sourced from the environment, have the user `unset NVFLEETINT_API_KEY NVFLEETINT_API_URL` in their shell (and remove them from any shell profile that exports them), then re-run `auth status`. Check whether the vars are *set*, not what they contain — never print the value of `NVFLEETINT_API_KEY`.

If a query fails with **exit code 77** or a 401/403, the user isn't authenticated (or the key lacks permission). Don't report this as "no nodes found." Instead, run `nvfleetint auth status` to confirm, then tell the user to generate an NGC API key at <https://org.ngc.nvidia.com/identity-access/service-keys> and run `nvfleetint auth add --api-key <key>` (add a `<name>` before `--api-key` only if they use more than one tenant; the same command rotates the key of a profile that already exists). Never ask the user to paste an API key into the chat, and never echo a key you happen to see — it's a secret. `auth list` and `auth status` never print keys, only whether one is configured.

## Worked example

User: *"Are any of my H100 nodes having problems?"*

1. `nvfleetint node list --gpu-type H100 --health Degraded,Unhealthy,Unknown --output json` — include `Unknown` so nodes the backend can't report on don't silently disappear. The tight filter keeps the result small, so the first page is usually the whole answer (check `hasMore`; only page further or add `--all` if it's set and you need the rest). For just the *count*, `--page-size 1` and read `total`. (On `alert list` the more-pages field is `pageCursorNext`, not `hasMore` — see [Counting without fetching](#counting-without-fetching).)
2. Read `total` and the `nodes` — `total` counts only the filtered matches, not the H100 fleet. Break the count out by state: *"Yes — 2 H100 nodes are in trouble: `gpu-node-12` (firmware check Failed) and `gpu-node-31` (agent Offline). 1 more, `gpu-node-07`, is reporting Unknown health."* Don't claim anything about the rest without querying for it.
3. If they want detail on one, follow with `nvfleetint node describe <uuid> --output json` and `nvfleetint alert list --node <uuid> --output json`.

That's the loop: pick the command, filter tightly, run with JSON, answer in prose, drill down on request.

## Discovering flags

This file covers the common paths. For the authoritative, current flag list of any command, run `nvfleetint <command> --help` (or `nvfleetint <command> <subcommand> --help`) rather than guessing — the CLI is the source of truth if it has changed.
