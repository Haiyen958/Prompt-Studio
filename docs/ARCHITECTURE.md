# Prompt Studio — Architecture & rules for contributors (humans and AI agents)

Canonical description of how the code is organized. Product behavior lives in
[PLAN.md](PLAN.md); decisions and history in [CHANGELOG.md](CHANGELOG.md) and
[plans/](plans/). Established by the architecture audit (P007–P008, 2026-11-16).

## Layers

```
Browser (web/*.js, vanilla, no build step)
   │  fetch JSON  /  fetch + ReadableStream (SSE)
   ▼
server.Middleware   Host/Origin, session token (cookie or X-Prompt-Studio-Token),
                    JSON-only writes, body limits, panic recovery, request log
   ▼
handlers/*          decode → studio.Service → writeServiceError / sseErrorPayload
   ▼
studio.Service      every business rule (validation, version tree, pins, settings,
                    generate/refine pipeline)
   ├──► ai          providers (OpenAI, OpenAI-compatible, Anthropic, Mock),
   │                ===META=== splitter, meta-prompt templates
   └──► storage     SQL + transactions only; schema migrations; FTS5 index
          └──► secrets   API key in secrets.json (0600), never in SQLite
```

### Dependency rules

| Package | May import (internal) |
|---|---|
| `cmd/prompt-studio` | everything |
| `handlers` | `studio`, `server`, `ai` (error codes/variables), `storage` (types and sentinel errors only) |
| `studio` | `ai`, `storage` |
| `storage` | `secrets`, `security` (legacy v1 decrypt only) |
| `ai`, `server`, `secrets`, `security` | nothing internal (leaves) |

`TestHandlersDoNotCallRepositoryDirectly` fails if a handler calls `svc.Repo()`
(`Repo()` exists for tests only).

## Where new code goes

| Change | Put it in |
|---|---|
| New endpoint | route in `handlers/router.go`, handler file per API group, a `studio.Service` method, errors through `writeServiceError` |
| New business rule / validation | `studio` (input errors wrap `studio.ErrInvalidInput` → 400 automatically) |
| New query or table access | the matching `storage/<aggregate>.go`; inside a transaction use `tx`, **never** `r.db` (the pool has one connection — it deadlocks) |
| Schema change | append a step to `schemaMigrations` in `storage/migrate.go` (idempotent; never edit/reorder old steps) + a test opening a DB at the previous version |
| New setting | default in `storage.DefaultSettings`, parse in `studio.ParseSettings`, validation in `studio.Service.UpdateSettings`, field in `SettingsResponse` |
| New LLM call that yields a version | reuse `studio.runPromptStream` / `donePayload` |
| UI | follow `CSS_DESIGN_SYSTEM.md` and `.agents/rules/ui-design-system.md`; branch on error `code`, never on message text |

## Contracts

- **Errors:** every JSON error is `{"code","error"}`; SSE `error` is `{"code","message"}` with a
  Vietnamese message. Codes used by the UI: `unconfigured`, `auth_error` (→ "Mở Cài đặt"),
  `rate_limit`, `timeout`, `bad_response`, `network`, `internal_error` (→ "Thử lại").
  Unexpected errors are logged and returned as a generic `internal_error`.
- **SSE `done`:** completed versions carry `intent`, `assumptions`, `truncated`; partial
  (stopped / failed mid-stream) versions do not. Pinned by
  `studio/stream_characterization_test.go`.
- **Security:** token in `<data-dir>/session.token`; only `?token=` hands out the cookie;
  all `/api/*` and pages require it; the stored API key is only sent to the saved
  provider + Base URL.
- **Data:** `prompt_versions` rows are never updated; schema version in `PRAGMA user_version`;
  migrations back up existing databases first (`prompt-studio.pre-schema-vN.bak.db`).

## Working rules for AI agents

1. Run `scripts/check.sh` before and after every change; report the commands and exit codes.
2. One session per area at a time. Check other sessions (and file mtimes) before editing;
   commit each finished step so it can be reverted alone.
3. Do not change behavior while refactoring: add characterization tests first.
4. Every bug fix gets a regression test that fails on the old code.
5. Constants and business defaults have one home (see the table above).
6. Eval results must state the provider; mock runs are not a quality gate.
7. `docs/` and `design/` at the repository root are canonical; there is no other copy.
