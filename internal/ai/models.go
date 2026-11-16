package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// ModelInfo is one entry of a provider's model catalog.
type ModelInfo struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"` // human-readable name when the provider gives one
}

// ModelLister is implemented by providers that can list their models (used by Settings).
// It is optional so test doubles of Provider need not implement it.
type ModelLister interface {
	ListModels(ctx context.Context) ([]ModelInfo, error)
}

var (
	// ErrModelRequired: the provider has no safe default model; the user must pick one.
	ErrModelRequired = errors.New("no model selected for this provider")
	// ErrModelsUnsupported: the configured provider cannot list models.
	ErrModelsUnsupported = errors.New("this provider cannot list models")
)

const listModelsTimeout = 15 * time.Second

// openAINonChatPrefixes are official OpenAI model families that cannot write prompts
// (embeddings, speech, images, moderation). Everything else is listed.
var openAINonChatPrefixes = []string{
	"text-embedding", "tts-", "whisper", "dall-e", "omni-moderation", "text-moderation",
	"gpt-image", "babbage", "davinci",
}

func isOpenAIChatModel(id string) bool {
	for _, p := range openAINonChatPrefixes {
		if strings.HasPrefix(id, p) {
			return false
		}
	}
	for _, s := range []string{"-transcribe", "-tts", "-realtime", "-audio", "-search"} {
		if strings.Contains(id, s) {
			return false
		}
	}
	return true
}

// statusError maps a non-2xx response to the shared error values.
func statusError(resp *http.Response) error {
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ErrAuth
	case resp.StatusCode == http.StatusTooManyRequests:
		return ErrRateLimit
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%w: status %d: %s", ErrBadResponse, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}

func getJSON(ctx context.Context, client *http.Client, rawURL string, headers map[string]string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return ErrTimeout
		}
		return err
	}
	defer resp.Body.Close()
	if err := statusError(resp); err != nil {
		return err
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out); err != nil {
		return fmt.Errorf("%w: model list: %v", ErrBadResponse, err)
	}
	return nil
}

// ListModels returns GET {base}/models. Official OpenAI results exclude non-chat
// families; OpenAI-compatible servers are listed as is.
func (p *OpenAIProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, listModelsTimeout)
	defer cancel()
	headers := map[string]string{}
	if k := strings.TrimSpace(p.APIKey); k != "" {
		headers["Authorization"] = "Bearer " + k
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := getJSON(ctx, p.client(), p.BaseURL+"/models", headers, &body); err != nil {
		return nil, err
	}
	out := make([]ModelInfo, 0, len(body.Data))
	for _, m := range body.Data {
		if m.ID == "" || (!p.OpenAICompatible && !isOpenAIChatModel(m.ID)) {
			continue
		}
		out = append(out, ModelInfo{ID: m.ID})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// ListModels returns GET {base}/models (newest first, as Anthropic orders them),
// following pagination.
func (p *AnthropicProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, listModelsTimeout)
	defer cancel()
	headers := map[string]string{
		"x-api-key":         strings.TrimSpace(p.APIKey),
		"anthropic-version": "2023-06-01",
	}
	var out []ModelInfo
	after := ""
	for page := 0; page < 10; page++ {
		q := url.Values{"limit": {"1000"}}
		if after != "" {
			q.Set("after_id", after)
		}
		var body struct {
			Data []struct {
				ID          string `json:"id"`
				DisplayName string `json:"display_name"`
			} `json:"data"`
			HasMore bool   `json:"has_more"`
			LastID  string `json:"last_id"`
		}
		if err := getJSON(ctx, p.client(), p.BaseURL+"/models?"+q.Encode(), headers, &body); err != nil {
			return nil, err
		}
		for _, m := range body.Data {
			if m.ID != "" {
				out = append(out, ModelInfo{ID: m.ID, Name: m.DisplayName})
			}
		}
		if !body.HasMore || body.LastID == "" {
			break
		}
		after = body.LastID
	}
	return out, nil
}

// ListModels returns the single offline model.
func (m *MockProvider) ListModels(context.Context) ([]ModelInfo, error) {
	return []ModelInfo{{ID: m.Model, Name: "Mock (offline)"}}, nil
}
