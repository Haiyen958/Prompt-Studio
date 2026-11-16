package studio

// Characterization tests for the generate/refine streaming pipeline (P1-b).
// They pin the CURRENT observable behavior — SSE event order, `done` payload keys,
// return values and persisted versions — so the shared pipeline refactor cannot
// silently change the contract the frontend relies on.

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"prompt-studio/internal/ai"
	"prompt-studio/internal/secrets"
	"prompt-studio/internal/storage"
)

const charMeta = "\n===META===\n" +
	`{"intent":"Tóm tắt biên bản họp","assumptions":[{"label":"Độ dài","value":"Ngắn","alternatives":["Dài","Vừa"]}]}`

// charProvider emits chunks; then cancels (cancel != nil) or returns usage/err.
type charProvider struct {
	chunks []string
	usage  ai.Usage
	err    error
	cancel context.CancelFunc
}

func (p *charProvider) Ping(context.Context) error { return nil }
func (p *charProvider) Stream(ctx context.Context, _ ai.Request, onDelta func(string)) (ai.Usage, error) {
	for _, c := range p.chunks {
		onDelta(c)
	}
	if p.cancel != nil {
		p.cancel()
		<-ctx.Done()
		return p.usage, ctx.Err()
	}
	return p.usage, p.err
}

func newCharService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	store := secrets.NewFileStore(filepath.Join(dir, "secrets.json"))
	db, err := storage.OpenDB(filepath.Join(dir, "char.db"), store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := NewService(storage.NewRepo(db, store))
	svc.SetMockMode(true) // provider config needs no key; the factory below decides the provider
	return svc
}

type recorder struct{ events []SSEEvent }

func (r *recorder) emit(e SSEEvent) { r.events = append(r.events, e) }

// names returns event names with consecutive deltas collapsed.
func (r *recorder) names() []string {
	var out []string
	for _, e := range r.events {
		if e.Event == "delta" && len(out) > 0 && out[len(out)-1] == "delta" {
			continue
		}
		out = append(out, e.Event)
	}
	return out
}

func (r *recorder) doneKeys() []string {
	for _, e := range r.events {
		if e.Event == "done" {
			m := e.Data.(map[string]any)
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			return keys
		}
	}
	return nil
}

func (r *recorder) done() map[string]any {
	for _, e := range r.events {
		if e.Event == "done" {
			return e.Data.(map[string]any)
		}
	}
	return nil
}

var (
	fullDoneKeys    = []string{"assumptions", "incomplete", "intent", "latencyMs", "session", "sessionId", "truncated", "usage", "variables", "version", "versionId"}
	partialDoneKeys = []string{"incomplete", "latencyMs", "session", "sessionId", "usage", "variables", "version", "versionId"}
)

type charCase struct {
	name      string
	provider  func(cancel context.CancelFunc) *charProvider
	wantErr   error // nil = success
	wantNames []string
	wantKeys  []string // nil = no done event
	// expectations for the saved version (nil = none saved)
	wantVersion *struct {
		incomplete  bool
		assumptions int
	}
	wantTruncated bool
}

func ver(incomplete bool, assumptions int) *struct {
	incomplete  bool
	assumptions int
} {
	return &struct {
		incomplete  bool
		assumptions int
	}{incomplete, assumptions}
}

func charCases() []charCase {
	return []charCase{
		{
			name: "success with meta",
			provider: func(context.CancelFunc) *charProvider {
				return &charProvider{chunks: []string{"## Vai trò\nBạn là {{vai_tro}}.", charMeta}}
			},
			wantNames:   []string{"delta", "done"},
			wantKeys:    fullDoneKeys,
			wantVersion: ver(false, 1),
		},
		{
			name: "success without meta",
			provider: func(context.CancelFunc) *charProvider {
				return &charProvider{chunks: []string{"## Vai trò\nBạn là trợ lý."}}
			},
			wantNames:   []string{"delta", "done"},
			wantKeys:    fullDoneKeys,
			wantVersion: ver(false, -1), // -1: generate 0, refine inherits base (1)
		},
		{
			name: "truncated before meta",
			provider: func(context.CancelFunc) *charProvider {
				return &charProvider{chunks: []string{"## Vai trò\nBạn là trợ"}, usage: ai.Usage{Truncated: true}}
			},
			wantNames:     []string{"delta", "done"},
			wantKeys:      fullDoneKeys,
			wantVersion:   ver(true, -1),
			wantTruncated: true,
		},
		{
			name: "truncated inside meta",
			provider: func(context.CancelFunc) *charProvider {
				return &charProvider{chunks: []string{"## Vai trò\nBạn là trợ lý.\n===META===\n{\"intent\":\"x"}, usage: ai.Usage{Truncated: true}}
			},
			wantNames:     []string{"delta", "done"},
			wantKeys:      fullDoneKeys,
			wantVersion:   ver(false, -1),
			wantTruncated: true,
		},
		{
			name: "timeout after partial output",
			provider: func(context.CancelFunc) *charProvider {
				return &charProvider{chunks: []string{"## Vai trò\nĐang viết dở"}, err: ai.ErrTimeout}
			},
			wantErr:     ai.ErrTimeout,
			wantNames:   []string{"delta", "done"},
			wantKeys:    partialDoneKeys,
			wantVersion: ver(true, 0),
		},
		{
			name: "stop after partial output",
			provider: func(cancel context.CancelFunc) *charProvider {
				return &charProvider{chunks: []string{"## Vai trò\nĐang viết dở"}, cancel: cancel}
			},
			wantNames:   []string{"delta", "done"},
			wantKeys:    partialDoneKeys,
			wantVersion: ver(true, 0),
		},
		{
			name:      "error before output",
			provider:  func(context.CancelFunc) *charProvider { return &charProvider{err: ai.ErrRateLimit} },
			wantErr:   ai.ErrRateLimit,
			wantNames: nil,
		},
		{
			name:      "stop before output",
			provider:  func(cancel context.CancelFunc) *charProvider { return &charProvider{cancel: cancel} },
			wantErr:   context.Canceled,
			wantNames: nil,
		},
		{
			name:      "meta only (empty prompt)",
			provider:  func(context.CancelFunc) *charProvider { return &charProvider{chunks: []string{charMeta}} },
			wantErr:   ai.ErrBadResponse,
			wantNames: nil,
		},
	}
}

func TestCharacterizeGenerateStream(t *testing.T) {
	for _, tc := range charCases() {
		t.Run(tc.name, func(t *testing.T) {
			svc := newCharService(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p := tc.provider(cancel)
			svc.SetProviderFactory(func(ai.ProviderConfig) (ai.Provider, error) { return p, nil })

			rec := &recorder{}
			detail, err := svc.StartSessionAndGenerate(ctx, GenerateInput{Request: "Viết prompt tóm tắt biên bản họp", Target: "api"}, rec.emit)

			if !errors.Is(err, tc.wantErr) && !(tc.wantErr == nil && err == nil) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			// Generate always announces the session first.
			wantNames := append([]string{"session"}, tc.wantNames...)
			if !reflect.DeepEqual(rec.names(), wantNames) {
				t.Fatalf("events = %v, want %v", rec.names(), wantNames)
			}
			if !reflect.DeepEqual(rec.doneKeys(), tc.wantKeys) {
				t.Fatalf("done keys = %v, want %v", rec.doneKeys(), tc.wantKeys)
			}

			sessions, _ := svc.Repo().ListSessions(storage.SessionFilter{})
			if tc.wantVersion == nil {
				if len(sessions) != 0 {
					t.Fatalf("no version expected, but %d session(s) remain", len(sessions))
				}
				if detail != nil {
					t.Fatalf("detail = %+v, want nil", detail)
				}
				return
			}
			if len(sessions) != 1 {
				t.Fatalf("sessions = %d, want 1", len(sessions))
			}
			d, _ := svc.Repo().GetSessionDetail(sessions[0].ID, false)
			if len(d.Versions) != 1 {
				t.Fatalf("versions = %d, want 1", len(d.Versions))
			}
			v := d.Versions[0]
			wantAssump := tc.wantVersion.assumptions
			if wantAssump < 0 {
				wantAssump = 0
			}
			if v.Incomplete != tc.wantVersion.incomplete || len(v.Assumptions) != wantAssump ||
				v.Source != "generate" || v.ParentID != "" || v.Target != "api" || v.TemplateVersion != ai.GenerateTemplateVersion {
				t.Fatalf("version = %+v", v)
			}
			// Success returns the detail; cancellation with a saved partial too; timeout returns nil.
			if (tc.wantErr == nil) != (detail != nil) {
				t.Fatalf("detail returned = %v with err %v", detail != nil, err)
			}
			if done := rec.done(); done != nil && tc.wantKeys[0] == "assumptions" {
				if done["truncated"] != tc.wantTruncated || done["incomplete"] != v.Incomplete {
					t.Fatalf("done truncated/incomplete = %v/%v", done["truncated"], done["incomplete"])
				}
				wantTitle := "Viết prompt tóm tắt biên bản họp"
				if tc.name == "success with meta" {
					wantTitle = "Tóm tắt biên bản họp"
				}
				if d.Session.Title != wantTitle || done["intent"] != wantTitle {
					t.Fatalf("title = %q intent = %v, want %q", d.Session.Title, done["intent"], wantTitle)
				}
			}
		})
	}
}

func TestCharacterizeRefineStream(t *testing.T) {
	for _, tc := range charCases() {
		t.Run(tc.name, func(t *testing.T) {
			svc := newCharService(t)
			// Base session: v1 with one assumption, target chat.
			base := &charProvider{chunks: []string{"## Vai trò\nBạn là trợ lý.", charMeta}}
			svc.SetProviderFactory(func(ai.ProviderConfig) (ai.Provider, error) { return base, nil })
			detail, err := svc.StartSessionAndGenerate(context.Background(), GenerateInput{Request: "Viết prompt", Target: "chat"}, nil)
			if err != nil {
				t.Fatal(err)
			}
			sessID, v1 := detail.Session.ID, detail.Versions[0]

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p := tc.provider(cancel)
			svc.SetProviderFactory(func(ai.ProviderConfig) (ai.Provider, error) { return p, nil })

			rec := &recorder{}
			got, err := svc.RefineSessionPrompt(ctx, sessID, RefineInput{FromVersionID: v1.ID, Instruction: "Ngắn hơn", Target: "agent"}, rec.emit)

			if !errors.Is(err, tc.wantErr) && !(tc.wantErr == nil && err == nil) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if !reflect.DeepEqual(rec.names(), tc.wantNames) {
				t.Fatalf("events = %v, want %v", rec.names(), tc.wantNames)
			}
			if !reflect.DeepEqual(rec.doneKeys(), tc.wantKeys) {
				t.Fatalf("done keys = %v, want %v", rec.doneKeys(), tc.wantKeys)
			}

			d, _ := svc.Repo().GetSessionDetail(sessID, false)
			if tc.wantVersion == nil {
				if len(d.Versions) != 1 || got != nil {
					t.Fatalf("versions = %d got = %+v, want only v1 and nil", len(d.Versions), got)
				}
				return
			}
			if len(d.Versions) != 2 {
				t.Fatalf("versions = %d, want 2", len(d.Versions))
			}
			v := d.Versions[1]
			wantAssump := tc.wantVersion.assumptions
			if wantAssump < 0 {
				wantAssump = 1 // refine keeps the base version's assumptions when meta is missing
			}
			if v.Incomplete != tc.wantVersion.incomplete || len(v.Assumptions) != wantAssump ||
				v.Source != "improve" || v.ParentID != v1.ID || v.Instruction != "Ngắn hơn" ||
				v.Target != "agent" || v.TemplateVersion != ai.RefineTemplateVersion {
				t.Fatalf("version = %+v", v)
			}
			// Success and stop-with-partial return the version; timeout returns nil.
			if (tc.wantErr == nil) != (got != nil) {
				t.Fatalf("returned version = %v with err %v", got != nil, err)
			}
			if got != nil && got.ID != v.ID {
				t.Fatalf("returned version %s, stored %s", got.ID, v.ID)
			}
			if done := rec.done(); done != nil && tc.wantKeys[0] == "assumptions" {
				if done["truncated"] != tc.wantTruncated || done["intent"] != d.Session.Title {
					t.Fatalf("done truncated = %v intent = %v", done["truncated"], done["intent"])
				}
			}
			if d.Session.Target != "agent" {
				t.Fatalf("session target = %q, want agent (last used)", d.Session.Target)
			}
		})
	}
}
