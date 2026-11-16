package handlers

import (
	"encoding/json"
	"net/http"

	"prompt-studio/internal/server"
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
	// Validate before switching to SSE so input errors stay plain 400 JSON responses.
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
		_ = server.WriteSSEEvent(w, flusher, "error", sseErrorPayload(r, err))
	}
}

func (h *GenerateHandler) RefinePrompt(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	var req studio.RefineInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid_json", "Dữ liệu JSON không hợp lệ")
		return
	}

	// Resolve the session before switching to SSE so a bad id is a plain 404.
	if _, err := h.svc.SessionExists(sessionID); err != nil {
		writeServiceError(w, r, err, sessionNotFound)
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
		_ = server.WriteSSEEvent(w, flusher, "error", sseErrorPayload(r, err))
	}
}
