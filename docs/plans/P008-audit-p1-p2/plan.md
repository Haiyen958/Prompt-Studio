# P008 — Architecture audit follow-up: P1-b … P2 (+ small P3)

**Status:** DONE (2026-11-16). Owner approved the remaining roadmap ("DUYỆT") after P007.
**Workflow:** DEV-KIT `refactor` (characterization first) + `migration` (P1-d) + small `bugfix`es.
**Architecture reference:** [docs/ARCHITECTURE.md](../../ARCHITECTURE.md) (new).

## Tasks and commits

- [x] **P1-b — Shared generate/refine pipeline** (`c32af0f`). 18 characterization cases
  (9 scenarios × generate/refine) pin SSE order, `done` shapes, return values and stored
  versions; `runPromptStream` + `donePayload` replace ~170 duplicated lines. Mutation check:
  two deliberate payload/assumption changes → 6 and 3 failing cases.
- [x] **P1-c — Handler/service boundary + error contract** (`226c48c`). Handlers call only
  `studio.Service` (guard test); `handlers/errors.go`; `internal_error` instead of raw SQL
  text; SSE errors with Vietnamese messages; Studio banner "Mở Cài đặt" / "Thử lại"
  (PLAN §3); failed refine keeps the typed instruction; duplicate collection name → 400.
- [x] **P1-d — Versioned schema migrations** (`6654f81`). `PRAGMA user_version`, ordered
  idempotent steps in transactions, `VACUUM INTO` backup of existing databases, refusal of
  newer schemas, fatal FTS5 failure.
- [x] **P2 — Maintainability** (`faa57ef`, `95a4e7c`). `repo.go` split by aggregate (pure
  move, line multiset identical); dead `SystemKeychain`/duplicate `KeyHint` removed; v1
  migrator no longer creates `master.key`; graceful shutdown (`server.Run`); Stop reload
  backoff ~3 s; stale `prompt-studio/` folder removed.
- [x] **P3 (small)** (`708418a`). Real OpenAI token usage (`stream_options.include_usage`,
  official API only); `scripts/check.sh`.

## Verification Evidence

- `scripts/check.sh` → "All checks passed." (gofmt, `go mod tidy -diff`, build, vet,
  `go test -count=1 ./...`, `node --check web/*.js`).
- P1-d on a read-only copy (SQLite backup API) of the real `~/.prompt-studio/prompt-studio.db`
  (user_version 0, no `target` column): after migration user_version 2, column added,
  session/version content hashes identical, accent-insensitive search OK; backup equals the
  pre-migration database; the baseline binary (`0666ce5`) still opens the migrated copy (rollback OK).
  The real database itself was not modified.
- Graceful shutdown: `TestRunCancelsRequestsAndWaitsForHandlers`; live SIGINT on the binary →
  "Prompt Studio stopped cleanly".
- Not verified: the Studio error-banner buttons and Stop backoff in a real browser.

## Deferred (needs owner input or a browser)

- **P3 `web/app.js` module split** (2,000+ lines): high regression risk with no browser or
  frontend tests in this environment. Recommended prerequisite: a manual smoke checklist or
  browser automation.
- Default Anthropic model `claude-3-5-sonnet-latest` (and the Settings datalist) is outdated:
  product decision on the new default.
- Real-provider eval run (results are still mock-only).
- `docs/plans/README.md` / `scripts/plan.mjs` referenced by AGENTS.md do not exist.
