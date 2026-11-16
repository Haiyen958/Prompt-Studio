# P011 — Auto-fill Variables, Inline Adaptive Clarification & Target Hint

**Status:** DONE (2026-11-16). Owner authorized execution (`/goal thực thi`).
**Workflow:** DEV-KIT `feature` (Meta-Prompt v2.1 + Additive Clarification Schema + Inline Studio UX + Eval Suite).
**Spec & Architecture:** [docs/PLAN.md](../../PLAN.md), [docs/ARCHITECTURE.md](../../ARCHITECTURE.md), [CSS_DESIGN_SYSTEM.md](../../../CSS_DESIGN_SYSTEM.md).

## Decisions Locked

- **`D-P011-1` (4-Tier Variable Resolution & Auto-fill):** Meta-Prompt (`generate.v2.1` / `refine.v2.1`) and target guides (`targets/{agent,chat,api}.md`) resolve information in 4 tiers:
  1. *Explicitly provided* facts (module, bug symptom, metrics, constraints) are filled directly into the prompt — never converted into redundant `{{pham_vi_module}}` / `{{mo_ta_loi}}` placeholders unless the user asks for a reusable template.
  2. *Safely inferred* framing (role, structure, tone) is filled directly and listed in `assumptions` with 2–3 `alternatives`.
  3. *Discoverable by target* (`target = "agent"`): codebase details (language, framework, directory layout, test commands) are delegated to the coding agent to inspect in the repository rather than emitted as `{{ngon_ngu}}` / `{{framework}}` variables or asked upfront.
  4. *Critical / high-impact missing decisions* (destructive DB mutations, production fix autonomy, breaking scope) default to the safest option in `v1` and surface at the front of `assumptions` with `impact: "high"` + concise `question`.
  Runtime `{{ten_bien}}` placeholders are reserved strictly for `target = "api"` user payloads, explicit reusable-template requests, or external data the user must paste at run time.
- **`D-P011-2` (Non-Blocking Inline Adaptive Clarification):** Preserves `PLAN.md §1` (*no modal; result first, ask later*) and plain-text `===META===` streaming (`StreamSplitter`). `ai.Assumption` and `storage.AssumptionRecord` add optional `impact` (`"high"`) and `question` fields (`omitempty`, zero SQLite migration, unchanged SSE `done` top-level keys). High-impact items sort first (capped at 3 high-impact, 4 total), render as `.assumption-chip--high` with popover question + safe-default indicator, and drop `impact: "high"` once confirmed/overridden via 1-click refine (`source = "assumption"`).
- **`D-P011-3` (Inline Target Auto-Detection Hint):** Instead of asking "IDE Agent vs Chat AI?" in a popup, Studio detects strong `agent` or `api` intent signals in `#requestInput` and shows a non-blocking 1-click `#targetHintBtn` chip next to `#targetSelect` when the active target mismatches the request (respecting manual `#targetSelect` changes per draft).

## Tasks

- [x] **T1 (Additive Clarification Schema & StreamSplitter):** Extended `ai.Assumption` & `storage.AssumptionRecord` with `Impact` (`"high"`) and `Question`; normalized (`"high"` / `"critical"` → `"high"`), deduplicated alternatives, capped (max 3 high, max 4 total), and stable-sorted high-impact assumptions first in `StreamSplitter.Finalize()`; cleared resolved `impact: "high"` on `source == "assumption"` refine via `resolveConfirmedAssumption` in `studio.Service`.
- [x] **T2 (Meta-Prompt v2.1 & Target Guides):** Upgraded `generate.v2.md`, `refine.v2.md`, `targets/agent.md`, `targets/chat.md`, and `targets/api.md` with 4-tier variable resolution, repo-discovery delegation for `agent` (removing forced `{{pham_vi_module}}`/`{{mo_ta_loi}}`), and high-impact safe defaults; bumped `GenerateTemplateVersion = "generate.v2.1"` and `RefineTemplateVersion = "refine.v2.1"`.
- [x] **T3 (MockProvider 4-Tier Intelligence):** Upgraded `internal/ai/mock.go` so concrete `agent` and self-contained `chat` requests auto-fill explicit details without redundant `{{variables}}`, data-dependent/reusable/API requests emit appropriate `{{variables}}`, and high-risk/ambiguous-scope requests emit `impact: "high"` + `question` (clearing `impact` upon assumption refine); hardened `detectTargetFromMetaPrompt` to match exact target section headings.
- [x] **T4 (Frontend Inline Adaptive Clarification & Target Hint):** Added `UI.icons.alertCircle`, `.assumption-chip--high` + `.assumptions-section--needs-review` + `.assumption-popover--high` in `@layer pages` (100% design tokens, Dark & Light verified, mobile-first `>= 44px` touch target, zero inline styles), `Escape` popover dismissal with focus return, `#targetHintBtn` + Unicode-safe `detectSuggestedTarget` in `web/app.js`, and updated `web/showcase.html`.
- [x] **T5 (Automated Unit, Integration & Eval Suite):** Added unit tests in `internal/ai/splitter_test.go`, HTTP/SSE + Backup/Restore + pure-Node VM DOM tests in `internal/handlers/p011_test.go`, and expanded `evals/` to 23 scenarios with `expect_zero_vars`, `expect_min_vars`, `expect_high_impact`, `must_contain_any`, and `forbid_placeholders` machine assertions.
- [x] **T6 (Verification Gate & Docs Reconciliation):** Passed `./scripts/check.sh`, `go run ./evals` (`23/23` machine checks `100.0%`, `23/23` blind judge wins `100.0%`), rebuilt `./bin/prompt-studio`, and synchronized `docs/PLAN.md`, `docs/CHANGELOG.md`, `README.md`, and `CSS_DESIGN_SYSTEM.md`.

## Verification Evidence

- `./scripts/check.sh` → exit code `0` (`gofmt`, `go mod tidy -diff`, `go build ./...`, `go vet ./...`, `go test -count=1 ./...`, `node --check web/*.js`).
- `TestFrontendDOMIDsAndDesignSystemInvariants` → PASS (all `el("...")` match real HTML IDs; zero inline styles, zero `!important`, zero `#id` CSS selectors).
- `TestFrontendClarificationAndTargetHintJSWithNode` → PASS (Payment API 500 request suggests `Agent lập trình` over `API`, Unicode Vietnamese diacritic boundaries, manual override suppression, `.assumption-chip--high` + `.assumption-popover--high` question rendering, XSS escaping, and `Escape` focus return).
- `TestAutoFillPayment500ZeroPlaceholdersAndImmediateCompare` & `TestAdaptiveClarificationConfirmClearsHighImpactAndSurvivesBackupRestore` → PASS.
- `go run ./evals` → exit code `0` (`23/23` machine checks passed `100.0%`, `23/23` blind A/B judge wins `100.0%`, `0` injection breaches; saved `evals/results/generate.v2.1-2026-11-16.json`).
