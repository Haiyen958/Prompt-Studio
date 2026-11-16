# P007 — Architecture audit: P0 hardening + P1-a settings consolidation

**Status:** DONE (2026-11-16). Scope approved by owner after the architecture audit: P0 + P1-a, baseline commit allowed.
**Workflow:** DEV-KIT `bugfix` (reproduce → regression test → minimal fix → verify) + small `refactor` (P1-a).
**Authoritative Spec:** [docs/PLAN.md](../../PLAN.md) §2 (security, limits).
**Baseline commit:** `0666ce5` (first commit of the repository).

## Decisions

- **`D-P007-1`** (owner, supersedes `D-P005-3`): stable session token stored in `<data-dir>/session.token` (0600); only `?token=` hands out the cookie (`Max-Age` 400 days); every `/api/*` request and every HTML page require the token. Static JS/CSS stay public.

## Tasks

- [x] **H1 — Startup token was decorative.** Any local `GET /` received the token cookie; `GET /api/*` (incl. `/api/export`) needed no token. Fix: `security.LoadOrCreateSessionToken`, middleware auth for all API/page requests, constant-time compare, 401 page. Tests: `TestReadsAndPagesRequireToken`, `TestLoadOrCreateSessionTokenIsStableAndPrivate`.
- [x] **H2 — Stored API key sent to any base URL.** `POST /api/settings/test` used the stored key with a caller-supplied `baseUrl`. Fix: stored key only for the same provider + endpoint. Test: `TestTestConnectionNeverSendsStoredKeyToAnotherEndpoint`.
- [x] **H3 — Backups > 4 MB could not be restored.** Middleware capped every write at 4 MB before the import handler's 32 MB limit. Fix: per-route limit `server.MaxImportBytes`. Test: `TestImportAcceptsBackupLargerThanWriteLimit`.
- [x] **H4 — Manual edit accepted a foreign/unknown parent.** Foreign parent was stored in `parent_id`; unknown parent returned 500. Fix: `studio.Service.CreateManualVersion` (logic moved out of the handler), 400 `version_mismatch`. Test: `TestManualVersionRejectsForeignOrUnknownParent`.
- [x] **P1-a — Settings defaults/validation + module hygiene.** `storage.DefaultSettings()` is the single default source (seed key `language` → `prompt_language`); `studio.ParseSettings` / `studio.Service.UpdateSettings` validate the whole update before writing; temperature `0` now persists (backend and `web/settings.js`); `ai.NewProvider` default max tokens 2048 → 2000; `go mod tidy` removed 7 unused direct modules (gin, gorm, glebarez/sqlite, gorilla/websocket, godotenv, gopsutil, x/net). Test: `TestUpdateSettingsValidatesBeforeWriting`.

## Behavior changes (user-visible)

- Opening `http://127.0.0.1:7788/` without the cookie shows a 401 page asking to open the printed `?token=` URL once. The token survives restarts, so this happens once per browser.
- Test Connection after changing provider/Base URL requires typing the key again (otherwise `unconfigured`).
- `PUT /api/settings` returns 400 `invalid_settings` (+ `field`) for out-of-range values instead of silently storing/ignoring them.

## Verification Evidence

- `gofmt -l internal cmd evals` → none; `go build ./...` → 0; `go vet ./...` → 0; `go test -count=1 ./...` → 0; `go mod tidy -diff` → 0; `node --check web/*.js` → OK.
- Regression proof: the four H1–H4 tests run against baseline `0666ce5` (git worktree) → all FAIL with the original defects (200 instead of 401; stored key sent to `http://127.0.0.1:9/v1`; `http: request body too large`; 500 FK error); with the fix → PASS.
- Live probe (`--mock`, temp data dir, fake key): `GET /` and `GET /api/export` without cookie → 401, no `Set-Cookie`; `?token=` → `Max-Age=34560000; HttpOnly`; Test Connection to a local sink → no `Authorization` header received; 7 MB import → `ok:true`; restart → same token, old cookie still 200; `session.token` perm 600.
- Not verified: visual check in a real browser.

## Follow-up (not in scope, see audit)

P1-b characterization tests + shared generate/refine pipeline; P1-c handlers stop calling `svc.Repo()` + error-code contract in UI (401/429 per PLAN §3); P1-d `PRAGMA user_version` migrations; P2 repo split, dead `internal/security` code, duplicate `prompt-studio/` folder, graceful shutdown; P3 `app.js` modules. Real-provider eval run still pending (results are mock-only).
