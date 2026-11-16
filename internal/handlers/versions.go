package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"prompt-studio/internal/ai"
	"prompt-studio/internal/server"
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
		if errors.Is(err, studio.ErrManualContentEmpty) {
			server.WriteError(w, http.StatusBadRequest, "empty_content", err.Error())
			return
		}
		writeServiceError(w, r, err, sessionNotFound)
		return
	}

	server.WriteJSON(w, http.StatusCreated, map[string]any{
		"version":   ver,
		"variables": ai.ExtractVariables(ver.Content),
	})
}
