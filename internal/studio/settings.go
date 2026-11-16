package studio

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"prompt-studio/internal/ai"
	"prompt-studio/internal/storage"
)

// Accepted ranges and values for user-editable settings (mirrors web/settings.html).
const (
	MinTemperature = 0.0
	MaxTemperature = 2.0
	MinMaxTokens   = 128
	MaxMaxTokens   = 16384
	MinTimeoutSec  = 5
	MaxTimeoutSec  = 300
)

var (
	validProviders = []string{"openai", "anthropic", "openai-compatible", "mock"}
	validThemes    = []string{"dark", "light", "system"}
	validLanguages = []string{"match", "vi", "en"}
	validTargets   = []string{"chat", "api", "agent"}
)

// ErrInvalidSettings wraps every *InvalidSettingError.
var ErrInvalidSettings = errors.New("cấu hình không hợp lệ")

// InvalidSettingError names the rejected field (JSON name) and why.
type InvalidSettingError struct {
	Field  string
	Reason string
}

func (e *InvalidSettingError) Error() string { return fmt.Sprintf("%s: %s", e.Field, e.Reason) }
func (e *InvalidSettingError) Unwrap() error { return ErrInvalidSettings }

// AppSettings is the parsed, defaulted view of the settings table.
type AppSettings struct {
	Provider         string
	BaseURL          string
	Model            string
	Temperature      float64
	MaxTokens        int
	TimeoutSec       int
	Theme            string
	PromptLanguage   string
	DefaultTarget    string
	LastCollectionID string
}

// ParseSettings applies storage.DefaultSettings to missing or unparsable stored values.
// Stored values outside the editable ranges are kept (they may predate validation);
// only non-positive token/timeout values and negative temperatures fall back.
func ParseSettings(kv map[string]string) AppSettings {
	def := storage.DefaultSettings()
	get := func(key string) string {
		if v := strings.TrimSpace(kv[key]); v != "" {
			return v
		}
		return def[key]
	}
	defFloat := func(key string) float64 { f, _ := strconv.ParseFloat(def[key], 64); return f }
	defInt := func(key string) int { n, _ := strconv.Atoi(def[key]); return n }

	temp, err := strconv.ParseFloat(get("temperature"), 64)
	if err != nil || temp < 0 {
		temp = defFloat("temperature")
	}
	maxTok, err := strconv.Atoi(get("max_tokens"))
	if err != nil || maxTok <= 0 {
		maxTok = defInt("max_tokens")
	}
	timeoutSec, err := strconv.Atoi(get("timeout_sec"))
	if err != nil || timeoutSec <= 0 {
		timeoutSec = defInt("timeout_sec")
	}

	return AppSettings{
		Provider:         get("provider"),
		BaseURL:          strings.TrimSpace(kv["base_url"]),
		Model:            get("model"),
		Temperature:      temp,
		MaxTokens:        maxTok,
		TimeoutSec:       timeoutSec,
		Theme:            get("theme"),
		PromptLanguage:   get("prompt_language"),
		DefaultTarget:    ai.NormalizeTarget(get("default_target")),
		LastCollectionID: get("last_collection_id"),
	}
}

// Timeout returns the LLM idle timeout as a duration.
func (a AppSettings) Timeout() time.Duration { return time.Duration(a.TimeoutSec) * time.Second }

// SettingsUpdate is a partial update; nil fields are left unchanged.
type SettingsUpdate struct {
	Provider       *string
	BaseURL        *string
	Model          *string
	Temperature    *float64
	MaxTokens      *int
	TimeoutSec     *int
	Theme          *string
	PromptLanguage *string
	DefaultTarget  *string
	APIKey         *string
	Tested         *bool
}

// UpdateSettings validates the whole update before writing anything, stores a new API key
// (secrets file) and the other values (SQLite). Saving never depends on Test Connection
// (PLAN.md §2); an untested new key only yields a warning.
func (s *Service) UpdateSettings(u SettingsUpdate) (warning string, err error) {
	updates := make(map[string]string)

	if v, ok := trimmedNonEmpty(u.Provider); ok {
		v = strings.ToLower(v)
		if !contains(validProviders, v) {
			return "", &InvalidSettingError{"provider", "phải là một trong " + strings.Join(validProviders, ", ")}
		}
		updates["provider"] = v
	}
	if u.BaseURL != nil {
		updates["base_url"] = strings.TrimSpace(*u.BaseURL)
	}
	if v, ok := trimmedNonEmpty(u.Model); ok {
		updates["model"] = v
	}
	if u.Temperature != nil {
		t := *u.Temperature
		if t < MinTemperature || t > MaxTemperature {
			return "", &InvalidSettingError{"temperature", fmt.Sprintf("phải trong khoảng %.0f–%.0f", MinTemperature, MaxTemperature)}
		}
		updates["temperature"] = strconv.FormatFloat(t, 'f', 2, 64)
	}
	if u.MaxTokens != nil {
		if *u.MaxTokens < MinMaxTokens || *u.MaxTokens > MaxMaxTokens {
			return "", &InvalidSettingError{"maxTokens", fmt.Sprintf("phải trong khoảng %d–%d", MinMaxTokens, MaxMaxTokens)}
		}
		updates["max_tokens"] = strconv.Itoa(*u.MaxTokens)
	}
	if u.TimeoutSec != nil {
		if *u.TimeoutSec < MinTimeoutSec || *u.TimeoutSec > MaxTimeoutSec {
			return "", &InvalidSettingError{"timeoutSec", fmt.Sprintf("phải trong khoảng %d–%d giây", MinTimeoutSec, MaxTimeoutSec)}
		}
		updates["timeout_sec"] = strconv.Itoa(*u.TimeoutSec)
	}
	if v, ok := trimmedNonEmpty(u.Theme); ok {
		if !contains(validThemes, v) {
			return "", &InvalidSettingError{"theme", "phải là một trong " + strings.Join(validThemes, ", ")}
		}
		updates["theme"] = v
	}
	if v, ok := trimmedNonEmpty(u.PromptLanguage); ok {
		if !contains(validLanguages, v) {
			return "", &InvalidSettingError{"promptLanguage", "phải là một trong " + strings.Join(validLanguages, ", ")}
		}
		updates["prompt_language"] = v
	}
	if v, ok := trimmedNonEmpty(u.DefaultTarget); ok {
		if !contains(validTargets, strings.ToLower(v)) {
			return "", &InvalidSettingError{"defaultTarget", "phải là một trong " + strings.Join(validTargets, ", ")}
		}
		updates["default_target"] = strings.ToLower(v)
	}

	// A model ID only makes sense for the provider it belongs to: switching provider
	// requires picking a model in the same update (except the offline mock), and
	// Anthropic has no default model (IDs get retired).
	stored, err := s.repo.GetSettings()
	if err != nil {
		return "", err
	}
	effProvider := strings.ToLower(strings.TrimSpace(stored["provider"]))
	if v, ok := updates["provider"]; ok {
		if v != effProvider && v != "mock" {
			if _, picked := updates["model"]; !picked {
				return "", &InvalidSettingError{"model", "hãy chọn model cho provider mới"}
			}
		}
		effProvider = v
	}
	effModel := strings.TrimSpace(stored["model"])
	if v, ok := updates["model"]; ok {
		effModel = v
	}
	if effProvider == "anthropic" && effModel == "" {
		return "", &InvalidSettingError{"model", "Anthropic cần chọn model"}
	}

	if key, ok := trimmedNonEmpty(u.APIKey); ok {
		if err := s.repo.SetAPIKey(key); err != nil {
			return "", err
		}
		if u.Tested == nil || !*u.Tested {
			warning = "Đã lưu cấu hình, nhưng API key mới chưa được kiểm tra kết nối (Test Connection)."
		}
	}
	if len(updates) > 0 {
		if err := s.repo.SaveSettings(updates); err != nil {
			return "", err
		}
	}
	return warning, nil
}

func trimmedNonEmpty(p *string) (string, bool) {
	if p == nil {
		return "", false
	}
	v := strings.TrimSpace(*p)
	return v, v != ""
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// ProbeRequest is what Settings sends to test a connection or list models before saving.
type ProbeRequest struct {
	Provider   string
	BaseURL    string
	Model      string
	APIKey     string
	TimeoutSec int
}

// ProbeConfig resolves a provider config for Test Connection / model listing from the form
// values, falling back to the saved settings. The stored API key is only used for the
// saved provider + Base URL (PLAN.md §2: the key travels one way); probing another
// endpoint requires typing the key again.
func (s *Service) ProbeConfig(req ProbeRequest) ai.ProviderConfig {
	current, _, _, _ := s.LoadProviderConfig()
	provider := strings.TrimSpace(req.Provider)
	if provider == "" {
		provider = current.Provider
	}
	sameProvider := strings.EqualFold(provider, current.Provider)
	baseURL := strings.TrimSpace(req.BaseURL)
	if baseURL == "" && sameProvider {
		baseURL = current.BaseURL
	}
	model := strings.TrimSpace(req.Model)
	if model == "" && sameProvider {
		model = current.Model
	}
	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" && sameProvider && sameEndpoint(baseURL, current.BaseURL) {
		apiKey = current.APIKey
	}
	timeout := 15 * time.Second
	if req.TimeoutSec > 0 {
		timeout = time.Duration(req.TimeoutSec) * time.Second
	}
	return ai.ProviderConfig{Provider: provider, BaseURL: baseURL, Model: model, APIKey: apiKey, Timeout: timeout}
}

// ListModels returns the model catalog of the provider described by cfg.
func (s *Service) ListModels(ctx context.Context, cfg ai.ProviderConfig) ([]ai.ModelInfo, error) {
	prov, err := s.providerFactory(cfg)
	if err != nil {
		return nil, err
	}
	lister, ok := prov.(ai.ModelLister)
	if !ok {
		return nil, ai.ErrModelsUnsupported
	}
	return lister.ListModels(ctx)
}

// sameEndpoint compares base URLs ignoring surrounding spaces, trailing slashes and case.
func sameEndpoint(a, b string) bool {
	norm := func(s string) string { return strings.ToLower(strings.TrimRight(strings.TrimSpace(s), "/")) }
	return norm(a) == norm(b)
}
