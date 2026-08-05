---
name: node-rca-rcca
description: Investigate one degraded or unhealthy NVIDIA Fleet Intelligence node and generate an evidence-backed HTML RCA/RCCA from live nvfleetint data. Use for node incident analysis, health-transition explanations, root-cause analysis, corrective actions, or post-incident reports. Do not use for fleet-wide status reporting.
---

# Node RCA/RCCA

Investigate one node and produce an evidence-backed offline HTML RCA/RCCA. Use
`fleet-health-report` for fleet-wide status.

Read [`references/cli-contract.md`](references/cli-contract.md) before querying,
[`references/report-writing.md`](references/report-writing.md) before creating
scratch files, and
[`references/html-report-template.md`](references/html-report-template.md)
before writing the report.

## Rules

- Read only. Use fresh JSON evidence from this invocation.
- Separate observed facts from inference. Use confidence `Confirmed`, `Likely`,
  or `Not confirmed`; never invent causes, impact, timestamps, or remediation.
- Preserve timestamps/zones; mark unavailable fields `N/A`.
- Keep full payloads in a private temp directory, but only concise projections in
  context. Fetch once, parse the saved file repeatedly.
- Leave only the final report. On auth/API/evidence failure, publish nothing and
  name the failed command.

## Collect evidence

Default to seven days. Pin absolute boundaries for every node/event query; keep
the relative duration only for optional APIs that lack suitable scoped use:

```bash
end=$(date -u +%Y-%m-%dT%H:%M:%SZ)
start=$(date -u -v-7d +%Y-%m-%dT%H:%M:%SZ)  # Linux: -d '7 days ago'
window=168h  # optional blast-radius query only
```

Go durations end at `h`; convert days to hours. `7d` is invalid.

Batching:

1. Resolve UUID serially.
2. In parallel collect node describe, health, events/buckets, and active/full
   alert timelines.
3. Apply the fast-path gate.
4. Describe the selected alerts in parallel.

### 1. Resolve the node

For a hostname/partial hostname, search every page with the identity view:

```bash
nvfleetint node list --hostname <hostname> --view basic --all --output json
```

Confirm multiple matches. Use detail view only when metadata is needed to
disambiguate.

### 2. Capture current state

```bash
nvfleetint node describe <node_uuid> --output json > "$work/node-describe.json"
jq '{nodeUUID, hostname, healthStatus, nodeGroup, computeZone, gpuType, gpuCount,
     agentStatus, agentVersion, gpuDriverVersion, kernelVersion,
     integrityCheck, integrityCheckReason, firmwareCheck,
     healthyComponentCount, degradedComponentCount, unhealthyComponentCount,
     lastIntegrityCheckTS, lastUpdatedTS, tags}' "$work/node-describe.json"
```

Widen the projection against that file for suspected hardware
(`.resources.nicInfo`, `.resources.gpuInfo`, `gpuFirmwareVersions`); never
refetch describe.

### 3. Capture health transitions

```bash
nvfleetint node health <node_uuid> --start "$start" --end "$end" --output json
```

Read intervals from `machineStatus` and summary from `healthSummary`. Derive
last normal, first bad interval, current state, and flapping. This command is a
non-paginated single object: it accepts neither `--all` nor `--window`.

### 4. Capture events

```bash
nvfleetint event list --node <node_uuid> --start "$start" --end "$end" --all --output json
nvfleetint event buckets --node <node_uuid> --start "$start" --end "$end" --output json
```

Use list for event/component counts and buckets for recurrence/bursts. Add
`--component` only when narrowing a supported hypothesis. Never replace the
pinned boundaries with `--window`; relative windows drift as collection runs.

### 5. Capture alert history

```bash
nvfleetint alert timeline --node <node_uuid> --active --all --output json
nvfleetint alert timeline --node <node_uuid> --all --output json
```

The active query is authoritative for active count. On timeline rows,
Critical/Warning means active severity; Detected/Resolved means inactive audit
history. Never use `alertStatus != "Resolved"` as active.

Use a narrow node-scoped alert list only when timeline fields are insufficient:

```bash
nvfleetint alert list --node <node_uuid> --state Triggered --output json
```

### Fast path

Skip multi-alert selection when all are true:

- at most one active alert;
- zero events/buckets;
- one health segment with no transition.

Describe the single active alert, if any, then analyze. With no active alert,
skip alert describe. Otherwise continue.

### 6. Describe decisive alerts

Select only alerts that change the conclusion: earliest relevant, each active,
and most repeated on the suspected component; cap at 3–5. Record summarized
omissions.

```bash
nvfleetint alert describe <alert_uuid> --node <node_uuid> --output json   > "$work/alert-<alert_uuid>.json"
```

Run independent describes in parallel and parse saved files.

### 7. Optional blast radius

Only when requested:

```bash
nvfleetint report error --view list --group-by node   --window "$window" --all --output json
```

## Validate and analyze

Apply shared completeness rules. Empty alerts/events are valid; a missing node
describe is an error.

Build:

- Timeline: last normal, first symptom, current state, active/resolved status.
- Impact: only observed health, component, zone/group, GPU, agent,
  integrity/firmware, and severity evidence.
- Root cause: most specific supported cause and confidence.
- Contributing factors: only repeated/flapping/stale/check-failure evidence.
- RCCA: containment, corrective, preventive, and validation actions. Use
  owner/due date `TBD` unless supplied.
- Read-only validation commands with expected healthy outcomes.

### Optional code enrichment

Only translate raw generic codes/reasons already observed (for example XID 79).
Never send fleet identifiers externally. Prefer official NVIDIA documentation,
attribute source/title/URL, and confine enrichment to RCCA actions plus an
optional Reference section. It cannot change evidence, root cause, or confidence;
omit it when uncertain.

## Build and deliver

Follow the HTML template and the single-write/validation/cleanup sequence in the
report-writing reference. Inline all CSS; use no external assets; HTML-escape
backend strings and summarize rather than dump JSON.

Required order: executive summary, node details, impact, timeline, root cause,
contributing factors, actions, validation, optional Reference, evidence appendix,
assumptions/unknowns.

Cross-check health, active-alert count (from `--active`), root cause, and
confidence against validated JSON. Return the report path, node, collection
time, and window. For a compact example, read
[`references/example.md`](references/example.md).
