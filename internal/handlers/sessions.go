package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"prompt-studio/internal/server"
	"prompt-studio/internal/storage"
	"prompt-studio/internal/studio"
)

type SessionsHandler struct {
	svc *studio.Service
}

const sessionNotFound = "Không tìm thấy phiên"

func NewSessionsHandler(svc *studio.Service) *SessionsHandler {
	return &SessionsHandler{svc: svc}
}

func (h *SessionsHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	savedParam := strings.ToLower(strings.TrimSpace(q.Get("saved")))
	savedOnly := savedParam == "1" || savedParam == "true"

	limit := clampInt(q.Get("limit"), 50, 1, 200)
	offset := clampInt(q.Get("offset"), 0, 0, 1<<30)

	// Fetch one extra row to know whether another page exists.
	list, err := h.svc.ListSessions(storage.SessionFilter{
		Query:        q.Get("q"),
		SavedOnly:    savedOnly,
		CollectionID: q.Get("collection"),
		Sort:         q.Get("sort"),
		Limit:        limit + 1,
		Offset:       offset,
	})
	if err != nil {
		writeInternalError(w, r, err)
		return
	}
	hasMore := len(list) > limit
	if hasMore {
		list = list[:limit]
	}
	if list == nil {
		list = []storage.Session{}
	}
	server.WriteJSON(w, http.StatusOK, map[string]any{
		"sessions":   list,
		"hasMore":    hasMore,
		"nextOffset": offset + len(list),
	})
}

func (h *SessionsHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	detail, err := h.svc.OpenSession(id)
	if err != nil {
		writeServiceError(w, r, err, sessionNotFound)
		return
	}
	server.WriteJSON(w, http.StatusOK, detail)
}

func (h *SessionsHandler) PasteSession(w http.ResponseWriter, r *http.Request) {
	var in studio.PasteInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid_json", "Dữ liệu JSON không hợp lệ")
		return
	}
	sess, ver, existingID, err := h.svc.PastePrompt(in)
	if err != nil {
		if errors.Is(err, storage.ErrDuplicateSession) {
			server.WriteJSON(w, http.StatusConflict, map[string]any{
				"code":              "duplicate_prompt",
				"error":             "Prompt này đã có trong thư viện",
				"existingSessionId": existingID,
			})
			return
		}
		if errors.Is(err, studio.ErrInvalidInput) {
			server.WriteError(w, http.StatusBadRequest, "invalid_paste", err.Error())
			return
		}
		writeServiceError(w, r, err, sessionNotFound)
		return
	}
	server.WriteJSON(w, http.StatusCreated, map[string]any{
		"session": sess,
		"version": ver,
	})
}

func (h *SessionsHandler) UpdateSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Title           *string `json:"title"`
		Description     *string `json:"description"`
		CollectionID    *string `json:"collectionId"`
		PinnedVersionID *string `json:"pinnedVersionId"`
		Target          *string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid_json", "Dữ liệu JSON không hợp lệ")
		return
	}

	sess, err := h.svc.UpdateSession(id, storage.SessionPatch{
		Title:           req.Title,
		Description:     req.Description,
		CollectionID:    req.CollectionID,
		PinnedVersionID: req.PinnedVersionID,
		Target:          req.Target,
	})
	if err != nil {
		writeServiceError(w, r, err, sessionNotFound)
		return
	}
	server.WriteJSON(w, http.StatusOK, sess)
}

func (h *SessionsHandler) SaveSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		CollectionID string `json:"collectionId"`
		Title        string `json:"title"`
		VersionID    string `json:"versionId"` // version to pin as the saved one; empty = latest
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			server.WriteError(w, http.StatusBadRequest, "invalid_json", "Dữ liệu JSON không hợp lệ")
			return
		}
	}

	sess, err := h.svc.SaveSession(id, req.CollectionID, req.Title, req.VersionID)
	if err != nil {
		writeServiceError(w, r, err, sessionNotFound)
		return
	}
	server.WriteJSON(w, http.StatusOK, sess)
}

func (h *SessionsHandler) UnsaveSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.svc.UnsaveSession(id); err != nil {
		writeServiceError(w, r, err, sessionNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SessionsHandler) DeleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.svc.DeleteSession(id); err != nil {
		writeServiceError(w, r, err, sessionNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func clampInt(raw string, def, min, max int) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return def
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}
