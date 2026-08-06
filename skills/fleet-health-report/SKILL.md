---
name: fleet-health-report
description: Generate a standalone fleet-wide HTML health snapshot from live nvfleetint data, including node health, capacity, alerts, error trends, and machines needing attention. Use for fleet dashboards, executive summaries, recurring issue analysis, or fleet-wide reports. Do not use for a single-node root-cause investigation.
---

# Fleet Health Report

Generate one offline HTML snapshot from fresh `nvfleetint` evidence. Use
`node-rca-rcca` for one-node investigations.

Read [`references/cli-contract.md`](references/cli-contract.md) before querying
and [`references/html-report-template.md`](references/html-report-template.md)
before writing. Read [`references/workspace.md`](references/workspace.md)
before capturing full lists.

## Rules

- Read only; never mutate fleet state.
- Use live JSON evidence from this invocation. Record command, exit status, and
  collection time; never invent missing values.
- Preserve timestamps/zones and mark unavailable values `N/A`.
- **Resolve scope first.** Ask for the entire fleet, compute-zone names, or
  node-group names when omitted. Never ask the user for IDs; accept one only if
  volunteered. Keep resolved IDs internal.
- **Probe before `--all`.**
- Use one alert strategy at a time and one capture file per pull. The client
  retries failed pages; never retry the whole command in a loop.
- Give alert collection at most 10 minutes and one strategy transition. On
  required-data failure, publish nothing and return probe totals plus the failed
  command/partition.
- Leave only the final HTML; follow the workspace reference for capture isolation
  and cleanup.

## Workflow

### 1. Resolve profile, scope, and window

Follow the contract's profile rules and pass one profile consistently.

Name filters are partial matches. For exact report scope, enumerate lightweight
identities, match names locally, and confirm ambiguous matches using recognizable
detail metadata—not raw IDs:

```bash
nvfleetint computezone list --view basic --all --output json
nvfleetint nodegroup list --view basic --all --output json
```

In normalized `--all` output, match `.items[].name` and retain its `.id`.

Then apply internal IDs to node queries:

```bash
nvfleetint node list --compute-zone-ids <zone-ids> ...
nvfleetint node list --nodegroup-ids <nodegroup-ids> ...
```

Use `overview` as the headline only for an entire-fleet report. For scoped
reports, derive totals from filtered nodes. The alert and error-report APIs
cannot filter by zone/group; state this limitation and never label tenant-wide
data as scoped.

Pin comparison boundaries once (24h default):

```bash
now=$(date +%s)
if date -u -d "@$now" +%Y-%m-%dT%H:%M:%SZ >/dev/null 2>&1; then
  # GNU/Linux date
  end=$(date -u -d "@$now" +%Y-%m-%dT%H:%M:%SZ)
  cur_start=$(date -u -d "@$((now - 86400))" +%Y-%m-%dT%H:%M:%SZ)
  prev_start=$(date -u -d "@$((now - 172800))" +%Y-%m-%dT%H:%M:%SZ)
elif date -u -r "$now" -v-1H +%Y-%m-%dT%H:%M:%SZ >/dev/null 2>&1; then
  # BSD/macOS date
  end=$(date -u -r "$now" +%Y-%m-%dT%H:%M:%SZ)
  cur_start=$(date -u -r "$now" -v-24H +%Y-%m-%dT%H:%M:%SZ)
  prev_start=$(date -u -r "$now" -v-48H +%Y-%m-%dT%H:%M:%SZ)
else
  echo "unsupported date implementation" >&2
  exit 1
fi
```

### 2. Probe cardinality

Run before full pulls, with supported scope filters:

```bash
nvfleetint node list <scope> --view basic --page-size 1 --output json
nvfleetint nodegroup list <scope> --view basic --page-size 1 --output json
nvfleetint computezone list <scope> --view basic --page-size 1 --output json
nvfleetint alert list --page-size 1 --output json
```

Read top-level totals. If an alert probe returns items but total is zero/absent,
treat cardinality as unknown and use the large-fleet path.

### 3. Collect inventory

```bash
nvfleetint overview --output json                         # entire fleet only
nvfleetint computezone list <scope> --all --output json
nvfleetint nodegroup list <scope> --all --output json
nvfleetint node list <scope> --all --output json
```

Keep overview metrics. For demonstrated slow node pages above 1,000 rows, use the
contract's longer per-request timeout guidance.

### 4. Collect alerts adaptively

Use **5,000 alerts** as the fixed threshold:

- Below 5,000: try one `alert list --all --output json`.
- At/above 5,000 or unknown: probe each severity, then run in parallel into
  separate captures:
  - `alert list --severity Critical --all --output json`
  - `alert list --severity Warning --all --output json`
- If the small unfiltered pull ends in timeout/final 5xx, malformed JSON, or an
  incomplete envelope, discard its capture and switch once to the split. Never
  retry unfiltered.
- Never overlap strategies or writers. On `Extra data`/truncation, discard the
  capture; if either severity partition then fails, stop.

Critical and Warning are disjoint and cover the documented domain. Validate each
partition before union. For scoped reports, join the complete alert set to the
scoped node UUIDs; surface alerts lacking UUIDs as unassignable.

### 5. Collect trends

```bash
nvfleetint report error --view list --group-by error   --start "$cur_start" --end "$end" --all --output json
nvfleetint report error --view list --group-by error   --start "$prev_start" --end "$cur_start" --all --output json
```

Sum row `count`; pagination total counts grouped rows, not errors. In scoped
reports, label this tenant-wide or omit it for strictly scoped evidence. Query
alert timeline or error-by-node only when materially useful.

## Validate and derive

Apply the shared completeness contract. Additionally:

- Validate `overview` as a single object.
- For split alerts, record
  `Critical.count + Warning.count` (reported total, otherwise validated item
  count). Duplicate IDs across severities are a contract error.
- Compare composed count with the unfiltered probe. Accept and disclose drift up
  to `max(100, 1% of probe)`. Beyond that, refresh the three one-row alert
  probes once; stop if still outside the bound.
- For entire-fleet reports, reconcile overview node/group/zone totals with list
  totals. Refresh probes once on an order-of-magnitude mismatch, then stop.

Derive only these semantics:

| Output | Source/rule |
| --- | --- |
| Totals and backend health % | Entire fleet: overview; scoped: filtered nodes. Label source. |
| Node status counts | Complete nodes; retain unknown/missing values. |
| Active alerts | Complete single/composed alerts by severity/component. Preserve unexpected severity as Other. |
| Derived health score | `100 * healthy / total`, labeled report-derived. Never average with backend score. |
| Fleet metrics | Present overview `metrics` fields verbatim; label tenant-wide in scoped reports. |
| Trend | Current/previous summed errors, delta, direction, percent; previous zero means new increase/no change. |
| Recurrence | Errors positive in both windows; label current-only new and previous-only no longer observed. |
| Attention | Critical count + Unhealthy first; then warnings/other alerts and Degraded/Unknown; then node-local agent/firmware/verification outliers. |

Join alerts to nodes by UUID. Stream captures or load once to build full
per-node severity/component counts. Cap only presentation tables (top 25 by
default) and state `showing 25 of N`.

A nonhealthy operational value shared by 100% of evaluated nodes is a
fleet-uniform signal: count it in overall status and mention it once, but exclude
it from top-N tie-breaking.

Overall status:

- Critical: any Critical alert or Unhealthy node.
- Needs attention: otherwise any Warning/Other alert, Degraded/Unknown node,
  offline/unknown agent, failed/unknown firmware, or non-verified/missing
  verification.
- Healthy: at least one node and no attention signal.
- No data: no nodes, or every node lacks health.

## Build and deliver

Follow the HTML template. Use semantic HTML, inline CSS/optional JS, no external
assets, HTML-escape backend strings, and include:

1. At a glance: scope, timestamp, totals, health, alerts, derived formula, and
   backend score when applicable.
2. Distribution and operational signals.
3. Equal-window trend.
4. Issue concentration.
5. Machines needing immediate attention.
6. Evidence-based action summary when supported.

Validate markup, escaping, headline values, closing HTML, and every required
section by ID:

```bash
for id in at-a-glance distribution trend concentration attention; do
  grep -q "id=\"$id\"" "$out" || exit 1
done
grep -q '</html>' "$out" && ! grep -qE '\[[a-z_]+\]' "$out"
```

Remove temporary artifacts and return the report path, scope, collection time,
and horizon. On evidence failure, return no report.
