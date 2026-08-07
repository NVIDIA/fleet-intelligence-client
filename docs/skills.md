# Fleet Intelligence Agent Skills

Fleet Intelligence ships three portable [Agent Skills](https://agentskills.io/)
for harnesses such as Claude Code, Cursor, and Codex:

| Skill | Use it for |
| --- | --- |
| `nvfleetint` | Ad hoc fleet questions and CLI authentication |
| `fleet-health-report` | A fleet-wide, standalone HTML health snapshot |
| `node-rca-rcca` | An evidence-backed HTML RCA/RCCA for one node |

Each skill uses the shared `SKILL.md` format. Harness-specific installation
paths and optional UI metadata do not change the core workflow.

## Install

Install all skills for Claude Code, Cursor, and Codex:

```bash
npx skills add NVIDIA/fleet-intelligence-client \
  --skill '*' \
  --agent claude-code \
  --agent cursor \
  --agent codex
```

Add `--global` to install them for the current user instead of the current
project. Select individual skills with repeated `--skill <name>` flags.

Install and authenticate `nvfleetint` before using any skill. Start a new agent
session if an installed or updated skill is not discovered immediately.

## Report skills

The two report skills are reference implementations with strict evidence and
completeness rules. Adapt their report sections and presentation to the task,
but preserve the live-data, read-only, no-secrets, and evidence-versus-inference
requirements.

Both produce standalone, print-friendly HTML documents using their bundled
`references/html-report-template.md` guidance:

- `fleet-health-report` covers fleet distribution, operational signals, trends,
  issue concentration, and machines needing attention.
- `node-rca-rcca` covers one node's timeline, impact, root cause, contributing
  factors, corrective actions, and validation plan.

Use `nvfleetint` to explore ad hoc questions before or after generating either
report.
