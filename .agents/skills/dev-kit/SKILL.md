---
name: dev-kit
description: >-
  Change development framework router and governance runbook (DEV-KIT v1.0) for incremental changes on an existing codebase. Use this skill when the user invokes DEV-KIT, requests an incremental system change that spans multiple categories, or needs routing across feature, bugfix, refactor, migration, and optimization workflows.
---

# DEV-KIT — Change Development Framework

Governs incremental changes on an existing project without replacing canonical DOCS-KIT documentation or `scripts/plan.mjs` task state.

Full specification: [DEV-KIT.md](../../kits/DEV-KIT/DEV-KIT.md)

## 1. Classify and Route the Request

Select the dominant workflow (attach secondary checks from other workflows when a request is mixed, without duplicating plan tracking):

| Type | Trigger | Dedicated Skill / Workflow |
|---|---|---|
| `feature` | New user-visible capability, module, or integration | Activate `feature` skill ([feature.md](../../kits/DEV-KIT/workflows/feature.md)) |
| `bugfix` | Observed behavior differs from documented/expected behavior | Activate `bugfix` skill ([bugfix.md](../../kits/DEV-KIT/workflows/bugfix.md)) |
| `refactor` | Behavior-preserving structural improvement | Activate `refactor` skill ([refactor.md](../../kits/DEV-KIT/workflows/refactor.md)) |
| `migration` | Schema, data, runtime, or provider transition | Activate `migration` skill ([migration.md](../../kits/DEV-KIT/workflows/migration.md)) |
| `optimization` | Measured latency, throughput, memory, or cost bottleneck | Activate `optimization` skill ([optimization.md](../../kits/DEV-KIT/workflows/optimization.md)) |

## 2. Mandatory Gates (G0 → G6)

1. **G0 — Source Discovery (Read-Only):** Read `AGENTS.md`, canonical docs (`02 > 03 > 04 > 10 > 01 > 09 > 07 > 08 > 05 > 06`), `docs/plans/README.md`, code entrypoints, tests, and `git status`. Cite file paths and symbols/lines. Never read live `.env` secrets.
2. **G1 — Impact & Decision Discovery:** Map impacted modules, baseline behavior, risks, and genuinely unresolved product decisions (present 2–3 options + consequences + recommendation in Vietnamese when speaking Vietnamese; pause until owner decides).
3. **G2 — Specification:** Define scope, negative scope, flows, permissions, edge cases, and testable acceptance criteria.
4. **G3 — Plan Proposal:** Draft contract/migration deltas and project-native tasks via `node scripts/plan.mjs` (verify `scripts/plan.mjs` exists and inspect its CLI help first; stop and report if missing).
5. **G4 — Readiness Verdict:** Validate traceability and blockers. Issue `READY`, `CONDITIONALLY READY`, or `NOT READY`. Never begin implementation before explicit owner approval and applicable readiness.
6. **G5 — Implementation:** Execute approved tasks, write/update tests, run verification commands, and record real exit codes/CI evidence.
7. **G6 — Reconciliation:** Update canonical docs (`docs/01`..`docs/10`, `docs/CHANGELOG.md`) per repository approval rules, run `node scripts/check-docs.mjs --strict` and `node scripts/plan.mjs check` if present, and close the plan cleanly.

## 3. Guardrails & Templates

- Rules: [decisions-and-scope.md](../../kits/DEV-KIT/rules/decisions-and-scope.md), [reuse-and-compatibility.md](../../kits/DEV-KIT/rules/reuse-and-compatibility.md), [safety-and-approval.md](../../kits/DEV-KIT/rules/safety-and-approval.md)
- Templates: [impact-matrix.md](../../kits/DEV-KIT/templates/impact-matrix.md), [feature-spec.md](../../kits/DEV-KIT/templates/feature-spec.md), [contract-delta.md](../../kits/DEV-KIT/templates/contract-delta.md), [readiness-review.md](../../kits/DEV-KIT/templates/readiness-review.md)
