# P010 — Compare Polish, Storage/Import Hardening, Inline Variable Fill & Version Diff

**Status:** DONE (2026-11-16). Owner approved full scope (Package A + Package B).
**Workflow:** DEV-KIT `bugfix` (A1–A4, regression test first) + `feature` (B1–B4).
**Spec & Architecture:** [docs/PLAN.md](../../PLAN.md), [docs/ARCHITECTURE.md](../../ARCHITECTURE.md), [CSS_DESIGN_SYSTEM.md](../../../CSS_DESIGN_SYSTEM.md).

## Decisions Locked (Owner)

- **`D-P010-1`:** "Điền biến & Copy" in Studio and Library uses an inline collapsible panel inside the result/detail card (no modal), sharing per-session variable values in memory with the Compare screen; the primary `Copy` button continues to copy the raw template with `{{variables}}` in one click.
- **`D-P010-2`:** "Xem thay đổi" (Version Diff) renders an inline line-by-line diff inside the result card (`--color-success-soft` / `--color-error-soft`), defaulting to the direct parent (`parent_id`) with a selector to compare against `v1` or any other version in the session.

## Tasks

- [x] **T1 (A2 — Storage FTS on Unsave):** `UnsaveSession` runs inside a transaction and calls `syncSessionFTSTx` so `sessions_fts` switches immediately from the previously pinned version to the latest version. Test: `TestUnsaveSessionSyncsFTSIndex`.
- [x] **T2 (A4 — Backup Import Safety):** `ImportAll` remaps imported `collection_id`s (and `last_collection_id`) when a backup contains a user collection matching an existing local collection name under a different `id`, and filters imported `settings` keys/values to known valid keys and ranges. Test: `TestImportRemapsDuplicateCollectionNamesAndSanitizesSettings`.
- [x] **T3 (A1 — Backend Compare Error Contract):** `CompareHandler` translates `side_error` via `sseErrorPayload` (Vietnamese messages + server-side logging for unexpected errors), and `CompareVersion` avoids emitting a duplicate top-level `error` when both sides already emitted `side_error`. Test: `TestCompareSideErrorUsesVietnameseContractAndNoDuplicateError`.
- [x] **T4 (B3 — Markdown Renderer Upgrade):** Extended `renderMarkdownToHTML` (`web/app.js`) + `@layer pages` (`web/css/06-pages.css`) with `h1`–`h4` hierarchy, `*italic*` (safe around `*` in inline code), `> blockquote`, `---` horizontal rules, and GFM pipe tables (`.table-wrap > .table.table--compact`) with full XSS escaping and `{{var}}` pills.
- [x] **T5 (A1 — Frontend Compare Polish):** Added `Dừng` button while Compare streams and cleaned up `.ai-caret`/status on `AbortError`/`Esc`; synced `compareSaveBtn` with 3-state pin status + toast feedback; added "Xem toàn bộ prompt / Thu gọn" toggle for Side B and `Dữ liệu thử` summary on Side A; persisted test variable values per session in frontend state.
- [x] **T6 (A3 + B1 — Library API Split & Inline Variable Fill + Copy):** Rendered two-part `system`/`user` `.api-block`s + "Copy JSON messages" in Library detail for `target = "api"`; added inline collapsible "Điền biến" panel (`D-P010-1`) in Studio and Library with "Copy bản đã điền" (and "Copy JSON đã điền" when `target = "api"`).
- [x] **T7 (B2 + B4 — Version Lineage/Diff & Settings Endpoint Presets):** Added parent lineage (`· từ vN`) on `versionSourceBadge` and Library version list, inline line-diff toggle + base version selector (`D-P010-2`) in Studio, and `OpenAI-compatible` Base URL preset chips (`Ollama`, `LM Studio`, `OpenRouter`, `Gemini`, `Groq`) in `/settings`.
- [x] **T8 (Verification & Docs Reconciliation):** Passed `./scripts/check.sh`, `TestFrontendDOMIDsAndDesignSystemInvariants`, Node unit assertions on Markdown/Diff/Variables, compiled `./bin/prompt-studio`, and reconciled `docs/PLAN.md` + `docs/CHANGELOG.md`.

## Verification Evidence

- Red-to-green regression proof: `TestUnsaveSessionSyncsFTSIndex`, `TestImportRemapsDuplicateCollectionNamesAndSanitizesSettings`, and `TestCompareSideErrorUsesVietnameseContractAndNoDuplicateError` all failed on the pre-fix code with the exact reported defects, and pass after the fixes.
- `./scripts/check.sh` → exit code `0` (`gofmt`, `go mod tidy -diff`, `go build ./...`, `go vet ./...`, `go test -count=1 ./...`, `node --check web/*.js`).
- `TestFrontendDOMIDsAndDesignSystemInvariants` → PASS (every `el("...")` in `app.js` and `settings.js` matches a real `id="..."` in `index.html` / `settings.html`; zero inline styles, zero `!important`, zero `#id` CSS selectors).
- JS logic verification (`node` VM harness) → PASS (`h1`–`h4`, `*italic*`, inline code star safety, XSS `<script>` escaping, `blockquote`, `hr`, GFM pipe tables, variable filling, LCS line diff, parent lineage labels).
