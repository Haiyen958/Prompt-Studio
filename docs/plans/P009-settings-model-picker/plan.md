# P009 — Choose the model in Settings from the provider's live list

**Status:** DONE (2026-11-16). Owner choices: pick the model in Settings (not per session); Anthropic has no default model ("bắt buộc chọn").
**Workflow:** DEV-KIT `feature`. **Spec:** [docs/PLAN.md](../../PLAN.md) §1 (unchanged principle), §3 Settings.

## Tasks

- [x] `ai.ModelLister` (optional interface) for OpenAI / OpenAI-compatible (`GET /models`), Anthropic (`GET /v1/models`, paginated) and Mock; OpenAI official list excludes embedding/speech/image/moderation families.
- [x] Anthropic default model removed; empty model → `ai.ErrModelRequired` (`model_required`).
- [x] `studio.Service.ProbeConfig` (key policy shared with Test Connection) and `ListModels`; `UpdateSettings` requires a model when the provider changes (except mock) and for Anthropic.
- [x] `POST /api/settings/models`; Test Connection rewritten on `ProbeConfig`; Vietnamese probe failures.
- [x] Settings UI: "Tải danh sách model", live datalist with display names, auto-load when a key is saved, provider switch clears/restores the model, client-side guard before save; Studio banner maps `model_required` to "Mở Cài đặt".

## Verification Evidence

- `scripts/check.sh` → All checks passed.
- Tests: `TestListModelsFromProvider` (OpenAI filtering/sorting, Anthropic pagination + names, auth error, stored key not sent to another endpoint), `TestAnthropicRequiresChosenModel`; `TestUpdateSettingsValidatesBeforeWriting` updated to send a model with the provider switch.
- Smoke on the binary with a local fake Anthropic catalog: list with typed key → 2 models; bad key → `auth_error`; save without model → 400 `field:model`; save with model → stored; list with stored key → OK.
- Not verified: the Settings page interaction in a real browser.
