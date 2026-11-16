package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"prompt-studio/internal/ai"
	"prompt-studio/internal/server"
	"prompt-studio/internal/storage"
	"prompt-studio/internal/studio"
)

type GenerateHandler struct {
	svc *studio.Service
}

func NewGenerateHandler(svc *studio.Service) *GenerateHandler {
	return &GenerateHandler{svc: svc}
}

func (h *GenerateHandler) CreateSessionAndGenerate(w http.ResponseWriter, r *http.Request) {
	var in studio.GenerateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid_json", "Dữ liệu JSON không hợp lệ")
		return
	}
	if err := studio.ValidateRequestText(in.Request); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	flusher, ok := server.PrepareSSE(w)
	if !ok {
		server.WriteError(w, http.StatusInternalServerError, "sse_unsupported", "Không hỗ trợ streaming")
		return
	}

	_, err := h.svc.StartSessionAndGenerate(r.Context(), in, func(evt studio.SSEEvent) {
		_ = server.WriteSSEEvent(w, flusher, evt.Event, evt.Data)
	})
	if err != nil {
		_ = server.WriteSSEEvent(w, flusher, "error", map[string]any{
			"code":    ai.ErrorCode(err),
			"message": err.Error(),
		})
	}
}

func (h *GenerateHandler) RefinePrompt(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	var req studio.RefineInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid_json", "Dữ liệu JSON không hợp lệ")
		return
	}

	if _, err := h.svc.Repo().GetSessionDetail(sessionID, false); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			server.WriteError(w, http.StatusNotFound, "not_found", "Không tìm thấy phiên")
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	flusher, ok := server.PrepareSSE(w)
	if !ok {
		server.WriteError(w, http.StatusInternalServerError, "sse_unsupported", "Không hỗ trợ streaming")
		return
	}

	_, err := h.svc.RefineSessionPrompt(r.Context(), sessionID, req, func(evt studio.SSEEvent) {
		_ = server.WriteSSEEvent(w, flusher, evt.Event, evt.Data)
	})
	if err != nil {
		_ = server.WriteSSEEvent(w, flusher, "error", map[string]any{
			"code":    ai.ErrorCode(err),
			"message": err.Error(),
		})
	}
}
