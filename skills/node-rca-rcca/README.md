# Node RCA/RCCA

Produce a structured Root Cause Analysis (RCA) and Root Cause Corrective Action
(RCCA) document for a single degraded or unhealthy NVIDIA Fleet Intelligence
node, from live read-only `nvfleetctl` evidence. See [`SKILL.md`](./SKILL.md) for
the full skill definition.

## This skill is a reference

Treat this skill as a **reference implementation**, not a fixed template. It
captures one opinionated evidence workflow and report structure that work well
for a general single-node investigation, but the collected commands and report
sections are meant to be adapted to the incident in front of you.

The evidence commands under **Collect live evidence** and the required sections
under **Build the RCA/RCCA document** in [`SKILL.md`](./SKILL.md) are the
defining knobs. If you need a different investigation — for example one scoped to
a single component, or a fleet-wide recurrence analysis rather than one node —
build your own skill modeled on this one with the queries and sections you care
about.

## Report format

The output is a standalone, print-friendly **HTML** document
([`references/html-report-template.md`](./references/html-report-template.md)),
since this is the format that works best for both agent and human readability. Keep
the operating rules, completeness checks, and evidence-vs-inference discipline
unchanged — those keep the RCA honest.

## Relationship to other skills

- `nvfleetctl` — the general-purpose skill for answering ad-hoc fleet questions.
  Use it to identify which node is unhealthy in the first place.
- `fleet-health-report` — a fleet-wide HTML health snapshot across many nodes.
  Use it when the scope is the whole fleet rather than one node's root cause.
