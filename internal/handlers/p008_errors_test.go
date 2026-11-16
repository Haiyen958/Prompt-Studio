package handlers_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"prompt-studio/internal/ai"
)

// P1-c: SSE `error` events carry a stable code and a Vietnamese, actionable message.
func TestStreamErrorsHaveCodeAndVietnameseMessage(t *testing.T) {
	const token = "p008-err-token"
	ts, svc, _ := setupTestEnv(t, token)
	client := ts.Client()

	cases := map[error]struct{ code, contains string }{
		ai.ErrAuth:      {"auth_error", "Cài đặt"},
		ai.ErrRateLimit: {"rate_limit", "thử lại"},
		ai.ErrNoAPIKey:  {"unconfigured", "Cài đặt"},
		ai.ErrTimeout:   {"timeout", "thử lại"},
	}
	for provErr, want := range cases {
		useProvider(svc, &scriptedProvider{err: provErr})
		evt, ok := findEvent(generateSSE(t, client, ts.URL, token), "error")
		if !ok {
			t.Fatalf("%v: no error event", provErr)
		}
		msg, _ := evt.Data["message"].(string)
		if evt.Data["code"] != want.code || !strings.Contains(msg, want.contains) {
			t.Fatalf("%v: code = %v message = %q, want %s containing %q", provErr, evt.Data["code"], msg, want.code, want.contains)
		}
	}
}

// P1-c: duplicate collection names are a 400 input error, not a raw SQLite message.
func TestDuplicateCollectionNameIsInputError(t *testing.T) {
	const token = "p008-col-token"
	ts, _, _ := setupTestEnv(t, token)
	client := ts.Client()

	for i, wantStatus := range []int{http.StatusCreated, http.StatusBadRequest} {
		resp := doJSONRequest(t, client, http.MethodPost, ts.URL+"/api/collections", token, map[string]any{"name": "Marketing"})
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if resp.StatusCode != wantStatus {
			t.Fatalf("create #%d: status = %d body = %v", i+1, resp.StatusCode, body)
		}
		if i == 1 {
			msg, _ := body["error"].(string)
			if body["code"] != "collection_error" || !strings.Contains(msg, "đã tồn tại") || strings.Contains(msg, "UNIQUE") {
				t.Fatalf("duplicate: body = %v", body)
			}
		}
	}

	resp := doJSONRequest(t, client, http.MethodPost, ts.URL+"/api/collections", token, map[string]any{"name": "  "})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty name: status = %d, want 400", resp.StatusCode)
	}
}
