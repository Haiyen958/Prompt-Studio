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

type VersionsHandler struct {
	svc *studio.Service
}

func NewVersionsHandler(svc *studio.Service) *VersionsHandler {
	return &VersionsHandler{svc: svc}
}

type CreateManualVersionRequest struct {
	ParentID    string `json:"parentId"`
	Content     string `json:"content"`
	Instruction string `json:"instruction"`
}

func (h *VersionsHandler) CreateManualVersion(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	var req CreateManualVersionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid_json", "Dữ liệu JSON không hợp lệ")
		return
	}

	ver, err := h.svc.CreateManualVersion(sessionID, req.ParentID, req.Content, req.Instruction)
	if err != nil {
		switch {
		case errors.Is(err, studio.ErrManualContentEmpty):
			server.WriteError(w, http.StatusBadRequest, "empty_content", err.Error())
		case errors.Is(err, storage.ErrVersionMismatch):
			server.WriteError(w, http.StatusBadRequest, "version_mismatch", "Phiên bản gốc không thuộc phiên này")
		case errors.Is(err, storage.ErrNotFound):
			server.WriteError(w, http.StatusNotFound, "not_found", "Không tìm thấy phiên")
		default:
			server.WriteError(w, http.StatusInternalServerError, "db_error", err.Error())
		}
		return
	}

	server.WriteJSON(w, http.StatusCreated, map[string]any{
		"version":   ver,
		"variables": ai.ExtractVariables(ver.Content),
	})
}
