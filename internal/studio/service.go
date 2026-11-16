package studio

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"prompt-studio/internal/ai"
	"prompt-studio/internal/storage"
)

const (
	MaxRequestChars = 8000
	MaxPasteChars   = 32000
)

var (
	ErrRequestEmpty      = errors.New("yêu cầu không được để trống")
	ErrRequestTooLong    = fmt.Errorf("yêu cầu vượt quá giới hạn %d ký tự", MaxRequestChars)
	ErrPasteEmpty        = errors.New("nội dung prompt không được để trống")
	ErrPasteTooLong      = fmt.Errorf("nội dung dán vượt quá giới hạn %d ký tự", MaxPasteChars)
	ErrCompareNotAllowed = errors.New("phiên dán prompt không có yêu cầu gốc để so sánh")
	ErrCompareIncomplete = errors.New("phiên bản chưa xong không thể so sánh — hãy Tạo lại trước")
)

// MissingVariablesError lists prompt variables that have no test value.
type MissingVariablesError struct {
	Names []string
}

func (e *MissingVariablesError) Error() string {
	return "cần nhập Dữ liệu thử cho: " + strings.Join(e.Names, ", ")
}

type SSEEvent struct {
	Event string `json:"event"` // "session" | "delta" | "side_done" | "done" | "error"
	Data  any    `json:"data"`
}

type GenerateInput struct {
	Request  string `json:"request"`
	Language string `json:"language"`
	Target   string `json:"target"`
}

type RefineInput struct {
	FromVersionID string `json:"fromVersionId"`
	Instruction   string `json:"instruction"`
	Source        string `json:"source"` // "improve" | "regenerate" | "assumption"
	Target        string `json:"target"`
}

type PasteInput struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	Content      string `json:"content"`
	CollectionID string `json:"collectionId"`
	Target       string `json:"target"`
}

type CompareInput struct {
	Variables map[string]string `json:"variables"`
}

// MockModel is the model ID reported while the server runs with --mock.
const MockModel = "mock-prompt-engineer-v2"

type Service struct {
	repo            *storage.Repo
	providerFactory func(cfg ai.ProviderConfig) (ai.Provider, error)
	mockMode        bool
}

func NewService(repo *storage.Repo) *Service {
	return &Service{
		repo:            repo,
		providerFactory: ai.NewProvider,
	}
}

func (s *Service) SetProviderFactory(fn func(cfg ai.ProviderConfig) (ai.Provider, error)) {
	s.providerFactory = fn
}

// SetMockMode routes every LLM call to the offline mock provider for this process only.
// It never touches persisted settings, so the user's real provider config survives.
func (s *Service) SetMockMode(on bool) {
	s.mockMode = on
}

func (s *Service) MockMode() bool {
	return s.mockMode
}

func (s *Service) LoadTestProvider(cfg ai.ProviderConfig) (ai.Provider, error) {
	return s.providerFactory(cfg)
}

func (s *Service) Repo() *storage.Repo {
	return s.repo
}

func (s *Service) LoadProviderConfig() (ai.ProviderConfig, string, bool, error) {
	settings, err := s.repo.GetSettings()
	if err != nil {
		return ai.ProviderConfig{}, "", false, err
	}
	apiKey, hint, hasKey, err := s.repo.GetAPIKey()
	if err != nil {
		return ai.ProviderConfig{}, "", false, err
	}

	app := ParseSettings(settings)
	cfg := ai.ProviderConfig{
		// Provider and model stay raw: an empty model lets ai.NewProvider pick the
		// provider-specific default instead of the OpenAI one.
		Provider:    settings["provider"],
		BaseURL:     settings["base_url"],
		Model:       settings["model"],
		APIKey:      apiKey,
		Temperature: app.Temperature,
		MaxTokens:   app.MaxTokens,
		Timeout:     app.Timeout(),
	}
	if s.mockMode {
		cfg.Provider = "mock"
		cfg.Model = MockModel
		hasKey = true
	}
	return cfg, hint, hasKey, nil
}

func ValidateRequestText(req string) error {
	trimmed := strings.TrimSpace(req)
	if trimmed == "" {
		return ErrRequestEmpty
	}
	if utf8.RuneCountInString(trimmed) > MaxRequestChars {
		return ErrRequestTooLong
	}
	return nil
}

func ValidatePasteText(content string) error {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return ErrPasteEmpty
	}
	if utf8.RuneCountInString(trimmed) > MaxPasteChars {
		return ErrPasteTooLong
	}
	return nil
}

// ErrManualContentEmpty is returned when a manual edit has no content.
var ErrManualContentEmpty = errors.New("nội dung prompt không được để trống")

// CreateManualVersion appends a hand-edited version. parentID, when given, must be a
// version of the same session (storage.ErrVersionMismatch otherwise); when empty the
// latest version is the parent. The new version inherits the parent's assumptions and target.
func (s *Service) CreateManualVersion(sessionID, parentID, content, instruction string) (*storage.PromptVersion, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, ErrManualContentEmpty
	}
	detail, err := s.repo.GetSessionDetail(sessionID, false)
	if err != nil {
		return nil, err
	}

	var parent *storage.PromptVersion
	parentID = strings.TrimSpace(parentID)
	if parentID != "" {
		pv, _, pErr := s.repo.GetVersion(parentID)
		if errors.Is(pErr, storage.ErrNotFound) || (pErr == nil && pv.SessionID != sessionID) {
			return nil, storage.ErrVersionMismatch
		}
		if pErr != nil {
			return nil, pErr
		}
		parent = pv
	} else if len(detail.Versions) > 0 {
		parent = &detail.Versions[len(detail.Versions)-1]
	}

	instr := strings.TrimSpace(instruction)
	if instr == "" {
		instr = "Sửa tay"
	}
	ver := storage.PromptVersion{
		SessionID:       sessionID,
		Source:          "manual",
		Instruction:     instr,
		Content:         content,
		TemplateVersion: "manual",
		Model:           "manual",
	}
	if parent != nil {
		ver.ParentID = parent.ID
		ver.Assumptions = parent.Assumptions
		ver.Target = parent.Target
	}
	return s.repo.CreateVersion(ver)
}

func (s *Service) PastePrompt(in PasteInput) (*storage.Session, *storage.PromptVersion, string, error) {
	if err := ValidatePasteText(in.Content); err != nil {
		return nil, nil, "", err
	}
	target := ai.NormalizeTarget(in.Target)
	if strings.TrimSpace(in.Target) == "" && strings.Contains(in.Content, ai.UserSeparator) {
		target = "api"
	}
	return s.repo.CreatePastedSession(
		strings.TrimSpace(in.Title),
		strings.TrimSpace(in.Description),
		strings.TrimSpace(in.Content),
		strings.TrimSpace(in.CollectionID),
		target,
	)
}

func (s *Service) StartSessionAndGenerate(ctx context.Context, in GenerateInput, emit func(SSEEvent)) (*storage.SessionDetail, error) {
	if err := ValidateRequestText(in.Request); err != nil {
		return nil, err
	}

	cfg, _, _, err := s.LoadProviderConfig()
	if err != nil {
		return nil, err
	}
	prov, err := s.providerFactory(cfg)
	if err != nil {
		return nil, err
	}

	settings, _ := s.repo.GetSettings()
	language := defaultOpt(in.Language, settings["prompt_language"], "match")
	target := ai.NormalizeTarget(defaultOpt(in.Target, settings["default_target"], "chat"))

	_ = s.repo.SaveSettings(map[string]string{
		"default_target": target,
	})

	reqText := strings.TrimSpace(in.Request)
	sess, err := s.repo.CreateSession("generated", "", "", reqText, target)
	if err != nil {
		return nil, err
	}
	// A session that ends without any version (401/429/network error before the first
	// token, or Stop before any output) is useless history noise: remove it.
	versionSaved := false
	defer func() {
		if !versionSaved {
			_ = s.repo.DeleteSession(sess.ID)
		}
	}()

	if emit != nil {
		emit(SSEEvent{
			Event: "session",
			Data: map[string]any{
				"sessionId": sess.ID,
				"session":   sess,
			},
		})
	}

	rendered, err := ai.RenderGeneratePrompt(ai.GenerateTemplateData{
		Request:  reqText,
		Language: language,
		Target:   target,
	})
	if err != nil {
		return nil, err
	}

	res, err := s.runPromptStream(ctx, promptStream{
		cfg:      cfg,
		prov:     prov,
		rendered: rendered,
		draft: storage.PromptVersion{
			SessionID:       sess.ID,
			Source:          "generate",
			Target:          target,
			TemplateVersion: ai.GenerateTemplateVersion,
		},
		beforeSave: func(split ai.SplitResult) {
			if split.MetaValid && split.Meta.Intent != "" {
				title := split.Meta.Intent
				if updated, uErr := s.repo.UpdateSession(sess.ID, storage.SessionPatch{Title: &title}); uErr == nil {
					sess = updated
				}
			}
		},
	}, emit)
	versionSaved = res.Version != nil
	if err != nil {
		return nil, err
	}
	if res.Version != nil && emit != nil {
		emit(SSEEvent{Event: "done", Data: donePayload(*sess, res)})
	}
	if res.StreamErr != nil {
		// Stop (client cancel) with a saved partial version is a normal end, not an error.
		if res.Partial && errors.Is(res.StreamErr, context.Canceled) {
			return s.repo.GetSessionDetail(sess.ID, false)
		}
		return nil, res.StreamErr
	}
	return s.repo.GetSessionDetail(sess.ID, false)
}

func (s *Service) RefineSessionPrompt(ctx context.Context, sessionID string, in RefineInput, emit func(SSEEvent)) (*storage.PromptVersion, error) {
	detail, err := s.repo.GetSessionDetail(sessionID, false)
	if err != nil {
		return nil, err
	}

	var baseVer *storage.PromptVersion
	if strings.TrimSpace(in.FromVersionID) != "" {
		v, _, vErr := s.repo.GetVersion(strings.TrimSpace(in.FromVersionID))
		if vErr != nil {
			return nil, vErr
		}
		if v.SessionID != sessionID {
			return nil, storage.ErrNotFound
		}
		baseVer = v
	} else if len(detail.Versions) > 0 {
		last := detail.Versions[len(detail.Versions)-1]
		baseVer = &last
	} else {
		return nil, storage.ErrNotFound
	}

	cfg, _, _, err := s.LoadProviderConfig()
	if err != nil {
		return nil, err
	}
	prov, err := s.providerFactory(cfg)
	if err != nil {
		return nil, err
	}

	settings, _ := s.repo.GetSettings()
	language := defaultOpt("", settings["prompt_language"], "match")

	// Target belongs to each version: refining v1 (chat) as "api" must not relabel v1.
	target := baseVer.Target
	if target == "" {
		target = detail.Session.Target
	}
	if strings.TrimSpace(in.Target) != "" {
		target = ai.NormalizeTarget(in.Target)
	}
	if target != detail.Session.Target {
		// sessions.target only remembers the last-used target (dropdown default).
		if updated, uErr := s.repo.UpdateSession(sessionID, storage.SessionPatch{Target: &target}); uErr == nil {
			detail.Session = *updated
		}
	}

	instr := strings.TrimSpace(in.Instruction)
	source := normalizeRefineSource(in.Source, instr)

	// Per PLAN.md §5: refine context strictly includes ONLY original request, active version, and new instruction.
	rendered, err := ai.RenderRefinePrompt(ai.RefineTemplateData{
		Request:       detail.Session.Request,
		CurrentPrompt: baseVer.Content,
		Instruction:   instr,
		Language:      language,
		Target:        target,
	})
	if err != nil {
		return nil, err
	}

	res, err := s.runPromptStream(ctx, promptStream{
		cfg:      cfg,
		prov:     prov,
		rendered: rendered,
		draft: storage.PromptVersion{
			SessionID:       sessionID,
			ParentID:        baseVer.ID,
			Source:          source,
			Target:          target,
			Instruction:     instr,
			Assumptions:     baseVer.Assumptions, // kept when the LLM returns none
			TemplateVersion: ai.RefineTemplateVersion,
		},
	}, emit)
	if err != nil {
		return nil, err
	}
	if res.Version != nil && emit != nil {
		emit(SSEEvent{Event: "done", Data: donePayload(detail.Session, res)})
	}
	if res.StreamErr != nil {
		// Stop (client cancel) with a saved partial version is a normal end, not an error.
		if res.Partial && errors.Is(res.StreamErr, context.Canceled) {
			return res.Version, nil
		}
		return nil, res.StreamErr
	}
	return res.Version, nil
}

// CompareVersion executes raw request (side="raw") and active prompt (side="prompt")
// in parallel on the configured LLM without persisting outputs to SQLite.
func (s *Service) CompareVersion(ctx context.Context, versionID string, varValues map[string]string, emit func(SSEEvent)) error {
	ver, sess, err := s.repo.GetVersion(strings.TrimSpace(versionID))
	if err != nil {
		return err
	}
	if err := CheckComparable(ver, sess, varValues); err != nil {
		return err
	}

	cfg, _, _, err := s.LoadProviderConfig()
	if err != nil {
		return err
	}
	prov, err := s.providerFactory(cfg)
	if err != nil {
		return err
	}

	rawContent := sess.Request
	if extra := formatTestDataAppendix(varValues); extra != "" {
		rawContent = rawContent + "\n\n" + extra
	}
	rawMsgs := []ai.Message{{Role: "user", Content: rawContent}}

	filledPrompt := ai.FillVariables(ver.Content, varValues)
	var promptMsgs []ai.Message
	if ver.Target == "api" {
		if sys, usr, ok := ai.SplitAPIParts(filledPrompt); ok {
			promptMsgs = []ai.Message{
				{Role: "system", Content: sys},
				{Role: "user", Content: usr},
			}
		}
	}
	if len(promptMsgs) == 0 {
		promptMsgs = []ai.Message{{Role: "user", Content: filledPrompt}}
	}

	var mu sync.Mutex
	safeEmit := func(evt SSEEvent) {
		if emit == nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		emit(evt)
	}

	var wg sync.WaitGroup
	var firstErr error
	var failed int
	var errMu sync.Mutex

	runSide := func(side string, msgs []ai.Message) {
		defer wg.Done()
		start := time.Now()
		var fullText strings.Builder
		usage, sErr := prov.Stream(ctx, ai.Request{
			Messages:    msgs,
			Temperature: cfg.Temperature,
			MaxTokens:   cfg.MaxTokens,
		}, func(delta string) {
			fullText.WriteString(delta)
			safeEmit(SSEEvent{
				Event: "delta",
				Data: map[string]any{
					"side":  side,
					"delta": delta,
				},
			})
		})
		if sErr != nil {
			errMu.Lock()
			failed++
			if firstErr == nil {
				firstErr = sErr
			}
			errMu.Unlock()
			if !errors.Is(sErr, context.Canceled) {
				// Report per side so the UI shows the error in the right column while
				// the other side keeps streaming.
				safeEmit(SSEEvent{
					Event: "side_error",
					Data: map[string]any{
						"side":    side,
						"code":    ai.ErrorCode(sErr),
						"message": sErr.Error(),
					},
				})
			}
			return
		}
		safeEmit(SSEEvent{
			Event: "side_done",
			Data: map[string]any{
				"side":      side,
				"words":     len(strings.Fields(fullText.String())),
				"latencyMs": time.Since(start).Milliseconds(),
				"usage":     usage,
			},
		})
	}

	wg.Add(2)
	go runSide("raw", rawMsgs)
	go runSide("prompt", promptMsgs)
	wg.Wait()

	if firstErr != nil {
		if errors.Is(firstErr, context.Canceled) || ctx.Err() != nil {
			return nil
		}
		if failed == 2 {
			return firstErr
		}
	}

	safeEmit(SSEEvent{
		Event: "done",
		Data: map[string]any{
			"model":     cfg.Model,
			"versionId": ver.ID,
			"sessionId": sess.ID,
		},
	})
	return nil
}

// CheckComparable validates a compare request before any LLM call: pasted sessions have
// no original request, incomplete versions are not worth paying for, and unfilled
// {{variables}} would make the comparison meaningless.
func CheckComparable(ver *storage.PromptVersion, sess *storage.Session, varValues map[string]string) error {
	if sess.Origin == "pasted" || strings.TrimSpace(sess.Request) == "" {
		return ErrCompareNotAllowed
	}
	if ver.Incomplete {
		return ErrCompareIncomplete
	}
	var missing []string
	for _, name := range ai.ExtractVariables(ver.Content) {
		if strings.TrimSpace(varValues[name]) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return &MissingVariablesError{Names: missing}
	}
	return nil
}

func formatTestDataAppendix(vars map[string]string) string {
	if len(vars) == 0 {
		return ""
	}
	keys := make([]string, 0, len(vars))
	for k, v := range vars {
		if strings.TrimSpace(v) != "" {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("Dữ liệu thử:")
	for _, k := range keys {
		b.WriteString("\n- ")
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(strings.TrimSpace(vars[k]))
	}
	return b.String()
}

func toStorageAssumptions(list []ai.Assumption) []storage.AssumptionRecord {
	if len(list) == 0 {
		return nil
	}
	out := make([]storage.AssumptionRecord, 0, len(list))
	for _, a := range list {
		out = append(out, storage.AssumptionRecord{
			Label:        a.Label,
			Value:        a.Value,
			Alternatives: a.Alternatives,
		})
	}
	return out
}

func normalizeRefineSource(source, instr string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "assumption":
		return "assumption"
	case "regenerate":
		return "regenerate"
	case "improve":
		return "improve"
	default:
		if instr == "" {
			return "regenerate"
		}
		return "improve"
	}
}

func defaultOpt(v, settingVal, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	if strings.TrimSpace(settingVal) != "" {
		return strings.TrimSpace(settingVal)
	}
	return fallback
}
