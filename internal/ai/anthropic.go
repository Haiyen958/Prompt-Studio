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

type AnthropicProvider struct {
	BaseURL    string
	APIKey     string
	Model      string
	Timeout    time.Duration
	HTTPClient *http.Client
}

func (p *AnthropicProvider) client() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	return &http.Client{Timeout: p.Timeout}
}

func (p *AnthropicProvider) streamClient() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	return streamHTTPClient
}

func (p *AnthropicProvider) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	body := map[string]any{
		"model":      p.Model,
		"max_tokens": 8,
		"messages": []map[string]string{
			{"role": "user", "content": "Reply with OK"},
		},
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/messages", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", strings.TrimSpace(p.APIKey))
	req.Header.Set("anthropic-version", "2023-06-01")

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

func (p *AnthropicProvider) Stream(ctx context.Context, r Request, onDelta func(string)) (Usage, error) {
	var usage Usage
	ctx, wd, stop := startStreamWatchdog(ctx, p.Timeout)
	defer stop()
	var systemParts []string
	var messages []map[string]string

	for _, m := range r.Messages {
		if m.Role == "system" {
			systemParts = append(systemParts, m.Content)
		} else {
			role := m.Role
			if role != "assistant" {
				role = "user"
			}
			messages = append(messages, map[string]string{
				"role":    role,
				"content": m.Content,
			})
		}
	}
	if len(messages) == 0 {
		messages = append(messages, map[string]string{"role": "user", "content": "Thực hiện theo hướng dẫn trên."})
	}

	payload := map[string]any{
		"model":       p.Model,
		"max_tokens":  r.MaxTokens,
		"temperature": r.Temperature,
		"messages":    messages,
		"stream":      true,
	}
	if len(systemParts) > 0 {
		payload["system"] = strings.Join(systemParts, "\n\n")
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return usage, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/messages", bytes.NewReader(b))
	if err != nil {
		return usage, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("x-api-key", strings.TrimSpace(p.APIKey))
	req.Header.Set("anthropic-version", "2023-06-01")

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

	for scanner.Scan() {
		wd.Touch()
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		var evt struct {
			Type    string `json:"type"`
			Message *struct {
				Usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			} `json:"message"`
			Delta *struct {
				Type       string `json:"type"`
				Text       string `json:"text"`
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
			Usage *struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			continue
		}
		if evt.Message != nil {
			usage.InputTokens = evt.Message.Usage.InputTokens
		}
		if evt.Usage != nil && evt.Usage.OutputTokens > 0 {
			usage.OutputTokens = evt.Usage.OutputTokens
		}
		if evt.Type == "message_delta" && evt.Delta != nil && evt.Delta.StopReason == "max_tokens" {
			usage.Truncated = true
		}
		if evt.Type == "content_block_delta" && evt.Delta != nil && evt.Delta.Text != "" {
			if onDelta != nil {
				onDelta(evt.Delta.Text)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return usage, wd.mapErr(err)
	}
	return usage, nil
}
