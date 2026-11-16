package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"prompt-studio/internal/ai"
	"prompt-studio/internal/server"
	"prompt-studio/internal/storage"
	"prompt-studio/internal/studio"
)

type CompareHandler struct {
	svc *studio.Service
}

func NewCompareHandler(svc *studio.Service) *CompareHandler {
	return &CompareHandler{svc: svc}
}

func (h *CompareHandler) CompareVersion(w http.ResponseWriter, r *http.Request) {
	versionID := r.PathValue("id")
	var in studio.CompareInput
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil && !errors.Is(err, io.EOF) {
			server.WriteError(w, http.StatusBadRequest, "invalid_json", "Dữ liệu JSON không hợp lệ")
			return
		}
	}

	ver, sess, err := h.svc.Repo().GetVersion(versionID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			server.WriteError(w, http.StatusNotFound, "not_found", "Không tìm thấy phiên bản prompt")
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if err := studio.CheckComparable(ver, sess, in.Variables); err != nil {
		var missing *studio.MissingVariablesError
		switch {
		case errors.As(err, &missing):
			server.WriteJSON(w, http.StatusBadRequest, map[string]any{
				"code":      "missing_variables",
				"error":     err.Error(),
				"variables": missing.Names,
			})
		case errors.Is(err, studio.ErrCompareIncomplete):
			server.WriteError(w, http.StatusBadRequest, "compare_incomplete", err.Error())
		default:
			server.WriteError(w, http.StatusBadRequest, "compare_not_allowed", err.Error())
		}
		return
	}

	flusher, ok := server.PrepareSSE(w)
	if !ok {
		server.WriteError(w, http.StatusInternalServerError, "sse_unsupported", "Không hỗ trợ streaming")
		return
	}

	if err := h.svc.CompareVersion(r.Context(), versionID, in.Variables, func(evt studio.SSEEvent) {
		_ = server.WriteSSEEvent(w, flusher, evt.Event, evt.Data)
	}); err != nil {
		_ = server.WriteSSEEvent(w, flusher, "error", map[string]any{
			"code":    ai.ErrorCode(err),
			"message": err.Error(),
		})
	}
}
