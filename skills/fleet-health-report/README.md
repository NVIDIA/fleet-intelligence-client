# Fleet Health Report

Generate a standalone HTML snapshot of NVIDIA Fleet Intelligence health from live
`nvfleetctl` backend data. See [`SKILL.md`](./SKILL.md) for the full skill
definition.

## This skill is a reference

Treat this skill as a **reference implementation**, not a fixed template. It
captures one opinionated set of sections, metrics, and visual conventions that
work well for a general fleet health snapshot, but the report contents are meant
to be customized.

The evidence commands under **Collect live data** and the required sections
under **Build the HTML snapshot** in [`SKILL.md`](./SKILL.md) are the defining
knobs. If you want a different report — for example, one focused purely on alert
trends, on a single compute zone, or on capacity and GPU utilization — build your
own skill modeled on this one but with the sections you care about.

## Report format

The output is a standalone, print-friendly **HTML** document
([`references/html-report-template.md`](./references/html-report-template.md)),
since this is the format that works best for both agent and human readability.
Keep the operating rules, completeness checks, and metric-derivation discipline
unchanged — those keep the report honest regardless of layout.

## Relationship to other skills

- `nvfleetctl` — the general-purpose skill for answering ad-hoc fleet questions.
  Use it to explore before or after generating a snapshot.
- `node-rca-rcca` — a structured RCA/RCCA document for one degraded or unhealthy
  node. Use it when the scope is a single node's root cause rather than the whole
  fleet.
