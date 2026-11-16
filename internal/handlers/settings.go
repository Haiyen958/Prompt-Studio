package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"prompt-studio/internal/ai"
	"prompt-studio/internal/server"
	"prompt-studio/internal/storage"
	"prompt-studio/internal/studio"
)

type SettingsHandler struct {
	svc *studio.Service
}

func NewSettingsHandler(svc *studio.Service) *SettingsHandler {
	return &SettingsHandler{svc: svc}
}

type SettingsResponse struct {
	Provider         string  `json:"provider"`
	BaseURL          string  `json:"baseUrl"`
	Model            string  `json:"model"`
	Temperature      float64 `json:"temperature"`
	MaxTokens        int     `json:"maxTokens"`
	TimeoutSec       int     `json:"timeoutSec"`
	Theme            string  `json:"theme"`
	PromptLanguage   string  `json:"promptLanguage"`
	DefaultTarget    string  `json:"defaultTarget"`
	LastCollectionID string  `json:"lastCollectionId"`
	HasKey           bool    `json:"hasKey"`
	KeyHint          string  `json:"keyHint"`
	MockMode         bool    `json:"mockMode"`
	Warning          string  `json:"warning,omitempty"`
}

type UpdateSettingsRequest struct {
	Provider       *string  `json:"provider"`
	BaseURL        *string  `json:"baseUrl"`
	Model          *string  `json:"model"`
	Temperature    *float64 `json:"temperature"`
	MaxTokens      *int     `json:"maxTokens"`
	TimeoutSec     *int     `json:"timeoutSec"`
	Theme          *string  `json:"theme"`
	PromptLanguage *string  `json:"promptLanguage"`
	DefaultTarget  *string  `json:"defaultTarget"`
	APIKey         *string  `json:"apiKey"`
	Tested         *bool    `json:"tested"`
}

type TestConnectionRequest struct {
	Provider   string `json:"provider"`
	BaseURL    string `json:"baseUrl"`
	Model      string `json:"model"`
	APIKey     string `json:"apiKey"`
	TimeoutSec int    `json:"timeoutSec"`
}

func (h *SettingsHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	resp, err := h.buildSettingsResponse()
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "settings_error", err.Error())
		return
	}
	server.WriteJSON(w, http.StatusOK, resp)
}

func (h *SettingsHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req UpdateSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid_json", "Dữ liệu JSON không hợp lệ")
		return
	}

	updates := make(map[string]string)
	if req.Provider != nil && strings.TrimSpace(*req.Provider) != "" {
		updates["provider"] = strings.ToLower(strings.TrimSpace(*req.Provider))
	}
	if req.BaseURL != nil {
		updates["base_url"] = strings.TrimSpace(*req.BaseURL)
	}
	if req.Model != nil && strings.TrimSpace(*req.Model) != "" {
		updates["model"] = strings.TrimSpace(*req.Model)
	}
	if req.Temperature != nil {
		updates["temperature"] = strconv.FormatFloat(*req.Temperature, 'f', 2, 64)
	}
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		updates["max_tokens"] = strconv.Itoa(*req.MaxTokens)
	}
	if req.TimeoutSec != nil && *req.TimeoutSec > 0 {
		updates["timeout_sec"] = strconv.Itoa(*req.TimeoutSec)
	}
	if req.Theme != nil && strings.TrimSpace(*req.Theme) != "" {
		updates["theme"] = strings.TrimSpace(*req.Theme)
	}
	if req.PromptLanguage != nil && strings.TrimSpace(*req.PromptLanguage) != "" {
		updates["prompt_language"] = strings.TrimSpace(*req.PromptLanguage)
	}
	if req.DefaultTarget != nil && strings.TrimSpace(*req.DefaultTarget) != "" {
		updates["default_target"] = ai.NormalizeTarget(*req.DefaultTarget)
	}

	var warning string
	// Per PLAN.md §3: saving is NEVER blocked by skipped or failed Test Connection; only return a warning.
	if req.APIKey != nil && strings.TrimSpace(*req.APIKey) != "" {
		newKey := strings.TrimSpace(*req.APIKey)
		if err := h.svc.Repo().SetAPIKey(newKey); err != nil {
			server.WriteError(w, http.StatusInternalServerError, "key_save_error", err.Error())
			return
		}
		if req.Tested == nil || !*req.Tested {
			warning = "Đã lưu cấu hình, nhưng API key mới chưa được kiểm tra kết nối (Test Connection)."
		}
	}

	if len(updates) > 0 {
		if err := h.svc.Repo().SaveSettings(updates); err != nil {
			server.WriteError(w, http.StatusInternalServerError, "settings_save_error", err.Error())
			return
		}
	}

	resp, err := h.buildSettingsResponse()
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "settings_error", err.Error())
		return
	}
	resp.Warning = warning
	server.WriteJSON(w, http.StatusOK, resp)
}

func (h *SettingsHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	var req TestConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid_json", "Dữ liệu JSON không hợp lệ")
		return
	}

	currentCfg, _, _, _ := h.svc.LoadProviderConfig()
	provider := defaultStr(req.Provider, currentCfg.Provider)
	baseURL := strings.TrimSpace(req.BaseURL)
	if baseURL == "" && strings.EqualFold(provider, currentCfg.Provider) {
		baseURL = currentCfg.BaseURL
	}
	model := defaultStr(req.Model, currentCfg.Model)
	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" {
		apiKey = currentCfg.APIKey
	}

	timeout := 15 * time.Second
	if req.TimeoutSec > 0 {
		timeout = time.Duration(req.TimeoutSec) * time.Second
	}

	start := time.Now()
	prov, err := h.svc.
		LoadTestProvider(ai.ProviderConfig{
			Provider: provider,
			BaseURL:  baseURL,
			Model:    model,
			APIKey:   apiKey,
			Timeout:  timeout,
		})
	if err != nil {
		server.WriteJSON(w, http.StatusOK, map[string]any{
			"ok":        false,
			"latencyMs": time.Since(start).Milliseconds(),
			"code":      ai.ErrorCode(err),
			"error":     err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	pingErr := prov.Ping(ctx)
	latencyMs := time.Since(start).Milliseconds()

	if pingErr != nil {
		server.WriteJSON(w, http.StatusOK, map[string]any{
			"ok":        false,
			"latencyMs": latencyMs,
			"code":      ai.ErrorCode(pingErr),
			"error":     pingErr.Error(),
		})
		return
	}

	server.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"latencyMs": latencyMs,
	})
}

func (h *SettingsHandler) DeleteKey(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Repo().DeleteAPIKey(); err != nil {
		server.WriteError(w, http.StatusInternalServerError, "key_delete_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SettingsHandler) ExportData(w http.ResponseWriter, r *http.Request) {
	bundle, err := h.svc.Repo().ExportAll()
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "export_error", err.Error())
		return
	}
	filename := fmt.Sprintf("prompt-studio-backup-%s.json", time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(bundle)
}

func (h *SettingsHandler) ImportData(w http.ResponseWriter, r *http.Request) {
	var bundle storage.BackupBundle
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32*1024*1024)).Decode(&bundle); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid_backup", "File sao lưu JSON không hợp lệ: "+err.Error())
		return
	}
	if err := h.svc.Repo().ImportAll(&bundle); err != nil {
		server.WriteError(w, http.StatusBadRequest, "import_error", err.Error())
		return
	}
	server.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"sessions":    len(bundle.Sessions),
		"collections": len(bundle.Collections),
	})
}

func (h *SettingsHandler) buildSettingsResponse() (*SettingsResponse, error) {
	settings, err := h.svc.Repo().GetSettings()
	if err != nil {
		return nil, err
	}
	_, hint, hasKey, err := h.svc.Repo().GetAPIKey()
	if err != nil {
		return nil, err
	}
	prov := defaultStr(settings["provider"], "openai")
	if strings.EqualFold(prov, "mock") || strings.EqualFold(prov, "openai-compatible") {
		if strings.EqualFold(prov, "mock") {
			hasKey = true
			if hint == "" {
				hint = "••••••••••mock"
			}
		}
	}

	temp, _ := strconv.ParseFloat(settings["temperature"], 64)
	if temp <= 0 {
		temp = 0.4
	}
	maxTok, _ := strconv.Atoi(settings["max_tokens"])
	if maxTok <= 0 {
		maxTok = 2000
	}
	timeoutSec, _ := strconv.Atoi(settings["timeout_sec"])
	if timeoutSec <= 0 {
		timeoutSec = 60
	}

	return &SettingsResponse{
		Provider:         prov,
		BaseURL:          settings["base_url"],
		Model:            defaultStr(settings["model"], "gpt-4o-mini"),
		Temperature:      temp,
		MaxTokens:        maxTok,
		TimeoutSec:       timeoutSec,
		Theme:            defaultStr(settings["theme"], "dark"),
		PromptLanguage:   defaultStr(settings["prompt_language"], "match"),
		DefaultTarget:    defaultStr(settings["default_target"], "chat"),
		LastCollectionID: defaultStr(settings["last_collection_id"], storage.SystemCollectionID),
		HasKey:           hasKey,
		KeyHint:          hint,
		MockMode:         h.svc.MockMode(),
	}, nil
}

func defaultStr(v, fb string) string {
	if strings.TrimSpace(v) == "" {
		return fb
	}
	return strings.TrimSpace(v)
}
