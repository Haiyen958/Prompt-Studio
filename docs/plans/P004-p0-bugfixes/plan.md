# P004 — P0 Bugfixes from the v2 Critical Review

**Status:** DONE (2026-11-16). Requested by owner after the v2 critical review.
**Workflow:** DEV-KIT `bugfix` (reproduce → regression test → minimal fix → verify).
**Authoritative Spec:** [docs/PLAN.md](../../PLAN.md) §2 (limits), §3 (special states).

---

## Tasks

- [x] **B1 — Deadlock on collection change.** `UpdateSession` called `resolveCollectionID` (uses `r.db`) inside a tx with `SetMaxOpenConns(1)`. Fix: `resolveCollectionIDWith(tx, …)`. Test: `TestUpdateSessionCollectionDoesNotDeadlock` (reproduced: hung > 3 s before the fix).
- [x] **B2 — `--mock` overwrote persisted provider settings.** Fix: `Service.SetMockMode` (process-scoped), `mockMode` in `GET /api/settings`, sidebar label. Test: `TestMockModeDoesNotPersistProvider`.
- [x] **B3 — Empty sessions + stale UI after failed/stopped generate.** Fix: delete the session when no version is saved; Studio resets session/versions at generate start, clears them when no version arrives, and retries reload after Stop. Tests: `TestGenerateErrorBeforeOutputLeavesNoEmptySession`, `TestStopBeforeOutputLeavesNoEmptySession`.
- [x] **B4 — Total timeout cut healthy streams; truncation undetected.** Fix: `streamWatchdog` idle timeout, `Usage.Truncated` from `finish_reason=length` / `stop_reason=max_tokens`, prompt cut before `===META===` saved `incomplete`, `truncated` in `done`, UI notice. Tests: `TestOpenAIStreamTimeoutIsIdleNotTotal`, `TestOpenAIStreamStallReturnsErrTimeout`, `TestOpenAIStreamUserCancelIsNotTimeout`, `TestOpenAIStreamDetectsLengthTruncation`, `TestAnthropicStreamDetectsMaxTokens`, `TestTruncatedPromptIsMarkedIncomplete`, `TestTruncatedInsideMetaKeepsPromptComplete`.

## Out of scope (pending owner decisions — see review)

- P1: "Save" should pin the viewed version; open-in-Studio should open the pinned version; per-version `target`; Compare error `side`; history pagination/retention.
- P2: exact-Origin check + JSON content-type enforcement; newer OpenAI params (`max_completion_tokens`).
- Real-model eval run (current `evals/results` are mock-only).

## Verification Evidence

- `go vet ./...` → EXIT 0; `gofmt -l internal cmd` → no files.
- `go test -count=1 ./...` → EXIT 0 (`ai`, `handlers`, `secrets`, `security`, `storage` PASS).
- Mutation check: with each fix disabled, all 8 new handler/ai regression tests FAIL; restored → PASS.
- `node --check web/app.js` → OK.
- Smoke: `./bin/prompt-studio --mock` on a temp data dir — PUT settings (anthropic) → `mockMode:true`, generate streams 83 deltas, PATCH `collectionId` returns immediately, DB keeps `provider=anthropic`.
