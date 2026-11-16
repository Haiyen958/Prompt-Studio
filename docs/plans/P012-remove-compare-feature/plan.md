# P012 — Remove "So sánh với yêu cầu gốc" (Side-by-Side Compare) Feature

**Status:** DONE (2026-11-16). Owner approved full UI + Backend removal (`D-P012-1`) and keeping offline CLI `evals/` blind A/B judge (`D-P012-2`).
**Workflow:** DEV-KIT `feature` / `refactor` (Breaking API & UI removal with explicit owner approval).
**Spec & Architecture:** [docs/PLAN.md](../../PLAN.md), [docs/ARCHITECTURE.md](../../ARCHITECTURE.md), [CSS_DESIGN_SYSTEM.md](../../../CSS_DESIGN_SYSTEM.md).

## Decisions Locked (Owner)

- **`D-P012-1` (Full UI + Backend Removal):** Remove the "So sánh với yêu cầu gốc" feature end-to-end across Studio UI (`#openCompareBtn`, `#compareView`, Compare JS handlers, `.compare-*` CSS), HTTP router (`POST /api/versions/{id}/compare`, `internal/handlers/compare.go`), domain service (`CompareVersion`, `CheckComparable`, `CompareInput`, `MissingVariablesError`, `ErrCompareNotAllowed`, `ErrCompareIncomplete`, `formatTestDataAppendix`), and Compare-specific HTTP tests. Preserve `"Xem thay đổi"` (Version Diff, `D-P010-2`), `"Điền biến"` inline drawer (`D-P010-1`), `.back-link` (used by `/settings`), and static historical mockup `design/compare.html`.
- **`D-P012-2` (Keep Offline `evals/` Blind Judge):** Retain `go run ./evals` (`evals/main.go` and `MockProvider` judge/execution branches) as the developer CLI quality gate for Meta-Prompt changes.

## Tasks

- [x] **T1 (Backend Handler & Service Removal):** Unregistered `POST /api/versions/{id}/compare` in `internal/handlers/router.go`, removed `internal/handlers/compare.go`, and removed `CompareVersion`, `CheckComparable`, `CompareInput`, `MissingVariablesError`, `ErrCompare*`, and `formatTestDataAppendix` from `internal/studio/service.go`.
- [x] **T2 (Frontend Markup, JS & CSS Removal):** Removed `#openCompareBtn` and `#compareView` from `web/index.html`, updated `#studioVarsDrawer` subtitle, removed Compare state/handlers/bindings from `web/app.js`, and removed `.compare-*` rules from `web/css/06-pages.css` (preserving `.back-link` for `settings.html`).
- [x] **T3 (Test Suite Reconciliation & Removal Guard):** Updated `internal/handlers/{e2e_test.go,p005_test.go,p010_test.go,p011_test.go}` to remove Compare HTTP assertions while preserving `TestFrontendDOMIDsAndDesignSystemInvariants` and adding explicit guards in `TestAutoFillPayment500ZeroPlaceholdersAndCompareRemoved` verifying `POST /api/versions/{id}/compare` returns `404`/`405` and no Compare selectors/IDs remain in embedded assets.
- [x] **T4 (Docs Reconciliation):** Synchronized `docs/PLAN.md`, `docs/ARCHITECTURE.md`, `README.md`, `CSS_DESIGN_SYSTEM.md`, and `docs/CHANGELOG.md`.
- [x] **T5 (Verification Gate & Dev Server Hot-Restart):** Passed `./scripts/check.sh` and `go run ./evals` (`23/23` PASS), rebuilt `./bin/prompt-studio`, and restarted the local dev server on `127.0.0.1:7788`.

## Verification Evidence

- `./scripts/check.sh` → exit code `0` (`gofmt`, `go mod tidy -diff`, `go build ./...`, `go vet ./...`, `go test -count=1 ./...`, `node --check web/*.js`).
- `TestAutoFillPayment500ZeroPlaceholdersAndCompareRemoved` & `TestFrontendDOMIDsAndDesignSystemInvariants` → PASS.
- `$HOME/.local/go/bin/go run ./evals` → exit code `0` (`23/23` machine checks `100.0%`, `23/23` blind judge wins `100.0%`, `0` injection breaches).
