package storage

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"prompt-studio/internal/secrets"
)

func userVersion(t *testing.T, db *sql.DB) int {
	t.Helper()
	v, err := schemaVersion(db)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestFreshDatabaseIsAtCurrentSchemaWithoutBackup(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "prompt-studio.db"), secrets.NewFileStore(filepath.Join(dir, "s.json")))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if v := userVersion(t, db); v != SchemaVersion {
		t.Fatalf("user_version = %d, want %d", v, SchemaVersion)
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "*.bak.db")); len(matches) != 0 {
		t.Fatalf("fresh database must not be backed up, found %v", matches)
	}
}

// A database written before schema versioning (user_version 0, no target column) is
// backed up, migrated in place without data loss, and not backed up again on reopen.
func TestPreVersioningDatabaseIsBackedUpAndMigrated(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "prompt-studio.db")
	store := secrets.NewFileStore(filepath.Join(dir, "s.json"))

	db, err := OpenDB(dbPath, store)
	if err != nil {
		t.Fatal(err)
	}
	repo := NewRepo(db, store)
	sess, err := repo.CreateSession("generated", "", "", "Viết prompt tóm tắt", "agent")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateVersion(PromptVersion{SessionID: sess.ID, Source: "generate", Content: "Nội dung gốc"}); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`ALTER TABLE prompt_versions DROP COLUMN target`, `PRAGMA user_version = 0`} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	_ = db.Close()

	db, err = OpenDB(dbPath, store)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if v := userVersion(t, db); v != SchemaVersion {
		t.Fatalf("user_version = %d, want %d", v, SchemaVersion)
	}
	detail, err := NewRepo(db, store).GetSessionDetail(sess.ID, false)
	if err != nil || len(detail.Versions) != 1 || detail.Versions[0].Content != "Nội dung gốc" || detail.Versions[0].Target != "agent" {
		t.Fatalf("data after migration: %+v err %v", detail, err)
	}
	// Accent-insensitive search still works after migration.
	if list, err := NewRepo(db, store).ListSessions(SessionFilter{Query: "tom tat"}); err != nil || len(list) != 1 {
		t.Fatalf("FTS after migration: %d results err %v", len(list), err)
	}
	_ = db.Close()

	bak := filepath.Join(dir, "prompt-studio.pre-schema-v2.bak.db")
	info, err := os.Stat(bak)
	if err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("backup perm = %o, want 0600", info.Mode().Perm())
	}
	// The backup is the pre-migration database: user_version 0, data present.
	bdb, err := sql.Open("sqlite", bak)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := bdb.QueryRow(`SELECT COUNT(*) FROM prompt_versions`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("backup content: %d versions err %v", n, err)
	}
	if v := userVersion(t, bdb); v != 0 {
		t.Fatalf("backup user_version = %d, want 0", v)
	}
	_ = bdb.Close()

	// Reopen: nothing pending, backup untouched.
	db, err = OpenDB(dbPath, store)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	info2, _ := os.Stat(bak)
	if !info2.ModTime().Equal(info.ModTime()) {
		t.Fatal("backup was rewritten on a no-op open")
	}
}

func TestNewerSchemaIsRefusedAndUntouched(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "prompt-studio.db")
	store := secrets.NewFileStore(filepath.Join(dir, "s.json"))
	db, err := OpenDB(dbPath, store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 99`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	if _, err := OpenDB(dbPath, store); !errors.Is(err, ErrNewerSchema) {
		t.Fatalf("OpenDB err = %v, want ErrNewerSchema", err)
	}
	raw, _ := sql.Open("sqlite", dbPath)
	defer raw.Close()
	if v := userVersion(t, raw); v != 99 {
		t.Fatalf("refused database was modified: user_version = %d", v)
	}
}
