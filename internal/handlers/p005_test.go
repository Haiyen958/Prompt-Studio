package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"prompt-studio/internal/ai"
	"prompt-studio/internal/storage"
)

// generateSession runs a mock generation and returns (sessionID, versionID).
func generateSession(t *testing.T, client *http.Client, baseURL, token string) (string, string) {
	t.Helper()
	events := generateSSE(t, client, baseURL, token)
	done, ok := findEvent(events, "done")
	if !ok {
		t.Fatalf("generate produced no done event: %+v", events)
	}
	ver, _ := done.Data["version"].(map[string]any)
	return done.Data["sessionId"].(string), ver["id"].(string)
}

func refineSession(t *testing.T, client *http.Client, baseURL, token, sessionID string, body map[string]any) map[string]any {
	t.Helper()
	resp := doJSONRequest(t, client, http.MethodPost, baseURL+"/api/sessions/"+sessionID+"/refine", token, body)
	defer resp.Body.Close()
	done, ok := findEvent(parseSSEStream(t, resp.Body), "done")
	if !ok {
		t.Fatal("refine produced no done event")
	}
	ver, _ := done.Data["version"].(map[string]any)
	return ver
}

func getDetail(t *testing.T, client *http.Client, baseURL, token, sessionID string) storage.SessionDetail {
	t.Helper()
	resp := doJSONRequest(t, client, http.MethodGet, baseURL+"/api/sessions/"+sessionID, token, nil)
	defer resp.Body.Close()
	var d storage.SessionDetail
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	return d
}

// Save pins the viewed version: later experiments must not change what the Library shows.
func TestSavePinsViewedVersion(t *testing.T) {
	const token = "save-pin-token"
	ts, _, _ := setupTestEnv(t, token)
	client := ts.Client()

	sid, v1 := generateSession(t, client, ts.URL, token)
	saveResp := doJSONRequest(t, client, http.MethodPut, ts.URL+"/api/sessions/"+sid+"/save", token, map[string]any{"versionId": v1})
	saveResp.Body.Close()
	if saveResp.StatusCode != http.StatusOK {
		t.Fatalf("save status = %d", saveResp.StatusCode)
	}

	refineSession(t, client, ts.URL, token, sid, map[string]any{"fromVersionId": v1, "instruction": "Ngắn hơn"})

	d := getDetail(t, client, ts.URL, token, sid)
	if d.Session.PinnedVersionID != v1 {
		t.Fatalf("pinned = %q, want saved version %q", d.Session.PinnedVersionID, v1)
	}
	if len(d.Versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(d.Versions))
	}

	// Saving without versionId pins the latest version.
	resp := doJSONRequest(t, client, http.MethodPut, ts.URL+"/api/sessions/"+sid+"/save", token, map[string]any{})
	resp.Body.Close()
	d = getDetail(t, client, ts.URL, token, sid)
	if d.Session.PinnedVersionID != d.Versions[1].ID {
		t.Fatalf("save without versionId should pin latest, got %q", d.Session.PinnedVersionID)
	}

	// Unsave clears the pin.
	resp = doJSONRequest(t, client, http.MethodDelete, ts.URL+"/api/sessions/"+sid+"/save", token, nil)
	resp.Body.Close()
	d = getDetail(t, client, ts.URL, token, sid)
	if d.Session.PinnedVersionID != "" || d.Session.SavedAt != nil {
		t.Fatalf("unsave should clear saved_at and pin, got %+v", d.Session)
	}
}

func TestSaveOrPinRejectsForeignVersion(t *testing.T) {
	const token = "foreign-pin-token"
	ts, _, _ := setupTestEnv(t, token)
	client := ts.Client()

	sidA, _ := generateSession(t, client, ts.URL, token)
	_, vB := generateSession(t, client, ts.URL, token)

	resp := doJSONRequest(t, client, http.MethodPut, ts.URL+"/api/sessions/"+sidA+"/save", token, map[string]any{"versionId": vB})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("save with foreign version: status = %d, want 400", resp.StatusCode)
	}
	resp = doJSONRequest(t, client, http.MethodPatch, ts.URL+"/api/sessions/"+sidA, token, map[string]any{"pinnedVersionId": vB})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("pin foreign version: status = %d, want 400", resp.StatusCode)
	}
}

// Target belongs to each version: refining as "api" must not relabel the chat v1.
func TestTargetIsStoredPerVersion(t *testing.T) {
	const token = "per-version-target-token"
	ts, _, _ := setupTestEnv(t, token)
	client := ts.Client()

	sid, v1 := generateSession(t, client, ts.URL, token)
	v2 := refineSession(t, client, ts.URL, token, sid, map[string]any{"fromVersionId": v1, "target": "api"})
	if v2["target"] != "api" {
		t.Fatalf("v2 target = %v, want api", v2["target"])
	}
	// Refining v2 without a target keeps v2's target.
	v3 := refineSession(t, client, ts.URL, token, sid, map[string]any{"fromVersionId": v2["id"], "instruction": "Thêm ví dụ"})
	if v3["target"] != "api" {
		t.Fatalf("v3 target = %v, want inherited api", v3["target"])
	}

	d := getDetail(t, client, ts.URL, token, sid)
	if d.Versions[0].Target != "chat" || d.Versions[1].Target != "api" {
		t.Fatalf("targets = %q,%q want chat,api", d.Versions[0].Target, d.Versions[1].Target)
	}
	if d.Session.Target != "api" {
		t.Fatalf("session target should remember last used target, got %q", d.Session.Target)
	}
}

func TestCompareRejectsMissingVariablesAndIncomplete(t *testing.T) {
	const token = "compare-guard-token"
	ts, svc, _ := setupTestEnv(t, token)
	client := ts.Client()

	sid, _ := generateSession(t, client, ts.URL, token)
	d := getDetail(t, client, ts.URL, token, sid)
	if len(ai.ExtractVariables(d.Versions[0].Content)) == 0 {
		t.Skip("mock prompt has no variables")
	}
	resp := doJSONRequest(t, client, http.MethodPost, ts.URL+"/api/versions/"+d.Versions[0].ID+"/compare", token, map[string]any{})
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest || body["code"] != "missing_variables" {
		t.Fatalf("expected 400 missing_variables, got %d %+v", resp.StatusCode, body)
	}

	incomplete, err := svc.Repo().CreateVersion(storage.PromptVersion{SessionID: sid, Source: "regenerate", Content: "Prompt dở", Incomplete: true})
	if err != nil {
		t.Fatalf("CreateVersion: %v", err)
	}
	resp = doJSONRequest(t, client, http.MethodPost, ts.URL+"/api/versions/"+incomplete.ID+"/compare", token, map[string]any{})
	body = map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest || body["code"] != "compare_incomplete" {
		t.Fatalf("expected 400 compare_incomplete, got %d %+v", resp.StatusCode, body)
	}
}

// failRawProvider fails only the "raw" side (the bare original request).
type failRawProvider struct{ request string }

func (p *failRawProvider) Ping(context.Context) error { return nil }
func (p *failRawProvider) Stream(_ context.Context, r ai.Request, onDelta func(string)) (ai.Usage, error) {
	if len(r.Messages) == 1 && strings.HasPrefix(r.Messages[0].Content, p.request) {
		return ai.Usage{}, ai.ErrRateLimit
	}
	onDelta("Kết quả từ prompt.")
	return ai.Usage{}, nil
}

func TestCompareReportsErrorPerSide(t *testing.T) {
	const token = "compare-side-token"
	ts, svc, _ := setupTestEnv(t, token)
	client := ts.Client()

	sid, vid := generateSession(t, client, ts.URL, token)
	d := getDetail(t, client, ts.URL, token, sid)
	vars := map[string]string{}
	for _, n := range ai.ExtractVariables(d.Versions[0].Content) {
		vars[n] = "giá trị thử"
	}
	useProvider(svc, &failRawProvider{request: d.Session.Request})

	resp := doJSONRequest(t, client, http.MethodPost, ts.URL+"/api/versions/"+vid+"/compare", token, map[string]any{"variables": vars})
	defer resp.Body.Close()
	events := parseSSEStream(t, resp.Body)

	sideErr, ok := findEvent(events, "side_error")
	if !ok || sideErr.Data["side"] != "raw" || sideErr.Data["code"] != "rate_limit" {
		t.Fatalf("expected side_error for raw, got %+v", events)
	}
	promptDone := false
	for _, e := range events {
		if e.Event == "side_done" && e.Data["side"] == "prompt" {
			promptDone = true
		}
		if e.Event == "error" {
			t.Fatalf("one failed side must not abort the whole compare: %+v", e.Data)
		}
	}
	if _, ok := findEvent(events, "done"); !ok || !promptDone {
		t.Fatalf("expected prompt side_done and done, got %+v", events)
	}
}

func TestListSessionsPagination(t *testing.T) {
	const token = "pagination-token"
	ts, _, _ := setupTestEnv(t, token)
	client := ts.Client()
	for i := 0; i < 3; i++ {
		generateSession(t, client, ts.URL, token)
	}

	page := func(url string) (n int, hasMore bool, next float64) {
		resp := doJSONRequest(t, client, http.MethodGet, url, token, nil)
		defer resp.Body.Close()
		var out struct {
			Sessions   []storage.Session `json:"sessions"`
			HasMore    bool              `json:"hasMore"`
			NextOffset float64           `json:"nextOffset"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return len(out.Sessions), out.HasMore, out.NextOffset
	}

	n, more, next := page(ts.URL + "/api/sessions?limit=2")
	if n != 2 || !more || next != 2 {
		t.Fatalf("page 1: n=%d hasMore=%v next=%v", n, more, next)
	}
	n, more, _ = page(ts.URL + "/api/sessions?limit=2&offset=2")
	if n != 1 || more {
		t.Fatalf("page 2: n=%d hasMore=%v", n, more)
	}
}

func TestWritesRequireExactOriginAndJSON(t *testing.T) {
	const token = "csrf-token"
	ts, _, _ := setupTestEnv(t, token)
	client := ts.Client()

	// Another local port is a different site.
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/collections", strings.NewReader(`{"name":"X"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("X-Prompt-Studio-Token", token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("other loopback origin: status = %d, want 403", resp.StatusCode)
	}

	// Same origin is allowed.
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/collections", strings.NewReader(`{"name":"Cùng origin"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", ts.URL)
	req.Header.Set("X-Prompt-Studio-Token", token)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("same origin: status = %d, want 201", resp.StatusCode)
	}

	// Simple-request content types (no CORS preflight) are rejected.
	for _, ct := range []string{"text/plain", "application/x-www-form-urlencoded", ""} {
		req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/import", strings.NewReader(`{"sessions":[]}`))
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		req.Header.Set("X-Prompt-Studio-Token", token)
		resp, err = client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnsupportedMediaType {
			t.Fatalf("content-type %q: status = %d, want 415", ct, resp.StatusCode)
		}
	}
}
