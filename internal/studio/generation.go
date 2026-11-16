package studio

import (
	"context"
	"strings"
	"time"

	"prompt-studio/internal/ai"
	"prompt-studio/internal/storage"
)

// promptStream describes one generate/refine LLM call that produces a prompt version.
type promptStream struct {
	cfg      ai.ProviderConfig
	prov     ai.Provider
	rendered string // meta-prompt sent as the single user message
	// draft carries the version fields decided by the caller: SessionID, ParentID, Source,
	// Target, Instruction, TemplateVersion. Draft.Assumptions are the fallback used when the
	// LLM returns no assumptions (refine keeps the base version's; generate has none).
	draft storage.PromptVersion
	// beforeSave runs after a complete stream, before the version is stored (generate
	// uses it to turn meta.intent into the session title).
	beforeSave func(ai.SplitResult)
}

// promptStreamResult reports what runPromptStream did.
type promptStreamResult struct {
	Version   *storage.PromptVersion // nil when nothing was stored
	Partial   bool                   // Version holds output cut short by StreamErr
	StreamErr error                  // provider error (timeout, cancel, auth, …)
	Usage     ai.Usage
	LatencyMs int
}

// runPromptStream streams the meta-prompt, forwards prompt deltas as SSE `delta` events
// (the ===META=== block is never forwarded), and stores the resulting version:
//   - stream error after some output → the partial prompt is stored as incomplete
//     (no assumptions), result.Partial = true, result.StreamErr set;
//   - stream error before any output → nothing stored, result.StreamErr set;
//   - success → meta assumptions (or draft fallback) stored; a prompt cut by max tokens
//     before ===META=== is marked incomplete; an empty prompt is ai.ErrBadResponse.
//
// The returned error is non-nil only for failures after a successful stream (empty prompt,
// storage error); stream failures are reported through result.StreamErr.
func (s *Service) runPromptStream(ctx context.Context, job promptStream, emit func(SSEEvent)) (promptStreamResult, error) {
	start := time.Now()
	splitter := ai.NewStreamSplitter(func(delta string) {
		if emit != nil {
			emit(SSEEvent{Event: "delta", Data: map[string]string{"delta": delta}})
		}
	})

	usage, streamErr := job.prov.Stream(ctx, ai.Request{
		Messages:    []ai.Message{{Role: "user", Content: job.rendered}},
		Temperature: job.cfg.Temperature,
		MaxTokens:   job.cfg.MaxTokens,
	}, splitter.Feed)
	res := promptStreamResult{Usage: usage, LatencyMs: int(time.Since(start).Milliseconds())}

	v := job.draft
	v.Model = job.cfg.Model
	v.InputTokens = usage.InputTokens
	v.OutputTokens = usage.OutputTokens
	v.LatencyMs = res.LatencyMs

	if streamErr != nil {
		res.StreamErr = streamErr
		partial := splitter.CurrentPrompt()
		if partial == "" {
			return res, nil
		}
		v.Content = partial
		v.Incomplete = true
		v.Assumptions = nil
		if saved, err := s.repo.CreateVersion(v); err == nil {
			res.Version = saved
			res.Partial = true
		}
		return res, nil
	}

	split := splitter.Finalize()
	content := strings.TrimSpace(split.Prompt)
	if content == "" {
		return res, ai.ErrBadResponse
	}
	if job.beforeSave != nil {
		job.beforeSave(split)
	}
	v.Content = content
	// Hitting max tokens before ===META=== means the prompt itself was cut mid-way.
	// Cut inside the meta block keeps a complete prompt (only the assumptions are lost).
	v.Incomplete = usage.Truncated && !split.MetaPresent
	if a := toStorageAssumptions(split.Meta.Assumptions); len(a) > 0 {
		v.Assumptions = a
	}
	saved, err := s.repo.CreateVersion(v)
	if err != nil {
		return res, err
	}
	res.Version = saved
	return res, nil
}

// donePayload builds the SSE `done` event data. Partial versions (stopped or failed
// mid-stream) carry fewer fields than completed ones; the frontend relies on both shapes.
func donePayload(sess storage.Session, res promptStreamResult) map[string]any {
	ver := res.Version
	data := map[string]any{
		"sessionId":  sess.ID,
		"session":    sess,
		"versionId":  ver.ID,
		"version":    ver,
		"incomplete": ver.Incomplete,
		"variables":  ai.ExtractVariables(ver.Content),
		"usage":      res.Usage,
		"latencyMs":  res.LatencyMs,
	}
	if !res.Partial {
		data["intent"] = sess.Title
		data["assumptions"] = ver.Assumptions
		data["truncated"] = res.Usage.Truncated
	}
	return data
}
