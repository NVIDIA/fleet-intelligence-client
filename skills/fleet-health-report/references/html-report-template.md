# HTML Fleet Health Report Template

Produce a standalone `.html` file for the final fleet health snapshot.

## File Requirements

- Use one self-contained HTML file with `<!doctype html>`, `<html lang="en">`,
  `<meta charset="utf-8">`, and a responsive `<meta name="viewport">`.
- Use inline CSS and only optional inline JavaScript. Do not reference external
  fonts, scripts, CDNs, remote images, or stylesheets, so the file opens offline.
- HTML-escape all dynamic values from `nvfleetint` output.
- Do not embed raw credentials or a full raw-data dump. When showing only the
  highest-ranked rows, state the displayed count and the full population count.
- Use the NVIDIA-aligned dark visual system defined below: a flat dark canvas,
  panel containers, restrained semantic status colors, and quiet scannable
  tables. Keep it responsive, accessible, and print-friendly.

## Visual system

The palette, tokens, and styling rules below are the source of truth for this
report; do not rely on any external design system or prior styling knowledge.
Because the output is standalone HTML, encode these conventions as inline CSS
variables and semantic class names instead of importing packages or external
assets. Render in dark mode by default; do not add a light-mode fallback unless
the user explicitly asks for one. The `node-rca-rcca` report shares this system —
keep them consistent so the two reports read as one product.

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

- Treat `--nv-green` as the NVIDIA brand accent for masthead rules, links, focus
  states, and small highlights. Do not use brand green to mean healthy; healthy
  uses `--status-healthy`.
- Use a flat dark page canvas (`--surface-base`) with panel containers
  (`--surface-panel`), sunken chart/table regions (`--surface-sunken`), and
  subtle borders (`--border-base`). Avoid decorative gradients, radial washes,
  bokeh, and nested card-on-card layouts.
- Use `NVIDIA Sans, Arial, Helvetica, sans-serif`; do not load remote fonts. Set
  `color-scheme: dark`, keep text contrast high, and reserve `--text-secondary`
  for supporting labels and notes, not primary metrics.
- Use semantic status colors only for meaningful thresholds: healthy/success/
  running = green, warning/degraded/pending/needs attention = amber,
  critical/failed/offline = red, informational/in-progress = blue,
  unknown/inactive/no data = gray.
- Pair status color with visible text labels and concise evidence; never rely on
  color alone.
- Render status badges as solid pills with background + text color. Use badges
  for status only, not for categories such as compute zone, node group, GPU
  model, or component.
- Keep tables quiet and scannable on the dark canvas: no heavy filled table
  backgrounds, subtle row dividers, uppercase header labels, and horizontal
  scrolling on narrow screens.
- Match chart backgrounds to their containing panel, use secondary text for chart
  labels, and use low-contrast grid lines. For standalone SVG/canvas charts, use
  resolved hex or rgba values, not CSS variables inside SVG attributes.
- Choose chart type by the question: line charts for trends over time, horizontal
  bars for ranked categories, stacked bars for composed counts, donut charts only
  for 2-5 part-to-whole segments, and tables when exact values matter most.

## Page Structure

Use these sections in this order:

1. Header with report title, overall status badge, node health score, and
   generated timestamp.
2. Fleet-wide health / at a glance — report-derived node health score and
   formula, the backend `healthPercentage` from `overview` when present, overall
   status, node/GPU/CPU-core totals when present, healthy/degraded/unhealthy/
   unknown counts, active-alert totals by severity, and collection timestamp.
3. Fleet distribution and operational signals — concise health breakdowns by
   compute zone and node group, GPU type/capacity, agent connectivity, firmware,
   and verification, plus the `overview` fleet metrics (name, value with unit,
   aggregation, last-updated) when returned. Omit unavailable metrics rather than
   estimating them.
4. Trend direction — current versus previous equal windows, error totals, delta,
   percentage or zero-baseline wording, direction, and exact time boundaries.
5. Issue concentration — alert/error distribution by component or type and the
   share concentrated in the leading nodes, when fields support it.
6. Machines needing immediate attention — most-alerted/highest-risk nodes with
   hostname, UUID or shortened UUID, health, critical and warning counts,
   agent/firmware/verification signals, and concise evidence-based reasons.

## Status Styling

Map each status label to a semantic status color; use the badge classes in the
skeleton (`.ok`, `.warn`, `.bad`, `.info`, `.unknown`). Choose the class from the
label's actual value so a `Healthy` node never renders red:

- `Healthy`, `Online`, `Resolved`, `Verified`, `Passed`: green
  (`--status-healthy`).
- `Needs attention`, `Warning`, `Degraded`, `Detected`, `Pending`,
  `Unsupported`, `Unverified`:
  amber (`--status-warning`).
- `Critical`, `Unhealthy`, `Offline`, `Failed`, `Triggered`: red
  (`--status-critical`).
- Informational labels: blue (`--status-info`).
- `Unknown`, inactive, `Other`, or no-data labels: gray (`--status-unknown`).

## HTML Skeleton

Copy and adapt this skeleton. Replace bracketed placeholders with report content,
and add charts and tables sized to the actual dataset within the visual system.
Replace `[backend_health_badge]` with the backend-health badge only for an
entire-fleet report with a returned value; replace it with an empty string for a
scoped report or missing value.

```html
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Fleet Health Report — [status] on [date]</title>
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
    .stat-grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
      gap: 12px;
    }
    .stat {
      background: var(--surface-sunken);
      border: 1px solid var(--border-base);
      border-radius: 6px;
      padding: 12px 14px;
    }
    .stat .value { font-size: 26px; font-weight: 700; }
    .stat .label { color: var(--text-secondary); font-size: 12px; text-transform: uppercase; letter-spacing: 0.02em; }
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
      table, .stat { background: #fff; }
      .page-main { max-width: none; padding: 18px; }
    }
  </style>
</head>
<body>
  <div class="page-header">
    <h1>Fleet Health Report</h1>
    <div class="subtitle">Generated from read-only NVIDIA Fleet Intelligence data.</div>
    <div class="meta">
      <span class="badge [status_class]">Status: [status]</span>
      <span class="badge info">Node health score: [health_score]%</span>
      [backend_health_badge]
      <span class="badge info">Generated: [timestamp]</span>
    </div>
  </div>
  <div class="page-main">
    <div class="report-section" id="at-a-glance">
      <h2>Fleet-wide Health — At a Glance</h2>
      <div class="stat-grid">[at_a_glance_stats]</div>
      <p>[health_score_formula]</p>
    </div>

    <div class="report-section" id="distribution">
      <h2>Fleet Distribution and Operational Signals</h2>
      [distribution_breakdowns]
    </div>

    <div class="report-section" id="trend">
      <h2>Trend Direction</h2>
      <p>[trend_summary]</p>
      [trend_detail]
    </div>

    <div class="report-section" id="concentration">
      <h2>Issue Concentration</h2>
      [concentration_detail]
    </div>

    <div class="report-section" id="attention">
      <h2>Machines Needing Immediate Attention</h2>
      <table>
        <thead><tr><th>Hostname</th><th>UUID</th><th>Health</th><th>Critical</th><th>Warning</th><th>Signals</th><th>Reason</th></tr></thead>
        <tbody>[attention_rows]</tbody>
      </table>
    </div>
  </div>
</body>
</html>
```
