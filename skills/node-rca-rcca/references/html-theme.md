# Shared HTML Report Theme

Use this visual system for Fleet Intelligence HTML reports. Report sections and content belong in the calling skill, not this reference.

## File contract

- Produce one standalone semantic HTML file with UTF-8 and a responsive viewport.
- Use inline CSS and optional inline theme-control JavaScript only; load no remote fonts, scripts, stylesheets, images, or other assets.
- HTML-escape dynamic values. Pair every status color with visible text.
- Default to light mode and support a user-selectable dark mode by swapping CSS variables on `html[data-theme="dark"]`.

## Palette

```css
:root {
  color-scheme: light;
  --nv-green: #76b900;
  --surface-canvas: #f7f7f7;
  --surface-panel: #ffffff;
  --surface-sunken: #f7f7f7;
  --text-primary: #000000;
  --text-secondary: #636363;
  --border-base: rgba(0, 0, 0, 0.20);
  --status-healthy: #265600;
  --status-warning: #8d2600;
  --status-critical: #961515;
  --status-info: #0046a4;
  --status-unknown: #4b4b4b;
  --status-healthy-bg: #dafb7d;
  --status-warning-bg: #fcde7b;
  --status-critical-bg: #ffd7d7;
  --status-info-bg: #cbf5ff;
  --status-unknown-bg: #eeeeee;
  --shadow-panel: 0 4px 6px rgba(0, 0, 0, 0.12);
}

html[data-theme="dark"] {
  color-scheme: dark;
  --surface-canvas: #0c0c0c;
  --surface-panel: #000000;
  --surface-sunken: #161616;
  --text-primary: #ffffff;
  --text-secondary: #cccccc;
  --border-base: rgba(255, 255, 255, 0.20);
  --status-healthy: #76b900;
  --status-warning: #ef9100;
  --status-critical: #ff8181;
  --status-info: #10b1fb;
  --status-unknown: #eeeeee;
  --status-healthy-bg: #142700;
  --status-warning-bg: #441000;
  --status-critical-bg: #4b0404;
  --status-info-bg: #002050;
  --status-unknown-bg: #4b4b4b;
  --shadow-panel: none;
}
```

Use NVIDIA green only for brand accents, links, and focus states—not as the generic healthy color. Use local `NVIDIA Sans, Arial, Helvetica, sans-serif` with a 14px base size and 1.5 line height.

## Layout and components

- Canvas: `--surface-canvas`; centered content, maximum width 1180px, 24px desktop gutters and 12px mobile gutters.
- Product bar: 48px high, `--surface-panel`, bottom border, product name left, compact accessible Light/Dark control right. Hide it when printing.
- Page header: floating panel with 16px corners, 24px padding, subtle border and `--shadow-panel`.
- Report panels: white/light or black/dark `--surface-panel`, 16px corners, 24px padding, 16px vertical gap. Use one panel per report section.
- Metric cards: responsive grid, `--surface-sunken`, 8px corners, subtle border; uppercase 12px label and 24px bold value.
- Tables: full width, quiet background, 14px text, 8px block/12px inline cell padding, semibold header with a 2px divider, 1px row dividers, wrapping long values. Allow horizontal scrolling on narrow screens.
- Code: local monospace, `--surface-sunken`, 4px corners, subtle border, wrapping.
- Drill-downs: use `<details>` with an 8px bordered sunken container and a bold `<summary>`; keep expanded content compact.
- Charts: use panel backgrounds, secondary labels, low-contrast grid lines, and resolved colors for SVG/canvas attributes. Prefer horizontal bars for ranked categories and tables when exact values matter.

## Status labels

Use compact solid badges with 4px corners, 12px bold text, and a 1px semantic border. Use them only for statuses, not zone/group/model/component categories.

| Class | Meaning | Colors |
| --- | --- | --- |
| `ok` | Confirmed, Healthy, Online, Resolved, Verified, Passed | `--status-healthy*` |
| `warn` | Likely, Warning, Degraded, Detected, Pending, Unsupported, Unverified | `--status-warning*` |
| `bad` | Not confirmed, Critical, Unhealthy, Offline, Failed, Triggered | `--status-critical*` |
| `info` | Informational or in progress | `--status-info*` |
| `unknown` | Unknown, inactive, Other, or no data | `--status-unknown*` |

## Responsive and print behavior

Below 720px, reduce panel padding to 16px, panel radius to 12px, heading size, and table cell padding. Preserve table scrolling and readable status labels.

For print, force a white canvas with dark text, remove shadows, hide the product bar/theme control, avoid breaking a short panel across pages, and keep semantic labels legible without relying on background color.
