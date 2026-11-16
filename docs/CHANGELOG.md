# Changelog

All notable approved changes, plans, and decisions for **Prompt Studio** are documented in this file.

## [Unreleased] — 2026-11-16 (`P015-accent-color-presets`)

### Decisions Locked (owner)
- **`D-P015-1` (6 Curated Dual-Mode Accent Presets — Phương án 1):** Added 6 selectable primary/accent brand palettes (`crimson` default, `indigo`, `ocean`, `emerald`, `violet`, `amber`) driven exclusively by `html[data-accent="<preset>"]` in `web/css/07-themes.css` (`@layer themes`) across both Dark and Light modes, maintaining 100% WCAG 2.1 AA contrast (`>= 4.5:1`), zero inline styles, and zero raw colors outside `01-tokens.css` & `07-themes.css`.
- **`D-P015-2` (End-to-End Persistence & Flash-Free Boot):** Persisted `accent` in SQLite `settings`, `GET/PUT /api/settings` (`400 invalid_settings` with `field: "accent"` on unknown presets), and JSON Export/Import (`sanitizeImportedSettings`), plus synchronous `localStorage` (`ps_theme`, `ps_accent`) boot restoration in `web/api.js`.
- **`D-P015-3` (Decoupled General Preferences Auto-Save):** Updated `/settings` (`web/settings.js`) so changing `theme`, `accent`, or `promptLanguage` dispatches a partial `PUT /api/settings` without colliding with unsaved AI provider edits.

### Added
- `accent` setting default/validation/backup support in `internal/storage/{sqlite.go,export.go}`, `internal/studio/settings.go`, and `internal/handlers/settings.go`.
- Dual-mode CSS presets (`indigo`, `ocean`, `emerald`, `violet`, `amber`), `--swatch-*` tokens, and `.accent-picker` / `.accent-swatch` component (`web/css/{01-tokens.css,06-pages.css,07-themes.css}`), wired into `/settings` (`web/settings.html`, `web/settings.js`), Studio (`web/app.js`, `web/api.js`), and `/showcase` (`web/showcase.html`, `web/showcase.js`).
- Automated tests `TestAccentPresetSettingsAndBackupRoundtrip`, `TestSettingsAccentPickerAndBootAppearanceInNodeVM` (`internal/handlers/p011_test.go`), and `07-themes.css` / `settings.html` preset parity checks (`internal/handlers/p010_test.go`).

## [Unreleased] — 2026-11-16 (`P014-midnight-crimson-theme`)

### Decisions Locked (owner)
- **`D-P014-1` (Midnight Navy & Crimson–Rose–Coral Palette + Subtle Ambient Polish):** Replaced the Warm Graphite palette with Midnight Navy surfaces (`#070A18` bg, `#0A0E1F` sidebar, `#0C1124` cards/inputs/chips, `rgba(14, 19, 40, 0.88)` `.ai-frame`), top radial crimson glow (`rgba(214, 58, 86, 0.26)`), 7-tier WCAG AA/AAA cool-slate typography (`#F3F4F8` down to `#7E859C` placeholder at `5.01:1`), `.brand__mark` gradient (`#E8475A → #D9488C`), `.studio-hero__title` gradient (`#EE4B5E → #E2549A → #F0904F`), and `.btn--primary` gradient (`#C9304A → #B13A7A`, white text `5.24:1–5.59:1` AA) with `--shadow-primary` (partially updating `D-P006-2`).
- **`D-P014-2` (Single Primary CTA & Error Disambiguation):** Kept `+ Phiên mới` as `.btn--secondary` so `Tạo prompt` / `Copy` remain the sole visual hero per region, and shifted `--color-error` to Vermilion-Orange (`#FF7A45` on `#2C140E`) to avoid colliding with Rose–Pink selection/focus (`#F2717F` / `#F7A1AA`).
- **`D-P014-3` (Harmonized Crimson–Rose Light Theme & Cohesive AI Conic Sweep):** Updated `html[data-theme="light"]` (`07-themes.css`) to a matching Crimson–Berry accent (`#BE2944 → #A62E6E`) and aligned `--ai-1` / `--ai-mid` / `--ai-2` with the brand spectrum.

### Changed
- `web/css/{01-tokens.css,03-base.css,04-layout.css,05-components.css,06-pages.css,07-themes.css}`: Wired semantic surface, border, dashed-border (`#343C5C`), typography, gradient, focus, status (`#3DD68C` connected dot), and AI motion tokens across Dark and Light themes.
- `internal/handlers/p010_test.go` (`TestFrontendDOMIDsAndDesignSystemInvariants`): Added automated guards forbidding hardcoded `#hex` / `rgb()` / `rgba()` literals outside `01-tokens.css` & `07-themes.css` and verifying `07-themes.css` token parity with `:root`.

## [Unreleased] — 2026-11-16 (`P013-library-prompt-rename`)

### Added
- Inline non-modal prompt renaming in Library (`#libInlineRenameBtn`, `#libInlineRenameForm` with `Enter` save / `Escape` cancel, quick rename action button on each `.library-item` card, and `#libMoreMenu` trigger), keeping Studio's active session title synchronized (`web/index.html`, `web/app.js`, `web/css/06-pages.css`, `web/ui.js`).
- Backend `title` validation on `PATCH /api/sessions/{id}` and `PUT /api/sessions/{id}/save` (`studio.Service.UpdateSession`: rejects blank titles and titles exceeding `200` runes with `400` Vietnamese error messages) plus FTS5 rename search test (`TestLibraryPromptRenameValidationAndFTSSearch`).

### Fixed
- `storage.buildFTSQuery`: SQLite's `unicode61 remove_diacritics 2` tokenizer leaves Vietnamese stroked `đ`/`Đ` untouched; query-time `d`/`đ` expansion now allows accentless searches (e.g. `kiem dinh`, `doi ten`) to match existing and newly renamed titles immediately.

## [Unreleased] — 2026-11-16 (`P012-remove-compare-feature`)

### Decisions Locked (owner)
- **`D-P012-1` (Remove Side-by-Side Compare End-to-End):** Removed the "So sánh với yêu cầu gốc" feature across the Studio UI (`#openCompareBtn`, `#compareView`, Compare JS state/handlers, `.compare-*` CSS), HTTP router (`POST /api/versions/{id}/compare`, `internal/handlers/compare.go`), and domain service (`CompareVersion`, `CheckComparable`, `CompareInput`, `MissingVariablesError`, `ErrCompareNotAllowed`, `ErrCompareIncomplete`, `formatTestDataAppendix`). Preserved `"Xem thay đổi"` (Version Diff, `D-P010-2`), `"Điền biến"` inline drawer (`D-P010-1`), and historical static mockup `design/compare.html`.
- **`D-P012-2` (Retain Offline CLI `evals/` Blind Judge):** Kept `go run ./evals` (`evals/main.go`) intact as the developer CLI benchmark for Meta-Prompt quality verification.

### Removed
- `POST /api/versions/{id}/compare` route, `internal/handlers/compare.go`, `studio.Service.CompareVersion` + helpers, Studio `#openCompareBtn` + `#compareView` markup/JS, `.compare-*` CSS rules in `web/css/06-pages.css`, and Compare HTTP test cases.

## [Unreleased] — 2026-11-16 (`P011-autofill-adaptive-clarification`)

### Decisions Locked (owner)
- **`D-P011-1` (4-Tier Variable Resolution & Auto-fill):** Meta-Prompt (`generate.v2.1` / `refine.v2.1`) and target guides resolve context across 4 tiers: explicitly stated details are filled directly into the prompt (never stripped into redundant `{{pham_vi_module}}`/`{{mo_ta_loi}}` placeholders), safe inferences populate `assumptions`, repository-discoverable facts (`language`, `framework`, file layout) are delegated to `target = "agent"` to inspect in the codebase, and runtime `{{ten_bien}}` placeholders are reserved strictly for `target = "api"` user payloads, external pasted-at-runtime data, or explicit reusable-template requests.
- **`D-P011-2` (Non-Blocking Inline Adaptive Clarification):** Preserves `PLAN.md §1` (*no modal; result first, ask later*) and plain-text `===META===` streaming. Extends `ai.Assumption` / `storage.AssumptionRecord` additively with optional `impact: "high"` and `question` (zero SQLite schema migration, unchanged SSE `done` top-level keys). High-impact ambiguities apply a safe default in `v1`, sort ahead of normal assumptions (capped at 3 high-impact, 4 total), render as `.assumption-chip--high` with popover question + current default note, and automatically clear `impact: "high"` once confirmed/overridden via `source = "assumption"` refine.
- **`D-P011-3` (Inline Target Auto-Detection Hint):** Studio detects strong coding-agent or system-prompt-API signals in `#requestInput` (with Unicode-safe Vietnamese boundaries, prioritizing `agent` over subject-matter `"API"` mentions) and surfaces a 1-click `#targetHintBtn` (`Gợi ý: Đổi sang …`) next to `#targetSelect` unless manually overridden for the current draft.

### Added
- Additive `impact` and `question` fields on `ai.Assumption` and `storage.AssumptionRecord`, priority sorting + max-3 high-impact cap in `StreamSplitter.Finalize()`, and `resolveConfirmedAssumption` in `studio.Service`.
- Inline high-impact clarification UI (`.assumptions-section--needs-review`, `.assumption-chip--high`, `.assumption-popover--high`, `UI.icons.alertCircle`, `Escape` popover dismissal with focus return) and `#targetHintBtn` in Studio (`web/index.html`, `web/app.js`, `web/css/06-pages.css`, `web/ui.js`, `web/showcase.html`).
- Expanded `evals/` to 23 scenarios (`risk-01`, `risk-02`, `reusable-01`, plus `expect_zero_vars`, `expect_min_vars`, `expect_high_impact`, `must_contain_any`, `forbid_placeholders` checks) and added `internal/handlers/p011_test.go` (including pure-Node VM DOM verification).

### Changed
- Upgraded `internal/ai/templates/{generate.v2.md,refine.v2.md,targets/*.md}` (`generate.v2.1` / `refine.v2.1`) and `internal/ai/mock.go` to eliminate over-templating on concrete one-shot requests (`targets/agent.md` no longer forces `{{pham_vi_module}}`/`{{mo_ta_loi}}`) and fixed `MockProvider.detectTargetFromMetaPrompt` to match exact target section headers.

## [Unreleased] — 2026-11-16 (`P010-compare-library-vars-diff`)

### Decisions Locked (owner)
- **`D-P010-1`:** "Điền biến & Copy" in Studio and Library uses an inline collapsible drawer inside the result/detail card (no modal), sharing per-session variable values in memory with Compare; primary `Copy` continues to copy the raw template with `{{variables}}` in one click.
- **`D-P010-2`:** "Xem thay đổi" (Version Diff) renders an inline line-by-line diff inside the result card, defaulting to the direct parent (`parent_id`) with a selector to compare against `v1` or any other version in the session.

### Added
- Inline Variable Fill & Copy (`#studioVarsDrawer`, `#libVarsDrawer`) in Studio and Library with `Copy bản đã điền` (plus `Copy JSON đã điền` when `target = "api"`), sharing typed variable values per session with Compare.
- Inline line-by-line LCS Version Diff (`Xem thay đổi` + base version selector) and parent lineage labels (`· từ vN`) in Studio and Library version lists.
- Compare UI: `Dừng` button while streaming, `Xem toàn bộ prompt / Thu gọn` toggle for Side B, and `Dữ liệu thử` preview under Side A.
- Settings: quick `Base URL` preset chips (`Ollama`, `LM Studio`, `OpenRouter`, `Gemini`, `Groq`) when `OpenAI-compatible` is selected.
- Markdown renderer: `h1`–`h4` hierarchy, `*italic*` (without corrupting `*` inside inline code), `> blockquote`, `---` horizontal rules, and GFM pipe tables (`.table-wrap > .table.table--compact`).
- `TestFrontendDOMIDsAndDesignSystemInvariants` guarding DOM `id` lookups, zero inline styles, zero `!important`, and zero `#id` CSS selectors across embedded assets.

### Fixed
- `CompareHandler` / `studio.Service.CompareVersion`: `side_error` SSE payloads now use the Vietnamese error contract (`sseErrorPayload`) instead of raw English/internal error strings, and no longer emit a redundant top-level `error` event when both sides already reported `side_error`.
- Compare UI: pressing `Escape` or clicking `Dừng` now strips `.ai-caret` and marks unfinished columns `"Đã dừng"` instead of freezing on `"Đang chạy…"`; `compareSaveBtn` syncs all 3 pin states (`Lưu vào thư viện` / `Đã lưu` / `Cập nhật bản lưu (vN)`) with toast feedback.
- `storage.Repo.UnsaveSession`: runs in a transaction with `syncSessionFTSTx` so `sessions_fts` switches immediately from the unpinned version to the latest version.
- `storage.Repo.ImportAll`: remaps `collection_id` and `last_collection_id` when an imported backup contains a collection matching an existing local collection name under a different ID (preventing `UNIQUE(collections.name)` failures), and filters imported `settings` to known valid keys/ranges.
- Library detail view now splits `target = "api"` prompts on `===USER===` into two-part `system`/`user` `.api-block`s and exposes `Copy JSON messages` in the `⋯` menu.

## [Unreleased] — 2026-11-16 (`P009-settings-model-picker`)

### Decisions Locked (owner)
- **`D-P009-1`:** The model is chosen in Settings from the provider's live catalog (free typing still allowed); no per-session model choice (PLAN §1 unchanged).
- **`D-P009-2`:** Anthropic has no default model; switching provider requires picking a model in the same save.

### Added
- `POST /api/settings/models` (`{provider, baseUrl, apiKey}` → `{ok, models:[{id,name}]}` or `{ok:false, code, error}`), same API-key policy as Test Connection. OpenAI lists chat-capable families, OpenAI-compatible lists everything, Anthropic follows pagination.
- Settings: "Tải danh sách model" button (auto-loads when a key is saved); switching provider clears the model (switching back restores the saved one).

### Changed
- Removed the outdated `claude-3-5-sonnet-latest` default and the static model datalist. An empty Anthropic model now fails with `model_required` ("Mở Cài đặt" in the Studio banner).
- `PUT /api/settings` returns 400 `invalid_settings` (`field: "model"`) when the provider changes without a model, or Anthropic has none.
- Test Connection key/endpoint resolution moved to `studio.Service.ProbeConfig`; failures are explained in Vietnamese (network errors shown verbatim).

## [Unreleased] — 2026-11-16 (`P008-audit-p1-p2`)

### Added
- `docs/ARCHITECTURE.md`: layers, dependency rules, where new code goes, contracts, agent rules.
- `scripts/check.sh`: single verification gate (gofmt, tidy, build, vet, test, `node --check`).
- Versioned schema migrations (`PRAGMA user_version`) with automatic `VACUUM INTO` backup
  (`prompt-studio.pre-schema-vN.bak.db`) of existing databases; newer schemas are refused.
- Graceful shutdown: Ctrl+C/SIGTERM stops streams (partial prompts saved as incomplete), waits for handlers, closes the DB.
- Studio error banner actions: "Mở Cài đặt" (`unconfigured`, `auth_error`), "Thử lại" (`rate_limit`, `timeout`, `bad_response`, network).

### Changed
- Unexpected server errors return `internal_error` with a generic message (details only in the log); SSE `error` messages are Vietnamese.
- Duplicate collection names return 400 `collection_error` ("đã tồn tại") instead of a raw SQLite message.
- OpenAI (official API) streams request real token usage.
- Internal: shared generate/refine pipeline; handlers only call `studio.Service`; `storage/repo.go` split by aggregate.

### Fixed
- A failed refine no longer clears the typed instruction.
- After Stop, the Studio waits up to ~3 s (was ~0.9 s) for the partial version.
- The v1 migrator no longer creates an empty `master.key`; FTS5 creation failures are no longer ignored.

### Removed
- `prompt-studio/` (stale duplicate of `design/` and `docs/PLAN.md`), unused `security.SystemKeychain` and duplicate `security.KeyHint`.

## [Unreleased] — 2026-11-16 (`P007-audit-p0-hardening`)

### Decisions Locked
- **`D-P007-1`** (supersedes `D-P005-3`): stable session token in `<data-dir>/session.token` (0600); only the printed `?token=` URL hands out the (400-day) HttpOnly cookie; all `/api/*` requests and HTML pages require it.

### Security
- `internal/server/middleware.go`: any local `GET /` used to receive the token cookie, and `GET /api/*` (including `/api/export`) needed no token. Reads and pages now require the token.
- `internal/handlers/settings.go`: Test Connection no longer sends the stored API key to a different provider/Base URL.

### Fixed
- `POST /api/import`: backups larger than 4 MB were rejected by the generic write cap; import now allows 32 MB.
- `POST /api/sessions/{id}/versions`: a `parentId` from another session was stored as the parent (unknown ids returned 500); both now return 400 `version_mismatch`. Logic moved to `studio.Service.CreateManualVersion`.
- Temperature `0` was silently replaced by `0.4` (backend parse and `web/settings.js`).

### Changed
- `PUT /api/settings` validates the whole update first and returns 400 `invalid_settings` with `field`; nothing is written on error.
- Settings defaults have one source (`storage.DefaultSettings`); the unused `language` seed key is now `prompt_language`.
- `go.mod`: removed 7 unused direct modules; `modernc.org/sqlite` and `goccy/go-yaml` are now direct.

## [Unreleased] — 2026-11-16 (`P006-ui-audit`)

### Decisions Locked
- **`D-P006-1`:** Flat 0–4px corner radius (pills only for assumption/starter chips, status dots, spinners).
- **`D-P006-2`:** Keep the conic "AI busy" border; remove the glow halo and the blurred focus glow.
- **`D-P006-3`:** Mobile remains out of MVP scope; it only has to stay usable (no overflow).

### Fixed
- `web/css/03-base.css`: Root `font-size` was `0.875rem`, which shrank every rem token to 87.5% (body text rendered at 12.25px). The root is now `100%`.
- `web/css/08-utilities.css`: `.text-xs/sm/md/lg/xl` and `.font-medium/semibold` were used in markup but never defined.
- `web/app.js`: History, collection, library and version rows are now `<button>`s (keyboard accessible). Library actions report failures with an error toast instead of failing silently. Generate is locked while AI is unconfigured.
- `.btn--danger` text contrast (≈2.3:1 → AA) via the new `--color-on-danger`.

### Changed
- Tokens: 4px radius, 3 control heights + 44px touch target, card shadows replaced by borders, light theme tokens defined once.
- Buttons reduced to 5 variants (`primary`, `secondary`, `ghost`, `danger`, `danger-outline`). `btn--outline`, `btn--contrast` and `btn--saved` were removed, and saved state now uses `aria-pressed`.
- Studio result header: So sánh / Lưu / Copy plus a ⋯ menu (Tạo lại, Sửa tay, Tải .md, Copy JSON). The hero hides after the first result, and errors show under the request box.
- Library detail: a ⋯ menu distinguishes "Bỏ khỏi thư viện" (with Undo) from "Xóa vĩnh viễn" (confirm).
- New `web/ui.js` (toast, themed confirm/prompt dialog, dropdown menu keyboard support, live-region announcements). All native `alert/confirm/prompt` dialogs were removed.

## [2.1.0] — 2026-11-16 (`P005-save-pin-target-paging-security`)

### Decisions Locked (owner, 2026-11-16)
- **`D-P005-1`:** "Lưu" pins the version being viewed; a different viewed version offers "Cập nhật bản lưu (vN)". The separate Library pin button is removed.
- **`D-P005-2`:** History and Library are paginated ("Tải thêm"); no automatic deletion.
- **`D-P005-3`:** Writes require the exact app Origin and `Content-Type: application/json`; startup cookie flow unchanged.
- **`D-P005-4`:** `target` is stored per version.

### Changed
- `PUT /api/sessions/{id}/save` accepts `versionId` and always pins (latest when empty); unsave clears the pin; pinning a version from another session returns 400.
- `GET /api/sessions` accepts `limit`/`offset` and returns `hasMore`/`nextOffset`.
- `prompt_versions.target` added (additive `ALTER TABLE` on open; legacy rows inherit `sessions.target`); export/import carry it; refine inherits the base version's target unless one is given; manual edits inherit the parent's.
- Compare: blocked for unfilled `{{variables}}` (`missing_variables`) and incomplete versions (`compare_incomplete`); per-side failures stream `side_error` while the other side completes.
- Middleware: exact Origin match (other localhost ports rejected) and 415 for non-JSON write bodies.
- Studio/Library UI: save button states, "Mở trong Studio" opens the viewed/saved version, "Dùng cho" follows the viewed version, "Tải thêm" buttons, compare waits for test data.

### Removed
- Library "Ghim bản đang xem" button; no-op `PRAGMA foreign_keys` statements inside the import transaction.

## [2.0.1] — 2026-11-16 (`P004-p0-bugfixes`)

### Fixed
- `internal/storage/repo.go`: `UpdateSession` with `collectionId` (Library "Chuyển collection") deadlocked the whole server — it queried `r.db` inside an open transaction while the pool is capped at one connection. Collection resolution now runs on the transaction.
- `cmd/prompt-studio/main.go`, `internal/studio/service.go`: `--mock` no longer overwrites the persisted `provider`/`model` settings; mock mode is process-scoped and exposed as `mockMode` in `GET /api/settings`.
- `internal/studio/service.go`, `web/app.js`: A generation that fails (401/429/network) or is stopped before producing output no longer leaves an empty session in History, and the Studio no longer shows the previous session's versions under the new session. Stop now retries briefly while the server persists the partial version.
- `internal/ai/{provider,openai,anthropic}.go`: The LLM timeout is now an idle timeout (time to first token and gaps between chunks) instead of a total `http.Client.Timeout` that cut healthy long streams at 60 s.
- `internal/ai/{openai,anthropic}.go`, `internal/studio/service.go`, `web/app.js`: Output truncated by max tokens (`finish_reason=length` / `stop_reason=max_tokens`) is detected; a prompt cut before `===META===` is stored as `incomplete`, and `done` carries `truncated` so the UI tells the user to raise the limit.

### Known follow-up
- Data directories where an older build already ran `--mock` still contain `provider=mock`; re-select the real provider once in Settings.

## [2.0.0] — 2026-11-16 (`P003-v2-upgrade`)

### Decisions Locked
- **`D-V2-1`:** Canonicalized `docs/PLAN.md` and `design/` at the repository root and set compiled binary output to `./bin/prompt-studio`.
- **`D-V2-2`:** Adopted full Vietnamese UI matching `design/*.html`, default prompt language `"Theo yêu cầu"` (`match` / `vi` / `en`), 3 specialized targets (`chat`, `api`, `agent`), and 8 Vietnamese starter templates (`starters.vi.json`) using `setSelectionRange` without hijacking `Tab`.
- **`D-V2-3`:** Added automatic backup (`prompt-studio.v1.bak.db`) and v1-to-v2 SQLite auto-migrator (`migrate_v1.go`) unifying History and Library into `sessions` + immutable `prompt_versions` + `collections` + accent-insensitive `sessions_fts (unicode61 remove_diacritics 2)`, moving API keys exclusively into `~/.prompt-studio/secrets.json` (`0600`) and dropping the SQLite `secrets` table.

### Added
- `internal/secrets/file.go`: Dedicated `0600` file store (`secrets.json`) and masked `••••••••••a1b2` hints; API keys never touch SQLite, logs, or HTTP responses.
- `internal/ai/splitter.go` & `templates/{generate.v2.md,refine.v2.md,targets/*.md,starters.vi.json}`: Plain-text Markdown streaming splitter on standalone `===META===` lines (graceful fallback on malformed/missing meta, max 4 editable assumptions), `[[ ]]` template delimiters safe around `{{ten_bien}}`, and `SplitAPIParts` (`===USER===`) for 2-part `system`/`user` API prompts.
- `internal/studio/service.go` & `internal/handlers/compare.go`: Parallel 2-side SSE Compare (`POST /api/versions/{id}/compare`) streaming `raw` vs `prompt` responses with variable substitution, `incomplete = 1` partial version persistence on Stop/timeout, 1-click Save to `"Chưa phân loại"`, normalized whitespace SHA-256 duplicate paste check (`409 Conflict` with `existingSessionId`), and JSON Export/Import (`GET /api/export`, `POST /api/import`).
- `evals/`: Upgraded 20-case meta-prompt evaluation runner (`clear`, `ambiguous`, `ultra_short`, `pasted_content`, `injection`) with automated machine assertions and blind A/B LLM judging (`20/20` pass, `0` injection breaches).

### Changed
- Updated `web/css/01-tokens.css`, `06-pages.css`, `07-themes.css`, `web/index.html`, `web/app.js`, `web/settings.html`, `web/settings.js`, `web/showcase.html`, and `web/showcase.js` to the v2 Warm Graphite / Warm Paper palette (`design/tokens.css`), 4-state AI frame (`.ai-frame` + `.result-card.is-streaming` restricting `--ai-1`/`--ai-2` strictly to active AI states), 5-second Undo toast on unsaved session deletion, and 100% Vietnamese UX with zero inline `style="..."` attributes.

## [0.2.0] — 2026-11-16 (`P002-css-design-system`)

### Added
- `web/css/01-tokens.css` .. `08-utilities.css` & `web/style.css`: Modular zero-framework Vanilla CSS Design System organized via `@layer tokens, reset, base, layout, components, pages, themes, utilities;`.
- `web/showcase.html`, `web/showcase.js`, `web/css/showcase.css`, and route `GET /showcase`: Interactive Component & Token Showcase demonstrating live Dark/Light theme switching, 12-column responsive grid, 5-state AI input frame, and all 10 reusable BEM component families.
- `CSS_DESIGN_SYSTEM.md` & `.agents/rules/ui-design-system.md`: Canonical CSS architecture documentation and mandatory 10-point UI development checklist for AI Agents and developers.

### Changed
- Refactored `web/index.html`, `web/settings.html`, `web/app.js`, and `web/settings.js` to eliminate all 60+ inline `style` attributes and `.style.color` mutations, adopt mobile-first `min-width` breakpoints (`640px`, `768px`, `1024px`, `1280px`), add mobile drawer backdrop + Escape close, and resolve `901px–1180px` 3-column Library cramping.

## [0.1.0] — 2026-11-16 (`P001-prompt-studio-mvp`)

### Decisions Locked
- **`D-1`:** Implemented full Stages 1–8 (Core MVP 1–6 + Stage 7 Prompt Library + Stage 8 Targets & Starter Templates).
- **`D-2`:** Local single-user server (`127.0.0.1:7788`) with session cookie + `Origin`/`Host` CSRF defense, OS Keychain + AES-256-GCM SQLite fallback, and `OpenAI`, `OpenAI-compatible`, and `Anthropic` providers.
- **`D-3`:** English UI, default prompt language `English` (`English` / `Match request` / `Vietnamese`), and 3 detail levels (`concise`, `standard`, `detailed`).
- **`D-4`:** 1 collection per prompt (with protected `"Uncategorized"` system collection), optional LLM `suggest-meta` on paste, 5 prompt targets with 2-part `system`/`user` (`{{input}}`) split for `target=api`, and 8 starter templates with `Tab` placeholder navigation.

### Added
- `cmd/prompt-studio/main.go`: Single-binary entrypoint with `--addr`, `--data-dir`, and `--mock` flags.
- `internal/security/`: `master.key` (`0600`) generation, AES-256-GCM encryption, `KeyHint`, and `SystemKeychain`.
- `internal/storage/`: Pure-Go SQLite schema (`001_init.sql`) with WAL mode, foreign keys, FTS5 tables (`sessions_fts`, `library_fts`), immutable `prompt_versions` tree, and `library_revisions`.
- `internal/ai/`: Versioned meta-prompt templates (`generate.v1.md`, `refine.v1.md`, `library-meta.v1.md`), 5 target guides, 8 starter templates (`starters.en.json`, `starters.vi.json`), incremental `StreamParser`, and OpenAI/Anthropic/Mock providers.
- `internal/studio/` & `internal/handlers/`: Full REST + SSE endpoints with 1x malformed-JSON auto-retry and raw-text fallback.
- `web/`: Embedded vanilla HTML/CSS/JS Studio, Settings, and 3-column Library with 5-state `conic-gradient` AI border and Dark/Light themes.
- `evals/`: 25-case automated meta-prompt evaluation runner (`go run ./evals`).
