package storage

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"prompt-studio/internal/secrets"
	"prompt-studio/internal/security"
)

func TestV2SessionsLibraryVietnameseFTSAndExportImport(t *testing.T) {
	dir := t.TempDir()
	secretStore := secrets.NewFileStore(filepath.Join(dir, "secrets.json"))
	db, err := OpenDB(filepath.Join(dir, "test.db"), secretStore)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	repo := NewRepo(db, secretStore)

	// 1. Create generated session + version tree
	sess, err := repo.CreateSession("generated", "Tìm kiếm video quảng cáo", "", "Thiết kế tìm kiếm video quảng cáo hiệu năng cao", "chat")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	v1, err := repo.CreateVersion(PromptVersion{
		SessionID: sess.ID,
		Source:    "generate",
		Content:   "### Vai trò\nKỹ sư tìm kiếm cho {{nganh_hang}}.",
		Assumptions: []AssumptionRecord{
			{Label: "Nền tảng", Value: "Web desktop", Alternatives: []string{"Mobile app", "Cả hai"}},
		},
	})
	if err != nil {
		t.Fatalf("CreateVersion v1: %v", err)
	}

	v2, err := repo.CreateVersion(PromptVersion{
		SessionID:   sess.ID,
		ParentID:    v1.ID,
		Source:      "assumption",
		Instruction: "Đổi giả định Nền tảng thành Mobile app",
		Content:     "### Vai trò\nKỹ sư tìm kiếm ứng dụng di động cho {{nganh_hang}} và {{quoc_gia}}.",
	})
	if err != nil {
		t.Fatalf("CreateVersion v2: %v", err)
	}
	if v2.Seq != 2 || v2.ParentID != v1.ID {
		t.Fatalf("unexpected v2 seq/parent: %+v", v2)
	}

	// 2. Vietnamese accent-insensitive FTS5 search ("tim kiem quang cao" without diacritics!)
	found, err := repo.ListSessions(SessionFilter{Query: "tim kiem quang cao"})
	if err != nil {
		t.Fatalf("ListSessions FTS: %v", err)
	}
	if len(found) != 1 || found[0].ID != sess.ID {
		t.Fatalf("expected accent-insensitive FTS match for session %s, got %+v", sess.ID, found)
	}
	if found[0].VariablesCount != 2 {
		t.Fatalf("expected 2 distinct variables, got %d", found[0].VariablesCount)
	}

	// 3. One-click save to custom collection + delete collection safety
	col, err := repo.CreateCollection("Sản phẩm & UX")
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	saved, err := repo.SaveSessionToLibrary(sess.ID, col.ID, "", "")
	if err != nil || saved.SavedAt == nil || saved.CollectionID != col.ID {
		t.Fatalf("SaveSessionToLibrary failed: %+v err=%v", saved, err)
	}

	// Deleting collection moves session to "Chưa phân loại" without losing it
	if err := repo.DeleteCollection(col.ID); err != nil {
		t.Fatalf("DeleteCollection: %v", err)
	}
	afterColDel, err := repo.GetSessionDetail(sess.ID, false)
	if err != nil {
		t.Fatalf("GetSessionDetail: %v", err)
	}
	if afterColDel.Session.CollectionID != SystemCollectionID || afterColDel.Session.SavedAt == nil {
		t.Fatalf("expected session moved to %s and still saved, got %+v", SystemCollectionID, afterColDel.Session)
	}

	// 4. Pasted session + whitespace-normalized duplicate detection
	pastedSess, _, _, err := repo.CreatePastedSession("", "Mô tả mẫu", "Bạn là UX researcher.\nKhám phá {{chu_de}}.", "", "chat")
	if err != nil {
		t.Fatalf("CreatePastedSession: %v", err)
	}
	_, _, dupID, dupErr := repo.CreatePastedSession("Khác tên", "", "  Bạn   là UX researcher.   Khám phá {{chu_de}}. \n", "", "chat")
	if !errors.Is(dupErr, ErrDuplicateSession) || dupID != pastedSess.ID {
		t.Fatalf("expected duplicate detection pointing to %s, got id=%q err=%v", pastedSess.ID, dupID, dupErr)
	}

	// 5. Export & Import into fresh DB
	bundle, err := repo.ExportAll()
	if err != nil {
		t.Fatalf("ExportAll: %v", err)
	}
	if len(bundle.Sessions) != 2 {
		t.Fatalf("expected 2 exported sessions, got %d", len(bundle.Sessions))
	}

	db2, err := OpenDB(filepath.Join(t.TempDir(), "restored.db"))
	if err != nil {
		t.Fatalf("OpenDB 2: %v", err)
	}
	defer db2.Close()
	repo2 := NewRepo(db2, nil)
	if err := repo2.ImportAll(bundle); err != nil {
		t.Fatalf("ImportAll: %v", err)
	}
	restoredList, err := repo2.ListSessions(SessionFilter{SavedOnly: true})
	if err != nil || len(restoredList) != 2 {
		t.Fatalf("expected 2 restored saved sessions, got %d err=%v", len(restoredList), err)
	}
}

func TestLegacyV1DatabaseAutoMigration(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "prompt-studio.db")
	mk, err := security.EnsureMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatalf("EnsureMasterKey: %v", err)
	}
	ct, nonce, err := security.EncryptAESGCM(mk, "sk-legacy-v1-key-7788")
	if err != nil {
		t.Fatalf("EncryptAESGCM: %v", err)
	}

	// Construct a v1 database manually
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	_, err = rawDB.Exec(`
		CREATE TABLE sessions (id TEXT PRIMARY KEY, title TEXT NOT NULL, options TEXT NOT NULL, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL);
		CREATE TABLE turns (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, kind TEXT NOT NULL, content TEXT NOT NULL, created_at INTEGER NOT NULL);
		CREATE TABLE prompt_versions (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, parent_id TEXT, seq INTEGER NOT NULL, source TEXT NOT NULL, instruction TEXT, target TEXT NOT NULL DEFAULT 'chat', content TEXT NOT NULL, parts TEXT, assumptions TEXT, template_version TEXT, model TEXT, input_tokens INTEGER NOT NULL DEFAULT 0, output_tokens INTEGER NOT NULL DEFAULT 0, latency_ms INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL);
		CREATE TABLE secrets (name TEXT PRIMARY KEY, ciphertext BLOB NOT NULL, nonce BLOB NOT NULL, key_hint TEXT NOT NULL);
		CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
		INSERT INTO sessions VALUES ('s_legacy', 'Legacy Session', '{"target":"agent"}', 1000, 2000);
		INSERT INTO turns VALUES ('t_1', 's_legacy', 'request', 'Legacy user request', 1000);
		INSERT INTO prompt_versions VALUES ('v_1', 's_legacy', NULL, 1, 'generate', NULL, 'agent', 'Legacy prompt body', NULL, '["Assumed Go 1.22"]', 'generate.v1', 'gpt-4o', 10, 20, 100, 1500);
	`)
	if err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}
	if _, err := rawDB.Exec(`INSERT INTO secrets VALUES ('default_api_key', ?, ?, '••••7788')`, ct, nonce); err != nil {
		t.Fatalf("seed legacy secret: %v", err)
	}
	_ = rawDB.Close()

	// Open with v2 OpenDB -> triggers backup + secret migration + schema upgrade
	secretStore := secrets.NewFileStore(filepath.Join(dir, "secrets.json"))
	upgradedDB, err := OpenDB(dbPath, secretStore)
	if err != nil {
		t.Fatalf("OpenDB upgrade failed: %v", err)
	}
	defer upgradedDB.Close()

	if tableExists(upgradedDB, "secrets") {
		t.Fatalf("expected SQLite secrets table dropped after migration")
	}
	migratedKey, _, hasKey, err := secretStore.Get()
	if err != nil || !hasKey || migratedKey != "sk-legacy-v1-key-7788" {
		t.Fatalf("expected API key migrated to secrets.json, got has=%v key=%q err=%v", hasKey, migratedKey, err)
	}

	repo := NewRepo(upgradedDB, secretStore)
	detail, err := repo.GetSessionDetail("s_legacy", false)
	if err != nil {
		t.Fatalf("GetSessionDetail s_legacy: %v", err)
	}
	if detail.Session.Request != "Legacy user request" || detail.Session.Target != "agent" || len(detail.Versions) != 1 {
		t.Fatalf("unexpected migrated session detail: %+v", detail)
	}
}
