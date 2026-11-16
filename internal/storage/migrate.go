package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Schema migrations (audit P1-d).
//
// PRAGMA user_version records the last applied step. Each step runs in its own
// transaction together with the version bump, so a crash never leaves a half-applied
// step marked as done. Steps MUST be idempotent: databases created before versioning
// existed (user_version 0) already contain the objects of the early steps.
//
// To change the schema: append a step (never edit or reorder existing ones), bump
// nothing else — SchemaVersion follows the list — and add a test that opens a database
// at the previous version.

type migration struct {
	version int
	name    string
	apply   func(tx *sql.Tx) error
}

var schemaMigrations = []migration{
	{1, "v2 tables, FTS5 index, indexes", applyV2SchemaTx},
	{2, "prompt_versions.target (per-version target)", func(tx *sql.Tx) error {
		return ensureColumn(tx, "prompt_versions", "target", "TEXT")
	}},
}

// SchemaVersion is the schema version this build writes.
var SchemaVersion = schemaMigrations[len(schemaMigrations)-1].version

// ErrNewerSchema is returned when the database was written by a newer Prompt Studio.
var ErrNewerSchema = fmt.Errorf("database schema is newer than this Prompt Studio build supports")

func schemaVersion(q rowQuerier) (int, error) {
	var v int
	err := q.QueryRow(`PRAGMA user_version`).Scan(&v)
	return v, err
}

// migrateSchema applies pending steps, backing up an existing database first.
func migrateSchema(db *sql.DB, dbPath string) error {
	cur, err := schemaVersion(db)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if cur > SchemaVersion {
		return fmt.Errorf("%w (database v%d, supported v%d): use a newer build or restore a backup", ErrNewerSchema, cur, SchemaVersion)
	}
	if cur == SchemaVersion {
		return nil
	}

	if err := backupBeforeMigration(db, dbPath); err != nil {
		return err
	}

	for _, m := range schemaMigrations {
		if m.version <= cur {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if err := m.apply(tx); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("schema migration v%d (%s): %w", m.version, m.name, err)
		}
		// PRAGMA does not accept bound parameters; m.version is a trusted int.
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, m.version)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("schema migration v%d: set user_version: %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("schema migration v%d: commit: %w", m.version, err)
		}
	}
	return nil
}

// backupBeforeMigration writes a consistent copy (VACUUM INTO, WAL-safe) of a database
// that already holds user data to <dir>/prompt-studio.pre-schema-v<N>.bak.db (0600).
// Fresh or in-memory databases need no backup; an existing backup is never overwritten.
func backupBeforeMigration(db *sql.DB, dbPath string) error {
	if dbPath == ":memory:" || strings.HasPrefix(dbPath, "file:") || !tableExists(db, "sessions") {
		return nil
	}
	bak := filepath.Join(filepath.Dir(dbPath), fmt.Sprintf("prompt-studio.pre-schema-v%d.bak.db", SchemaVersion))
	if _, err := os.Stat(bak); err == nil {
		return nil
	}
	if _, err := db.Exec(`VACUUM INTO ?`, bak); err != nil {
		return fmt.Errorf("backup before schema migration (%s): %w", bak, err)
	}
	return os.Chmod(bak, 0600)
}

// applyV2SchemaTx creates the v2 tables, FTS index and indexes (all IF NOT EXISTS).
func applyV2SchemaTx(tx *sql.Tx) error {
	for _, stmt := range strings.Split(v2SchemaSQL, ";") {
		trimmed := strings.TrimSpace(stmt)
		if trimmed == "" {
			continue
		}
		_, err := tx.Exec(trimmed)
		if err == nil {
			continue
		}
		if !strings.Contains(strings.ToUpper(trimmed), "USING FTS5") {
			return fmt.Errorf("%q: %w", trimmed, err)
		}
		// Builds without remove_diacritics support still get (accent-sensitive) search.
		fallback := strings.Replace(trimmed, "tokenize = 'unicode61 remove_diacritics 2'", "tokenize = 'unicode61'", 1)
		if _, fbErr := tx.Exec(fallback); fbErr != nil {
			// Search would fail on every query without the index: refuse to start instead.
			return fmt.Errorf("create FTS5 index: %v; fallback: %w", err, fbErr)
		}
	}
	return nil
}
