package storage

import (
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"prompt-studio/internal/secrets"

	_ "modernc.org/sqlite"
)

//go:embed migrations/001_v2_schema.sql
var v2SchemaSQL string

const (
	SystemCollectionID   = "col_uncategorized"
	SystemCollectionName = "Chưa phân loại"
)

// OpenDB opens the SQLite database at dbPath, automatically upgrades any legacy v1
// schema (including moving legacy SQLite secrets to secretsStore and backing up the DB),
// applies the v2 schema, and seeds defaults.
func OpenDB(dbPath string, secretStore ...*secrets.Store) (*sql.DB, error) {
	if dbPath != ":memory:" && !strings.HasPrefix(dbPath, "file:") {
		if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
			return nil, err
		}
	}

	var store *secrets.Store
	if len(secretStore) > 0 {
		store = secretStore[0]
	}

	// Pre-open backup check if a legacy v1 file exists on disk
	if err := maybeUpgradeLegacyV1FileBackup(dbPath); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous = NORMAL;",
		"PRAGMA foreign_keys = ON;",
		"PRAGMA busy_timeout = 5000;",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("pragma %q: %w", p, err)
		}
	}

	if err := migrateLegacyV1IfNeeded(db, dbPath, store); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := migrateSchema(db, dbPath); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := seedDefaults(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

// DefaultSettings is the single source of default values for the settings table.
// They are seeded on open (INSERT OR IGNORE) and used as fallbacks when a stored value
// is missing or unparsable (see studio.ParseSettings).
func DefaultSettings() map[string]string {
	return map[string]string{
		"provider":           "openai",
		"base_url":           "",
		"model":              "gpt-4o-mini",
		"temperature":        "0.4",
		"max_tokens":         "2000",
		"timeout_sec":        "60",
		"theme":              "dark",
		"accent":             "crimson",
		"prompt_language":    "match",
		"default_target":     "chat",
		"last_collection_id": SystemCollectionID,
	}
}

// seedDefaults inserts the system collection and default settings when missing.
// It runs on every open and never overwrites user values.
func seedDefaults(db *sql.DB) error {
	_, err := db.Exec(
		`INSERT OR IGNORE INTO collections (id, name, is_system, sort_order) VALUES (?, ?, 1, 9999)`,
		SystemCollectionID, SystemCollectionName,
	)
	if err != nil {
		return fmt.Errorf("seed system collection: %w", err)
	}

	for k, v := range DefaultSettings() {
		if _, err := db.Exec(`INSERT OR IGNORE INTO settings (key, value) VALUES (?, ?)`, k, v); err != nil {
			return fmt.Errorf("seed setting %s: %w", k, err)
		}
	}
	return nil
}

// execQuerier is satisfied by both *sql.DB and *sql.Tx.
type execQuerier interface {
	Query(query string, args ...any) (*sql.Rows, error)
	Exec(query string, args ...any) (sql.Result, error)
}

// ensureColumn adds a nullable column to an existing table (additive migration);
// it is a no-op when the column already exists.
func ensureColumn(db execQuerier, table, column, decl string) error {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notNull, pk int
		var name, colType string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		if strings.EqualFold(name, column) {
			found = true
		}
	}
	rows.Close()
	if found {
		return nil
	}
	if _, err := db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, decl)); err != nil {
		return fmt.Errorf("add column %s.%s: %w", table, column, err)
	}
	return nil
}

var ulidAlphabet = []byte("0123456789ABCDEFGHJKMNPQRSTVWXYZ")

// NewID generates a lexicographically sortable 26-char ULID string.
func NewID() string {
	ms := uint64(time.Now().UnixMilli())
	var rnd [10]byte
	_, _ = rand.Read(rnd[:])

	var out [26]byte
	out[0] = ulidAlphabet[(ms>>45)&0x1F]
	out[1] = ulidAlphabet[(ms>>40)&0x1F]
	out[2] = ulidAlphabet[(ms>>35)&0x1F]
	out[3] = ulidAlphabet[(ms>>30)&0x1F]
	out[4] = ulidAlphabet[(ms>>25)&0x1F]
	out[5] = ulidAlphabet[(ms>>20)&0x1F]
	out[6] = ulidAlphabet[(ms>>15)&0x1F]
	out[7] = ulidAlphabet[(ms>>10)&0x1F]
	out[8] = ulidAlphabet[(ms>>5)&0x1F]
	out[9] = ulidAlphabet[ms&0x1F]

	hi := binary.BigEndian.Uint64(rnd[0:8])
	lo := binary.BigEndian.Uint16(rnd[8:10])

	for i := 0; i < 12; i++ {
		shift := uint(59 - i*5)
		out[10+i] = ulidAlphabet[(hi>>shift)&0x1F]
	}
	rem := ((hi & 0xF) << 16) | uint64(lo)
	out[22] = ulidAlphabet[(rem>>15)&0x1F]
	out[23] = ulidAlphabet[(rem>>10)&0x1F]
	out[24] = ulidAlphabet[(rem>>5)&0x1F]
	out[25] = ulidAlphabet[rem&0x1F]
	return string(out[:])
}
