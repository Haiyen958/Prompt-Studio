# P001 — Prompt Studio MVP & Post-MVP Core (Stages 1–8)

**Status:** DONE (2026-11-16). Owner decisions `D-1`..`D-4` recorded in §D. Tasks `T1`–`T8` implemented and verified (§H).  
**Workflow:** Initial implementation + DEV-KIT `feature` governance (Gates G0–G6).  
**Source Specification:** [Prompt Studio — Kế hoạch sản phẩm & kỹ thuật (MVP).md](../../../Prompt%20Studio%20%E2%80%94%20K%E1%BA%BF%20ho%E1%BA%A1ch%20s%E1%BA%A3n%20ph%E1%BA%A9m%20&%20k%E1%BB%B9%20thu%E1%BA%ADt%20(MVP).md)  
**Plan system:** This repository does not yet have `scripts/plan.mjs`. Following the workspace's DEV-KIT convention, this `plan.md` is the canonical task and readiness record.

---

## A. Change overview and scope

Build **Prompt Studio**, a local single-binary Go web application (`127.0.0.1:7788`) with embedded vanilla HTML/CSS/JS and pure-Go SQLite (`modernc.org/sqlite` with FTS5) that transforms natural-language requests into structured prompts on a single workspace screen: input → optional inline clarifying questions → streaming structured prompt → iterative refinement & version history → prompt library.

**In scope (Stages 1–8 per `D-1`)**

- **T1 (Stage 1) — Foundation, Security & Settings:**
  - Go module (`prompt-studio`), SQLite WAL + `foreign_keys=ON` + FTS5 schema migrations (`001_init.sql`), startup session token cookie (`ps_token`) issued on initial browser page load, `Origin`/`Host` CSRF validation on write requests, request body size cap, and structured safe logging (never logging API keys, `Authorization` headers, or system prompts).
  - Secret management (`internal/security/`): OS keychain primary (via `secret-tool` / OS credential helper when available) with automatic fallback to AES-256-GCM encrypted `secrets` table using `~/.prompt-studio/master.key` (`0600`).
  - Provider abstraction (`internal/ai/`): `OpenAI`, `OpenAI-compatible` (custom `baseUrl`), and `Anthropic` (`D-2`), including `Stream()` and `Ping()` + normalized errors (`ErrAuth`, `ErrRateLimit`, `ErrTimeout`, `ErrBadResponse`).
  - Settings API (`GET/PUT /api/settings`, `POST /api/settings/test`, `DELETE /api/settings/key`) + Settings UI (provider, base URL for OpenAI-compatible, model ID, write-only API key with `••••a1b2` hint, Replace/Delete key workflow requiring Test Connection before overwriting key, theme selector Dark/Light/System, advanced temperature/maxTokens/timeout).
- **T2 (Stage 2) — Meta-Prompt Templates & Evals Runner:**
  - Embedded versioned templates (`internal/ai/templates/`): `generate.v1.md`, `refine.v1.md`, `library-meta.v1.md`, and target guides `targets/{chat,api,agent,small,reasoning}.md`.
  - Prompt injection defense wrapping user input in `<user_request>...</user_request>` and clarifications in `<clarifications>...</clarifications>` with explicit system boundary instructions.
  - Evals suite (`evals/cases.yaml` with 25+ cases covering clear requests, ambiguous requests, ultra-short requests, prompt-injection attempts, and all 5 targets) + `go run ./evals` CLI supporting both deterministic offline verification (`--mock`) and live configured provider execution, writing reports to `evals/results/`.
- **T3 (Stage 3) — Streaming Generate, JSON Stream Parser & 5-State AI Input Border:**
  - Single-call analyze-or-generate flow (`POST /api/sessions`) with SSE events (`questions`, `delta`, `done`, `error`), incremental JSON stream parser that streams `prompt` deltas in real time when `"type":"prompt"`, automatic 1x retry on malformed JSON with raw-text fallback, and context cancellation when the user clicks **Stop**.
  - Dark-first Studio UI + Light Indigo theme (`data-theme` + `color-scheme`), CSS `conic-gradient` 5-state AI input frame (`idle`, `typing`, `analyzing`, `streaming`, `done`) with `prefers-reduced-motion` and `role="status"` accessibility.
- **T4 (Stage 4) — Inline Clarifying Questions Card & Assumptions:**
  - Inline clarifying card (`1–3` questions, single-select or multi-select chips + free-text `"Other"` input, pre-highlighted `default` chip, **Generate Prompt** and **Skip & Generate** buttons).
  - `POST /api/sessions/{id}/answers` forcing prompt output on the second turn without asking another round of questions; collapsible **Assumptions used** bar when defaults/assumptions were applied.
- **T5 (Stage 5) — Immutable Version Tree, Refinement & Manual Edits:**
  - Immutable `prompt_versions` (`seq`, `parent_id`, `source`: `generate | regenerate | improve | manual`).
  - Refinement bar with quick suggestion chips (*Shorter*, *More detailed*, *Focus on UX*, *Add examples*, *JSON output format*): empty instruction + Enter triggers `regenerate`; non-empty instruction triggers `improve` sending only `{original request, clarification answers, active version content, new instruction}` (`POST /api/sessions/{id}/refine`).
  - Manual edit mode (`POST /api/sessions/{id}/versions`), version switcher `‹ v3/5 ›`, Copy button, and `.md` download.
- **T6 (Stage 6) — Session History Sidebar, FTS5 Search & Error/Edge States:**
  - Left sidebar (desktop) / drawer (mobile) grouped by *Today*, *Previous 7 Days*, *Older*; auto-generated 60-char title from first request; rename (`PATCH /api/sessions/{id}`), delete (`DELETE /api/sessions/{id}`), and SQLite FTS5 search (`GET /api/sessions?q=`).
  - Full error/special states: unconfigured banner (locks Generate), `401` invalid key, `429` rate limit with Retry preserving input, timeout/network drop keeping partial stream, input length counter/validation (max 8,000 chars for Studio request, 32,000 chars for Library paste).
- **T7 (Stage 7) — Prompt Library & Revisions:**
  - Tables `collections` (with non-deletable system collection `"Uncategorized"`), `library_prompts` (with SHA-256 `content_hash` duplicate detection), `library_revisions`, and `library_fts`.
  - Endpoints: `/api/collections`, `/api/library`, `/api/library/{id}`, `/api/library/{id}/revisions`, `/api/library/{id}/use`, `/api/library/{id}/open-in-studio`, and `/api/library/suggest-meta`.
  - 3-column Library UI (collections, prompt cards with search/favorite/target filter/sort by recent use, detail & revision inspector) + Save from Studio popover (update existing or save as new if version already saved) + Paste Prompt modal + Open in Studio round-trip revision workflow.
- **T8 (Stage 8) — Prompt Targets (`D-4`) & Starter Templates:**
  - `"Use for"` target dropdown (`chat`, `api`, `agent`, `small`, `reasoning`, default `chat`) + post-result `"Convert for"` quick chips creating a new version.
  - Multi-part output (`parts: [{role: "system"|"user", content}]`) for `target=api` rendered as two distinct blocks (`System Prompt` and `User Template` with `{{input}}`), each with its own Copy button.
  - 8 starter templates embedded in `internal/ai/templates/starters.en.json` (and `starters.vi.json`), served via `GET /api/starters` and `POST /api/starters/{id}/use` (usage counter stored in `settings`), with `[placeholder]` Tab navigation in the Studio input box.

**Out of scope**

- Hosted multi-user OAuth/rate-limiting deployment.
- Gemini native adapter (deferred per `D-2`; users can still use Gemini via OpenAI-compatible endpoint if desired).
- Visual branching tree graph UI / side-by-side version diff viewer.
- Dynamic `{{variable}}` fill-in modal on Library copy.

---

## B. Codebase findings (evidence)

| # | Observation | Evidence |
|---|---|---|
| E1 | Workspace currently contains only the product & technical specification and DEV-KIT customizations; no Go module or git repo initialized yet. | `/home/accnet/prompt/Prompt Studio — Kế hoạch sản phẩm & kỹ thuật (MVP).md:1-628` |
| E2 | Go `go1.24.0 linux/amd64` is installed at `/home/accnet/go_dist/go/bin/go` and `/home/accnet/.local/go/bin/go`. | `/home/accnet/go_dist/go/bin/go` |
| E3 | Product specification had open checkboxes in lines 597–618 regarding deployment mode, providers, UI/prompt default languages, library cardinality, and API target splitting; all have now been resolved by owner decisions `D-1`..`D-4`. | `Prompt Studio — Kế hoạch sản phẩm & kỹ thuật (MVP).md:595-618` |

---

## C. Impact matrix

| Surface | Impact | Proposed delta | Risk | Regression check |
|---|---|---|---|---|
| Domain & Meta-prompts (`internal/ai/templates/`, `internal/studio/`) | additive | Versioned templates (`generate.v1`, `refine.v1`, `library-meta.v1`, 5 target guides, 8 starters), single-call JSON schema + streaming parser | Malformed LLM JSON or prompt injection | 1x auto-retry + raw fallback; `<user_request>` boundary tags; unit + eval tests |
| Database (`internal/storage/`) | additive | SQLite schema (`sessions`, `turns`, `prompt_versions`, `settings`, `secrets`, `collections`, `library_prompts`, `library_revisions` + FTS5 tables/triggers) | FTS5 sync on update/delete | SQLite repository unit tests covering CRUD, FTS5 search, collection deletion reassignment |
| HTTP & SSE API (`internal/server/`, `internal/handlers/`) | additive | REST + SSE endpoints under `/api/*` with cookie token auth + `Origin`/`Host` CSRF check | Premature stream close or CSRF bypass | Middleware & E2E handler tests |
| Security (`internal/security/`) | additive | Keychain + AES-256-GCM fallback (`~/.prompt-studio/master.key`, `0600`), write-only key handling | Accidental key exposure in responses or logs | Unit tests verifying `GET /api/settings` never returns key and encrypt/decrypt round-trip |
| Frontend (`web/`) | additive | Embedded single-screen Studio + Library + Settings vanilla HTML/CSS/JS with 5-state AI border & keyboard navigation | Mobile layout or reduced-motion accessibility | DOM/JS structural checks & E2E integration verification |

---

## D. Decisions

| ID | Question | Owner Decision (2026-11-16) | Consequence |
|---|---|---|---|
| `D-1` | Implementation scope for P001 (Stages 1–6 vs Stages 1–8)? | **All Stages 1–8** (Core MVP 1–6 + Stage 7 Prompt Library + Stage 8 Targets & Starter Templates). | Schema `001_init.sql`, backend services, and `web/` UI include Studio, Library, 5 Targets, and 8 Starter Templates from the start. |
| `D-2` | Runtime mode and supported AI providers in MVP? | **Local single-user (`127.0.0.1:7788`)** + **OpenAI, OpenAI-compatible, and Anthropic** providers (Gemini deferred). | Server binds strictly to `127.0.0.1:7788` (configurable via flag/env for tests), uses startup session cookie + `Origin`/`Host` validation, and implements OpenAI/OpenAI-compatible + Anthropic streaming adapters. |
| `D-3` | UI language, default prompt language, and detail levels? | **English UI**, default prompt language **`English`** (options: `English`, `Match request`, `Vietnamese`), and **3 detail levels** (`concise` <150w, `standard` 150–400w, `detailed` 400–900w). | All UI labels, system collection (`"Uncategorized"`), starter templates, and clarifying questions/assumptions default to English. |
| `D-4` | Library collection model, `suggest-meta`, and `target=api` output format? | **1 collection per prompt**, include optional **`POST /api/library/suggest-meta`**, and split **`target=api` into 2 blocks (`system` and `user` with `{{input}}`)** with separate Copy buttons. | `prompt_versions` and `library_prompts` store `target` and optional `parts` JSON; UI renders separate copyable cards when `parts` has `system` + `user`. |

---

## E. Contract delta

| Type | Contract | Details | Verification |
|---|---|---|---|
| SQLite Schema | `internal/storage/migrations/001_init.sql` | Tables: `sessions`, `turns`, `prompt_versions` (with `target`, `parts`), `settings`, `secrets`, `collections`, `library_prompts` (with `target`, `parts`), `library_revisions`, `sessions_fts`, `library_fts`. | `internal/storage/repo_test.go` |
| HTTP / SSE API | `/api/sessions*`, `/api/settings*`, `/api/collections*`, `/api/library*`, `/api/starters*` | Matches specification tables in §Kiến trúc Go, §Thư viện prompt, and §Nơi dùng prompt & mẫu khởi đầu. SSE emits `questions`, `delta`, `done`, `error`. | `internal/handlers/e2e_test.go` |
| Meta-Prompt JSON | `generate.v1.md`, `refine.v1.md`, `library-meta.v1.md` | Output schema `{type, intent, questions, prompt, parts, assumptions}` with streaming extraction of `prompt` field. | `internal/ai/stream_parser_test.go`, `evals/` |

---

## F. Tasks and Definition of Done (DoD)

### T1 — Foundation, SQLite Storage, Security & Settings (Stage 1)
- **Files:** `go.mod`, `cmd/prompt-studio/main.go`, `internal/security/{secrets.go,keychain.go,secrets_test.go}`, `internal/storage/{sqlite.go,migrations/001_init.sql,repo.go,repo_test.go}`, `internal/ai/{provider.go,openai.go,anthropic.go}`, `internal/server/{router.go,middleware.go,sse.go}`, `internal/handlers/settings.go`.
- **DoD:** Server binds `127.0.0.1:7788`; master key created with `0600`; API key encrypted via keychain or AES-256-GCM; `GET /api/settings` never returns raw key (`hasKey: true`, `keyHint: "••••a1b2"`); key replacement requires passing Test Connection; CSRF `Origin`/`Host` middleware blocks cross-origin writes; unit tests pass.

### T2 — Meta-Prompts, Target Guides & Evals Suite (Stage 2)
- **Files:** `internal/ai/templates.go`, `internal/ai/templates/{generate.v1.md,refine.v1.md,library-meta.v1.md,starters.en.json,starters.vi.json}`, `internal/ai/templates/targets/{chat,api,agent,small,reasoning}.md`, `evals/{main.go,cases.yaml}`.
- **DoD:** Templates render cleanly with `<user_request>` injection boundary; `go run ./evals --mock` (and live provider mode when configured) validates ≥ 90% accuracy across 25 cases with 0 prompt-injection breaches and writes JSON results to `evals/results/`.

### T3 — Streaming Generate, Incremental JSON Parser & 5-State AI UI (Stage 3)
- **Files:** `internal/ai/{stream_parser.go,stream_parser_test.go}`, `internal/studio/service.go`, `internal/handlers/generate.go`, `web/{index.html,style.css,api.js,app.js}`.
- **DoD:** Incremental JSON parser streams `prompt` field deltas to SSE before JSON closes; malformed JSON retries once then falls back to raw text; clicking Stop cancels upstream context; UI displays 5-state AI border (`conic-gradient`) and Dark/Light themes.

### T4 — Inline Clarifying Questions & Assumptions (Stage 4)
- **Files:** `internal/studio/service.go`, `internal/handlers/generate.go`, `web/app.js`, `web/style.css`.
- **DoD:** When `type=="questions"`, inline card renders 1–3 chip questions (single/multi + `"Other"` free text + pre-selected `default`); **Generate Prompt** or **Skip & Generate** calls `POST /api/sessions/{id}/answers`, forces prompt generation on the second call, and displays collapsible **Assumptions used**.

### T5 — Version History, Refine Bar & Manual Edit (Stage 5)
- **Files:** `internal/handlers/{generate.go,versions.go}`, `web/app.js`.
- **DoD:** Every generate/regenerate/improve/manual save creates a new immutable `prompt_versions` row (`seq` increments, `parent_id` tracks the version being refined); version switcher `‹ vM/N ›`, Copy, manual edit, and `.md` download work end-to-end.

### T6 — Session Sidebar, FTS5 Search & Error States (Stage 6)
- **Files:** `internal/handlers/sessions.go`, `web/{index.html,settings.html,settings.js,app.js,style.css}`.
- **DoD:** Sidebar groups sessions into *Today / Previous 7 Days / Older*, supports FTS5 search, rename, and delete; unconfigured banner, 401/429/timeout states, character counter, and `Ctrl/Cmd+Enter` shortcut work as specified.

### T7 — Prompt Library, Collections & Revisions (Stage 7)
- **Files:** `internal/handlers/library.go`, `internal/studio/service.go`, `web/{index.html,app.js,style.css}`.
- **DoD:** System collection `"Uncategorized"` cannot be deleted; deleting a user collection moves its prompts to `"Uncategorized"` in one transaction; saving from Studio, pasting with SHA-256 duplicate check (`409`), optional `suggest-meta`, copying (increments `use_count`), and **Open in Studio** → refine → save back as new `library_revisions` entry all pass tests.

### T8 — Prompt Targets (`chat/api/agent/small/reasoning`) & Starter Templates (Stage 8)
- **Files:** `internal/handlers/starters.go`, `internal/studio/service.go`, `web/{index.html,app.js,style.css}`, `internal/handlers/e2e_test.go`.
- **DoD:** Target selector and `"Convert for"` chips generate target-tailored prompts; `target=api` renders separate copyable `System Prompt` and `User Template` (`{{input}}`) blocks; 8 starter chips appear when input is empty, populate `[...]` placeholders on click, support `Tab` jumping across placeholders, and sort by usage count.

---

## G. Risks, rollback and regression tests

| Risk | Mitigation | Verification |
|---|---|---|
| Pure-Go SQLite FTS5 availability | Build and test with `modernc.org/sqlite` FTS5 support or standard SQLite FTS5 virtual table + synchronized triggers | `go test ./internal/storage/...` |
| LLM streaming JSON contains escaped quotes/newlines inside `"prompt"` | Purpose-built state-machine string extractor in `internal/ai/stream_parser.go` handling `\n`, `\"`, `\\`, `\uXXXX` across arbitrary chunk boundaries | `go test ./internal/ai/...` |
| Prompt injection overriding meta-prompt output format | Wrap user text in `<user_request>` tags + explicit system instruction + dedicated adversarial cases in `evals/cases.yaml` | `go run ./evals --mock` |
| API key leakage in responses or logs | `SettingsResponse` struct omits raw key by design; `secrets` table stores only AES-256-GCM ciphertext + 4-char hint | `go test ./internal/security/... ./internal/handlers/...` |

**Rollback:** All changes are self-contained in the newly initialized repository; SQLite database and master key in `~/.prompt-studio/` (or configurable `--data-dir`) can be reset or removed cleanly without system-wide side effects.

---

## H. Readiness & Execution Verdict

**Verdict:** `DONE` (all DoD criteria satisfied)

- Owner decisions `D-1`..`D-4` are explicitly approved and recorded.
- All tasks `T1`–`T8` (Stages 1–8) are implemented and verified with automated tests:
  - `go test -v -count=1 ./...` → **PASS (exit code 0)** across `internal/security`, `internal/storage`, `internal/ai`, and `internal/handlers` (E2E SSE clarify/answer/refine/versions, Library round-trip, API 2-part target, starters ordering, CSRF & secret protection).
  - `go run ./evals` → **PASS (exit code 0, 25/25 cases = 100.0%, 0 prompt-injection breaches)**.
  - `go build -o prompt-studio ./cmd/prompt-studio` → **PASS (exit code 0)** producing self-contained single binary `prompt-studio`.
