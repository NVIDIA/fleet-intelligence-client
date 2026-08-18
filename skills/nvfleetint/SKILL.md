---
name: nvfleetint
description: Query NVIDIA Fleet Intelligence with the nvfleetint CLI. Use for ad hoc questions about fleets, nodes, GPUs, node groups, compute zones, alerts, agent health, firmware, verification, inventory, errors, or authentication. For a fleet-wide HTML snapshot use fleet-health-report; for a single-node RCA/RCCA use node-rca-rcca.
---

# Query Fleet Intelligence

Use live `nvfleetint` JSON; never answer fleet-state questions from memory. Read [`references/cli-contract.md`](references/cli-contract.md) before querying. Read [`references/auth.md`](references/auth.md) only for setup/profile work.

## Method

1. Run the smallest server-filtered query that answers the question.
2. Use `--output json`; give the user prose, not command dumps.
3. For counts, use `--page-size 1` and read `.total`. For identities, add `--view basic`. Use `--all` only when every row is required.
4. Lead with the answer; add a small table only for comparisons/listings.
5. Never infer absent fields or mistake auth failure for no results.

## Choose a command

| Need | Command |
| --- | --- |
| Fleet totals/health/metrics | `overview` |
| Zones or node groups | `computezone list`, `nodegroup list` |
| Nodes/current detail | `node list`, `node describe <uuid>` |
| Node health history | `node health <uuid>` |
| Fleet alert records | `alert list` |
| Alert impact/investigation | `alert summary`, `alert node`, `alert describe`, `alert options` |
| Raw events/histogram | `event list`, `event buckets` |
| Customer tags | `tag list` |
| Inventory/error reports | `report inventory`, `report error` |
| Verify signed report | `report verify` |
| Filter/sort values a command accepts | `node options`, `nodegroup options`, `alert options`, `xidburst options` |

### Overview and inventory

```bash
nvfleetint overview --output json
nvfleetint overview --include-metrics=false --output json
nvfleetint computezone list --include-metrics=false --output json
nvfleetint computezone list --output json
nvfleetint nodegroup list --compute-zone-ids <zone-id> --health Degraded,Unhealthy --output json
nvfleetint nodegroup list --gpu-type H100 --sort-by health --order desc --output json
```

`overview` is one non-paginated object. Use lists for rows behind its counts.

### Nodes

```bash
nvfleetint node list --health Degraded,Unhealthy --output json
nvfleetint node list --agent-status Offline --output json
nvfleetint node list --hostname gpu-node-7 --view basic --output json
nvfleetint node list --compute-zone-names ord --output json
nvfleetint node list --nodegroup-names training --output json
nvfleetint node list --agent-type oob --bmc-hostname bmc-01 --output json
nvfleetint node describe <node-uuid> --output json
nvfleetint node describe <node-uuid> --agent-type oob --output json
nvfleetint node health <node-uuid> --start <rfc3339> --end <rfc3339> --output json
```

Ask users for human-readable zone/group names, never IDs. Name filters are comma-separated partial matches. For exact scope, resolve with `computezone list --view basic` or `nodegroup list --view basic`, clarify ambiguous names with recognizable detail metadata, then use IDs internally. Accept an ID already supplied by the user.

Detailed node list and describe query both agent views by default and return
`{inband: ..., oob: ...}` in JSON. Use `--agent-type inband|oob` when only one
view is needed. OOB list supports `--bmc-hostname`; OOB describe includes full
inventory JSON and supports table sections `managers`, `systems`, `chassis`,
and `firmware`. Node basic rejects health, agent, verification, and firmware
filters and supports sorting by `hostname`, `nodeUUID`, or `bmcHostname`.
`node health` requires both absolute boundaries. It does not support `--window`.

Filter values:

| Flag | Values |
| --- | --- |
| `--health` | Healthy, Degraded, Unhealthy, Unknown |
| `--agent-status` | Online, Offline, Unknown |
| `--verification-check` | Verified, Unverified, Degraded, Pending, Unsupported, Unknown |
| `--firmware-check` | Passed, Failed, Unknown |

Node sort keys are `hostname`, `nodeUUID`, `healthStatus`, `nodegroup`, `computezone`, `gpuType`, `gpuCount`, `verificationCheck`, `agentStatus`, `agentVersion`, `kernelVersion`, `gpuDriverVersion`, `gpuFirmwareVersions`, and `bmcHostname`. The backend spelling `integrityCheck` remains accepted as an alias for `verificationCheck`. Node-group sort keys are `health` and `nodes`.

### Alerts and events

```bash
nvfleetint alert list --severity Critical --output json
nvfleetint alert list --node <node-uuid> --state Triggered --output json
nvfleetint alert summary --output json
nvfleetint alert summary --view historical --output json
nvfleetint alert node <node-uuid> --output json
nvfleetint alert node <node-uuid> --view historical --output json
nvfleetint alert describe <alert-uuid> --node <node-uuid> --output json
nvfleetint alert options --output json
nvfleetint event list --window 24h --output json
nvfleetint event buckets --window 168h --max-buckets 50 --output json
```

Alert severity is Critical/Warning; state is Detected/Triggered/Resolved. `alert summary`, `alert node`, and `alert options` default to the active view; use `--view historical` for history. Summary returns impacted nodes plus fleet-wide alert aggregates. Node returns alerts for one node. Describe returns one alert's event history. In node-alert results, Critical/Warning values are active severity; Detected/Resolved are inactive audit values. Don't count every non-Resolved row as active.

Events require `--window` or both `--start`/`--end`. Durations use Go units through hours—no `d`. Event list paginates; buckets do not.

### Tags and reports

```bash
nvfleetint tag list --prefix gpu --output json
nvfleetint tag list --computezone <zone-id> --output json
nvfleetint report inventory --compute-zone-ids <zone-id> --nodegroup-ids <group-id> --all --output json
nvfleetint report inventory --format csv --signed --output-path ./reports/
nvfleetint report error --view list --group-by error --window 168h --severities Critical,Fatal --output json
nvfleetint report error --view graph --window 24h --step 5m --output json
nvfleetint report verify --csv report.csv --bundle report.sig.bundle
```

Use at most one tag scope flag: `--node`, `--nodegroup`, or `--computezone`; tag list has no pagination. Report-error list requires `--group-by error|node`; only list supports `--all` and CSV. Signed inventory requires CSV. Report filters accept compute zone IDs, node group IDs, and tags; error reports also accept `--errors`, `--severities`, and graph-only `--step`.

## Example

For “Are any H100 nodes having problems?”, query H100 nodes filtered to Degraded/Unhealthy/Unknown. Include Unknown, read the filtered total, list only returned problem nodes, and do not claim the remaining H100 fleet is healthy without a separate query. Drill into requested UUIDs with `node describe` and node-scoped alerts.
