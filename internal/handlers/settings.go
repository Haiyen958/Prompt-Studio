package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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

	warning, err := h.svc.UpdateSettings(studio.SettingsUpdate{
		Provider:       req.Provider,
		BaseURL:        req.BaseURL,
		Model:          req.Model,
		Temperature:    req.Temperature,
		MaxTokens:      req.MaxTokens,
		TimeoutSec:     req.TimeoutSec,
		Theme:          req.Theme,
		PromptLanguage: req.PromptLanguage,
		DefaultTarget:  req.DefaultTarget,
		APIKey:         req.APIKey,
		Tested:         req.Tested,
	})
	if err != nil {
		var invalid *studio.InvalidSettingError
		if errors.As(err, &invalid) {
			server.WriteJSON(w, http.StatusBadRequest, map[string]any{
				"code":  "invalid_settings",
				"field": invalid.Field,
				"error": "Cấu hình không hợp lệ — " + invalid.Error(),
			})
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "settings_save_error", err.Error())
		return
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
	sameProvider := strings.EqualFold(provider, currentCfg.Provider)
	baseURL := strings.TrimSpace(req.BaseURL)
	if baseURL == "" && sameProvider {
		baseURL = currentCfg.BaseURL
	}
	model := defaultStr(req.Model, currentCfg.Model)
	apiKey := strings.TrimSpace(req.APIKey)
	// The stored key only ever goes to the endpoint it was saved for (PLAN.md §2: the key
	// travels one way). Testing another provider or base URL requires typing the key again.
	if apiKey == "" && sameProvider && sameEndpoint(baseURL, currentCfg.BaseURL) {
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
	if err := h.svc.DeleteAPIKey(); err != nil {
		writeInternalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SettingsHandler) ExportData(w http.ResponseWriter, r *http.Request) {
	bundle, err := h.svc.Export()
	if err != nil {
		writeInternalError(w, r, err)
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
	// Body size is capped by server.Middleware (server.MaxImportBytes for this route).
	if err := json.NewDecoder(r.Body).Decode(&bundle); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid_backup", "File sao lưu JSON không hợp lệ: "+err.Error())
		return
	}
	if err := h.svc.Import(&bundle); err != nil {
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
	settings, hint, hasKey, err := h.svc.Settings()
	if err != nil {
		return nil, err
	}
	app := studio.ParseSettings(settings)
	if strings.EqualFold(app.Provider, "mock") {
		hasKey = true
		if hint == "" {
			hint = "••••••••••mock"
		}
	}

	return &SettingsResponse{
		Provider:         app.Provider,
		BaseURL:          app.BaseURL,
		Model:            app.Model,
		Temperature:      app.Temperature,
		MaxTokens:        app.MaxTokens,
		TimeoutSec:       app.TimeoutSec,
		Theme:            app.Theme,
		PromptLanguage:   app.PromptLanguage,
		DefaultTarget:    app.DefaultTarget,
		LastCollectionID: app.LastCollectionID,
		HasKey:           hasKey,
		KeyHint:          hint,
		MockMode:         h.svc.MockMode(),
	}, nil
}

// sameEndpoint compares base URLs ignoring surrounding spaces, trailing slashes and case.
func sameEndpoint(a, b string) bool {
	norm := func(s string) string { return strings.ToLower(strings.TrimRight(strings.TrimSpace(s), "/")) }
	return norm(a) == norm(b)
}

func defaultStr(v, fb string) string {
	if strings.TrimSpace(v) == "" {
		return fb
	}
	return strings.TrimSpace(v)
}
