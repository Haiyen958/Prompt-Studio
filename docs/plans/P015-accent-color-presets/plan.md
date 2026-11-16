# P015 — Curated Primary / Accent Color Presets

**Status:** DONE (2026-11-16). Owner selected **Phương án 1** (`Curated Accent Presets` via `html[data-accent="..."]`).
**Workflow:** DEV-KIT `feature` (Settings Persistence + Dual-Mode Theme Tokens + Settings/Showcase Swatch UI).
**Spec & Architecture:** [docs/PLAN.md](../../PLAN.md), [CSS_DESIGN_SYSTEM.md](../../../CSS_DESIGN_SYSTEM.md).

## Decisions Locked (Owner)

- **`D-P015-1` (6 Curated Dual-Mode Accent Presets — Phương án 1):** Allow users to switch the primary/accent brand palette across 6 curated presets (`crimson` default, `indigo`, `ocean`, `emerald`, `violet`, `amber`) via `html[data-accent="<preset>"]` in `web/css/07-themes.css` (`@layer themes`). Every preset defines harmonized Dark (`html:not([data-theme="light"])[data-accent="..."]`) and Light (`html[data-theme="light"][data-accent="..."]`) semantic token sets (primary button gradient/hover/active/shadow, 3-stop hero title gradient, brand badge gradient, top ambient glow, links, selection background/border/text, focus ring, and active AI sweep `--ai-1/--ai-mid/--ai-2`) verified for WCAG 2.1 AA contrast (`>= 4.5:1`), with zero inline styles (`element.style.*` / `style="..."`) and zero raw colors outside `01-tokens.css` & `07-themes.css`.
- **`D-P015-2` (End-to-End Persistence & Flash-Free Boot):** Persist `accent` in SQLite `settings` (`DefaultSettings()["accent"] = "crimson"`), expose & validate in `GET /api/settings` and `PUT /api/settings` (`400 invalid_settings` with `field: "accent"` on unknown values), preserve & sanitize in `GET /api/export` / `POST /api/import`, and cache `ps_theme` + `ps_accent` in `localStorage` inside `web/api.js` for synchronous boot application before `/api/settings` resolves.
- **`D-P015-3` (Decoupled General Preferences Save in Settings):** Changing general appearance/language controls (`themeSelect`, `promptLanguageSelect`, `#accentPicker`) sends a partial `PUT /api/settings` (`{ theme, accent, promptLanguage }`) with instant live preview, avoiding false validation blocks from unsaved AI provider edits.

## Tasks

- [x] **T1 (Backend Settings & Export/Import):** Added `accent` default, validation, API DTO wiring, and backup import sanitization across `internal/storage/{sqlite.go,export.go}`, `internal/studio/settings.go`, and `internal/handlers/settings.go`.
- [x] **T2 (CSS Tokens, Dual-Mode Accent Palettes & Swatch Component):** Added `--swatch-*` preview tokens in `web/css/01-tokens.css`, 5×2 Dark/Light preset blocks in `web/css/07-themes.css`, and `.accent-picker` / `.accent-swatch` styles in `web/css/06-pages.css`.
- [x] **T3 (Frontend Boot, Studio, Settings & Showcase):** Updated `web/api.js`, `web/app.js`, `web/settings.html`, `web/settings.js`, `web/showcase.html`, and `web/showcase.js`.
- [x] **T4 (Automated Tests, Docs Reconciliation & Live Server Restart):** Added backend + frontend tests (`internal/handlers/{p010_test.go,p011_test.go}`), updated `docs/PLAN.md`, `CSS_DESIGN_SYSTEM.md`, `docs/CHANGELOG.md`, passed `./scripts/check.sh` + `evals`, and restarted `./bin/prompt-studio`.

## Verification Evidence

- `./scripts/check.sh` → exit code `0` (`gofmt`, `go mod tidy -diff`, `go build ./...`, `go vet ./...`, `go test -count=1 ./...`, `node --check web/*.js`).
- `$HOME/.local/go/bin/go run ./evals` → exit code `0` (`23/23` PASS).
- Mathematical WCAG 2.1 AA contrast audit across all 6 accents × 2 themes (`12` combinations) → PASS (`4.51:1` to `13.26:1`).
