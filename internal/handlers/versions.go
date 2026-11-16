package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

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
	content := strings.TrimSpace(req.Content)
	if content == "" {
		server.WriteError(w, http.StatusBadRequest, "empty_content", "Nội dung prompt không được để trống")
		return
	}

	detail, err := h.svc.Repo().GetSessionDetail(sessionID, false)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			server.WriteError(w, http.StatusNotFound, "not_found", "Không tìm thấy phiên")
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	parentID := strings.TrimSpace(req.ParentID)
	var assumptions []storage.AssumptionRecord
	var target string
	if parentID != "" {
		if pv, _, pErr := h.svc.Repo().GetVersion(parentID); pErr == nil && pv.SessionID == sessionID {
			assumptions = pv.Assumptions
			target = pv.Target
		}
	} else if len(detail.Versions) > 0 {
		last := detail.Versions[len(detail.Versions)-1]
		parentID = last.ID
		assumptions = last.Assumptions
		target = last.Target
	}

	instr := strings.TrimSpace(req.Instruction)
	if instr == "" {
		instr = "Sửa tay"
	}

	ver, err := h.svc.Repo().CreateVersion(storage.PromptVersion{
		SessionID:       sessionID,
		ParentID:        parentID,
		Source:          "manual",
		Instruction:     instr,
		Content:         content,
		Assumptions:     assumptions,
		Incomplete:      false,
		Target:          target,
		TemplateVersion: "manual",
		Model:           "manual",
	})
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	server.WriteJSON(w, http.StatusCreated, map[string]any{
		"version":   ver,
		"variables": ai.ExtractVariables(ver.Content),
	})
}
