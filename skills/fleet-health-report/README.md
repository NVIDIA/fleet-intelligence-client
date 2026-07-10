# Fleet Health Report

Generate a standalone HTML snapshot of NVIDIA Fleet Intelligence health from live
`nvfleetctl` backend data. See [`SKILL.md`](./SKILL.md) for the full skill
definition.

## This skill is a reference

Treat this skill as a **reference implementation**, not a fixed template. It
captures one opinionated set of sections, metrics, and visual conventions that
work well for a general fleet health snapshot, but the report contents are meant
to be customized.

The sections listed under **Build the HTML snapshot → Include these sections** in
[`SKILL.md`](./SKILL.md) are the defining knobs. If you want a different report —
for example, one focused purely on alert trends, on a single compute zone, or on
capacity and GPU utilization — build your own skill modeled on this one but with
the sections you care about.

## Customizing the sections

To produce a variant report:

1. Copy this skill directory to a new skill name (e.g.
   `fleet-alert-report`).
2. Update the frontmatter `name` and `description` so the new skill triggers on
   the report you actually want.
3. Edit the **Include these sections** list to describe your sections. Keep the
   non-negotiable data rules, completeness checks, and metric-derivation
   guidance — those keep the report honest regardless of layout.
4. Adjust the collected `nvfleetctl` queries if your sections need data the
   default snapshot doesn't gather.

Everything else — the live-data rules, completeness proofs, and the
NVIDIA-aligned dark visual system — is designed to carry over unchanged, so a
customized report stays trustworthy and visually consistent.
