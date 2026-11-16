package ai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func openAIChunk(content, finish string) string {
	fr := "null"
	if finish != "" {
		fr = fmt.Sprintf("%q", finish)
	}
	return fmt.Sprintf(`data: {"choices":[{"delta":{"content":%q},"finish_reason":%s}]}`+"\n\n", content, fr)
}

// sseServer writes each chunk after `gap`, then optionally stalls for `stall`.
func sseServer(t *testing.T, chunks []string, gap, stall time.Duration, done bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		for _, c := range chunks {
			select {
			case <-time.After(gap):
			case <-r.Context().Done():
				return
			}
			_, _ = w.Write([]byte(c))
			fl.Flush()
		}
		if stall > 0 {
			select {
			case <-time.After(stall):
			case <-r.Context().Done():
				return
			}
		}
		if done {
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			fl.Flush()
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Regression P0-4: a healthy stream longer than the timeout must not be cut off;
// the timeout applies to idle gaps, not to the total duration.
func TestOpenAIStreamTimeoutIsIdleNotTotal(t *testing.T) {
	var chunks []string
	for i := 0; i < 8; i++ {
		chunks = append(chunks, openAIChunk(fmt.Sprintf("phần %d ", i), ""))
	}
	srv := sseServer(t, chunks, 60*time.Millisecond, 0, true) // ~480ms total
	p := &OpenAIProvider{BaseURL: srv.URL, Model: "m", Timeout: 250 * time.Millisecond}

	var got strings.Builder
	_, err := p.Stream(context.Background(), Request{Messages: []Message{{Role: "user", Content: "x"}}}, func(d string) {
		got.WriteString(d)
	})
	if err != nil {
		t.Fatalf("stream should succeed, got %v", err)
	}
	if !strings.Contains(got.String(), "phần 7") {
		t.Fatalf("stream was cut off: %q", got.String())
	}
}

func TestOpenAIStreamStallReturnsErrTimeout(t *testing.T) {
	srv := sseServer(t, []string{openAIChunk("bắt đầu", "")}, 0, 2*time.Second, true)
	p := &OpenAIProvider{BaseURL: srv.URL, Model: "m", Timeout: 200 * time.Millisecond}

	start := time.Now()
	_, err := p.Stream(context.Background(), Request{Messages: []Message{{Role: "user", Content: "x"}}}, nil)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("idle timeout fired too late: %v", time.Since(start))
	}
}

func TestOpenAIStreamUserCancelIsNotTimeout(t *testing.T) {
	srv := sseServer(t, []string{openAIChunk("bắt đầu", "")}, 0, 2*time.Second, true)
	p := &OpenAIProvider{BaseURL: srv.URL, Model: "m", Timeout: 5 * time.Second}

	ctx, cancel := context.WithCancel(context.Background())
	_, err := p.Stream(ctx, Request{Messages: []Message{{Role: "user", Content: "x"}}}, func(string) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestOpenAIStreamDetectsLengthTruncation(t *testing.T) {
	srv := sseServer(t, []string{openAIChunk("prompt dở", ""), openAIChunk("", "length")}, 0, 0, true)
	p := &OpenAIProvider{BaseURL: srv.URL, Model: "m", Timeout: time.Second}

	usage, err := p.Stream(context.Background(), Request{Messages: []Message{{Role: "user", Content: "x"}}}, nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if !usage.Truncated {
		t.Fatal("expected Truncated=true for finish_reason=length")
	}
}

func TestAnthropicStreamDetectsMaxTokens(t *testing.T) {
	chunks := []string{
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"prompt dở\"}}\n\n",
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"max_tokens\"},\"usage\":{\"output_tokens\":2000}}\n\n",
	}
	srv := sseServer(t, chunks, 0, 0, false)
	p := &AnthropicProvider{BaseURL: srv.URL, APIKey: "k", Model: "m", Timeout: time.Second}

	var got strings.Builder
	usage, err := p.Stream(context.Background(), Request{Messages: []Message{{Role: "user", Content: "x"}}}, func(d string) {
		got.WriteString(d)
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if !usage.Truncated || got.String() != "prompt dở" {
		t.Fatalf("expected truncated stream with text, got truncated=%v text=%q", usage.Truncated, got.String())
	}
}
