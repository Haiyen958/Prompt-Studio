package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func putSettings(t *testing.T, client *http.Client, baseURL, token string, body map[string]any) (int, map[string]any) {
	t.Helper()
	resp := doJSONRequest(t, client, http.MethodPut, baseURL+"/api/settings", token, body)
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// P1-a: invalid settings are rejected as a whole (nothing partially written).
func TestUpdateSettingsValidatesBeforeWriting(t *testing.T) {
	const token = "p007-settings-token"
	ts, svc, _ := setupTestEnv(t, token)
	client := ts.Client()

	invalid := []struct {
		field string
		body  map[string]any
	}{
		{"provider", map[string]any{"provider": "gemini"}},
		{"temperature", map[string]any{"temperature": 2.5}},
		{"temperature", map[string]any{"temperature": -0.1}},
		{"maxTokens", map[string]any{"maxTokens": 0}},
		{"maxTokens", map[string]any{"maxTokens": 100000}},
		{"timeoutSec", map[string]any{"timeoutSec": 1}},
		{"theme", map[string]any{"theme": "neon"}},
		{"promptLanguage", map[string]any{"promptLanguage": "fr"}},
		{"defaultTarget", map[string]any{"defaultTarget": "email"}},
	}
	for _, tc := range invalid {
		// A valid field next to the invalid one must not be written either.
		body := map[string]any{"model": "should-not-be-saved", "apiKey": "sk-should-not-be-saved"}
		for k, v := range tc.body {
			body[k] = v
		}
		status, out := putSettings(t, client, ts.URL, token, body)
		if status != http.StatusBadRequest || out["code"] != "invalid_settings" || out["field"] != tc.field {
			t.Fatalf("%v: status = %d body = %v, want 400 invalid_settings field=%s", tc.body, status, out, tc.field)
		}
	}
	stored, _ := svc.Repo().GetSettings()
	if stored["model"] == "should-not-be-saved" {
		t.Fatal("a rejected update partially wrote settings")
	}
	if _, _, hasKey, _ := svc.Repo().GetAPIKey(); hasKey {
		t.Fatal("a rejected update stored the API key")
	}

	// Temperature 0 is valid and must round-trip (it used to become 0.4).
	status, out := putSettings(t, client, ts.URL, token, map[string]any{
		"provider": "anthropic", "temperature": 0, "maxTokens": 4000, "timeoutSec": 120,
		"theme": "light", "promptLanguage": "en", "defaultTarget": "agent",
	})
	if status != http.StatusOK {
		t.Fatalf("valid update: status = %d body = %v", status, out)
	}
	if out["temperature"] != float64(0) || out["maxTokens"] != float64(4000) || out["timeoutSec"] != float64(120) ||
		out["theme"] != "light" || out["promptLanguage"] != "en" || out["defaultTarget"] != "agent" || out["provider"] != "anthropic" {
		t.Fatalf("valid update not reflected: %v", out)
	}
	cfg, _, _, err := svc.LoadProviderConfig()
	if err != nil || cfg.Temperature != 0 || cfg.MaxTokens != 4000 || cfg.Timeout.Seconds() != 120 {
		t.Fatalf("provider config = %+v err %v", cfg, err)
	}
}
