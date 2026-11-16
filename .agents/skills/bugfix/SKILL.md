---
name: bugfix
description: >-
  DEV-KIT lightweight Bugfix workflow for diagnosing, reproducing, and fixing defects where observed behavior diverges from expected or documented behavior. Use this skill when the user reports a bug, regression, error, or broken flow in existing code.
---

# DEV-KIT Bugfix Workflow

Canonical references: [workflows/bugfix.md](../../kits/DEV-KIT/workflows/bugfix.md) · [DEV-KIT.md](../../kits/DEV-KIT/DEV-KIT.md)

## Procedure

1. **Reproduce with Evidence (Read-Only First):**
   - Inspect `AGENTS.md`, `git status`, relevant domain docs, code paths, and existing tests.
   - Reproduce the actual fault using concrete inputs, environment details, and observable output/logs. If non-reproducible, report uncertainty and hypotheses with evidence rather than inventing a root cause.
2. **Verify Expected Behavior:**
   - Compare observed behavior against canonical domain rules (`docs/02-domain.md`, API contracts, tests).
   - Guardrail: Do **not** mislabel a net-new capability request as a bugfix to bypass `feature` review. If expected behavior is genuinely ambiguous, present 2–3 options to the owner and wait for a decision before changing behavior.
3. **Scope Minimal Root-Cause Fix:**
   - Pinpoint the smallest root-cause change, blast radius, security/permission implications, and rollback path.
   - For non-trivial fixes, track through the repository's native `docs/plans/` (`scripts/plan.mjs`) lifecycle; for small fixes, follow repository task-tracking and approval rules.
4. **Regression Test First:**
   - Add or update an automated regression test that fails on the defect (where feasible) before or alongside applying the minimal fix. Never delete or relax existing tests.
5. **Verify & Reconcile:**
   - Run targeted and risk-appropriate verification suites; report exact commands and exit codes.
   - Update canonical documentation and `docs/CHANGELOG.md` if any documented contract or edge-case specification needed clarification.
