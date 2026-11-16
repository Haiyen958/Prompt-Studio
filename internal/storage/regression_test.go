package storage

import (
	"path/filepath"
	"testing"
	"time"

	"prompt-studio/internal/secrets"
)

func newTestRepo(t *testing.T) *Repo {
	t.Helper()
	dir := t.TempDir()
	store := secrets.NewFileStore(filepath.Join(dir, "secrets.json"))
	db, err := OpenDB(filepath.Join(dir, "test.db"), store)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewRepo(db, store)
}

// Regression: UpdateSession with CollectionID used r.db inside an open tx while the
// pool is capped at one connection, deadlocking the whole server.
func TestUpdateSessionCollectionDoesNotDeadlock(t *testing.T) {
	repo := newTestRepo(t)
	sess, err := repo.CreateSession("generated", "", "", "Viết prompt review code", "chat")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	col, err := repo.CreateCollection("Code")
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	type result struct {
		sess *Session
		err  error
	}
	done := make(chan result, 1)
	go func() {
		s, err := repo.UpdateSession(sess.ID, SessionPatch{CollectionID: &col.ID})
		done <- result{s, err}
	}()

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatalf("UpdateSession: %v", res.err)
		}
		if res.sess.CollectionID != col.ID {
			t.Fatalf("collection = %q, want %q", res.sess.CollectionID, col.ID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("UpdateSession with CollectionID deadlocked")
	}

	settings, err := repo.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if settings["last_collection_id"] != col.ID {
		t.Fatalf("last_collection_id = %q, want %q", settings["last_collection_id"], col.ID)
	}
}

// Databases created before per-version targets get the column added on open, and old
// rows inherit the session target.
func TestOpenDBAddsVersionTargetColumn(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")
	store := secrets.NewFileStore(filepath.Join(dir, "secrets.json"))

	db, err := OpenDB(dbPath, store)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	repo := NewRepo(db, store)
	sess, err := repo.CreateSession("generated", "", "", "Viết prompt API", "api")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, err := repo.CreateVersion(PromptVersion{SessionID: sess.ID, Source: "generate", Content: "Hệ thống\n===USER===\n{{input}}"}); err != nil {
		t.Fatalf("CreateVersion: %v", err)
	}
	if _, err := db.Exec(`ALTER TABLE prompt_versions DROP COLUMN target`); err != nil {
		t.Fatalf("simulate legacy schema: %v", err)
	}
	// Databases from before schema versioning report user_version 0.
	if _, err := db.Exec(`PRAGMA user_version = 0`); err != nil {
		t.Fatalf("simulate pre-versioning database: %v", err)
	}
	_ = db.Close()

	db2, err := OpenDB(dbPath, store)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()
	detail, err := NewRepo(db2, store).GetSessionDetail(sess.ID, false)
	if err != nil {
		t.Fatalf("GetSessionDetail: %v", err)
	}
	if detail.Versions[0].Target != "api" {
		t.Fatalf("legacy version target = %q, want inherited api", detail.Versions[0].Target)
	}
}
