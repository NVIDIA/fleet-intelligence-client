---
name: node-rca-rcca
description: Investigate one degraded or unhealthy NVIDIA Fleet Intelligence node and generate an evidence-backed HTML RCA/RCCA from live nvfleetint data. Use for node incident analysis, health-transition explanations, root-cause analysis, corrective actions, or post-incident reports. Do not use for fleet-wide status reporting.
---

# Node RCA/RCCA

Produce a structured, evidence-backed RCA/RCCA document for one degraded or
unhealthy node: gather live `nvfleetint` evidence about its health transitions,
recurring events, alert history, and affected components, then synthesize a root
cause and corrective/preventive actions. Compose each report to fit the evidence
on hand, within the required sections and visual system below — no fixed renderer
or deterministic generation script.

## When to use

For a **single-node investigation** — the user names one node (UUID or hostname)
and wants to know why it degraded and what to do. For a fleet-wide snapshot across
many nodes, use the `fleet-health-report` skill instead.

## Operating rules

- **Read-only.** Use read-only Fleet Intelligence commands; never run
  write/delete/tag commands.
- **Live evidence only.** Run fresh `nvfleetint` queries this invocation. Never
  substitute examples, fixtures, cached output, prior reports, or invented values
  (UUIDs, hostnames, timestamps, causes, remediation). Separate observed facts
  from inference; label uncertain conclusions `Likely` or `Not confirmed`.
- **Fenced enrichment only.** Looking up plain-language explanations of raw codes
  (XID numbers) and firmware/integrity reason strings is the *only* permitted
  non-`nvfleetint` input, strictly bounded per "Enrich codes and check-reasons":
  it may inform only the RCCA actions and a **Reference** subsection, never the
  timeline, impact, root cause, or confidence.
- **JSON is the source.** Prefer `--output json`; parse stdout as JSON only after
  a zero exit status. Preserve exact timestamps and time zones; state when one is
  absent or ambiguous rather than guessing. Mark unavailable fields `N/A`.
- **No secrets.** Never expose service keys, credentials, env vars, auth headers,
  or raw config contents in commands, logs, the report, or chat.
- **One artifact.** The only file left in the workspace is the final report. Keep
  envelopes, parsed JSON, and scratch in memory or OS temp files that are cleaned
  up before finishing.
- **Fail loud.** If access, auth, or API failures block evidence collection, do
  not publish a report — state the exact command attempted and what is missing.

## Running the CLI

Use the installed `nvfleetint` binary with a suitable `--timeout` (e.g.
`--timeout 60s`); if it differs from the invocations below, check
`nvfleetint <command> --help`.

## Prerequisites

- Run commands through the harness's local command-execution capability. The
  examples use POSIX shell syntax; on Windows, use equivalent PowerShell while
  preserving the `nvfleetint` arguments and evidence rules.
- `nvfleetint` on `PATH` — confirm with `command -v nvfleetint` on POSIX or
  `Get-Command nvfleetint` in PowerShell.
- A structured JSON processor is available. The examples use `jq`; an equivalent
  parser is acceptable, but grepping human-readable table output is not.
- An authenticated session — confirm with `nvfleetint auth status` (diagnostic:
  exits `0` and prints a `Connection:` line rather than failing on a bad key).
  **Require `Connection: ok`.** Treat `Connection: unauthorized`/`unauthenticated`
  or `error: ...` — and on any data command, exit code `77`, HTTP 401/403, or a
  JSON `api_error` — as an auth failure, not an empty result. On any of these, ask
  the user to run `nvfleetint auth add --key <key>` (never ask
  them to paste a key into chat) and stop.
- Credentials live in named profiles. Run `nvfleetint auth list` first. If the
  user named an environment or tenant, use that profile. Otherwise, if more than
  one profile exists, **ask the user which one owns the node and wait for their
  answer** — do not fall back to the current profile (the one marked `*`) or the
  first one listed. Querying the wrong tenant yields either no such node or, if
  the hostname collides, evidence from a different machine entirely. Once the
  profile is known, pass the same `--profile <name>` to every command below,
  including `auth status`, so all the evidence comes from the fleet that owns
  the node.
- A target node — a UUID, or a hostname/partial hostname to resolve.

## Collect live evidence

Replace `<node_uuid>` throughout. Default the window to the last 7 days
(`[T-7d, T)`, `T` = UTC collection time) unless the user specifies another
horizon, and record the exact RFC3339 boundaries.

**Fix the window once, then use it everywhere.** Pin three values up front and
substitute them into every query below — `<start-rfc3339>`, `<end-rfc3339>`, and
`<window>`, the same span as a duration (`168h` for the 7-day default). The
literal `168h` and `-v-7d` in the commands below are the *default* spelling: when
the user asks for a different horizon, recompute all three and change `node
health`, `event list`, `event buckets`, and `report error` together. A report
whose health window and event window disagree is wrong even if each query
succeeded.

**One rule governs speed and context: project large payloads at the source with
`jq`, and keep only anchor fields, non-resolved alerts, and aggregate counts in
context.** Raw `node describe` / `alert timeline` / `alert describe` payloads are
re-read on every later turn — that prefill is the single biggest time sink here.
Filtering governs only what enters context, never what you retain: fetch full
payloads to temp files (below) so you can widen a `jq` projection against the same
file without re-fetching.

**Batching plan** (minimize turns):

- **Batch A (serial, first):** resolve the UUID (step 1) — everything depends on it.
- **Batch B (parallel, one turn):** steps 2–5 together — `node describe`,
  `node health`, `event list`, `event buckets`, and both `alert timeline` queries
  (full and `--active`) are independent once the UUID is known. Do **not** include
  `alert list` (a step-5 fallback).
- **Fast-path gate:** after validating Batch B, decide whether the incident is
  trivial and skip most of Batch C (see "Fast path").
- **Batch C (parallel, after B):** the capped `alert describe` calls (step 6).

Fold these into the batches rather than spending separate turns:

- **Inline the window** in `node health` with shell substitution
  (`--start "$(date -u -v-7d +%Y-%m-%dT%H:%M:%SZ)" --end "$(date -u +%Y-%m-%dT%H:%M:%SZ)"`
  on macOS; `date -u -d '7 days ago'` on Linux). Still record the boundaries used.
- **Read the report template early** —
  [`references/html-report-template.md`](references/html-report-template.md), a
  local file independent of evidence — during the prereq checks or Batch B. It
  contains the complete document skeleton, including the invariant `<head>`/CSS.

Before creating any scratch files, read and follow
[`references/scratch-workspace.md`](references/scratch-workspace.md). It defines
the exclusively created, validated scratch path and cleanup rules for both
POSIX and PowerShell environments.

### 1. Resolve the target node

If given only a hostname/partial hostname, resolve it and confirm when multiple
match — use `--all` (not just the first page) before deciding:

```bash
nvfleetint node list --hostname <hostname> --view detail --all --output json --timeout 60s
```

### 2. Capture current node state (anchor)

Current health, agent status, node group, compute zone, GPU type/count,
verification (integrity) check + reason, firmware check, component health counts.
Read verification state from `integrityCheck` / `integrityCheckReason` /
`lastIntegrityCheckTS` ("verification" is a display term only). Fetch once to a
temp file, then project anchor fields:

```bash
work="<validated-work-path>" # exact path created using scratch-workspace.md
nvfleetint node describe <node_uuid> --output json --timeout 60s > "$work/node-describe.json"
jq '{nodeUUID, hostname, healthStatus, nodeGroup, computeZone,
     gpuType, gpuCount, gpuArchitecture: .resources.gpuInfo.architecture,
     agentStatus, agentVersion, gpuDriverVersion, kernelVersion,
     integrityCheck, integrityCheckReason, integrityCheckExtraInfo, firmwareCheck,
     healthyComponentCount, degradedComponentCount, unhealthyComponentCount,
     lastIntegrityCheckTS, lastUpdatedTS, enrolledAt, tags}' "$work/node-describe.json"
```

Widen the projection against the **same file** when a suspected component needs it
(e.g. `.resources.nicInfo` for `network-ethernet` — the hardware detail lives
under `resources`, alongside `gpuInfo`; or top-level `gpuFirmwareVersions` for a
firmware-check failure) — never re-invoke `node describe`; the payload is already
on disk.

### 3. Capture node health transitions

```bash
nvfleetint node health <node_uuid> --start <start-rfc3339> --end <end-rfc3339> --output json --timeout 60s
```

Both timestamps required. There is no `items`/`pagination` envelope: read
per-interval segments from `machineStatus` (`status`, `startTime`, `endTime`) and
the aggregate `healthSummary`. Derive transitions from segment boundaries — last
known-normal state, first degraded/unhealthy interval, and any flapping.

### 4. Capture recurring events

```bash
nvfleetint event list --node <node_uuid> --window 168h --all --output json --timeout 60s
nvfleetint event buckets --node <node_uuid> --window 168h --output json --timeout 60s
```

`event list` enumerates events; `event buckets` aggregates them into time buckets
to reveal recurrence/bursts (`--max-buckets` to tune). A time range is required
(`--window` or `--start`/`--end`, consistent with the health window). Add
`--component <name>` to focus a suspected component. Project the list to
aggregates in context:

```bash
jq '{total: (.items|length), pagination,
     by_component: (.items|group_by(.component)|map({component: .[0].component, count: length}))}'
```

### 5. Capture alert history and active alerts

```bash
nvfleetint alert timeline --node <node_uuid> --active --all --output json --timeout 60s
nvfleetint alert timeline --node <node_uuid> --all --output json --timeout 60s
```

`--active` isolates what is still firing (usually small — pipe directly). The full
timeline can be large on a flapping node; reduce it to still-firing alerts plus
per-component counts, keeping `pagination` for the completeness checks:

```bash
jq '{firing: [.items[] | select(.alertStatus == "Critical" or .alertStatus == "Warning")],
     total: (.items | length),
     by_component: (.items | group_by(.component) | map({component: .[0].component, count: length})),
     pagination}'
```

**`alertStatus` on the timeline is not an alert state.** It carries
`Critical`/`Warning` — a *severity* — while the alert is active, and
`Detected`/`Resolved` once it is inactive (from the audit history). So
`select(.alertStatus != "Resolved")` does **not** mean "active": it keeps
`Detected`, which is an inactive value, and inflates the count. Treat only
`Critical`/`Warning` as still firing here, and let the `--active` query be the
authority for the active-alert count the report headlines.

For a large timeline, fetch to a temp file first (as in step 6) so a `jq` typo
can't trigger a re-fetch. **Fallback only** — when the timeline lacks fields you
need (state, severity, message) for the alerts that matter, add a node-scoped,
**narrowed** `alert list` (never `--all`); it supports
`--severity Critical|Warning`, `--state Detected|Triggered|Resolved`,
`--component <name>`:

```bash
nvfleetint alert list --node <node_uuid> --state Triggered --output json --timeout 60s
```

### Fast path (trivial incident)

Most degraded nodes have a single dominant cause and no event noise. **After
validating Batch B**, if all of these hold:

- **≤ 1 active alert** (from `alert timeline --active`), and
- **0 events** in the window (`event list` total `0`, `event buckets` empty), and
- **no flapping** — a single `machineStatus` segment over the window (no
  intra-window transition),

then take the fast path: **skip the multi-alert selection and huge-timeline
scanning machinery of step 6.** Describe just the single active alert (if any) to
get its reason string and onset — one `alert describe` — and go straight to
analysis and the report. Root cause can still be `Confirmed` when the anchor
(`node describe` integrity/component fields) and that one alert independently name
the same component and cause. With **0 active alerts**, skip Batch C entirely and
root-cause from the anchor and health segments alone. Note in the report that the
resolved-alert history was summarized from the timeline aggregate, not described
individually. Otherwise (multiple active alerts, event bursts, or flapping), run
the full step 6.

### 6. Describe the alerts that matter

Pull full detail only for alerts that change the analysis: the earliest relevant
alert, each active alert, and the most-repeated alert on the suspected component.
**Cap at ~3–5 UUIDs** — describing every alert on a flapping node is the largest
open-ended time sink and rarely adds signal past the first few. Select UUIDs from
Batch B, then run the `alert describe` calls as one parallel batch (`--node`
required).

**Fetch once to a temp file, then parse the file — never pipe the CLI into `jq`.**
A response can be hundreds of KB, so every fetch is expensive:

```bash
work="<validated-work-path>" # exact path created using scratch-workspace.md
out="$work/alert-<alert_uuid>.json"
nvfleetint alert describe <alert_uuid> --node <node_uuid> --output json --timeout 60s > "$out"
jq '<expression>' "$out"   # re-run jq against the file as many times as needed
```

- **Never** `alert describe ... | jq ...` — a `jq` typo silently re-runs the fetch.
- **Never** recover from a `jq` error by re-invoking the CLI; a parse failure means
  your `jq` was wrong — fix the expression and re-run it against the same file.
- Use a plain ASCII pipe `|`. If the schema is unknown, learn it in the **same**
  pass: the decisive values often live inside `timeline[]`, not the top-level
  object, so dump keys plus a sample (`jq '{keys: keys, sample: .timeline[0]}'`) or
  write one broad expression (first + last timeline event, `severity_changed`
  event, `[.timeline[].message] | unique`). **Stop as soon as the reason string
  and onset are in hand** — do not re-scan a multi-MB timeline to confirm what the
  list/timeline already established. Extract all aggregates in a single `jq` pass.

If more than ~5 alerts look relevant, describe the capped set and note in the
report which additional UUIDs were summarized rather than described, so the
omission is explicit.

### 7. Optional blast-radius context

When the user asks whether the failure is isolated or fleet-wide:

```bash
nvfleetint report error --view list --group-by node --window 168h --all --output json --timeout 60s
```

## Prove completeness

Validate every response before analysis:

1. Require exit status `0`, nonempty stdout, valid JSON, no top-level
   `error`/`api_error`.
2. For every `--all` query, require the merged collection `items` and a
   `pagination` with `hasMore` false and `pagesFetched` ≥ `1`. `--all` always
   normalizes to `{items, pagination}`, whatever the backend called the array —
   `alert timeline` returns `alerts` and `report error --view list` returns
   `nodes` on a single page, but both become `items` under `--all`. Do not look
   for the backend's key on an `--all` response; it is absent, and treating that
   as a failed check would reject a complete dataset. (Without `--all`, the
   payload is the raw backend body and the backend's own key applies.)
   When `pagination.total` is present and nonzero, require
   the collection length to equal it; when `total` is `0` or absent, rely on
   `hasMore` false (do not flag a nonzero collection with `total` `0` as a
   mismatch).
3. An empty result is valid only when genuinely empty (`total` `0`, `hasMore`
   false) — a node with no alerts/events is a real finding. But a node `describe`
   cannot find is an error: re-check the UUID.
4. Retry a completeness mismatch once; if it persists, stop and report the dataset
   as incomplete rather than analyzing a partial set.
5. Keep every validated dataset through report generation. A top-N *table* is
   fine; collecting or analyzing only a top-N *subset* is not.

## Analyze the evidence

- **Timeline** from health transitions, event buckets/list, alert timeline, and
  alert detail. Preserve timestamps/zones. Identify last known-normal, first
  symptom, current state, and whether alerts are active or resolved.
- **Impact** from evidence only: health state, affected component, node group,
  compute zone, GPU type/count, agent status, integrity/firmware checks, alert
  severity. If operational impact is not in the evidence, say it was not confirmed.
- **Root cause** — name the most specific cause the evidence supports, with a
  confidence level:
  - `Confirmed` — evidence directly identifies the failed component and cause.
  - `Likely` — evidence points to a probable cause but does not prove it.
  - `Not confirmed` — evidence supports only a symptom; state the strongest
    hypothesis and what would confirm it.
- **Contributing factors** only when evidence-backed (repeated alert transitions,
  flapping, stale/offline agent, failed integrity/firmware check, recurring
  component event).
- **RCCA** items: containment, corrective, preventive, validation. Owners/due
  dates only when provided; otherwise `TBD`.
- **Post-remediation validation** using read-only commands, usually
  `node describe` and `alert timeline --node <node_uuid> --active`, with the
  expected healthy result for each.
- Never manufacture timestamps, hostnames, causes, remediation, or utilization.

## Enrich codes and check-reasons (optional)

Optional, additive, tightly fenced. Inputs are only the **raw codes/reason strings
already in the telemetry** (GPU `XID` numbers, `integrityCheckReason` /
firmware-check reason strings). Purpose: translate opaque tokens into
plain-language explanations that make corrective actions actionable. It does not
change how root cause, confidence, timeline, or impact are derived.

- **Generic tokens only.** Query the bare code/string (e.g. "GPU XID 79",
  "double-bit ECC error"). Never send node UUIDs, hostnames, IPs, serial numbers,
  or any fleet identifier to an external source.
- **Authoritative first:** official NVIDIA docs (XID reference, DCGM/GPU health) →
  internal KB → general web. Capture source title + URL.
- **Confine it** to (a) the corrective/preventive RCCA actions and (b) a
  **Reference** subsection listing each code/reason, its meaning, and its source.
  It must not alter the timeline, impact, root cause, or confidence.
- **Attribute explicitly**; never present an external explanation as
  telemetry-derived, and never let it raise the confidence tier.
- **Bake it in** as static, attributed prose so the file still opens offline.
- **Omit when unsure** — leave the token as-is and omit the Reference subsection
  entirely when no enrichment was performed.

## Build the RCA/RCCA document

Standalone `.html`, named `node-rca-rcca-<hostname-or-node-uuid>.html` unless the
user gives a path. Follow the skeleton and status styling in
[`references/html-report-template.md`](references/html-report-template.md).

**Write the whole file in one shot.** The document chrome — `<!doctype>`,
`<head>`, the full `<style>` block, and the closing tags — is invariant; copy it
verbatim from the template's HTML Skeleton, then fill in the `page-header`
masthead (health + confidence badges) and the `page-main` sections. Emit it once,
in full, via a single heredoc:

```bash
slug=$(printf '%s' "<hostname-or-node-uuid>" | tr -c 'A-Za-z0-9._-' '-' \
  | sed 's/^[.-]*//')
[ -n "$slug" ] || { echo "refusing: empty report name" >&2; exit 1; }
out="node-rca-rcca-$slug.html"
node_uuid="<node_uuid>"
printf '%s' "$node_uuid" \
  | grep -qiE '^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$' \
  || { echo "refusing: malformed node UUID" >&2; exit 1; }
work="<validated-work-path>" # exact path created using scratch-workspace.md
cat > "$out" <<'NVFLEET_REPORT'
<!doctype html>
... entire document, chrome copied verbatim from the template ...
</html>
NVFLEET_REPORT
# same turn: fail on leftover placeholders, require the documented section count,
# confirm the document closed, then clean the validated scratch directory
n=$(grep -c 'id="[a-z-]*"' "$out"); echo "sections: $n"
! grep -qE '\[[a-z_]+\]' "$out" && { [ "$n" -eq 11 ] || [ "$n" -eq 10 ]; } \
  && grep -q '</html>' "$out" \
  && find "$work" -mindepth 1 -maxdepth 1 \( -type f -o -type l \) -delete \
  && rmdir -- "$work"
```

- **Quote the heredoc delimiter** (`<<'NVFLEET_REPORT'`) — the document embeds
  shell metacharacters (`$(date ...)`, `$PATH`, backticks) that must stay literal.
  Use a distinctive delimiter that cannot appear in the content.
- A shell redirect overwrites any prior report at the path in one shot (no
  read-before-write restriction). Compose the document once — don't emit a
  skeleton and edit sections in afterward.
- Chaining the `grep`/cleanup checks onto the write collapses the write and
  validation turns into one — the largest avoidable round-trip in the skill.
- Set `out` and `work` **in this same command**. Use the exact scratch path
  returned during exclusive creation; do not reconstruct it from the node UUID.
  Revalidate the path and ownership as specified in "Cleanup discipline" before
  reading or deleting anything beneath it.
- The `slug` line folds the hostname (or UUID) into a bare filename component —
  anything outside `[A-Za-z0-9._-]` becomes `-`, and leading dots/dashes are
  stripped, so a hostname carrying `/` or `..` cannot redirect `$out` out of the
  working directory or turn it into an option-looking argument. When the user
  gives an explicit output path, use it as given and skip the slug.
- The `grep -c` prints how many section `id`s made it into the file and the chain
  requires that count to be 11 (or 10 when the optional Reference section is
  omitted) — any other count fails the `&&` chain and leaves `$work` in place, so
  the document can be rewritten without re-collecting.

Requirements:

- Inline CSS only; no external fonts/scripts/images/network calls (opens offline).
- HTML-escape every backend-derived string.
- Do not dump raw JSON unless the user asks — summarize.
- Sections in order: executive summary (with status + confidence), node details,
  impact, incident timeline, root cause (confidence + evidence + reasoning),
  contributing factors, corrective/preventive actions, validation plan, optional
  **Reference** (only when enrichment was performed), evidence appendix (exact
  commands + one-line result each — provenance, not secrets), assumptions/unknowns.
- Show root-cause confidence prominently as `Confirmed`/`Likely`/`Not confirmed`,
  and choose each badge `[*_class]` from the template's status map by actual value.

## Deliver

The mechanical checks (placeholders, sections, closing tags, temp cleanup) already
ran in the write command's chained `&&` — don't repeat them. Here, do only the
judgment cross-check: confirm the headline values (health state, active-alert
count — from the `--active` query, not a `!= "Resolved"` filter — and root cause)
match the validated JSON, that the printed section count is right, and that the
report is the only generated file left. If a check flagged a problem, fix the
body and re-run the
single write command rather than editing the emitted file. Return a clickable path
plus the target node, collection time, and window. If generation stops for data
integrity, auth, or backend access, return no fabricated report and state the
concrete blocker.

For a compact end-to-end example, read
[`references/example.md`](references/example.md).
