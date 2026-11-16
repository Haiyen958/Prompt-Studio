# AGENTS.md — Project AI Agent Operating Rules

> Applies to Antigravity, Claude Code, Codex, and other coding agents operating in this workspace. This file governs **how to work**; canonical product behavior, contracts, and task specifications remain in the project's authoritative documents.

## 1. Read-Only Discovery (Mandatory First Step)

Before proposing or modifying code:

1. Inspect workspace structure and `git status` (when git is initialized). Never discard or overwrite unrelated local edits.
2. Read `README.md` and authoritative project specifications (e.g., `docs/` when DOCS-KIT is initialized, or current root product/MVP specs). Verify paths exist rather than assuming a template layout.
3. Classify the request: **initial implementation**, **feature**, **bugfix**, **refactor**, **migration**, **optimization**, or **documentation-only**.
4. Inspect existing code, schemas, API contracts, tests, locked decisions (`D*` / `ADR-*`), and plan state. Treat docs as agreed intent and code/tests as observed implementation; flag discrepancies explicitly.
5. Never read or expose live `.env` secrets, API keys, tokens, or private keys.

### Canonical DOCS-KIT Precedence (When Present)

`docs/02-domain.md > 03-database-schema.md > 04-api-contract.md > 10-integrations.md > 01-architecture-and-stack.md > 09-ui.md > 07-testing.md > 08-operations.md > 05-project-structure.md > 06-implementation-plan.md`

Never silently invent or override a business rule when sources conflict—report the conflict and request an owner decision.

## 2. DEV-KIT Workflow Routing

Incremental changes on an existing codebase are governed by `.agents/kits/DEV-KIT/DEV-KIT.md` and workspace skills in `.agents/skills/`:

| Change Type | Skill | Reference Workflow |
|---|---|---|
| General / mixed change routing | `dev-kit` | `.agents/kits/DEV-KIT/DEV-KIT.md` |
| New capability, module, or integration | `feature` | `.agents/kits/DEV-KIT/workflows/feature.md` |
| Defect where actual != expected behavior | `bugfix` | `.agents/kits/DEV-KIT/workflows/bugfix.md` |
| Behavior-preserving structural cleanup | `refactor` | `.agents/kits/DEV-KIT/workflows/refactor.md` |
| Schema, data, runtime, or platform transition | `migration` | `.agents/kits/DEV-KIT/workflows/migration.md` |
| Measured performance, cost, or resource issue | `optimization` | `.agents/kits/DEV-KIT/workflows/optimization.md` |

- **Mandatory sequence (Gates G0–G6):** Read-only discovery (G0) → Impact & decision discovery (G1) → Specification (G2) → Plan proposal (G3) → Readiness review (`READY` / `CONDITIONALLY READY` / `NOT READY`) (G4) → Controlled implementation (G5) → Docs & plan reconciliation (G6).
- **Plan integration:** Reuse `docs/plans/` and `node scripts/plan.mjs` when present (inspect actual CLI help/`docs/plans/README.md` first; never hand-edit generated `progress.md`, `INDEX.md`, or `log.jsonl`). If the repository does not yet have `docs/plans/` or `scripts/plan.mjs`, state the missing setup explicitly and propose a compatible next step rather than silently inventing a shadow tracker.

## 3. Engineering Guardrails

Detailed rules are loaded from `.agents/rules/`:

- **Decisions & Scope (`.agents/rules/decisions-and-scope.md`):** Check recorded `D` / `ADR` decisions before asking. Ask only genuinely unresolved product/business questions, presenting 2–3 options with trade-offs and a recommendation, then wait for owner approval. Never treat a technical recommendation as an approved product decision.
- **Reuse & Compatibility (`.agents/rules/reuse-and-compatibility.md`):** Reuse existing modules, schemas, and patterns within bounded contexts. Default to additive changes; breaking API/schema changes require explicit owner approval, rollout, and migration plans.
- **Safety & Evidence (`.agents/rules/safety-and-approval.md`):** Planning/readiness never authorizes coding, protected doc edits, commits, pushes, or deployments on its own. Never skip/weaken tests, use `--no-verify`, or fabricate command outputs. Report actual commands and exit codes; state `not available` when a check/script does not exist.

## 4. Language & Communication

- Communicate with the user in **Vietnamese** when the user writes in Vietnamese.
- Follow repository conventions for code identifiers, commit messages, and canonical technical documentation (defaulting to English unless the project specifies otherwise).
