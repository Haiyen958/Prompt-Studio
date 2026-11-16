# DEV-KIT v1.0 — Change Development Framework for AI Coding Agents

> Use **after** a project has been specified with PLAN-KIT and DOCS-KIT and has a repository. DEV-KIT governs *changes to an existing system*, starting with Feature Development. It complements, rather than replaces, DOCS-KIT §7 (`docs/plans/`, `scripts/plan.mjs`).

## 1. Mission and boundary

- PLAN-KIT decides **what product to build**; DOCS-KIT formalizes **how the product behaves and is implemented**; AUDIT-KIT (if installed) independently validates readiness; DEV-KIT governs **incremental changes** on the real codebase.
- Canonical rules remain in `AGENTS.md`, `docs/02-domain.md`, `docs/01-architecture-and-stack.md`, and other DOCS-KIT files. DOCS-KIT's conflict precedence is `02 > 03 > 04 > 10 > 01 > 09 > 07 > 08 > 05 > 06`. Never create a competing source of truth in DEV-KIT.
- The existing `node scripts/plan.mjs` controls plan state. Do not create a second task-status engine or manually modify its generated outputs.
- The feature workflow is production-ready as an **instructional kit**. Other workflows are initial policies, not full dedicated implementations.
- The kit itself does not install IDE commands, hooks, scripts or an automated readiness validator. Any enforcement beyond instructions must be added to the project separately.

## 2. Start here

1. Open the existing repository in Codex or Claude Code.
2. Supply this file and say: `Run DEV-KIT feature for: <request>`. See `prompts/feature-start.md`.
3. Agent performs read-only discovery and impact analysis before proposing changes.
4. Agent asks only genuinely unresolved product decisions, recording them as `D`/`ADR` proposals; never invent outcomes or silently override existing decisions.
5. Agent writes a feature plan using **the actual repository's** `docs/plans/README.md` and `scripts/plan.mjs` CLI (verify those exist before invoking).
6. Obtain explicit owner approval, then conduct feature readiness review. An optional AUDIT-KIT can conduct independent review.
7. Implement only after approval and readiness. Validate DoD, capture CI/runner evidence according to project rules, update documentation, and close the plan via existing lifecycle.

Conversation language: Vietnamese when user is speaking Vietnamese; technical identifiers and source files retain repository conventions. Documentation language: follow `AGENTS.md`/project DOCS-KIT requirements (usually English).

## 3. Routing requests

| Type | Trigger | Entry workflow |
|---|---|---|
| feature | new user-visible capability, new module, integration | `workflows/feature.md` |
| bugfix | expected vs actual behavior differs | `workflows/bugfix.md` |
| refactor | behavior-preserving structure changes | `workflows/refactor.md` |
| migration | schema/data/runtime/platform transition | `workflows/migration.md` |
| optimization | measured performance/cost/resource issue | `workflows/optimization.md` |

For mixed requests: identify dominant intent and attach required checks from other workflow(s), without duplicating plan tracking. Reclassify with owner awareness when change grows beyond scope.

## 4. Mandatory sequence and gates

| Gate | Phase | Required artifact / evidence | Stop condition |
|---|---|---|---|
| G0 | Source discovery | file-backed context, current git state, authoritative docs | no reliable source or scope |
| G1 | Impact and decision discovery | module map, baseline behavior, risk and decision questions | unresolved scope-changing decision |
| G2 | Specification | requirements/negative scope, flows, state, permissions, edge cases, acceptance tests | contradictory or untestable requirement |
| G3 | Plan proposal | contract delta, migration/rollback (if applicable), tasks, dependencies and DoD | unapproved plan |
| G4 | Readiness | reviewed plan, current source versions, no unresolved blockers | NO-GO |
| G5 | Implementation | tests, evidence, staged code changes | failing DoD / unauthorized change |
| G6 | Reconciliation | docs/CHANGELOG, contracts, regression and plan report | undocumented divergence |

G4 is a documented **decision**, not a claim that an automated gate exists. Only say gate is machine-enforced after an actual validator has been installed and verified.

## 5. Read-only discovery protocol

Read `AGENTS.md`; check `CLAUDE.md` if present; inspect docs, plan directory, package structure, source, tests, migrations and git status. Verify whether files/scripts exist; do not assume sample tree is installed. Search code for relevant entrypoints and current API contracts. Cite file paths and symbols/line spans when describing current behavior. Treat docs as intentions and code/tests as implementation evidence; explicitly flag discrepancies. Never read real `.env` secrets, private keys, or credentials. Do not execute network-impacting/mutating commands during discovery.

Follow `rules/` for safety, reuse, compatibility, questions and evidence.

## 6. Feature discovery and decisions

Before proposing implementation, establish: user goal, actor/role, triggering action, input/output, happy path, failure paths, UI states (if applicable), tenant boundary, data retention, audit needs, integrations, success criteria, out-of-scope. **Do not ask questions already answered** in authoritative sources. For each genuinely undecided behavior, show 2–3 options with consequences and a recommendation; pause that decision until owner chooses. A technical design recommendation is not an approved product decision.

## 7. Integration with existing DOCS-KIT plan system

When present, follow these documented lifecycle stages (exact syntax from project `docs/plans/README.md`, not guesses):

```
node scripts/plan.mjs new <slug> "<Plan name>"
# Write/update the generated plan.md as instructed by its template
# sync → owner approval → approve → start → run → done → report → close
node scripts/plan.mjs check
```

The **verbs** after `new` are lifecycle concepts: check actual command help/README before running any command. Generated `progress.md`, `INDEX.md`, `log.jsonl` are owned by existing scripts. Do not edit them manually. Preserve original progress, questions, and notes according to DOCS-KIT rules. If the repo lacks the plan system, stop and propose a compatible setup; do not silently bootstrap a shadow implementation.

## 8. Update canonical documentation by affected area

- Business invariants / permissions / decisions → `docs/02-domain.md` (owner-approved decision records).
- Architecture/technical decisions → `docs/01-architecture-and-stack.md`, relevant `docs/adr/`.
- Data or migrations → `docs/03-database-schema.md` and source schema.
- API/errors/events → `docs/04-api-contract.md` and canonical contract source.
- Integrations → `docs/10-integrations.md`; UI → `docs/09-ui.md`.
- Source tree → `docs/05-project-structure.md`; task-level DoD → active plan and `docs/07-testing.md` if global conventions change.
- Operations/rollback → `docs/08-operations.md`; changes and reopened work → `docs/CHANGELOG.md`.

Follow the repository's documented approval rules before editing protected files. Avoid duplicating authoritative rules into feature notes: link to them. Run `node scripts/check-docs.mjs --strict` and `node scripts/plan.mjs check` **if present** at required checkpoints; report unavailable checks explicitly.

## 9. Output contract

Before coding, present: (A) change overview + scope, (B) codebase findings with evidence, (C) impact matrix, (D) decisions needed/approved, (E) contract/migration changes, (F) task dependencies + measurable DoD, (G) risks, rollback and regression tests, (H) readiness verdict with reasons.

`READY` = approved scope/plan, no blocking product decisions, feasible dependencies, and required contract/rollback/test evidence. `CONDITIONALLY READY` = permitted only for explicitly isolated work according to owner/project policy. `NOT READY` = block affected implementation. Never interpret this kit as approval to start coding by default.

After coding: list changed files, commands with exit codes, tests/evidence, contract/document updates, unresolved risks, and actual plan state.

## 10. Non-goals

No whole-project re-planning for small changes; no automatic approval; no silent API breaking change; no deletion/masking of failing tests; no false completion claims; no commit/push/deploy without project/user authorization; no new abstraction solely to suit a pattern.

## 11. Files in the kit

- `workflows/feature.md`: complete step-by-step change lifecycle.
- `workflows/{bugfix,refactor,migration,optimization}.md`: lightweight targeted entry workflows.
- `templates/`: bounded artifacts to attach to an existing `docs/plans/P00n-*` plan.
- `rules/`: enforceable-by-review engineering guardrails.
- `prompts/`: copy/paste starting and review prompts.

## 12. Upgrade strategy

v1.0: feature workflow + remaining workflow scaffolds + review templates.
Later: add a verified readiness validator, automated diff-based impact analysis, additional CI checks, and IDE command wrappers as *separate integration tasks*. Do not claim these exist merely because described here.
