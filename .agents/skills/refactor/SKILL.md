---
name: refactor
description: >-
  DEV-KIT behavior-preserving Refactor workflow for restructuring code, improving maintainability, or cleaning up module boundaries without altering external behavior. Use this skill when the user asks to refactor, clean up, modularize, or simplify existing code.
---

# DEV-KIT Refactor Workflow

Canonical references: [workflows/refactor.md](../../kits/DEV-KIT/workflows/refactor.md) · [reuse-and-compatibility.md](../../kits/DEV-KIT/rules/reuse-and-compatibility.md)

## Procedure

1. **Declare Objective & Boundary (Read-Only First):**
   - Inspect `AGENTS.md`, `git status`, module boundaries (`docs/05-project-structure.md` if present), callers, and public contracts.
   - State the exact maintainability problem, target scope, and strict behavior-preservation boundary.
2. **Establish Baseline & Characterization Tests:**
   - Verify existing unit/integration tests pass and capture baseline behavior (and performance baseline if hot paths are touched).
   - Add characterization tests where coverage is insufficient to prove behavior parity.
3. **Plan & Obtain Approval (When Multi-Task or Structural):**
   - Integrate multi-step refactors with `docs/plans/` (`scripts/plan.mjs`) when present, and obtain owner approval before broad structural edits.
4. **Execute Small Reversible Steps:**
   - Apply incremental, reviewable transformations. Avoid bundling feature additions, schema changes, or unrequested dependency churn.
   - Re-verify module import boundaries, public types/APIs, and test parity after each step.
5. **Escalate When Behavior or Persistence Changes:**
   - Stop and escalate to the `feature` or `migration` skill if completing the change requires altering functional outcomes, API contracts, or persisted data representations.
6. **Report Evidence:**
   - List changed files, characterization/regression commands with actual exit codes, rollback steps, and plan status.
