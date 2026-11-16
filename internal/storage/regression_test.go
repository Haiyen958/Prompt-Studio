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

// Regression (P010 T1): UnsaveSession cleared pinned_version_id without re-syncing
// sessions_fts, leaving search stuck on the old pinned version instead of the latest one.
func TestUnsaveSessionSyncsFTSIndex(t *testing.T) {
	repo := newTestRepo(t)
	sess, err := repo.CreateSession("generated", "", "", "Yêu cầu chung", "chat")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	v1, err := repo.CreateVersion(PromptVersion{
		SessionID: sess.ID,
		Source:    "generate",
		Content:   "Nội dung phiên bản đầu tiên tu_khoa_ban_cu_v1",
	})
	if err != nil {
		t.Fatalf("CreateVersion v1: %v", err)
	}
	if _, err := repo.SaveSessionToLibrary(sess.ID, "", "Prompt thử FTS", v1.ID); err != nil {
		t.Fatalf("SaveSessionToLibrary: %v", err)
	}
	if _, err := repo.CreateVersion(PromptVersion{
		SessionID: sess.ID,
		ParentID:  v1.ID,
		Source:    "improve",
		Content:   "Nội dung phiên bản mới nhất tu_khoa_ban_moi_v2",
	}); err != nil {
		t.Fatalf("CreateVersion v2: %v", err)
	}

	// While pinned to v1, FTS indexes v1's content.
	pinnedHits, err := repo.ListSessions(SessionFilter{Query: "tu_khoa_ban_cu_v1"})
	if err != nil || len(pinnedHits) != 1 {
		t.Fatalf("while pinned: expected 1 hit for v1 keyword, got %d (err=%v)", len(pinnedHits), err)
	}

	// Unsave clears pinned_version_id → FTS must now index the latest version (v2).
	if err := repo.UnsaveSession(sess.ID); err != nil {
		t.Fatalf("UnsaveSession: %v", err)
	}
	v2Hits, err := repo.ListSessions(SessionFilter{Query: "tu_khoa_ban_moi_v2"})
	if err != nil || len(v2Hits) != 1 {
		t.Fatalf("after unsave: expected 1 hit for v2 keyword, got %d (err=%v)", len(v2Hits), err)
	}
	oldHits, err := repo.ListSessions(SessionFilter{Query: "tu_khoa_ban_cu_v1"})
	if err != nil || len(oldHits) != 0 {
		t.Fatalf("after unsave: expected 0 hits for stale v1 keyword, got %d (err=%v)", len(oldHits), err)
	}
}

// Regression (P010 T2): Importing a backup from another instance whose user collection
// shares a name with an existing local collection (different ID) failed with UNIQUE
// constraint on collections.name, and raw invalid/unknown settings were written unchecked.
func TestImportRemapsDuplicateCollectionNamesAndSanitizesSettings(t *testing.T) {
	repo := newTestRepo(t)
	localCol, err := repo.CreateCollection("Marketing")
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	savedAt := int64(1700000000000)
	bundle := &BackupBundle{
		Version:    2,
		ExportedAt: savedAt,
		Settings: map[string]string{
			"theme":               "light",
			"prompt_language":     "vi",
			"temperature":         "999", // invalid (> 2.0) -> must be ignored
			"unknown_rogue_field": "bad", // unknown -> must be ignored
			"api_key":             "sk-leak",
			"last_collection_id":  "col_remote_marketing",
		},
		Collections: []Collection{
			{ID: "col_remote_marketing", Name: "Marketing", IsSystem: false, SortOrder: 3},
			{ID: "col_remote_marketing_lower", Name: "marketing", IsSystem: false, SortOrder: 4},
		},
		Sessions: []SessionDetail{
			{
				Session: Session{
					ID:           "01HZYXIMPORTTEST0000000001",
					Title:        "Chiến dịch Tết",
					Origin:       "generated",
					Request:      "Viết kế hoạch quảng cáo Tết",
					Target:       "chat",
					CollectionID: "col_remote_marketing",
					SavedAt:      &savedAt,
					CreatedAt:    savedAt,
					OpenedAt:     savedAt,
				},
				Versions: []PromptVersion{
					{
						ID:        "01HZYXIMPORTVER00000000001",
						SessionID: "01HZYXIMPORTTEST0000000001",
						Seq:       1,
						Source:    "generate",
						Content:   "Prompt chiến dịch Tết cho {{san_pham}}",
						Target:    "chat",
						CreatedAt: savedAt,
					},
				},
			},
		},
	}

	if err := repo.ImportAll(bundle); err != nil {
		t.Fatalf("ImportAll failed on duplicate collection name: %v", err)
	}

	got, err := repo.GetSessionDetail("01HZYXIMPORTTEST0000000001", false)
	if err != nil {
		t.Fatalf("GetSessionDetail: %v", err)
	}
	if got.Session.CollectionID != localCol.ID {
		t.Fatalf("imported session collectionId = %q, want remapped local %q", got.Session.CollectionID, localCol.ID)
	}

	settings, err := repo.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if settings["theme"] != "light" || settings["prompt_language"] != "vi" {
		t.Fatalf("valid settings not imported: %+v", settings)
	}
	if settings["last_collection_id"] != localCol.ID {
		t.Fatalf("last_collection_id = %q, want remapped %q", settings["last_collection_id"], localCol.ID)
	}
	if _, exists := settings["unknown_rogue_field"]; exists {
		t.Fatalf("unknown setting key should not be imported: %+v", settings)
	}
	if settings["temperature"] == "999" {
		t.Fatalf("out-of-range temperature 999 should be rejected on import")
	}
}
