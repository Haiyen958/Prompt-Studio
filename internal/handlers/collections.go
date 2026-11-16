package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"prompt-studio/internal/server"
	"prompt-studio/internal/storage"
	"prompt-studio/internal/studio"
)

type CollectionsHandler struct {
	svc *studio.Service
}

func NewCollectionsHandler(svc *studio.Service) *CollectionsHandler {
	return &CollectionsHandler{svc: svc}
}

func (h *CollectionsHandler) ListCollections(w http.ResponseWriter, r *http.Request) {
	cols, totalSaved, err := h.svc.Repo().ListCollections()
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if cols == nil {
		cols = []storage.Collection{}
	}
	server.WriteJSON(w, http.StatusOK, map[string]any{
		"collections": cols,
		"totalSaved":  totalSaved,
	})
}

func (h *CollectionsHandler) CreateCollection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid_json", "Dữ liệu JSON không hợp lệ")
		return
	}
	col, err := h.svc.Repo().CreateCollection(req.Name)
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "collection_error", err.Error())
		return
	}
	server.WriteJSON(w, http.StatusCreated, col)
}

func (h *CollectionsHandler) RenameCollection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid_json", "Dữ liệu JSON không hợp lệ")
		return
	}
	col, err := h.svc.Repo().RenameCollection(id, req.Name)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			server.WriteError(w, http.StatusNotFound, "not_found", "Không tìm thấy collection")
			return
		}
		if errors.Is(err, storage.ErrProtectedResource) {
			server.WriteError(w, http.StatusForbidden, "protected_collection", err.Error())
			return
		}
		server.WriteError(w, http.StatusBadRequest, "collection_error", err.Error())
		return
	}
	server.WriteJSON(w, http.StatusOK, col)
}

func (h *CollectionsHandler) DeleteCollection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.svc.Repo().DeleteCollection(id); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			server.WriteError(w, http.StatusNotFound, "not_found", "Không tìm thấy collection")
			return
		}
		if errors.Is(err, storage.ErrProtectedResource) {
			server.WriteError(w, http.StatusForbidden, "protected_collection", err.Error())
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
