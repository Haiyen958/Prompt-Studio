package ai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

var (
	ErrAuth        = errors.New("authentication failed (401/403): invalid or expired API key")
	ErrRateLimit   = errors.New("rate limit or quota exceeded (429)")
	ErrTimeout     = errors.New("upstream AI request timed out")
	ErrBadResponse = errors.New("upstream AI returned an invalid response")
	ErrNoAPIKey    = errors.New("AI provider API key is not configured")
)

type Message struct {
	Role    string `json:"role"` // "system" | "user" | "assistant"
	Content string `json:"content"`
}

type Request struct {
	Messages    []Message
	Temperature float64
	MaxTokens   int
}

type Usage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
	// Truncated is true when the provider stopped because it hit MaxTokens
	// (OpenAI finish_reason "length", Anthropic stop_reason "max_tokens").
	Truncated bool `json:"truncated,omitempty"`
}

type Provider interface {
	Stream(ctx context.Context, req Request, onDelta func(string)) (Usage, error)
	Ping(ctx context.Context) error
}

type ProviderConfig struct {
	Provider    string
	BaseURL     string
	Model       string
	APIKey      string
	Temperature float64
	MaxTokens   int
	Timeout     time.Duration
}

func NewProvider(cfg ProviderConfig) (Provider, error) {
	p := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 2000 // keep in sync with storage.DefaultSettings()["max_tokens"]
	}
	if p == "mock" {
		return NewMockProvider(cfg.Model), nil
	}
	if strings.TrimSpace(cfg.APIKey) == "" && p != "openai-compatible" {
		return nil, ErrNoAPIKey
	}

	switch p {
	case "openai", "":
		baseURL := strings.TrimSpace(cfg.BaseURL)
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		return &OpenAIProvider{
			BaseURL: strings.TrimRight(baseURL, "/"),
			APIKey:  cfg.APIKey,
			Model:   defaultString(cfg.Model, "gpt-4o-mini"),
			Timeout: cfg.Timeout,
		}, nil
	case "openai-compatible":
		baseURL := strings.TrimSpace(cfg.BaseURL)
		if baseURL == "" {
			baseURL = "http://127.0.0.1:11434/v1"
		}
		return &OpenAIProvider{
			BaseURL:          strings.TrimRight(baseURL, "/"),
			APIKey:           cfg.APIKey,
			Model:            defaultString(cfg.Model, "llama3.1"),
			Timeout:          cfg.Timeout,
			OpenAICompatible: true,
		}, nil
	case "anthropic":
		baseURL := strings.TrimSpace(cfg.BaseURL)
		if baseURL == "" {
			baseURL = "https://api.anthropic.com/v1"
		}
		return &AnthropicProvider{
			BaseURL: strings.TrimRight(baseURL, "/"),
			APIKey:  cfg.APIKey,
			// No default: Anthropic retires model IDs, so the user picks one from the
			// live list in Settings (Stream/Ping return ErrModelRequired until then).
			Model:   strings.TrimSpace(cfg.Model),
			Timeout: cfg.Timeout,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported provider %q (supported in MVP: openai, openai-compatible, anthropic)", cfg.Provider)
	}
}

func ErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrNoAPIKey):
		return "unconfigured"
	case errors.Is(err, ErrModelRequired):
		return "model_required"
	case errors.Is(err, ErrModelsUnsupported):
		return "unsupported"
	case errors.Is(err, ErrAuth):
		return "auth_error"
	case errors.Is(err, ErrRateLimit):
		return "rate_limit"
	case errors.Is(err, ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, ErrBadResponse):
		return "bad_response"
	default:
		return "internal_error"
	}
}

func defaultString(val, fallback string) string {
	if strings.TrimSpace(val) == "" {
		return fallback
	}
	return strings.TrimSpace(val)
}

// streamHTTPClient has no total timeout: http.Client.Timeout also covers reading the
// body, which would cut off a healthy stream that simply takes longer than the limit.
// Streams are bounded by streamWatchdog instead.
var streamHTTPClient = &http.Client{}

// streamWatchdog cancels a streaming request when no data arrives for `idle`
// (covers both time-to-first-byte and stalls between chunks).
type streamWatchdog struct {
	idle  time.Duration
	timer *time.Timer
	fired atomic.Bool
}

func startStreamWatchdog(parent context.Context, idle time.Duration) (context.Context, *streamWatchdog, func()) {
	if idle <= 0 {
		idle = 60 * time.Second
	}
	ctx, cancel := context.WithCancel(parent)
	w := &streamWatchdog{idle: idle}
	w.timer = time.AfterFunc(idle, func() {
		w.fired.Store(true)
		cancel()
	})
	return ctx, w, func() {
		w.timer.Stop()
		cancel()
	}
}

// Touch resets the idle deadline after receiving data.
func (w *streamWatchdog) Touch() {
	w.timer.Reset(w.idle)
}

// mapErr converts a cancellation caused by the watchdog into ErrTimeout while leaving
// user-initiated cancellation (context.Canceled from the parent) untouched.
func (w *streamWatchdog) mapErr(err error) error {
	if err == nil {
		return nil
	}
	if w.fired.Load() || errors.Is(err, context.DeadlineExceeded) {
		return ErrTimeout
	}
	return err
}
