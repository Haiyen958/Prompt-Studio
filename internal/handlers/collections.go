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
	cols, totalSaved, err := h.svc.ListCollections()
	if err != nil {
		writeInternalError(w, r, err)
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
	col, err := h.svc.CreateCollection(req.Name)
	if err != nil {
		writeCollectionError(w, r, err)
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
	col, err := h.svc.RenameCollection(id, req.Name)
	if err != nil {
		writeCollectionError(w, r, err)
		return
	}
	server.WriteJSON(w, http.StatusOK, col)
}

func (h *CollectionsHandler) DeleteCollection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.svc.DeleteCollection(id); err != nil {
		writeCollectionError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeCollectionError keeps the historical `collection_error` code for invalid names.
func writeCollectionError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, studio.ErrInvalidInput) {
		server.WriteError(w, http.StatusBadRequest, "collection_error", err.Error())
		return
	}
	writeServiceError(w, r, err, "Không tìm thấy collection")
}
