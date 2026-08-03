# HTML RCA/RCCA Report Template

Produce a standalone `.html` file for the final RCA/RCCA report.

## File Requirements

- Use one self-contained HTML file with `<!doctype html>`, `<html lang="en">`,
  `<meta charset="utf-8">`, and a responsive `<meta name="viewport">`. These, and
  the CSS, are given verbatim in the skeleton below — copy them as-is.
- Use inline CSS only. Do not reference external fonts, scripts, images, or
  stylesheets, so the final file is fully self-contained and opens offline.
- HTML-escape all dynamic values from `nvfleetint` output.
- Do not embed full raw JSON unless the user explicitly asks for it.
- Include command provenance and evidence summaries, not secrets or config file
  contents.
- Use the NVIDIA-aligned dark visual system defined below: a flat dark canvas,
  panel containers, restrained semantic status colors, and tables that wrap long
  commands/messages. Keep it print-friendly.

## Visual system

Match the `fleet-health-report` skill so the two reports read as one product.
Render in dark mode by default; do not add a light-mode fallback unless the user
explicitly asks for one. The palette and tokens below are the source of truth —
they are encoded as inline CSS variables and semantic class names in the skeleton
below; do not rely on any external design system.

- Treat `--nv-green` as the NVIDIA brand accent for the masthead rule, links,
  and small highlights only. Do not use brand green to mean healthy; healthy uses
  `--status-healthy`.
- Use the flat dark page canvas (`--surface-base`) with panel containers
  (`--surface-panel`) and sunken table regions (`--surface-sunken`). Avoid
  gradients, radial washes, and nested card-on-card layouts.
- Use `NVIDIA Sans, Arial, Helvetica, sans-serif`; do not load remote fonts. Set
  `color-scheme: dark` and keep text contrast high, reserving `--text-secondary`
  for supporting labels rather than primary values.
- Render status badges as solid pills (background + text color) and pair every
  status color with a visible text label; never rely on color alone. Use badges
  for status only, not for categories such as node group, compute zone, or GPU
  model.
- Keep tables quiet on the dark canvas: subtle row dividers, uppercase header
  labels, no heavy filled backgrounds, and horizontal scrolling on narrow
  screens.

## Page Structure

Use these sections in this order:

1. Header with report title, node identifier, generated timestamp, health status
   badge, and root cause confidence badge.
2. Executive Summary.
3. Node Details.
4. Impact.
5. Incident Timeline.
6. Root Cause.
7. Contributing Factors.
8. Corrective and Preventive Actions.
9. Validation Plan.
10. Reference (optional — include only when code or reason-string enrichment was
    performed; omit the entire section otherwise).
11. Evidence Appendix.
12. Assumptions and Unknowns.

## Status Styling

Map each status label to a semantic status color; use the badge classes in the
skeleton (`.ok`, `.warn`, `.bad`, `.info`, `.unknown`). Every `[*_class]`
placeholder (e.g. `[health_class]`, `[agent_status_class]`, `[confidence_class]`)
must be replaced with the class chosen from this mapping for that label's actual
value — never leave a hardcoded class, so a `Healthy` node never renders red and a
`Confirmed` root cause never renders amber:

- `Confirmed`, `Healthy`, `Online`, `Resolved`, `Verified`, `Passed`: green
  (`--status-healthy`).
- `Likely`, `Warning`, `Degraded`, `Detected`, `Pending`, `Unsupported`: amber
  (`--status-warning`).
- `Not confirmed`, `Critical`, `Unhealthy`, `Offline`, `Failed`, `Triggered`:
  red (`--status-critical`).
- Informational labels: blue (`--status-info`).
- `Unknown`, inactive, or no-data labels: gray (`--status-unknown`).

## Evidence Appendix

Populate `[evidence_rows]` with one row per command actually run, including
`node describe`, `node health`, `event list`, `event buckets`, `alert timeline`
(full and `--active`), and each `alert describe`. Each row lists the exact
command, its purpose, and a one-line result summary — never secrets or raw JSON.

## Reference (optional)

Include the `#reference` section only when the optional code/reason enrichment
step produced at least one explanation; omit the whole `<div>` otherwise. Each
row pairs a raw code or reason string (for example `XID 79`, or an
`integrityCheckReason` value) with its plain-language meaning and an explicit
source (title and URL). This section is attribution for external explanations
only — it must never restate telemetry as if it were external, and its content
must not have influenced the timeline, impact, root cause, or confidence badge.
Bake the looked-up text in as static prose so the file still opens offline.

## HTML Skeleton

Write the whole document from this skeleton in one shot. Everything from
`<!doctype html>` through the end of the `</style>` block is **invariant** — copy
it byte-for-byte every run, and do not restyle, reorder, or "improve" the CSS.
The palette/tokens described above are exactly what this CSS encodes.

The `<title>` is a fixed, generic string; the per-node title
(`RCA/RCCA: [node] [health] on [date]`) lives in the `<h1>` masthead.

Only the body content varies: replace bracketed placeholders with report content
and choose each `[*_class]` from the Status Styling map above.

```html
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>NVIDIA Fleet Intelligence — Node RCA/RCCA</title>
  <style>
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
    * { box-sizing: border-box; }
    html { color-scheme: dark; }
    body {
      margin: 0;
      background: var(--surface-base);
      color: var(--text-primary);
      font-family: "NVIDIA Sans", Arial, Helvetica, sans-serif;
      line-height: 1.45;
    }
    .page-header {
      background: var(--surface-panel);
      border-bottom: 2px solid var(--nv-green);
      padding: 28px 36px 22px;
    }
    .page-main {
      max-width: 1180px;
      margin: 0 auto;
      padding: 24px 36px 42px;
    }
    h1, h2, h3 { margin: 0; }
    h1 { font-size: 30px; margin-bottom: 8px; }
    h2 { font-size: 19px; margin-bottom: 12px; }
    h3 { font-size: 15px; margin: 16px 0 8px; color: var(--text-secondary); }
    a { color: var(--nv-green); }
    .report-section {
      background: var(--surface-panel);
      border: 1px solid var(--border-base);
      border-radius: 8px;
      margin-bottom: 18px;
      padding: 16px;
      overflow: hidden;
    }
    table {
      width: 100%;
      border-collapse: collapse;
      font-size: 13px;
      background: var(--surface-sunken);
    }
    th, td {
      border-bottom: 1px solid var(--border-base);
      padding: 9px 8px;
      text-align: left;
      vertical-align: top;
      overflow-wrap: anywhere;
    }
    th {
      color: var(--text-secondary);
      font-size: 12px;
      text-transform: uppercase;
      letter-spacing: 0.02em;
    }
    code {
      background: var(--surface-raised);
      border: 1px solid var(--border-base);
      border-radius: 4px;
      padding: 1px 4px;
      font-size: 12px;
      overflow-wrap: anywhere;
    }
    ul { margin: 0; padding-left: 20px; }
    li { margin-bottom: 4px; }
    .subtitle { color: var(--text-secondary); max-width: 980px; }
    .meta {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      margin-top: 14px;
    }
    .badge {
      display: inline-block;
      border-radius: 999px;
      padding: 3px 8px;
      font-size: 12px;
      font-weight: 700;
      color: var(--status-unknown);
      background: var(--status-unknown-bg);
      white-space: nowrap;
    }
    .ok { background: var(--status-healthy-bg); color: var(--status-healthy); }
    .warn { background: var(--status-warning-bg); color: var(--status-warning); }
    .bad { background: var(--status-critical-bg); color: var(--status-critical); }
    .info { background: var(--status-info-bg); color: var(--status-info); }
    .unknown { background: var(--status-unknown-bg); color: var(--status-unknown); }
    @media (max-width: 720px) {
      .page-header { padding: 22px 18px; }
      .page-main { padding: 18px 12px 30px; }
      .report-section { padding: 14px; }
      h1 { font-size: 24px; }
      table { font-size: 12px; }
      th, td { padding: 8px 6px; }
    }
    @media print {
      html { color-scheme: light; }
      body { background: #fff; color: #1d2430; }
      .page-header, .report-section { border-color: #bbb; background: #fff; }
      table { background: #fff; }
      .page-main { max-width: none; padding: 18px; }
    }
  </style>
</head>
<body>
  <div class="page-header">
    <h1>RCA/RCCA: [node] [health] on [date]</h1>
    <div class="subtitle">Generated from read-only NVIDIA Fleet Intelligence evidence.</div>
    <div class="meta">
      <span class="badge [health_class]">Health: [health]</span>
      <span class="badge [confidence_class]">Root Cause: [confidence]</span>
      <span class="badge info">Generated: [timestamp]</span>
    </div>
  </div>
  <div class="page-main">
    <div class="report-section" id="summary">
      <h2>Executive Summary</h2>
      <p>[summary]</p>
    </div>

    <div class="report-section" id="node-details">
      <h2>Node Details</h2>
      <table>
        <tbody>
          <tr><th>Node UUID</th><td><code>[node_uuid]</code></td></tr>
          <tr><th>Hostname</th><td>[hostname]</td></tr>
          <tr><th>Health</th><td><span class="badge [health_class]">[health]</span></td></tr>
          <tr><th>Agent status</th><td><span class="badge [agent_status_class]">[agent_status]</span></td></tr>
          <tr><th>Node group</th><td>[node_group]</td></tr>
          <tr><th>Compute zone</th><td>[compute_zone]</td></tr>
          <tr><th>GPU type/count</th><td>[gpu_type] / [gpu_count]</td></tr>
          <tr><th>Verification (integrity) check</th><td>[integrity_check]</td></tr>
          <tr><th>Firmware check</th><td>[firmware_check]</td></tr>
        </tbody>
      </table>
    </div>

    <div class="report-section" id="impact">
      <h2>Impact</h2>
      <ul>[impact_items]</ul>
    </div>

    <div class="report-section" id="timeline">
      <h2>Incident Timeline</h2>
      <table>
        <thead><tr><th>Time</th><th>Source</th><th>Event</th><th>Evidence</th></tr></thead>
        <tbody>[timeline_rows]</tbody>
      </table>
    </div>

    <div class="report-section" id="root-cause">
      <h2>Root Cause</h2>
      <p><strong>Confidence:</strong> <span class="badge [confidence_class]">[confidence]</span></p>
      <p><strong>Cause:</strong> [cause]</p>
      <h3>Evidence</h3>
      <ul>[root_cause_evidence_items]</ul>
      <h3>Reasoning</h3>
      <p>[reasoning]</p>
    </div>

    <div class="report-section" id="contributing-factors">
      <h2>Contributing Factors</h2>
      <ul>[contributing_factor_items]</ul>
    </div>

    <div class="report-section" id="actions">
      <h2>Corrective and Preventive Actions</h2>
      <table>
        <thead><tr><th>Type</th><th>Action</th><th>Owner</th><th>Due</th><th>Status</th><th>Validation</th></tr></thead>
        <tbody>[action_rows]</tbody>
      </table>
    </div>

    <div class="report-section" id="validation">
      <h2>Validation Plan</h2>
      <ul>[validation_items]</ul>
    </div>

    <!-- Optional: include only when code/reason enrichment was performed; omit otherwise. -->
    <div class="report-section" id="reference">
      <h2>Reference</h2>
      <p class="subtitle">Plain-language explanations of codes and check-reasons from external sources, provided for attribution only. These are not telemetry-derived facts and did not affect the timeline, impact, root cause, or confidence.</p>
      <table>
        <thead><tr><th>Code / Reason</th><th>Meaning</th><th>Source</th></tr></thead>
        <tbody>[reference_rows]</tbody>
      </table>
    </div>

    <div class="report-section" id="evidence">
      <h2>Evidence Appendix</h2>
      <table>
        <thead><tr><th>Command</th><th>Purpose</th><th>Result Summary</th></tr></thead>
        <tbody>[evidence_rows]</tbody>
      </table>
    </div>

    <div class="report-section" id="unknowns">
      <h2>Assumptions and Unknowns</h2>
      <ul>[unknown_items]</ul>
    </div>
  </div>
</body>
</html>
```
