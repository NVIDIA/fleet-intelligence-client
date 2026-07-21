---
name: node-rca-rcca
description: Produce a structured Root Cause Analysis (RCA) and Root Cause Corrective Action (RCCA) document for a specific degraded or unhealthy NVIDIA Fleet Intelligence node, using live nvfleetctl evidence. Use when the user asks to investigate a node health issue, root-cause a degraded or unhealthy node, explain why a node went unhealthy, analyze a node incident or alert history, or write an RCA/RCCA, post-incident, or corrective-action report for a node.
author: Emily Zhang <emizhang@nvidia.com>
---

# Node RCA/RCCA

Produce a structured, evidence-backed RCA/RCCA document for one degraded or
unhealthy node. Gather live `nvfleetctl` evidence about that node's health
transitions, recurring events, alert history, and affected components, then
synthesize a root cause and corrective/preventive actions. Compose the report to
fit the evidence available on each invocation, working within the required
sections and visual system below; do not use a fixed renderer or a deterministic
report-generation script.

## When to use

Use this skill for a **single-node investigation** — the user names or points at
one node (by UUID or hostname) and wants to know why it degraded and what to do
about it. For a fleet-wide snapshot across many nodes, use the
`fleet-health-report` skill instead.

## Instructions

1. Read the **Operating rules** before running anything.
2. **Resolve the target node** to a stable node UUID.
3. **Collect live evidence** with fresh `nvfleetctl` queries using
   `--output json`, covering health transitions, events, and alerts.
4. **Prove completeness** by validating every response before analysis.
5. **Analyze** the evidence into a timeline, a root cause with a confidence
   level, and RCCA actions — never inventing facts.
6. **Build the RCA/RCCA document** using the required sections and visual system.
7. **Deliver** the single report file, remove temporary artifacts, and return a
   clickable path.

## Operating rules

- Use read-only Fleet Intelligence commands only. This skill never changes fleet
  state; do not run write/delete/tag commands.
- Run fresh `nvfleetctl` backend queries during the current invocation. Never
  substitute examples, fixtures, cached output, prior reports, assumptions, or
  invented values (UUIDs, hostnames, timestamps, causes, remediation).
- Treat `nvfleetctl` output as **evidence**. Separate observed facts from
  inference, and label uncertain conclusions as `Likely` or `Not confirmed`.
- Prefer `--output json` for evidence capture. Use table output only for quick
  human scanning, never as the parsed source of a metric.
- Capture stdout, stderr, exit status, the exact command, and collection time
  for every query. Parse stdout as JSON only after a zero exit status.
- Preserve exact timestamps and time zones from command output. State when a
  timestamp is absent or ambiguous instead of guessing.
- Never expose service keys, credentials, environment variables, authorization
  headers, or raw config file contents in commands, logs, the report, or chat.
- Do not create persistent intermediate artifacts. The only file left in the
  workspace must be the final report. Keep command envelopes, parsed JSON, and
  scratch data in memory or OS-managed temporary locations that are cleaned up
  before finishing.
- If access, authentication, or API failures block evidence collection, do not
  publish a report. State the exact command attempted and what evidence is
  missing.

## Network execution

Every `nvfleetctl` backend command hits the Fleet Intelligence API over the
network. Run backend data commands (`node`, `alert`, `event`, `report`) with the
current AI agent platform's approved network-enabled or escalated command
mechanism. Do not hard-code platform-specific sandbox flags in this skill;
examples include Codex `sandbox_permissions: "require_escalated"` with a concise
justification, or another platform's equivalent approved network-enabled command
runner. Purely local metadata commands such as `which nvfleetctl`,
`nvfleetctl --help`.

In restricted sandboxes, a backend command may fail with a misleading TLS,
certificate, DNS, or network error before network-enabled execution is approved.
Treat that first restricted-environment failure as likely sandbox-related and
retry once with the platform's approved network-enabled mechanism. If no such
mechanism is available, stop and report that live Fleet Intelligence data cannot
be collected in the current environment. If the same class of error occurs after
approved network-enabled execution, report it as a real backend, network, TLS,
or proxy blocker instead of masking it as sandbox behavior.

Use the installed `nvfleetctl` binary and run each query with a suitable
`--timeout` (for example `--timeout 60s`). If the installed CLI differs from the
invocations below, inspect `nvfleetctl <command> --help`.

## Prerequisites

- `nvfleetctl` is installed and on `PATH`. Confirm with `which nvfleetctl`.
- The session is authenticated. Confirm without exposing secrets:

  ```bash
  nvfleetctl auth status
  ```

  `auth status` is diagnostic: it exits `0` and reports a `Connection:` line
  rather than failing on a bad key. Require `Connection: ok`. Treat
  `Connection: unauthorized` (a missing, invalid, or expired key),
  `unauthenticated`, or `error: ...` as an authentication/authorization failure,
  never an empty result. Likewise, treat exit code `77`, HTTP 401/403, or a JSON
  `api_error` on any subsequent data command as an auth failure. On any of these,
  ask the user to authenticate (`nvfleetctl auth login`) without asking them to
  paste a key into chat, and stop.

- A target node — a node UUID, or a hostname / partial hostname to resolve.

## Collect live evidence

Replace `<node_uuid>` with the resolved UUID throughout. Choose an investigation
window that comfortably brackets the incident; default to the last 7 days
(`[T-7d, T)` where `T` is the UTC collection time) unless the user specifies
another horizon, and record the exact RFC3339 boundaries you use.

### 1. Resolve the target node

If the user gives only a hostname or partial hostname, resolve it to a UUID and
confirm the intended node when multiple match:

```bash
nvfleetctl node list --hostname <hostname> --view detail --all --output json --timeout 60s
```

Use `--all` or explicit pagination before deciding the match set. Do not
disambiguate or select a UUID from only the first page of partial hostname
matches.

### 2. Capture current node state

```bash
nvfleetctl node describe <node_uuid> --output json --timeout 60s
```

This is the anchor evidence: current health, agent status, node group, compute
zone, GPU type/count, verification (integrity) check and reason, firmware check,
and component health counts. The backend JSON keeps its original field names —
read verification state from `integrityCheck` (with `integrityCheckReason`,
`lastIntegrityCheckTS`); "verification" is a display term only.

### 3. Capture node health transitions

```bash
nvfleetctl node health <node_uuid> --start <start-rfc3339> --end <end-rfc3339> --output json --timeout 60s
```

Both `--start` and `--end` are required RFC3339 timestamps. The raw backend JSON
is not a list of discrete transition events: read the per-interval segments from
`machineStatus` (each with `status`, `startTime`, `endTime`) and the aggregate
`healthSummary` (healthy/degraded/unhealthy percentages and durations); there is
no `items`/`pagination` envelope. Derive transitions from the boundaries between
consecutive `machineStatus` segments — use them to find the last known-normal
state, the first degraded/unhealthy interval, and any flapping between states.

### 4. Capture recurring events for the node

```bash
nvfleetctl event list --node <node_uuid> --window 168h --all --output json --timeout 60s
nvfleetctl event buckets --node <node_uuid> --window 168h --output json --timeout 60s
```

`event list` enumerates the node's events over the window; `event buckets`
aggregates them into time buckets to reveal recurrence and bursts (tune with
`--max-buckets`). A time range is required — use `--window <duration>` for a
relative range or `--start`/`--end` for an absolute range consistent with the
node health window. Add `--component <name>` to focus on a suspected failed
component.

### 5. Capture alert history and active alerts

```bash
nvfleetctl alert timeline --node <node_uuid> --all --output json --timeout 60s
nvfleetctl alert timeline --node <node_uuid> --active --all --output json --timeout 60s
```

The full timeline gives fired/resolved transitions across the incident; the
`--active` view isolates what is still firing now. When the timeline lacks enough
summary fields, add a node-scoped alert list:

```bash
nvfleetctl alert list --node <node_uuid> --all --output json --timeout 60s
```

`alert list` supports `--severity Critical|Warning`, `--state
Detected|Triggered|Resolved`, and `--component <name>` to narrow the set.

### 6. Describe the alerts that matter

For the earliest relevant alert, each currently active alert, repeated alerts,
and alerts tied to the suspected failed component, pull full detail (`--node` is
required):

```bash
nvfleetctl alert describe <alert_uuid> --node <node_uuid> --output json --timeout 60s
```

### 7. Optional blast-radius context

When the user asks whether the failure is isolated or fleet-wide, gather
recurrence context across nodes:

```bash
nvfleetctl report error --view list --group-by node --window 168h --all --output json --timeout 60s
```

## Prove completeness

Validate every response before analysis:

1. Require exit status `0`, nonempty stdout, valid JSON, and no top-level `error`
   / `api_error` object.
2. For every `--all` query, require the merged collection array (`items` for
   most commands; `nodes` for `report error --view list`) and a `pagination`
   object with `hasMore` false and `pagesFetched` at least `1`. The CLI only
   reports `pagination.total` when the backend does — when `total` is present
   and nonzero, require the collection length to equal it; when `total` is `0`
   or absent, rely on `hasMore` false (the CLI infers termination from a short
   final page), so do not treat a nonzero collection with `total` `0` as a
   mismatch.
3. Treat an empty result as valid only when the payload is genuinely empty
   (`total` `0`, `hasMore` false). A node with no alerts or events is a real
   finding, not a collection failure — but a node that `describe` cannot find is
   an error, so re-check the UUID.
4. Retry a completeness mismatch once (the backend may change during
   pagination). If it still mismatches, stop and report the dataset as
   incomplete rather than analyzing a partial set.
5. Keep every validated dataset through report generation. Showing a top-N table
   is allowed; collecting or analyzing only a top-N subset is not.

## Analyze the evidence

- Build a **timeline** from node health transitions, event buckets/list, alert
  timeline entries, and alert detail events. Preserve source timestamps and time
  zones. Identify the last known-normal state, the first observed symptom, the
  current state, and whether alerts are active or resolved.
- Determine **impact** from evidence only: health state, affected component,
  node group, compute zone, GPU type/count, agent status, verification
  (integrity) check, firmware check, and alert severity. If operational impact is
  not in the evidence, say impact was not confirmed rather than inferring it.
- Name the most specific **root cause** the evidence supports, and assign a
  confidence level:
  - `Confirmed` — evidence directly identifies the failed component and cause.
  - `Likely` — evidence points to a probable cause but does not prove it.
  - `Not confirmed` — evidence supports only a symptom; state the strongest
    supported hypothesis and what additional evidence would confirm it.
- List **contributing factors** only when supported by evidence — for example
  repeated alert transitions, flapping health, stale/offline agent status, failed
  verification or firmware checks, or a recurring component event in the buckets.
- Convert the RCA into **RCCA** items: containment, corrective action,
  preventive action, and validation. Include owners and due dates only when the
  user or evidence provides them; otherwise use `TBD`.
- Recommend **post-remediation validation** using read-only commands, usually
  `node describe` and `alert timeline --node <node_uuid> --active`, with the
  expected healthy result for each.
- Mark unavailable fields as `N/A`. Never manufacture timestamps, hostnames,
  alert causes, remediation, or utilization.

## Build the RCA/RCCA document

Produce a standalone `.html` file. Read
[`references/html-report-template.md`](references/html-report-template.md) before
drafting it, and follow that skeleton and status styling. Name it
`node-rca-rcca-<hostname-or-node-uuid>.html` unless the user provides an output
path.

Requirements:

- Use inline CSS and no external fonts, scripts, images, or network calls so the
  file opens offline.
- HTML-escape every backend-derived string before inserting it.
- Do not dump full raw JSON unless the user explicitly asks; summarize instead.
- Include, in order: an executive summary with current node status and root-cause
  confidence, node details, impact, a chronological incident timeline, root cause
  (with confidence, evidence, and reasoning), contributing factors, corrective
  and preventive actions, a validation plan, an evidence appendix with command
  provenance, and assumptions/unknowns.
- Show the root-cause confidence prominently as `Confirmed`, `Likely`, or
  `Not confirmed`.
- The evidence appendix lists the exact commands run and a one-line result
  summary each — command provenance, not secrets or raw config.

## Deliver

Inspect the generated file locally enough to catch malformed markup, missing
sections, unescaped content, and inconsistent facts. Cross-check headline values
(health state, active-alert count, root cause) against the validated JSON before
finishing. Confirm the report is the only generated file left in the workspace
and remove any temporary artifacts. Return a clickable path to the report and
briefly note the target node, collection time, and investigation window. If
generation stops for data integrity, authentication, or backend access, return
no fabricated report and state the concrete blocker.

## Example

> User: "Node gpu-h100-3271 has been flaky — write me an RCA."

1. Confirm auth with `nvfleetctl auth status`.
2. Resolve the hostname:
   `nvfleetctl node list --hostname gpu-h100-3271 --view detail --output json`,
   and confirm the single matching node UUID.
3. Set the window to the last 7 days (`T` = collection time). Collect evidence:
   - `nvfleetctl node describe <uuid> --output json`
   - `nvfleetctl node health <uuid> --start <T-7d> --end <T> --output json`
   - `nvfleetctl event list --node <uuid> --window 168h --all --output json`
   - `nvfleetctl event buckets --node <uuid> --window 168h --output json`
   - `nvfleetctl alert timeline --node <uuid> --all --output json`
   - `nvfleetctl alert timeline --node <uuid> --active --all --output json`
   - `nvfleetctl alert describe <alert_uuid> --node <uuid> --output json` for the
     earliest and each active alert.
4. Validate completeness on every response.
5. Analyze: health flips from `Healthy` to `Degraded` at the first XID alert; the
   `event buckets` output shows the same GPU component recurring; the active
   `alert timeline` still shows a Critical alert on that component. Record root
   cause as `Likely` (recurring GPU component fault) if detail does not confirm a
   specific failed part, and write containment / corrective / preventive / and
   validation actions.
6. Produce `node-rca-rcca-gpu-h100-3271.html`, remove temporary files, and return
   its path with the node, collection time, and 7-day window.
