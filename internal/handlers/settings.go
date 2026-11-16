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
	cfg := h.svc.ProbeConfig(studio.ProbeRequest(req))

	start := time.Now()
	err := func() error {
		prov, err := h.svc.LoadTestProvider(cfg)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(r.Context(), cfg.Timeout)
		defer cancel()
		return prov.Ping(ctx)
	}()
	latencyMs := time.Since(start).Milliseconds()
	if err != nil {
		server.WriteJSON(w, http.StatusOK, probeFailure(r, err, latencyMs))
		return
	}
	server.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "latencyMs": latencyMs})
}

// ListModels returns the live model catalog of the provider in the form (before saving),
// with the same API-key policy as Test Connection. Failures are 200 {ok:false, code, error}
// like Test Connection, so Settings can show them inline.
func (h *SettingsHandler) ListModels(w http.ResponseWriter, r *http.Request) {
	var req TestConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid_json", "Dữ liệu JSON không hợp lệ")
		return
	}
	start := time.Now()
	models, err := h.svc.ListModels(r.Context(), h.svc.ProbeConfig(studio.ProbeRequest(req)))
	latencyMs := time.Since(start).Milliseconds()
	if err != nil {
		server.WriteJSON(w, http.StatusOK, probeFailure(r, err, latencyMs))
		return
	}
	if models == nil {
		models = []ai.ModelInfo{}
	}
	server.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "models": models, "latencyMs": latencyMs})
}

// probeFailure explains a failed Test Connection / model listing. Unlike generation
// errors, unknown failures (DNS, connection refused, bad URL) are shown verbatim: they
// are about the endpoint the user is typing, not server internals.
func probeFailure(r *http.Request, err error, latencyMs int64) map[string]any {
	code := ai.ErrorCode(err)
	msg := "Không kết nối được tới provider: " + err.Error()
	switch code {
	case "unconfigured", "auth_error", "rate_limit", "timeout", "bad_response", "model_required":
		msg = userMessage(r, code, err)
	case "unsupported":
		msg = "Provider này không hỗ trợ lấy danh sách model — hãy gõ Model ID."
	}
	return map[string]any{"ok": false, "latencyMs": latencyMs, "code": code, "error": msg}
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

func defaultStr(v, fb string) string {
	if strings.TrimSpace(v) == "" {
		return fb
	}
	return strings.TrimSpace(v)
}
