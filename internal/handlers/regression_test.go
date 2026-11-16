package handlers_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"prompt-studio/internal/ai"
	"prompt-studio/internal/storage"
)

// scriptedProvider emits fixed chunks, then returns the given usage/error.
type scriptedProvider struct {
	chunks []string
	usage  ai.Usage
	err    error
	block  bool // wait for ctx cancellation after emitting chunks
}

func (p *scriptedProvider) Ping(context.Context) error { return nil }
func (p *scriptedProvider) Stream(ctx context.Context, _ ai.Request, onDelta func(string)) (ai.Usage, error) {
	for _, c := range p.chunks {
		onDelta(c)
	}
	if p.block {
		<-ctx.Done()
		return p.usage, ctx.Err()
	}
	return p.usage, p.err
}

func useProvider(svcSetter interface {
	SetProviderFactory(func(ai.ProviderConfig) (ai.Provider, error))
}, p ai.Provider) {
	svcSetter.SetProviderFactory(func(ai.ProviderConfig) (ai.Provider, error) { return p, nil })
}

func generateSSE(t *testing.T, client *http.Client, baseURL, token string) []parsedSSEEvent {
	t.Helper()
	resp := doJSONRequest(t, client, http.MethodPost, baseURL+"/api/sessions", token, map[string]any{
		"request": "Viết prompt tóm tắt biên bản họp",
		"target":  "chat",
	})
	defer resp.Body.Close()
	return parseSSEStream(t, resp.Body)
}

func findEvent(events []parsedSSEEvent, name string) (parsedSSEEvent, bool) {
	for _, e := range events {
		if e.Event == name {
			return e, true
		}
	}
	return parsedSSEEvent{}, false
}

// Regression P0-3: an LLM error before any output must not leave an empty session in history.
func TestGenerateErrorBeforeOutputLeavesNoEmptySession(t *testing.T) {
	const token = "orphan-token"
	ts, svc, _ := setupTestEnv(t, token)
	useProvider(svc, &scriptedProvider{err: ai.ErrAuth})

	events := generateSSE(t, ts.Client(), ts.URL, token)
	errEvt, ok := findEvent(events, "error")
	if !ok || errEvt.Data["code"] != "auth_error" {
		t.Fatalf("expected auth_error event, got %+v", events)
	}

	list, err := svc.Repo().ListSessions(storage.SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no sessions after failed generate, got %d", len(list))
	}
}

// Regression P0-3: pressing Stop before the first token must not leave an empty session.
func TestStopBeforeOutputLeavesNoEmptySession(t *testing.T) {
	const token = "stop-empty-token"
	ts, svc, _ := setupTestEnv(t, token)
	useProvider(svc, &scriptedProvider{block: true})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body, _ := json.Marshal(map[string]any{"request": "Viết prompt dịch tài liệu", "target": "chat"})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, ts.URL+"/api/sessions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Prompt-Studio-Token", token)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("POST /api/sessions: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	for {
		line, rErr := reader.ReadString('\n')
		if strings.HasPrefix(line, "event: session") {
			cancel()
			break
		}
		if rErr != nil {
			t.Fatalf("stream ended before session event: %v", rErr)
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		list, _ := svc.Repo().ListSessions(storage.SessionFilter{})
		if len(list) == 0 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("empty session still present after Stop before first token")
}

// Regression P0-4: hitting max tokens before ===META=== must be saved as incomplete and reported.
func TestTruncatedPromptIsMarkedIncomplete(t *testing.T) {
	const token = "truncated-token"
	ts, svc, _ := setupTestEnv(t, token)
	useProvider(svc, &scriptedProvider{
		chunks: []string{"## Vai trò\nBạn là thư ký cuộc họp.\n\n## Nhiệm vụ\nTóm tắt các quyết đ"},
		usage:  ai.Usage{OutputTokens: 2000, Truncated: true},
	})

	events := generateSSE(t, ts.Client(), ts.URL, token)
	done, ok := findEvent(events, "done")
	if !ok {
		t.Fatalf("expected done event, got %+v", events)
	}
	if done.Data["truncated"] != true || done.Data["incomplete"] != true {
		t.Fatalf("expected truncated+incomplete, got %+v", done.Data)
	}
	version, _ := done.Data["version"].(map[string]any)
	if version["incomplete"] != true {
		t.Fatalf("expected persisted version incomplete, got %+v", version)
	}
}

// Truncation inside the meta block keeps a complete prompt (only assumptions are lost).
func TestTruncatedInsideMetaKeepsPromptComplete(t *testing.T) {
	const token = "truncated-meta-token"
	ts, svc, _ := setupTestEnv(t, token)
	useProvider(svc, &scriptedProvider{
		chunks: []string{"## Vai trò\nBạn là thư ký cuộc họp.\n===META===\n{\"intent\": \"Tóm tắt"},
		usage:  ai.Usage{OutputTokens: 2000, Truncated: true},
	})

	events := generateSSE(t, ts.Client(), ts.URL, token)
	done, ok := findEvent(events, "done")
	if !ok {
		t.Fatalf("expected done event, got %+v", events)
	}
	if done.Data["truncated"] != true || done.Data["incomplete"] != false {
		t.Fatalf("expected truncated but complete prompt, got %+v", done.Data)
	}
}

// Regression P0-2: --mock must not overwrite the persisted provider configuration.
func TestMockModeDoesNotPersistProvider(t *testing.T) {
	const token = "mock-mode-token"
	ts, svc, _ := setupTestEnv(t, token)
	if err := svc.Repo().SaveSettings(map[string]string{"provider": "anthropic", "model": "claude-sonnet-5-5"}); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	svc.SetMockMode(true)

	cfg, _, hasKey, err := svc.LoadProviderConfig()
	if err != nil {
		t.Fatalf("LoadProviderConfig: %v", err)
	}
	if cfg.Provider != "mock" || !hasKey {
		t.Fatalf("expected mock provider in mock mode, got %q hasKey=%v", cfg.Provider, hasKey)
	}

	resp := doJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/settings", token, nil)
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["mockMode"] != true || out["provider"] != "anthropic" || out["model"] != "claude-sonnet-5-5" {
		t.Fatalf("settings response should expose mockMode and keep real config, got %+v", out)
	}

	stored, _ := svc.Repo().GetSettings()
	if stored["provider"] != "anthropic" || stored["model"] != "claude-sonnet-5-5" {
		t.Fatalf("persisted settings were modified: %+v", stored)
	}
}
