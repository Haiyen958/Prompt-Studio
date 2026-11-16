package handlers

import (
	"net/http"

	"prompt-studio/internal/ai"
	"prompt-studio/internal/server"
	"prompt-studio/internal/studio"
)

type StartersHandler struct {
	svc *studio.Service
}

func NewStartersHandler(svc *studio.Service) *StartersHandler {
	return &StartersHandler{svc: svc}
}

func (h *StartersHandler) ListStarters(w http.ResponseWriter, r *http.Request) {
	starters, err := ai.LoadStarters()
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "starters_error", err.Error())
		return
	}
	server.WriteJSON(w, http.StatusOK, map[string]any{"starters": starters})
}
