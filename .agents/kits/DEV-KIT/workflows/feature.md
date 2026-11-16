# Feature workflow — full lifecycle

**Entry:** `DEV-KIT feature: <request>`. **Exit:** documented and tested change closed through the current DOCS-KIT plan system.

## F0 — Baseline snapshot (read-only)
- Check current branch, dirty files, package/service graph, `AGENTS.md` and applicable docs. Never discard user edits.
- Locate related code, API routes, schemas, migrations, tests and UI entrypoints. Record *evidence*, not assumptions.
- Read `docs/plans/README.md` and check actual available `scripts/plan.mjs` commands before planning.
**Output:** context + baseline/test status + known documentation/code conflicts.

## F1 — Feature intake
Define user outcome, actor, entry points, success signal, V1 scope, explicit exclusions, security/privacy, target environments. Identify whether this is really a feature or a bugfix. Mark unknowns.
**Output:** concise change request; no coding.

## F2 — Impact analysis
Build a matrix: domain/invariants, authorization/multi-tenancy, API/events, DB/migrations, UI/loading/error, integrations, jobs/concurrency, observability, CI/tests, operations. For each: `none | additive | modifying | breaking`, impacted file/symbol, evidence, owner and mitigation. Identify existing abstractions to reuse.
**Output:** `templates/impact-matrix.md` completed.

## F3 — Decisions and adversarial scenarios
Check source-of-truth decisions first. Ask only missing decisions that materially change behavior, preferably one at a time. Offer A/B/C and side effects. Examine duplicates, retries, interrupted flow, revoked roles, concurrency, bad inputs, inaccessible external systems, reversibility and state transitions (as applicable).
**Output:** approved decision IDs and unresolved blockers; do not invent defaults for business decisions.

## F4 — Specification and contract delta
Specify user stories, workflows, inputs/outputs, permission matrix, invariants, error states, metrics and examples. Describe exact OpenAPI/JSON Schema, DB constraints/migrations, events and client types *as deltas from the canonical source*. Specify rollout, rollback and backwards compatibility, including old clients. Do not edit protected canonical docs before authorization.
**Output:** `templates/feature-spec.md` and `templates/contract-delta.md` tailored to scope.

## F5 — Plan creation using DOCS-KIT
Create via `node scripts/plan.mjs new ...` **only after** inspecting actual plan conventions. Fill generated `plan.md`: goal, scope, decision references, affected docs, tasks and DoD. Express task dependencies in descriptions or native plan format if supported. Each task: ID, prereqs, read-first, allowed files, detailed steps, tests, rollback/recovery and observable DoD. Prefer small reviewable tasks. Follow `sync` workflow from project docs; obtain owner approval before calling `approve`.
**Output:** project-native plan, not duplicate private task tracker.

## F6 — Pre-implementation review
Independently validate each mandatory requirement traces to design → contract → task → test. Inspect compatibility, migration reversibility and edge cases. Classify findings P0/P1 blockers, P2 recommended, P3 optional; distinguish defect/risk/missing decision/assumption. Require evidence. Review diff from baseline. If independent AUDIT-KIT available, hand off for cross-agent review; otherwise do internal review but label it accordingly.
**Output:** `templates/readiness-review.md` and `READY / CONDITIONALLY READY / NOT READY`. **No coding** until owner approval and applicable readiness.

## F7 — Controlled implementation
Follow `AGENTS.md` and plan lifecycle: start task → implement code/tests → run checks → record runner/CI evidence → mark done only when requirements met. Do not skip/relax tests, edit generated status by hand, expand scope without a new decision, or push/deploy without permission. Block via `docs/QUESTIONS.md` where required.

## F8 — Docs, regression, close
Synchronize protected docs via documented approval path; update `docs/CHANGELOG.md`, check impact and reopen previously completed tasks when necessary. Run `check-docs --strict`, `plan.mjs check` and project test/verify commands when present. Produce native plan report and close per project CLI. Document notable exceptions and next steps.

## Feature DoD checklist
- [ ] User-visible behavior and excluded scope match approved decisions.
- [ ] Authorization + tenant separation verified where relevant.
- [ ] Schema/API/events have versioned compatible contracts or approved breaking-change plan.
- [ ] Happy, boundary, failure and regression paths tested.
- [ ] Migration, rollout and rollback documented when applicable.
- [ ] Canonical docs and task plan agree with code and tests.
- [ ] Commands and CI/runner evidence recorded, no failures concealed.
- [ ] Repository plan status reflects real outcome; owner-facing summary provided.
