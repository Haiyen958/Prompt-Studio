package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenAIProvider struct {
	BaseURL          string
	APIKey           string
	Model            string
	Timeout          time.Duration
	OpenAICompatible bool
	HTTPClient       *http.Client
}

func (p *OpenAIProvider) client() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	return &http.Client{Timeout: p.Timeout}
}

func (p *OpenAIProvider) streamClient() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	return streamHTTPClient
}

func (p *OpenAIProvider) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	body := map[string]any{
		"model": p.Model,
		"messages": []map[string]string{
			{"role": "user", "content": "Reply with OK"},
		},
		"max_tokens": 5,
		"stream":     false,
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(p.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(p.APIKey))
	}

	resp, err := p.client().Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return ErrTimeout
		}
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrAuth
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return ErrRateLimit
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%w: status %d: %s", ErrBadResponse, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}

func (p *OpenAIProvider) Stream(ctx context.Context, r Request, onDelta func(string)) (Usage, error) {
	var usage Usage
	ctx, wd, stop := startStreamWatchdog(ctx, p.Timeout)
	defer stop()
	payload := map[string]any{
		"model":       p.Model,
		"messages":    r.Messages,
		"temperature": r.Temperature,
		"max_tokens":  r.MaxTokens,
		"stream":      true,
	}
	// Official OpenAI reports real token usage in a final chunk only when asked.
	// OpenAI-compatible servers (Ollama, LM Studio, …) may reject unknown fields, so
	// they keep the character-based estimate below.
	if !p.OpenAICompatible {
		payload["stream_options"] = map[string]any{"include_usage": true}
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return usage, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return usage, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if strings.TrimSpace(p.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(p.APIKey))
	}

	resp, err := p.streamClient().Do(req)
	if err != nil {
		return usage, wd.mapErr(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return usage, ErrAuth
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return usage, ErrRateLimit
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return usage, fmt.Errorf("%w: status %d: %s", ErrBadResponse, resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var outChars int
	for scanner.Scan() {
		wd.Touch()
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Usage != nil {
			usage.InputTokens = chunk.Usage.PromptTokens
			usage.OutputTokens = chunk.Usage.CompletionTokens
		}
		for _, c := range chunk.Choices {
			if c.FinishReason == "length" {
				usage.Truncated = true
			}
			if c.Delta.Content != "" {
				outChars += len(c.Delta.Content)
				if onDelta != nil {
					onDelta(c.Delta.Content)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return usage, wd.mapErr(err)
	}
	if usage.OutputTokens == 0 && outChars > 0 {
		usage.OutputTokens = (outChars + 3) / 4
	}
	if usage.InputTokens == 0 {
		var inChars int
		for _, m := range r.Messages {
			inChars += len(m.Content)
		}
		usage.InputTokens = (inChars + 3) / 4
	}
	return usage, nil
}
