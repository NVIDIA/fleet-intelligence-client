---
name: fleet-health-report
description: Generate a standalone fleet-wide HTML health snapshot from live nvfleetint data, including node health, capacity, active-alert impact, recent errors, and machines needing immediate attention. Use for fleet dashboards, executive summaries, or scoped fleet reports. Do not use for a single-node root-cause investigation.
---

# Fleet Health Report

Generate one offline HTML snapshot from fresh `nvfleetint` JSON. Read the [CLI contract](references/cli-contract.md), [HTML theme](references/html-theme.md), and [workspace guide](references/workspace.md) before collecting data.

## Workflow

### 1. Resolve and verify the profile

Resolve credentials before any report query:

```bash
nvfleetint auth list --output json
nvfleetint auth status --profile <profile> --output json
```

Use the user-named profile or the sole configured profile. If multiple profiles exist and none was requested, ask which one to use and identify the current one as the default suggestion. Require `connection` equal to `ok`, then pass the same explicit `--profile <profile>` to every API-backed command below.

### 2. Collect overview

Collect the tenant overview before inventory:

```bash
nvfleetint overview --profile <profile> --output json
```

Use it for the entire-fleet headline. In a scoped report, label it fleet-wide context and derive scoped totals from the filtered node list instead.

### 3. Collect inventory and resolve scope

Accept the entire fleet, compute-zone names, or node-group names. List compute zones first, node groups second, and nodes third. Resolve supplied names to IDs internally; clarify only ambiguous name matches.

Probe each list with the same filters, `--view basic` where supported, and `--page-size 1` before its full pull. Collect the full lists in this order:

```bash
nvfleetint computezone list --all --profile <profile> --output json
nvfleetint nodegroup list --all --profile <profile> --output json
nvfleetint node list <scope> --all --profile <profile> --output json
```

After the first two lists, apply resolved `--compute-zone-ids` or `--nodegroup-ids` to the node query.

### 4. Collect recent errors

Pin one 24-hour error window. Use GNU `date -u -d "@$now"` and `date -u -d "@$((now - 86400))"`, or BSD/macOS `date -u -r "$now"` and `date -u -r "$now" -v-24H`, formatted as RFC3339 UTC.

```bash
nvfleetint report error --view list --group-by error \
  --start "$start" --end "$end" --all \
  --profile <profile> --output json
```

Sum row `count`; pagination total counts grouped rows, not error occurrences. The error API cannot filter by zone/group, so label it tenant-wide in a scoped report or omit it when strictly scoped evidence is required.

### 5. Collect filtered active alerts

Discover the server-supported filter values first:

```bash
nvfleetint alert options --view active --profile <profile> --output json
```

From the returned `componentTypes` options, build a comma-separated list of component IDs excluding exact IDs `psirt` and `agent_liveness`. Stop if no component IDs remain.

Request the filtered count of all affected nodes and up to 10 machines ordered by active-alert count:

```bash
nvfleetint alert summary <scope> --view active \
  --component-type <component-types> \
  --sort-by alert --order desc --page-size 10 \
  --profile <profile> --output json
```

Use summary `.total` for all Nodes with Active Alerts and `.totalCritical`/`.totalWarning` for filtered fleet-wide severity totals. The bounded page contains up to 10 machines ordered by active-alert count; state `showing N of total` when applicable. Do not fetch every affected node merely to count or rank them.

For each returned UUID only, fetch its full filtered drill-down:

```bash
nvfleetint alert node <node_uuid> --view active \
  --component-type <component-types> --all \
  --profile <profile> --output json
```

Run at most four node calls concurrently. This workflow makes at most 12 calls: one options call, one summary call, and up to 10 node calls.

### 6. Derive and write the report

- Entire fleet: show `overview.healthPercentage` once as `Fleet Health Percentage`. Scoped: show `100 * healthy / total` once as `Healthy Node Percentage` and its formula. Do not show both or repeat the percentage in Fleet Summary.
- Keep node health/severity at source level; present fleet counts and distributions without an aggregate fleet severity badge.
- Join the returned summary machines to inventory by UUID and preserve the returned order.
- Use node-alert rows only for those machines' drill-down detail; do not infer unseen component distribution.

Apply the shared HTML theme and use these four report sections:

1. Fleet Summary: executive summary, fleet totals, and health distribution.
2. Fleet Distribution: compute zones, node groups, capacity, and operational signals.
3. Active Alerts: filtered Nodes with Active Alerts, severity aggregates, Machines Needing Immediate Attention (up to 10), and one expandable per-machine drill-down showing component, status, start time, last update, and available evidence text.
4. Error Distribution: grouped recent errors over the pinned window.

### 7. Validate and deliver

Apply the CLI contract's completeness checks. The bounded summary page is the only intentional partial result; use its top-level aggregates for fleet-wide claims. For an entire-fleet report, reconcile overview totals with complete inventory totals.

Validate the final HTML:

```bash
for id in at-a-glance distribution alerts errors; do
  grep -q "id=\"$id\"" "$out" || exit 1
done
grep -q '</html>' "$out" && ! grep -qE '\[[a-z_]+\]' "$out"
```

Leave only the final HTML and return its path, scope, collection time, and error window.
