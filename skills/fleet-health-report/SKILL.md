---
# SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

name: fleet-health-report
description: Generate a standalone HTML snapshot of NVIDIA Fleet Intelligence health from live nvfleetctl backend data. Use when the user asks for a fleet health report, status dashboard, executive health summary, most-alerted nodes, alert or error trends, recurring fleet issues, or an HTML fleet snapshot.
author: Emily Zhang <emizhang@nvidia.com>
---

# Fleet Health Report

Produce a self-contained HTML report from live Fleet Intelligence data. Compose the layout and presentation to fit the data available on each invocation, working within the visual system and required sections defined below; do not use a fixed renderer or deterministic report-generation script.

## Instructions

1. Read the **Non-negotiable data rules** before running anything.
2. **Collect live data** with fresh `nvfleetctl` queries, using `--all --output json` and the two adjacent trend windows.
3. **Prove completeness** by validating every response before deriving metrics.
4. **Derive metrics** exactly as specified — do not invent semantics or values.
5. **Build the HTML snapshot** using the required sections and the NVIDIA-aligned dark visual system.
6. **Deliver** the single `.html` file, remove temporary artifacts, and return a clickable path.

## Non-negotiable data rules

- Run fresh `nvfleetctl` backend queries during the current invocation. Never substitute examples, fixtures, cached output, prior reports, assumptions, or invented values.
- Add both `--all` and `--output json` to every command that requests fleet, alert, inventory, or report data and that supports `--all` (see the per-command notes under Collect live data). Do not use a non-paginated data command or a view that rejects `--all`.
- Treat CLI help, version, and local authentication inspection as metadata operations rather than fleet-data requests. Do not cite them as evidence of fleet health.
- Capture stdout, stderr, exit status, the exact command, and collection time for every query. Parse stdout as JSON only after a zero exit status.
- Do not create persistent intermediate artifacts. The only file generated in the workspace must be the final standalone `.html` report. Keep any command envelopes, parsed JSON, helper scripts, scratch data, or temporary validation files in memory, or use OS-managed temporary locations that are cleaned up before finishing. Do not leave `*.envelope.json`, `*.json`, `*.py`, logs, or auxiliary files behind.
- Never expose service keys, credentials, environment variables, or authorization headers in commands, logs, source data, HTML, or chat.
- Do not publish a successful report when any required query is missing, stale, invalid, unauthorized, or incomplete. Explain the failure and identify the affected command without guessing the missing results.

## Collect live data

### Network execution

Every `nvfleetctl` backend data command hits the Fleet Intelligence API over the network. Run backend data commands (`computezone`, `nodegroup`, `node`, `alert`, `report`) with the current AI agent platform's approved network-enabled or escalated command mechanism. Do not hard-code platform-specific sandbox flags in this skill; examples include Codex `sandbox_permissions: "require_escalated"` with a concise justification, or another platform's equivalent approved unsandboxed/network-enabled command runner. Purely local metadata commands such as `which nvfleetctl`, `nvfleetctl --help`, and `nvfleetctl auth status` may run without network escalation.

In restricted sandboxes, a backend command may fail with a misleading TLS, certificate, DNS, or network error before network-enabled execution is approved. Treat that first restricted-environment failure as likely sandbox-related and retry once with the platform's approved network-enabled mechanism. If no such mechanism is available, stop and report that live Fleet Intelligence data cannot be collected in the current environment. If the same class of error occurs after approved network-enabled execution, report it as a real backend, network, TLS, or proxy blocker instead of masking it as sandbox behavior.

Use the installed `nvfleetctl` binary. Run these required snapshot queries with a suitable timeout:

```bash
nvfleetctl computezone list --all --output json --timeout 60s
nvfleetctl nodegroup list --all --output json --timeout 60s
nvfleetctl node list --all --output json --timeout 60s
nvfleetctl alert list --all --output json --timeout 60s
```

Use two adjacent, equal-duration absolute windows for trend and recurrence analysis. Default to 24-hour windows unless the user requests another horizon. Let `T` be the UTC collection time, then compare `[T-24h, T)` with `[T-48h, T-24h)` and record the actual RFC3339 boundaries:

```bash
nvfleetctl report error --view list --group-by error --start <current-start> --end <current-end> --all --output json --timeout 60s
nvfleetctl report error --view list --group-by error --start <previous-start> --end <previous-end> --all --output json --timeout 60s
```

Use `report error --view list`, not `overview` or `graph`, because only list view supports `--all`. Sum each returned row's `count` to calculate the number of errors in a window; `pagination.total` is the number of grouped rows, not the error-event count.

Run additional queries only when they materially clarify the report, and still include both required flags. Examples include:

```bash
nvfleetctl alert timeline --all --output json --timeout 60s
nvfleetctl alert timeline --node <node-uuid> --all --output json --timeout 60s
nvfleetctl report error --view list --group-by node --start <start> --end <end> --all --output json --timeout 60s
```

If the installed CLI differs, inspect `nvfleetctl <command> --help`.

## Prove completeness

Validate every fleet-data response independently before calculating metrics:

1. Require exit status `0`, nonempty stdout, valid JSON, and no top-level `error` object.
2. Require a top-level `items` array and `pagination` object from every `--all` query.
3. Require `pagination.hasMore` to be `false`, `pagination.pagesFetched` to be at least `1`, and `items.length` to equal `pagination.total`.
4. Treat an empty fleet as valid only when `items` is empty, `total` is `0`, and `hasMore` is `false`. Report metrics as `N/A` where a denominator is zero.
5. Retry a completeness mismatch once because the backend may change during pagination. If it still mismatches, stop and report the dataset as incomplete.
6. Keep every validated dataset through report generation. Displaying a top-N table is allowed, but collecting or analyzing only a top-N subset is not.

Treat exit code `77`, HTTP 401/403, or a JSON `api_error` as authentication or authorization failure, never as an empty fleet. Ask the user to authenticate without requesting that they paste a key into chat. A certificate, TLS, DNS, or network failure on a backend command should be retried once with the platform's approved network-enabled execution mechanism when the first attempt ran in a restricted sandbox; after a network-enabled retry, surface the failure as the concrete backend access blocker.

## Derive metrics without inventing semantics

- Join nodes and alerts by the stable node UUID. Preserve unmatched alert records and label unavailable node metadata instead of dropping them.
- Count health, agent, firmware, and verification states from the complete node dataset. Keep `Unknown`, missing, and unexpected values visible rather than coercing them to healthy.
  - The backend JSON keeps its original field names: read verification state from `integrityCheck` (with `integrityCheckReason`, `lastIntegrityCheckTS`) and location from `geoLocation`. "Verification" and "location" are display terms only — the JSON never contains those keys.
- Count active alerts by severity and component from the complete alert dataset. Preserve unexpected severities as `Other`.
- Calculate the displayed **node health score** as `100 * healthy nodes / total nodes`, rounded reasonably. Label it as a report-derived node health rate and show the formula; do not imply that the backend supplied a composite score.
- Derive at-a-glance status transparently:
  - Use **Critical** when any active Critical alert or Unhealthy node exists.
  - Otherwise use **Needs attention** when any active Warning or `Other` (unrecognized-severity) alert, Degraded or Unknown node, offline or unknown agent, failed or unknown firmware check, or degraded, unverified, pending, unsupported, unknown, or missing verification state exists.
  - Otherwise use **Healthy** when at least one node exists and no attention signal exists.
  - Use **No data** only when no nodes were returned, or when *every* returned node is missing the required health fields. When only some nodes lack a required field, keep the known values, count the missing ones as `Unknown`/`N/A`, and still derive the status from the populated nodes — never suppress a valid metric because part of the fleet is unpopulated.
- Rank machines needing attention using explicit evidence. Prioritize critical-alert count, Unhealthy health, total active-alert count, warning-alert count, Degraded or Unknown health, offline agent, failed firmware, and verification problems. Show the reasons and counts used; do not present the ranking as a backend-defined risk score.
- Calculate trend from the summed error counts in the two equal windows. Show current, previous, absolute delta, direction, and percentage change. If the previous value is zero, show `new increase` or `no change` instead of an infinite percentage.
- Identify recurring issues as error names present with positive counts in both windows. Rank by current count, persistence, affected-node count when available, and change. Label types found only in the current window as new and types found only in the previous window as no longer observed. Do not claim event-level recurrence from aggregate data.
- Mark unavailable fields as `N/A`. Never manufacture timestamps, hostnames, alert causes, remediation, utilization, or capacity.

## Build the HTML snapshot

Create one standalone `.html` file in the current workspace or the user's requested path, and leave no other generated files in the workspace. Use semantic HTML with inline CSS and only optional inline JavaScript. Avoid external fonts, CDNs, remote images, network calls, and runtime dependencies so the file opens offline. Escape every backend-derived string before inserting it into HTML.

Choose charts, tables, color, typography, density, and layout based on the actual dataset. Use the NVIDIA-aligned visual system below for every report. Make the result responsive, accessible, and print-friendly. Do not embed raw credentials or a full raw-data dump. When showing only the highest-ranked rows, state the displayed count and the full population count.

### NVIDIA-aligned dark visual system

The concrete palette, tokens, and styling rules below are the source of truth for this report; do not rely on any external design system or prior styling knowledge. Because the output is standalone HTML, encode these conventions as inline CSS variables and semantic class names instead of importing packages or external assets. Generate the report in dark mode by default; do not include a light-mode fallback unless the user explicitly requests it.

Use this dark baseline palette:

```css
:root {
  --nv-green: #76b900;
  --surface-base: #0b1117;
  --surface-panel: #121a23;
  --surface-sunken: #0f1620;
  --surface-raised: #182230;
  --text-primary: #f3f6fb;
  --text-secondary: #a8b3c4;
  --border-base: #2b3645;
  --status-healthy: #7ce4aa;
  --status-warning: #f7c566;
  --status-critical: #ff8a80;
  --status-info: #8bbcff;
  --status-unknown: #c1cad8;
  --status-healthy-bg: #063f2a;
  --status-warning-bg: #4a3004;
  --status-critical-bg: #4a1514;
  --status-info-bg: #102f58;
  --status-unknown-bg: #273241;
}
```

Apply these styling rules:

- Treat `--nv-green` as the NVIDIA brand accent for masthead rules, links, focus states, and small highlights. Do not use brand green to mean healthy; use `--status-healthy`.
- Use a flat dark page canvas (`--surface-base`) with panel containers (`--surface-panel`), sunken chart/table regions (`--surface-sunken`), and subtle borders (`--border-base`). Avoid decorative gradients, radial washes, bokeh, and nested card-on-card layouts.
- Use `NVIDIA Sans, Arial, Helvetica, sans-serif` as the font stack. Do not load remote fonts.
- Set `color-scheme: dark`, keep text contrast high, and use `--text-secondary` only for supporting labels and notes, not primary metrics.
- Use semantic status colors only for meaningful thresholds: healthy/success/running = green, warning/degraded/pending/needs attention = amber, critical/failed/offline = red, informational/in-progress = blue, unknown/inactive/no data = gray.
- Pair status color with visible text labels and concise evidence; never rely on color alone.
- Render status badges as solid pills with background + text color. Use badges for status only, not for categories such as compute zone, node group, GPU model, or component.
- Keep tables quiet and scannable on the dark canvas: no heavy filled table backgrounds, subtle row dividers, concise headers, and horizontal scrolling on narrow screens.
- Match chart backgrounds to their containing panel, use secondary text for chart labels, and use low-contrast grid lines. For standalone SVG/canvas charts, use resolved hex or rgba values, not CSS variables inside SVG attributes.
- Choose chart type by the question: line charts for trends over time, horizontal bars for ranked categories, stacked bars for composed counts, donut charts only for 2-5 part-to-whole segments, and tables when exact values matter most.

Include these sections:

1. **Fleet-wide health / at a glance** — derived node health score and formula, overall status, node and GPU totals when present, healthy/degraded/unhealthy/unknown counts, active-alert totals by severity, and collection timestamp.
2. **Fleet distribution and operational signals** — concise health breakdowns by compute zone and node group, GPU type/capacity, agent connectivity, firmware, and verification. Omit unavailable metrics rather than estimating them.
3. **Trend direction** — current versus previous equal windows, error totals, delta, percentage or zero-baseline wording, direction, and exact time boundaries.
4. **Issue concentration** — alert/error distribution by component or type and the share concentrated in the leading nodes, when fields support it.
5. **Machines needing immediate attention** — most-alerted/highest-risk nodes with hostname, UUID or shortened UUID, health, critical and warning counts, agent/firmware/verification signals, and concise evidence-based reasons.

Add a short evidence-based action summary when useful. Use remediation or suggested actions only when returned by the backend; otherwise describe what deserves investigation without prescribing unsupported fixes.

## Deliver

Open or inspect the generated HTML locally enough to catch malformed markup, missing sections, unescaped content, and obviously inconsistent totals. Cross-check headline values against the validated JSON before finishing. Before returning, verify that the report is the only generated file left in the workspace from the invocation; remove any temporary artifacts created during the run. Return a clickable path to the HTML report and briefly note its collection time and comparison horizon. If generation stops for data integrity, authentication, or backend access, return no fabricated report and state the concrete blocker.

## Examples

**Default 24-hour snapshot**

> User: "Generate a fleet health report."

Collect the required snapshot and trend queries, compare `[T-24h, T)` against `[T-48h, T-24h)`, validate completeness, then produce `fleet-health-report.html` and return its path with the collection time and comparison horizon.

**Custom window**

> User: "Give me an executive health summary over the last 7 days."

Same flow, but set the two adjacent windows to 7-day spans (`[T-7d, T)` versus `[T-14d, T-7d)`) and record the exact RFC3339 boundaries.
