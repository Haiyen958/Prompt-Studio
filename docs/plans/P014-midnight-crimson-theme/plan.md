# P014 — Midnight Navy & Crimson–Rose–Coral Color System

**Status:** DONE (2026-11-16). Owner approved optimal palette + visual polish (`đề xuất tối ưu và thực hiện`).
**Workflow:** DEV-KIT `feature` / `refactor` (Design System Token Overhaul + Ambient Hero Glow + WCAG 2.1 AA/AAA Audit).
**Spec & Architecture:** [docs/PLAN.md](../../PLAN.md), [CSS_DESIGN_SYSTEM.md](../../../CSS_DESIGN_SYSTEM.md).

## Decisions Locked (Owner)

- **`D-P014-1` (Midnight Navy & Crimson–Rose–Coral Palette + Subtle Ambient Polish):** Supersedes the Warm Graphite palette and partially updates `D-P006-2` to allow the subtle top radial hero glow (`rgba(214, 58, 86, 0.26)`), translucent `.ai-frame` (`rgba(14, 19, 40, 0.88)`), 3-stop hero title gradient (`#EE4B5E → #E2549A → #F0904F`), `.brand__mark` gradient (`#E8475A → #D9488C`), and `.btn--primary` gradient (`#C9304A → #B13A7A`, white text `5.24:1–5.59:1` AA) with soft primary shadow (`rgba(201, 48, 74, 0.38)`).
- **`D-P014-2` (Single Primary CTA Discipline & Error Disambiguation):** `+ Phiên mới` remains `.btn--secondary` (`#0C1124` + `#232A44` border) so `Tạo prompt` / `Copy` stay the sole visual hero per region; `--color-error` shifts to Vermilion-Orange (`#FF7A45` on `#2C140E`, `6.69:1` AA) so errors never collide with Rose–Pink selection/focus (`#F2717F` / `#F7A1AA`).
- **`D-P014-3` (Harmonized Crimson–Rose Light Theme & Cohesive AI Conic Sweep):** Light theme (`html[data-theme="light"]`) adopts a matching Crimson–Berry brand accent (`#BE2944 → #A62E6E`, `5.85:1+` AA), and `--ai-1` / `--ai-mid` / `--ai-2` align with `#EE4B5E → #E2549A → #F0904F`.

## Tasks

- [x] **T1 (Design Tokens — Dark & Light):** Updated `web/css/01-tokens.css` and `web/css/07-themes.css` with semantic surface, border, 7-tier typography, gradient, focus, status, and AI motion tokens.
- [x] **T2 (Base, Layout, Components & Pages Wiring):** Wired semantic tokens across `web/css/{03-base.css,04-layout.css,05-components.css,06-pages.css}`.
- [x] **T3 (Design System Invariant Tests & Docs Reconciliation):** Extended `TestFrontendDOMIDsAndDesignSystemInvariants` (`internal/handlers/p010_test.go`) to guard zero raw colors outside `01-tokens.css` & `07-themes.css` and verify theme token parity; updated `CSS_DESIGN_SYSTEM.md`, `docs/PLAN.md`, `docs/CHANGELOG.md`; passed `./scripts/check.sh` and restarted the dev server.

## Verification Evidence

- `./scripts/check.sh` → exit code `0` (`gofmt`, `go mod tidy -diff`, `go build ./...`, `go vet ./...`, `go test -count=1 ./...`, `node --check web/*.js`).
- `TestFrontendDOMIDsAndDesignSystemInvariants` → PASS (0 hardcoded hex/rgb outside token files, 100% `07-themes.css` ↔ `01-tokens.css` token parity).
- Mathematical WCAG 2.1 contrast audit (`node` luminance verifier) → PASS across all Dark & Light text/button/placeholder/status pairs (`>= 5.01:1`).
