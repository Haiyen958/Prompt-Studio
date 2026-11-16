package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"prompt-studio/internal/ai"
)

func postJSON(t *testing.T, client *http.Client, url, token string, body map[string]any) map[string]any {
	t.Helper()
	resp := doJSONRequest(t, client, http.MethodPost, url, token, body)
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out
}

// fakeCatalog serves GET /v1/models in the OpenAI or Anthropic shape and records the auth headers.
func fakeCatalog(t *testing.T, anthropic bool, gotAuth *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if anthropic {
			*gotAuth = r.Header.Get("x-api-key")
		} else {
			*gotAuth = r.Header.Get("Authorization")
		}
		if !strings.HasSuffix(r.URL.Path, "/models") {
			http.NotFound(w, r)
			return
		}
		if *gotAuth == "" || strings.HasSuffix(*gotAuth, "bad") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if anthropic {
			if r.URL.Query().Get("after_id") == "" {
				_, _ = w.Write([]byte(`{"data":[{"id":"claude-new","display_name":"Claude New"}],"has_more":true,"last_id":"claude-new"}`))
			} else {
				_, _ = w.Write([]byte(`{"data":[{"id":"claude-old","display_name":"Claude Old"}],"has_more":false,"last_id":"claude-old"}`))
			}
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-b"},{"id":"text-embedding-3-small"},{"id":"gpt-a"},{"id":"whisper-1"},{"id":"gpt-4o-mini-tts"}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Settings can list the provider's live models before saving; the stored key follows
// the Test Connection policy (same provider + endpoint only).
func TestListModelsFromProvider(t *testing.T) {
	const token = "p009-models-token"
	ts, svc, _ := setupTestEnv(t, token)
	client := ts.Client()
	svc.SetProviderFactory(ai.NewProvider) // real HTTP providers against the fake catalogs

	var openAIAuth, anthropicAuth string
	openaiSrv := fakeCatalog(t, false, &openAIAuth)
	anthropicSrv := fakeCatalog(t, true, &anthropicAuth)

	// OpenAI with a typed key: non-chat families filtered, sorted.
	out := postJSON(t, client, ts.URL+"/api/settings/models", token, map[string]any{
		"provider": "openai", "baseUrl": openaiSrv.URL + "/v1", "apiKey": "sk-typed",
	})
	if out["ok"] != true {
		t.Fatalf("openai list: %v", out)
	}
	var ids []string
	for _, m := range out["models"].([]any) {
		ids = append(ids, m.(map[string]any)["id"].(string))
	}
	if strings.Join(ids, ",") != "gpt-a,gpt-b" || openAIAuth != "Bearer sk-typed" {
		t.Fatalf("openai models = %v auth = %q", ids, openAIAuth)
	}

	// Anthropic: pagination followed, display names kept, newest first.
	out = postJSON(t, client, ts.URL+"/api/settings/models", token, map[string]any{
		"provider": "anthropic", "baseUrl": anthropicSrv.URL + "/v1", "apiKey": "sk-ant",
	})
	models, _ := out["models"].([]any)
	if out["ok"] != true || len(models) != 2 || models[0].(map[string]any)["name"] != "Claude New" || anthropicAuth != "sk-ant" {
		t.Fatalf("anthropic list: %v auth %q", out, anthropicAuth)
	}

	// Bad key → auth_error explained in Vietnamese.
	out = postJSON(t, client, ts.URL+"/api/settings/models", token, map[string]any{
		"provider": "openai", "baseUrl": openaiSrv.URL + "/v1", "apiKey": "sk-bad",
	})
	if out["ok"] != false || out["code"] != "auth_error" || !strings.Contains(out["error"].(string), "Cài đặt") {
		t.Fatalf("bad key: %v", out)
	}

	// Stored key is never sent to an endpoint other than the saved one.
	resp := doJSONRequest(t, client, http.MethodPut, ts.URL+"/api/settings", token, map[string]any{
		"provider": "openai", "model": "gpt-a", "apiKey": "sk-stored",
	})
	resp.Body.Close()
	openAIAuth = ""
	out = postJSON(t, client, ts.URL+"/api/settings/models", token, map[string]any{
		"provider": "openai", "baseUrl": openaiSrv.URL + "/v1",
	})
	if openAIAuth == "Bearer sk-stored" {
		t.Fatalf("stored key leaked to a different base URL: %v", out)
	}
}

// Anthropic has no default model: saving it without a model, or generating with an
// empty model, asks the user to pick one.
func TestAnthropicRequiresChosenModel(t *testing.T) {
	const token = "p009-anthropic-token"
	ts, svc, _ := setupTestEnv(t, token)
	client := ts.Client()

	status, out := putSettings(t, client, ts.URL, token, map[string]any{"provider": "anthropic", "apiKey": "sk-ant"})
	if status != http.StatusBadRequest || out["field"] != "model" {
		t.Fatalf("switch provider without model: %d %v", status, out)
	}
	status, out = putSettings(t, client, ts.URL, token, map[string]any{"provider": "anthropic", "model": "claude-picked", "apiKey": "sk-ant"})
	if status != http.StatusOK || out["model"] != "claude-picked" {
		t.Fatalf("switch provider with model: %d %v", status, out)
	}
	// Same provider, other fields only: no model needed.
	if status, out = putSettings(t, client, ts.URL, token, map[string]any{"provider": "anthropic", "theme": "light"}); status != http.StatusOK {
		t.Fatalf("same provider update: %d %v", status, out)
	}

	// A stored empty model (e.g. from an import) fails generation with model_required.
	if err := svc.Repo().SaveSettings(map[string]string{"model": ""}); err != nil {
		t.Fatal(err)
	}
	svc.SetProviderFactory(ai.NewProvider)
	evt, ok := findEvent(generateSSE(t, client, ts.URL, token), "error")
	if !ok || evt.Data["code"] != "model_required" {
		t.Fatalf("generate with empty Anthropic model: %v", evt)
	}
	p, err := ai.NewProvider(ai.ProviderConfig{Provider: "anthropic", APIKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	if got := p.(*ai.AnthropicProvider).Model; got != "" {
		t.Fatalf("anthropic default model = %q, want none", got)
	}
}
