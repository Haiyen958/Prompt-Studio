# P006 — UI/UX Audit & Design System Standardization

**Status:** IMPLEMENTED (2026-11-16). `go test ./...` reported passing by the P005 session after both changes were merged; UI E2E re-run on the merged code: 30/30 PASS.
**Workflow:** DEV-KIT `refactor` (behavior-preserving UI) + `bugfix` (P0 items). Gates G0–G5 done; G6 = this file + docs sync.
**Authoritative Spec:** [docs/PLAN.md](../../PLAN.md) §3–4, [CSS_DESIGN_SYSTEM.md](../../../CSS_DESIGN_SYSTEM.md).
**Parallel work:** `P005` (another session) edits `web/app.js` / `web/index.html` at the same time. Conventions were shared with that session.

---

## Decisions (owner said "thực hiện" to the audit's recommended options)

- **`D-P006-1` (radius):** Flat 0–4px. Controls and cards use 4px, badges 2px, modals 6px. Pills stay only for assumption/starter chips, status dots and spinners. This overrides `design/tokens.css` (`14px` / `10px`), which has been updated.
- **`D-P006-2` (AI effect):** Keep the conic moving border while AI is busy, because it signals state. Remove the `ai-glow` halo and the 28px blur focus glow; focus is now a 1px ring. `docs/PLAN.md` §4 updated.
- **`D-P006-3` (mobile):** Mobile stays out of MVP scope. The requirement is only "must not break" (no horizontal overflow at 390px; touch actions visible).

## Tasks

- [x] **P0-1:** `.text-xs/sm/md/lg/xl` and `.font-medium/semibold` were used 33× but never defined. Added to `08-utilities.css`.
- [x] **P0-2:** History rows, collections, library cards and version rows changed from `<div>` + click to `<button>`, so they are keyboard-operable.
- [x] **P0-3:** Library actions (move, rename, pin, unsave, delete, create/rename/delete collection) now have `try/catch` and show an error toast instead of failing silently.
- [x] **P0-4:** The Generate button gets `aria-disabled` and Ctrl+Enter is blocked while AI is unconfigured (PLAN §3). Typed text is kept.
- [x] **P0-5 (found during verification):** `html { font-size: 0.875rem }` made `1rem = 14px`, which shrank the entire token scale to 87.5% (body 12.25px, `text-xs` 10.5px). The root is now `100%`.
- [x] **Tokens:** radius, `--control-sm/md/lg/touch`, `--color-on-danger` (the old danger button had white on `#ff8a80` ≈ 2.3:1), removed `--text-2xl`, card shadow → border only, light tokens de-duplicated (the `system` block was dead code because `applyThemePreference` resolves it).
- [x] **Buttons:** 9 variants reduced to 5 (`primary`, `secondary`, `ghost`, `danger`, `danger-outline`). The saved state uses `aria-pressed`. Unicode/emoji icons replaced with SVG.
- [x] **Studio:** the result header keeps So sánh / Lưu / **Copy (only primary)** plus a ⋯ menu (Tạo lại, Sửa tay, Tải .md, Copy JSON). The hero is hidden once a result exists. The error banner moved under the request box. The refine send button is disabled while the box is empty. Copy is announced via a live region.
- [x] **Library:** a ⋯ menu separates "Bỏ khỏi thư viện (vẫn còn trong Lịch sử)" (now undoable via toast) from "Xóa vĩnh viễn" (themed confirm; focus defaults to Hủy). The collection select has a visible label. Selection no longer changes border width.
- [x] **Dialogs:** new `web/ui.js` (`UI.toast/confirm/prompt/announce/bindMenu`). All `window.alert/confirm/prompt` calls removed (app + settings).
- [x] **Compare:** the explainer paragraph is collapsed into `<details>`. "Copy prompt" is the only primary action.

## Verification Evidence

- `go build` / `go test ./...` → **not run: Go toolchain not installed on this machine.** The Go change is limited to adding `ui.js` to `web/embed.go`. Run both before release.
- Browser E2E (Playwright + Chromium headless) against the existing `bin/prompt-studio --mock` API, with the working-tree `web/` served through a local proxy: **30/30 checks PASS**. Checks covered: utilities resolve to 12px; 4px radius; one primary in the result card; the result header fits on one row at 780px; menu ↑↓/Esc with focus return; copy announced; refine button disabled when empty; aria-pressed save; history opens with Enter; list items are buttons; Esc cancels the dialog without stopping streams; rename via dialog; API 500 → error toast with the server message; unsave + Undo restores the prompt; delete-confirm defaults to Hủy; unconfigured → no request and input kept; 390px has no horizontal overflow; no JS console errors.
- Static: 0 undefined classes, 0 `window.alert/confirm/prompt`, 0 hex outside tokens/themes, 0 non-token `border-radius`, 0 inline styles.

## Known trade-offs / follow-ups

- The root rem fix makes the whole UI ~14% larger than the previously *rendered* UI. This is the size the tokens always documented, and the most visible change.
- Tạo lại / Sửa tay / Tải .md now take one extra click (in the ⋯ menu).
- Undo of "Bỏ khỏi thư viện" re-saves the session, so its `saved_at` becomes the undo time (affects the "Mới lưu" sort).
- The paste modal still lacks a focus trap (the dialog in `ui.js` has one).
- Re-run the E2E checks after `P005` lands, because both touch `app.js` / `index.html`.
