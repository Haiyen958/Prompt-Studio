# P005 — Save pins version, per-version target, pagination, write hardening

**Status:** DONE (2026-11-16). Owner chose the recommended option for all four questions (`D-P005-1`..`D-P005-4`, see [CHANGELOG](../../CHANGELOG.md)).
**Workflow:** DEV-KIT `feature` + `bugfix` (behavior changes approved by owner).
**Authoritative Spec:** [docs/PLAN.md](../../PLAN.md) §2, §3, §7 (API), §8 (data).

## Tasks

- [x] **T1 — Save = pin viewed version.** `SaveSessionToLibrary(id, col, title, versionID)`, `versionBelongsTx`, unsave clears the pin, PATCH pin validated. UI: save button states, save bar shows `bản vN`, open-in-Studio uses the viewed/saved version, Library pin button removed.
- [x] **T2 — Per-version target.** Column `prompt_versions.target` (schema + `ensureColumn` additive migration), shared `scanVersion`, generate/refine/manual/paste/import write it, Compare and the API split view read it, the dropdown follows the viewed version.
- [x] **T3 — Pagination.** `SessionFilter.Limit/Offset`, stable order (`…, s.id DESC`), `hasMore`/`nextOffset`; "Tải thêm" in History and Library.
- [x] **T4 — Write hardening.** Exact Origin match; 415 for non-JSON write bodies.
- [x] **T5 — Compare guards.** `CheckComparable` (pasted / incomplete / missing variables), `side_error` per side, UI waits for test data.

## Coordination

The UI-audit session (`prompt-85`) edited `web/app.js`/`web/index.html` concurrently. P005 frontend edits were applied surgically with anchor checks and keep its conventions (`UI.toast`/`UI.prompt`, `btn--secondary btn--sm` for "Tải thêm", `aria-pressed` for the saved state).

## Verification Evidence

- `gofmt -l internal cmd` → none; `go vet ./...` → EXIT 0; `go test -count=1 ./...` → EXIT 0.
- New tests: `TestSavePinsViewedVersion`, `TestSaveOrPinRejectsForeignVersion`, `TestTargetIsStoredPerVersion`, `TestCompareRejectsMissingVariablesAndIncomplete`, `TestCompareReportsErrorPerSide`, `TestListSessionsPagination`, `TestWritesRequireExactOriginAndJSON`, `TestOpenDBAddsVersionTargetColumn`.
- `node --check web/app.js` → OK.
- Smoke (`./bin/prompt-studio --mock`, temp data dir): page1 `2 / hasMore=true / nextOffset=2`; save with `versionId` pins v1; compare without variables → 400; `text/plain` write → 415; Origin `localhost:3000` → 403; served `app.js` contains the new code.
- Not verified: visual check in a real browser (no browser tool in this session).
