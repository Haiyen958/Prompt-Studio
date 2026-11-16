# P002 — Vanilla CSS Design System & UI Architecture Refactor

**Status:** DONE (2026-11-16). Owner approved architecture & migration plan (`css_design_system_plan.md`). All tasks `T1`–`T5` implemented and verified (§C).  
**Workflow:** DEV-KIT `refactor` + additive `feature` (`GET /showcase` & `CSS_DESIGN_SYSTEM.md`) (Gates G0–G6).  
**Plan system:** Canonical repository plan record (`scripts/plan.mjs` not present in repo).

---

## A. Scope & Objectives

1. Replace the monolithic `web/style.css` and 60+ inline styles across HTML/JS with a modular, zero-framework **Vanilla CSS Design System** governed by `@layer tokens, reset, base, layout, components, pages, themes, utilities;`.
2. Establish complete semantic **Design Tokens** (`01-tokens.css` & `07-themes.css`) covering Colors (Dark default + Light theme), 4px Spacing scale (`--space-1`..`--space-16`), Typography scale, Layout tokens, and Visual tokens (radii, shadows, borders, transitions, z-index).
3. Implement a mobile-first **Layout & 12-Column Grid System** (`04-layout.css`): `.container`, `.page-layout`, `.grid`, `.grid-cols-2..4`, `.col-span-1..12`, `.grid-auto-fit`, `.stack`, `.cluster`, `.sidebar-layout`, `.section`.
4. Implement **10 Reusable BEM Component Families** (`05-components.css`): Button, Input/Textarea/Select, Card/Panel, Modal/Dialog/Dropdown, Tabs/Navigation, Badge/Chip/Status, Table/Pagination, Toolbar/Filter bar, Tooltip/Popover, Loading/Empty state.
5. Refactor `web/settings.html`, `web/settings.js`, `web/index.html`, and `web/app.js` with zero JS functional regressions and full keyboard `:focus-visible` + `prefers-reduced-motion` accessibility.
6. Deliver interactive Component Showcase (`web/showcase.html` at `GET /showcase`), `CSS_DESIGN_SYSTEM.md`, and mandatory AI Agent UI Development Checklist (`.agents/rules/ui-design-system.md`).

---

## B. Completed Tasks

- [x] **T1:** Authored `web/css/01-tokens.css` .. `08-utilities.css` and `web/style.css` cascade entrypoint.
- [x] **T2:** Refactored `web/settings.html` and `web/settings.js` (removed all 26 inline styles and `.style.color` mutations).
- [x] **T3:** Refactored `web/index.html` and `web/app.js` (removed all 39 inline styles, added mobile drawer backdrop + `Esc` close, fixed `901px–1180px` Library 3-column responsive cramping, aligned BEM classes).
- [x] **T4:** Created `web/showcase.html`, `web/showcase.js`, and `web/css/showcase.css`; updated `web/embed.go`, `internal/handlers/router.go` (`GET /showcase`), and `internal/handlers/e2e_test.go` (`TestEmbeddedWebAssetsAndShowcaseRoute`).
- [x] **T5:** Created `CSS_DESIGN_SYSTEM.md`, `.agents/rules/ui-design-system.md` checklist, and reconciled `README.md` & `docs/CHANGELOG.md`.

---

## C. Verification Evidence (Gate G6)

- **Inline Style & CSS Hygiene Checks:**
  - `grep -rn 'style=' web/*.html web/*.js` → `0 matches` (exit status verified).
  - `grep -rn '!important' web/style.css web/css/*.css` → `0 declaration matches`.
  - `grep -rnE '^\s*#[a-zA-Z]' web/style.css web/css/*.css` → `0 ID selector matches`.
- **Unit & E2E Test Suite (`go test -v -count=1 ./...`):** Exit code `0` (`internal/ai`, `internal/handlers` including `TestEmbeddedWebAssetsAndShowcaseRoute`, `internal/security`, `internal/storage`).
- **Meta-Prompt Evals (`go run ./evals`):** Exit code `0` (`25/25 = 100.0%`, `0` injection breaches).
- **Single-Binary Build (`go build -o prompt-studio ./cmd/prompt-studio`):** Exit code `0` (`prompt-studio` binary rebuilt with embedded CSS layers and `/showcase`).
