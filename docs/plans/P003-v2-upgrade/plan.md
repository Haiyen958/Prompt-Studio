# P003 — Prompt Studio v2 Upgrade (`docs/PLAN.md` & `design/`)

**Status:** DONE (2026-11-16). Owner approved `p003_v2_upgrade_plan.md` (`D-V2-1`..`D-V2-3`).  
**Workflow:** DEV-KIT `feature` + `migration` + `refactor` (Gates G0–G6 completed).  
**Authoritative Spec:** [docs/PLAN.md](../../PLAN.md), [design/](../../../design/README.md), [CSS_DESIGN_SYSTEM.md](../../../CSS_DESIGN_SYSTEM.md).

---

## Decisions Locked (`D-V2-*`)

- **`D-V2-1`:** Canonicalize `docs/PLAN.md` and `design/` at repository root; output compiled binary to `./bin/prompt-studio` (and support `go run ./cmd/prompt-studio`).
- **`D-V2-2`:** Full Vietnamese UI matching `design/*.html`; default prompt generation language `"Theo yêu cầu"` (`match` / `vi` / `en`); 3 targets (`chat`, `api`, `agent`); 8 Vietnamese starter templates using `setSelectionRange`.
- **`D-V2-3`:** Automatic backup (`prompt-studio.v1.bak.db`) + schema migration to unified `sessions` + `prompt_versions` + accent-insensitive `sessions_fts (unicode61 remove_diacritics 2)`, migrating API keys to `~/.prompt-studio/secrets.json` (`0600`) and dropping SQLite `secrets` table.

---

## Tasks

- [x] **T1:** `internal/secrets/file.go` (`0600`), `internal/server/middleware.go` (startup `?token=`, `Host`/`Origin` on all requests), unified SQLite v2 schema + v1 migrator + `export.go` (`GET /api/export`, `POST /api/import`).
- [x] **T2:** `internal/ai/splitter.go` (`===META===`, `===USER===`, `{{variable}}` regex), `[[ ]]` templates (`generate.v2.md`, `refine.v2.md`, `targets/{chat,api,agent}.md`, `starters.vi.json`), v2 `MockProvider`, and `evals/` v2 runner + unit tests.
- [x] **T3–T5:** `internal/studio/service.go` & `internal/handlers/` for immediate streaming + editable assumption chips, `incomplete` versions on Stop/timeout, parallel SSE Compare (`POST /api/versions/{id}/compare`), version tree (`parent_id`), 1-click Save, Paste (`409` duplicate detection), and Collections.
- [x] **T6:** Update Vanilla CSS Design System (`01-tokens.css`..`08-utilities.css` with exact `design/tokens.css` palette & 4-state AI input/stream effects) and refactor `web/index.html`, `web/settings.html`, `web/showcase.html`, `web/app.js`, `web/settings.js`, `web/showcase.js` to Vietnamese v2 UX (zero inline styles).
- [x] **T7:** Full unit, migration, and E2E test suite (`go test -v -count=1 ./...`), `go run ./evals`, `./bin/prompt-studio` build, and documentation sync (`README.md`, `CSS_DESIGN_SYSTEM.md`, `docs/CHANGELOG.md`).

---

## Verification Evidence

- `grep -rn 'style=' web/*.html web/*.js` → **0 matches** (zero inline style attributes).
- `grep -rn '!important' web/css/*.css web/style.css` & `grep -rnE '^\s*#[A-Za-z]' web/css/*.css web/style.css` → **0 rule violations**.
- `go test -v -count=1 ./...` → **EXIT 0** (`internal/ai`, `internal/handlers`, `internal/secrets`, `internal/security`, `internal/storage` all PASS).
- `go run ./evals` → **EXIT 0** (`20/20` machine checks `100.0%`, `20/20` blind A/B judge wins `100.0%`, `0` injection breaches; report saved at `evals/results/generate.v2-2026-11-16.json`).
- `go build -o ./bin/prompt-studio ./cmd/prompt-studio` → **EXIT 0** (`./bin/prompt-studio` built cleanly).
