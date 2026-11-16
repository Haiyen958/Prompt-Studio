# P013 — Inline & Direct Prompt Renaming in Library

**Status:** DONE (2026-11-16). Owner requested: *"cho phép đổi tên các prompt trong thư viện"*.
**Workflow:** DEV-KIT `feature` + `bugfix` (Inline UX + Backend Title Validation + Vietnamese `d`/`đ` FTS5 Fix).
**Spec & Architecture:** [docs/PLAN.md](../../PLAN.md), [docs/ARCHITECTURE.md](../../ARCHITECTURE.md), [CSS_DESIGN_SYSTEM.md](../../../CSS_DESIGN_SYSTEM.md).

## Decisions & Design

- **`D-P013-1` (Discoverable, Non-Modal Inline Rename in Library):**
  1. Added a visible `Đổi tên` button (`#libInlineRenameBtn`) right next to `#libDetailTitle` in the Library detail pane; clicking it (or `"Đổi tên"` in `#libMoreMenu`, or the quick rename button on any `.library-item` row in `#libraryItemsList`) opens an inline form (`#libInlineRenameForm` with `#libInlineRenameInput`, `Lưu`, `Hủy`; `Enter` submits, `Escape` cancels and restores focus — zero modal per `PLAN.md §1`).
  2. Split `.library-item` cleanly into `<div class="library-item">` containing `<button class="library-item__main">` + `<span class="library-item__actions"><button class="btn btn--ghost btn--icon">` (no nested `<button>`, hover/focus-within reveal on desktop, always visible on touch `@media (hover: none)`).
  3. Kept `state.session.title` in sync when renaming a prompt that is also open in Studio.
- **`D-P013-2` (Backend Title Validation & Vietnamese `d`/`đ` FTS5 Search):** `studio.Service.UpdateSession` validates `patch.Title != nil` against empty/whitespace (`400 invalid_input`: `"tên prompt không được để trống"`) and `MaxSessionTitleChars = 200`, while `buildFTSQuery` expands `d`/`đ` variants at query time so accent-insensitive search matches stroked Vietnamese letters (`định`, `đổi`, `đơn`) on both existing and newly renamed prompts without altering the SQLite schema.

## Tasks

- [x] **T1 (Backend Validation & FTS `d`/`đ` Expansion):** Enforced non-empty and max-200-rune `title` validation in `studio.Service.UpdateSession` (`internal/studio/service.go`, `internal/studio/library.go`) and expanded `d`/`đ` in `buildFTSQuery` (`internal/storage/sessions.go`).
- [x] **T2 (Frontend Inline Rename & List Quick Action):** Added `UI.icons.edit` (`web/ui.js`), inline rename header controls in `web/index.html`, split `.library-item` + `.library-item__actions` in `web/app.js` & `web/css/06-pages.css`, and synchronized Studio state on rename.
- [x] **T3 (Automated Backend, FTS & JS VM Tests):** Added `TestLibraryPromptRenameValidationAndFTSSearch` and Node VM inline-rename + `Escape` cancel + split-card button assertions in `internal/handlers/p011_test.go`.
- [x] **T4 (Docs, Verification & Dev Server Restart):** Synchronized `docs/PLAN.md` & `docs/CHANGELOG.md`, passed `./scripts/check.sh`, rebuilt `./bin/prompt-studio`, and restarted the live dev server.

## Verification Evidence

- `./scripts/check.sh` → exit code `0` (`gofmt`, `go mod tidy -diff`, `go build ./...`, `go vet ./...`, `go test -count=1 ./...`, `node --check web/*.js`).
- `TestLibraryPromptRenameValidationAndFTSSearch` & `TestFrontendClarificationAndTargetHintJSWithNode` → PASS.
