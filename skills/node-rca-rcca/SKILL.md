---
name: node-rca-rcca
description: Investigate one NVIDIA Fleet Intelligence node and generate an evidence-backed HTML RCA/RCCA from live current and historical alerts plus authoritative corrective-action research. Use for node incident analysis, root-cause analysis, corrective actions, or post-incident reports.
---

# Node RCA/RCCA

Investigate one node and produce an evidence-backed offline HTML RCA/RCCA. Read the [CLI contract](references/cli-contract.md), [HTML theme](references/html-theme.md), and [workspace guide](references/workspace.md) before collecting data.

## Workflow

### 1. Resolve the inputs and profile

Require a hostname or node UUID and a profile. Ask a concise clarification when either is missing or ambiguous.

```bash
nvfleetint auth list --output json
nvfleetint auth status --profile <profile> --output json
```

Require `connection` equal to `ok`. Pass the same explicit `--profile <profile>` to every API-backed command.

### 2. Resolve exactly one node

For a hostname, search all identity pages and require one exact match. Ask the user to choose when a partial name returns multiple candidates.

```bash
nvfleetint node list --hostname <hostname> --view basic --all \
  --profile <profile> --output json
```

For a UUID, or after resolving a hostname, verify and capture the node once:

```bash
nvfleetint node describe <node_uuid> --profile <profile> --output json
```

Use the saved description for hostname, health, placement, GPU, agent, integrity, firmware, and component context.

### 3. Collect current and historical alerts

Fetch current alerts and complete historical alerts:

```bash
nvfleetint alert node <node_uuid> --without-psirt --all \
  --profile <profile> --output json
nvfleetint alert node <node_uuid> --view historical --without-psirt --all \
  --profile <profile> --output json
```

Describe every unique current alert:

```bash
nvfleetint alert describe <alert_uuid> --node <node_uuid> --profile <profile> --output json
```

Run at most four describe calls concurrently. Use each description's timeline, messages, errors, incidents, and suggested actions to identify the exact issue; treat missing optional fields as unavailable evidence.

Treat the default node view as current active alerts. Aggregate current and historical alerts by component ID/display name and status, deduplicating the same `alertUuid` across both sets. For each current alert, count prior historical rows with the same component ID after excluding its own `alertUuid`, and record the most recent prior occurrence. Empty alert sets are valid evidence.

### 4. Determine the root cause

Validate every response with the CLI contract before analysis. Correlate node state, current alert descriptions, and historical alerts by component and time. State:

- observed symptoms and impact;
- the most specific supported root cause;
- confidence as `Confirmed`, `Likely`, or `Not confirmed`;
- competing explanations and missing evidence when they affect the conclusion.

Do not promote correlation to causation. If evidence is insufficient, report the root cause as not confirmed and identify the next evidence needed.

### 5. Research corrective actions

After forming the evidence-based RCA, search the web using only observed generic component names, error codes, firmware/driver versions, and root-cause terms. Never include hostname, node UUID, profile, tenant, or customer data in a query.

Prefer official NVIDIA documentation, release notes, support articles, and knowledge-base material. Use other primary vendor documentation only when no relevant NVIDIA source exists. Cite the source title and URL beside each supported recommendation.

Turn the research into containment, corrective, preventive, and validation actions. Keep sourced guidance distinct from fleet evidence and mark any environment-dependent recommendation for operator confirmation.

### 6. Build and deliver

Apply the shared HTML theme and workspace workflow. Summarize saved JSON rather than embedding raw payloads. Use these sections:

1. Executive Summary: node, collection time, impact, root cause, and confidence.
2. Node Details: relevant node metadata.
3. Alert Evidence: first show aggregate current and historical counts grouped by component and status, then show a collapsed `<details>` breakdown for every current alert with its described issue, timeline evidence, status, component, timing, prior occurrence count, and most recent prior occurrence.
4. Root Cause Analysis: reasoning, competing explanations, and evidence gaps.
5. Corrective Action Plan: containment, corrective, preventive, and validation actions.
6. References: cited corrective-action sources.
7. Assumptions and Unknowns: assumptions and information still required.

Use section IDs `summary`, `node-details`, `evidence`, `root-cause`, `corrective-actions`, `references`, and `unknowns`, respectively.

Cross-check every headline claim against validated JSON or a cited source, then return the report path, resolved node, profile, and collection time.
