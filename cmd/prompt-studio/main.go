package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"prompt-studio/internal/handlers"
	"prompt-studio/internal/secrets"
	"prompt-studio/internal/security"
	"prompt-studio/internal/server"
	"prompt-studio/internal/storage"
	"prompt-studio/internal/studio"
	"prompt-studio/web"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:7788", "Địa chỉ lắng nghe local (mặc định 127.0.0.1:7788)")
	dataDir := flag.String("data-dir", "", "Thư mục dữ liệu SQLite và secrets.json (mặc định ~/.prompt-studio)")
	mockMode := flag.Bool("mock", false, "Chạy ở chế độ provider giả lập (offline demo / kiểm thử)")
	flag.Parse()

	if envAddr := os.Getenv("PROMPT_STUDIO_ADDR"); envAddr != "" {
		*addr = envAddr
	}
	resolvedDir := *dataDir
	if resolvedDir == "" {
		resolvedDir = os.Getenv("PROMPT_STUDIO_DATA_DIR")
	}
	if resolvedDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			resolvedDir = ".prompt-studio"
		} else {
			resolvedDir = filepath.Join(home, ".prompt-studio")
		}
	}

	if err := os.MkdirAll(resolvedDir, 0700); err != nil {
		log.Fatalf("Failed to create data directory %s: %v", resolvedDir, err)
	}

	secretStore := secrets.NewFileStore(filepath.Join(resolvedDir, "secrets.json"))

	dbPath := filepath.Join(resolvedDir, "prompt-studio.db")
	db, err := storage.OpenDB(dbPath, secretStore)
	if err != nil {
		log.Fatalf("Failed to open SQLite database at %s: %v", dbPath, err)
	}
	defer db.Close()

	repo := storage.NewRepo(db, secretStore)

	// Stable across restarts (0600 file in the data dir): open the printed URL once and
	// the long-lived HttpOnly cookie keeps the browser authenticated.
	sessionToken, err := security.LoadOrCreateSessionToken(filepath.Join(resolvedDir, "session.token"))
	if err != nil {
		log.Fatalf("Failed to load or create session token: %v", err)
	}

	svc := studio.NewService(repo)
	// --mock is process-scoped: it must not overwrite the persisted provider settings.
	svc.SetMockMode(*mockMode)
	router := handlers.NewRouter(svc, web.Assets, server.SecurityConfig{
		SessionToken: sessionToken,
		RequireToken: true,
	})

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", *addr, err)
	}
	fmt.Printf("Prompt Studio listening on http://%s/?token=%s (data-dir: %s)\n", *addr, sessionToken, resolvedDir)

	// Ctrl+C / SIGTERM: stop streams (partial prompts are saved as incomplete versions),
	// let handlers finish, then close the database via the deferred db.Close.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := server.Run(ctx, ln, router, 10*time.Second); err != nil {
		log.Printf("Server stopped: %v", err)
		return
	}
	log.Printf("Prompt Studio stopped cleanly")
}
