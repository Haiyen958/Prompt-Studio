package handlers_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"prompt-studio/internal/ai"
	"prompt-studio/internal/storage"
)

// noRedirectClient returns a client that surfaces redirects instead of following them.
func noRedirectClient(c *http.Client) *http.Client {
	cp := *c
	cp.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &cp
}

// H1: reads and pages require the token; only ?token= hands out the cookie.
func TestReadsAndPagesRequireToken(t *testing.T) {
	const token = "p007-auth-token"
	ts, _, _ := setupTestEnv(t, token)
	client := noRedirectClient(ts.Client())

	for _, path := range []string{"/", "/library", "/settings", "/api/export", "/api/sessions", "/api/settings"} {
		resp, err := client.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("GET %s without token: status = %d, want 401", path, resp.StatusCode)
		}
		if sc := resp.Header.Get("Set-Cookie"); sc != "" {
			t.Fatalf("GET %s without token must not hand out a cookie, got %q", path, sc)
		}
	}

	// Static assets hold no data and stay public (the 401 page itself needs none).
	resp, err := client.Get(ts.URL + "/app.js")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /app.js: status = %d, want 200", resp.StatusCode)
	}

	// A wrong token is not exchanged.
	resp, err = client.Get(ts.URL + "/?token=wrong")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized || resp.Header.Get("Set-Cookie") != "" {
		t.Fatalf("wrong token: status = %d cookie = %q", resp.StatusCode, resp.Header.Get("Set-Cookie"))
	}

	// The startup URL exchanges the token for a long-lived HttpOnly cookie.
	resp, err = client.Get(ts.URL + "/library?token=" + token)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/library" {
		t.Fatalf("token exchange: status = %d location = %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "ps_token" {
			cookie = c
		}
	}
	if cookie == nil || !cookie.HttpOnly || cookie.MaxAge <= 0 {
		t.Fatalf("expected persistent HttpOnly ps_token cookie, got %+v", cookie)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/export", nil)
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/export with cookie: status = %d, want 200", resp.StatusCode)
	}
}

// H2: the stored API key is only used for the endpoint it was saved for.
func TestTestConnectionNeverSendsStoredKeyToAnotherEndpoint(t *testing.T) {
	const token = "p007-key-token"
	ts, svc, _ := setupTestEnv(t, token)
	client := ts.Client()

	var gotKey, gotURL string
	svc.SetProviderFactory(func(cfg ai.ProviderConfig) (ai.Provider, error) {
		gotKey, gotURL = cfg.APIKey, cfg.BaseURL
		return ai.NewMockProvider(cfg.Model), nil
	})

	resp := doJSONRequest(t, client, http.MethodPut, ts.URL+"/api/settings", token, map[string]any{
		"provider": "openai", "baseUrl": "https://api.openai.com/v1/", "apiKey": "sk-stored-0000",
	})
	resp.Body.Close()

	cases := []struct {
		name    string
		body    map[string]any
		wantKey string
	}{
		{"same endpoint, no key typed", map[string]any{"provider": "openai"}, "sk-stored-0000"},
		{"same endpoint, trailing slash/case", map[string]any{"provider": "openai", "baseUrl": "HTTPS://api.openai.com/v1"}, "sk-stored-0000"},
		{"other base URL", map[string]any{"provider": "openai", "baseUrl": "http://127.0.0.1:9/v1"}, ""},
		{"other provider", map[string]any{"provider": "openai-compatible"}, ""},
		{"other base URL with typed key", map[string]any{"provider": "openai", "baseUrl": "http://127.0.0.1:9/v1", "apiKey": "sk-typed"}, "sk-typed"},
	}
	for _, tc := range cases {
		gotKey, gotURL = "<unset>", ""
		resp := doJSONRequest(t, client, http.MethodPost, ts.URL+"/api/settings/test", token, tc.body)
		resp.Body.Close()
		if gotKey != tc.wantKey {
			t.Fatalf("%s: provider got key %q (url %q), want %q", tc.name, gotKey, gotURL, tc.wantKey)
		}
	}
}

// H3: a backup larger than the generic 4 MB write cap can be restored.
func TestImportAcceptsBackupLargerThanWriteLimit(t *testing.T) {
	const token = "p007-import-token"
	ts, _, _ := setupTestEnv(t, token)
	client := ts.Client()

	big := strings.Repeat("Nội dung prompt rất dài. ", 250_000) // ≈ 6.5 MB UTF-8
	bundle := storage.BackupBundle{
		Version: 2,
		Sessions: []storage.SessionDetail{{
			Session: storage.Session{ID: "01IMPORTBIG0000000000000000", Title: "Lớn", Origin: "pasted", Target: "chat", CreatedAt: 1, OpenedAt: 1},
			Versions: []storage.PromptVersion{{
				ID: "01IMPORTBIGV000000000000000", Seq: 1, Source: "pasted", Content: big, CreatedAt: 1,
			}},
		}},
	}
	b, _ := json.Marshal(bundle)
	if len(b) <= 4*1024*1024 {
		t.Fatalf("test bundle too small: %d bytes", len(b))
	}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/import", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Prompt-Studio-Token", token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("import %d bytes: status = %d body = %s", len(b), resp.StatusCode, body)
	}

	// Other writes keep the 4 MB cap.
	huge := map[string]any{"content": strings.Repeat("x", 5*1024*1024)}
	resp = doJSONRequest(t, client, http.MethodPost, ts.URL+"/api/sessions/paste", token, huge)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("5 MB paste: status = %d, want 400", resp.StatusCode)
	}
}

// H4: a manual edit cannot attach to a version of another session.
func TestManualVersionRejectsForeignOrUnknownParent(t *testing.T) {
	const token = "p007-manual-token"
	ts, _, _ := setupTestEnv(t, token)
	client := ts.Client()

	sessA, verA := generateSession(t, client, ts.URL, token)
	_, verB := generateSession(t, client, ts.URL, token)

	for name, parent := range map[string]string{"foreign": verB, "unknown": "01NOSUCHVERSION000000000000"} {
		resp := doJSONRequest(t, client, http.MethodPost, ts.URL+"/api/sessions/"+sessA+"/versions", token,
			map[string]any{"parentId": parent, "content": "Sửa tay"})
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest || body["code"] != "version_mismatch" {
			t.Fatalf("%s parent: status = %d body = %v, want 400 version_mismatch", name, resp.StatusCode, body)
		}
	}
	if n := len(getDetail(t, client, ts.URL, token, sessA).Versions); n != 1 {
		t.Fatalf("rejected edits must not create versions, got %d", n)
	}

	resp := doJSONRequest(t, client, http.MethodPost, ts.URL+"/api/sessions/"+sessA+"/versions", token,
		map[string]any{"parentId": verA, "content": "Sửa tay hợp lệ"})
	var ok struct {
		Version storage.PromptVersion `json:"version"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&ok)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated || ok.Version.ParentID != verA || ok.Version.Source != "manual" {
		t.Fatalf("valid manual edit: status = %d version = %+v", resp.StatusCode, ok.Version)
	}

	resp = doJSONRequest(t, client, http.MethodPost, ts.URL+"/api/sessions/"+sessA+"/versions", token,
		map[string]any{"content": "   "})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty manual edit: status = %d, want 400", resp.StatusCode)
	}
}
