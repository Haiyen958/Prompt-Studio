package handlers

import (
	"io/fs"
	"net/http"

	"prompt-studio/internal/server"
	"prompt-studio/internal/studio"
)

func NewRouter(svc *studio.Service, webFS fs.FS, secCfg server.SecurityConfig) http.Handler {
	mux := http.NewServeMux()

	settingsH := NewSettingsHandler(svc)
	genH := NewGenerateHandler(svc)
	cmpH := NewCompareHandler(svc)
	sessH := NewSessionsHandler(svc)
	verH := NewVersionsHandler(svc)
	colH := NewCollectionsHandler(svc)
	startH := NewStartersHandler(svc)

	// Settings & Backup API
	mux.HandleFunc("GET /api/settings", settingsH.GetSettings)
	mux.HandleFunc("PUT /api/settings", settingsH.UpdateSettings)
	mux.HandleFunc("POST /api/settings/test", settingsH.TestConnection)
	mux.HandleFunc("DELETE /api/settings/key", settingsH.DeleteKey)
	mux.HandleFunc("GET /api/export", settingsH.ExportData)
	mux.HandleFunc("POST /api/import", settingsH.ImportData)

	// Sessions, Generate, Paste, Refine, Versions & Save API
	mux.HandleFunc("POST /api/sessions", genH.CreateSessionAndGenerate)
	mux.HandleFunc("POST /api/sessions/paste", sessH.PasteSession)
	mux.HandleFunc("POST /api/sessions/{id}/refine", genH.RefinePrompt)
	mux.HandleFunc("POST /api/sessions/{id}/versions", verH.CreateManualVersion)
	mux.HandleFunc("POST /api/versions/{id}/compare", cmpH.CompareVersion)

	mux.HandleFunc("GET /api/sessions", sessH.ListSessions)
	mux.HandleFunc("GET /api/sessions/{id}", sessH.GetSession)
	mux.HandleFunc("PATCH /api/sessions/{id}", sessH.UpdateSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", sessH.DeleteSession)
	mux.HandleFunc("PUT /api/sessions/{id}/save", sessH.SaveSession)
	mux.HandleFunc("DELETE /api/sessions/{id}/save", sessH.UnsaveSession)

	// Collections API
	mux.HandleFunc("GET /api/collections", colH.ListCollections)
	mux.HandleFunc("POST /api/collections", colH.CreateCollection)
	mux.HandleFunc("PATCH /api/collections/{id}", colH.RenameCollection)
	mux.HandleFunc("DELETE /api/collections/{id}", colH.DeleteCollection)

	// Starter Templates API
	mux.HandleFunc("GET /api/starters", startH.ListStarters)

	// Static Web UI (embedded)
	if webFS != nil {
		fileServer := http.FileServer(http.FS(webFS))
		serveHTML := func(filename string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				b, err := fs.ReadFile(webFS, filename)
				if err != nil {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = w.Write(b)
			}
		}
		mux.HandleFunc("GET /library", serveHTML("index.html"))
		mux.HandleFunc("GET /settings", serveHTML("settings.html"))
		mux.HandleFunc("GET /showcase", serveHTML("showcase.html"))
		mux.Handle("GET /", fileServer)
	}

	return server.Middleware(secCfg, mux)
}
