package storage

import (
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"prompt-studio/internal/secrets"
	"prompt-studio/internal/security"
)

func tableExists(db *sql.DB, name string) bool {
	var found string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type IN ('table', 'view') AND name = ?`, name).Scan(&found)
	return err == nil && found == name
}

func columnExists(db *sql.DB, table, col string) bool {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var cname, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &cname, &ctype, &notnull, &dflt, &pk); err == nil && cname == col {
			return true
		}
	}
	return false
}

func maybeUpgradeLegacyV1FileBackup(dbPath string) error {
	if dbPath == ":memory:" || strings.HasPrefix(dbPath, "file:") {
		return nil
	}
	if _, err := os.Stat(dbPath); err != nil {
		return nil
	}
	bakPath := filepath.Join(filepath.Dir(dbPath), "prompt-studio.v1.bak.db")
	if _, err := os.Stat(bakPath); err == nil {
		return nil
	}

	// Peek quickly to see if legacy tables exist before copying
	tmpDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil
	}
	isLegacy := tableExists(tmpDB, "turns") || tableExists(tmpDB, "library_prompts") || tableExists(tmpDB, "secrets")
	_ = tmpDB.Close()
	if !isLegacy {
		return nil
	}

	src, err := os.Open(dbPath)
	if err != nil {
		return nil
	}
	defer src.Close()

	dst, err := os.OpenFile(bakPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	return err
}

// migrateLegacyV1IfNeeded migrates v1 tables (sessions+turns, library_prompts, secrets)
// into the unified v2 schema without losing user history or saved prompts, and moves
// any encrypted SQLite API key into secrets.json (0600) before dropping SQLite secrets.
func migrateLegacyV1IfNeeded(db *sql.DB, dbPath string, secretStore *secrets.Store) error {
	isLegacy := tableExists(db, "turns") || tableExists(db, "library_prompts") || tableExists(db, "secrets") ||
		(tableExists(db, "sessions") && !columnExists(db, "sessions", "origin"))
	if !isLegacy {
		return nil
	}

	// 1. Migrate legacy SQLite secret to secrets.json (0600) if present
	if tableExists(db, "secrets") && secretStore != nil && dbPath != ":memory:" && !strings.HasPrefix(dbPath, "file:") {
		_, _, alreadyHas, _ := secretStore.Get()
		if !alreadyHas {
			masterKeyPath := filepath.Join(filepath.Dir(dbPath), "master.key")
			// Load only: a v1 database without master.key has no decryptable key to move.
			if mk, err := security.LoadMasterKey(masterKeyPath); err == nil {
				var ct, nonce []byte
				if err := db.QueryRow(`SELECT ciphertext, nonce FROM secrets WHERE name = 'default_api_key'`).Scan(&ct, &nonce); err == nil {
					if plain, decErr := security.DecryptAESGCM(mk, ct, nonce); decErr == nil && strings.TrimSpace(plain) != "" {
						_ = secretStore.Set(plain)
					}
				}
			}
		}
	}

	// 2. Read legacy collections
	type legacyCol struct {
		id        string
		name      string
		isSystem  int
		sortOrder int
	}
	var cols []legacyCol
	if tableExists(db, "collections") {
		rows, err := db.Query(`SELECT id, name, is_system, sort_order FROM collections`)
		if err == nil {
			for rows.Next() {
				var c legacyCol
				if rows.Scan(&c.id, &c.name, &c.isSystem, &c.sortOrder) == nil {
					if c.isSystem == 1 || c.id == SystemCollectionID {
						c.id = SystemCollectionID
						c.name = SystemCollectionName
						c.isSystem = 1
						c.sortOrder = 9999
					}
					cols = append(cols, c)
				}
			}
			rows.Close()
		}
	}

	// 3. Read legacy sessions + request turns + versions
	type legacySess struct {
		id        string
		title     string
		target    string
		request   string
		createdAt int64
		updatedAt int64
	}
	var sessions []legacySess
	if tableExists(db, "sessions") {
		rows, err := db.Query(`SELECT id, title, options, created_at, updated_at FROM sessions`)
		if err == nil {
			for rows.Next() {
				var s legacySess
				var optsRaw string
				if rows.Scan(&s.id, &s.title, &optsRaw, &s.createdAt, &s.updatedAt) == nil {
					var opts struct {
						Target string `json:"target"`
					}
					_ = json.Unmarshal([]byte(optsRaw), &opts)
					s.target = normalizeTarget(opts.Target)
					sessions = append(sessions, s)
				}
			}
			rows.Close()
		}
	}
	if tableExists(db, "turns") {
		for i := range sessions {
			var req string
			if err := db.QueryRow(`SELECT content FROM turns WHERE session_id = ? AND kind = 'request' ORDER BY created_at ASC LIMIT 1`, sessions[i].id).Scan(&req); err == nil {
				sessions[i].request = req
			}
		}
	}

	type legacyVer struct {
		id, sessionID, source, content, tmplVer, model string
		parentID, instruction, assumptions             sql.NullString
		seq, inTok, outTok, latMs                      int
		createdAt                                      int64
	}
	var versions []legacyVer
	if tableExists(db, "prompt_versions") {
		rows, err := db.Query(`SELECT id, session_id, parent_id, seq, source, instruction, content, assumptions, template_version, model, input_tokens, output_tokens, latency_ms, created_at FROM prompt_versions`)
		if err == nil {
			for rows.Next() {
				var v legacyVer
				if rows.Scan(&v.id, &v.sessionID, &v.parentID, &v.seq, &v.source, &v.instruction, &v.content, &v.assumptions, &v.tmplVer, &v.model, &v.inTok, &v.outTok, &v.latMs, &v.createdAt) == nil {
					// Convert v1 string assumptions ["a"] into v2 object assumptions [{"label":"Giả định","value":"a"}]
					if v.assumptions.Valid && strings.TrimSpace(v.assumptions.String) != "" {
						var strList []string
						if json.Unmarshal([]byte(v.assumptions.String), &strList) == nil && len(strList) > 0 {
							var v2Assump []AssumptionRecord
							for _, item := range strList {
								v2Assump = append(v2Assump, AssumptionRecord{Label: "Giả định", Value: item})
							}
							if b, mErr := json.Marshal(v2Assump); mErr == nil {
								v.assumptions.String = string(b)
							}
						}
					}
					versions = append(versions, v)
				}
			}
			rows.Close()
		}
	}

	// 4. Read legacy library_prompts so saved prompts become saved v2 sessions
	type legacyLib struct {
		id, colID, title, desc, content, target, source string
		createdAt, updatedAt                            int64
	}
	var libs []legacyLib
	if tableExists(db, "library_prompts") {
		rows, err := db.Query(`SELECT id, collection_id, title, description, content, target, source, created_at, updated_at FROM library_prompts`)
		if err == nil {
			for rows.Next() {
				var lp legacyLib
				if rows.Scan(&lp.id, &lp.colID, &lp.title, &lp.desc, &lp.content, &lp.target, &lp.source, &lp.createdAt, &lp.updatedAt) == nil {
					libs = append(libs, lp)
				}
			}
			rows.Close()
		}
	}

	// 5. Rebuild clean v2 schema inside transaction
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`PRAGMA foreign_keys = OFF;`); err != nil {
		return err
	}

	dropOrder := []string{
		`DROP TABLE IF EXISTS library_fts`,
		`DROP TABLE IF EXISTS library_revisions`,
		`DROP TABLE IF EXISTS library_prompts`,
		`DROP TABLE IF EXISTS sessions_fts`,
		`DROP TABLE IF EXISTS turns`,
		`DROP TABLE IF EXISTS prompt_versions`,
		`DROP TABLE IF EXISTS sessions`,
		`DROP TABLE IF EXISTS collections`,
		`DROP TABLE IF EXISTS secrets`,
	}
	for _, d := range dropOrder {
		if _, err := tx.Exec(d); err != nil {
			return err
		}
	}

	for _, stmt := range strings.Split(v2SchemaSQL, ";") {
		trimmed := strings.TrimSpace(stmt)
		if trimmed == "" {
			continue
		}
		if _, err := tx.Exec(trimmed); err != nil {
			if strings.Contains(strings.ToUpper(trimmed), "USING FTS5") {
				continue
			}
			return err
		}
	}

	if _, err := tx.Exec(`INSERT OR IGNORE INTO collections (id, name, is_system, sort_order) VALUES (?, ?, 1, 9999)`, SystemCollectionID, SystemCollectionName); err != nil {
		return err
	}
	for _, c := range cols {
		if c.id == SystemCollectionID {
			continue
		}
		_, _ = tx.Exec(`INSERT OR IGNORE INTO collections (id, name, is_system, sort_order) VALUES (?, ?, ?, ?)`, c.id, c.name, c.isSystem, c.sortOrder)
	}

	latestContentBySession := map[string]string{}
	for _, v := range versions {
		latestContentBySession[v.sessionID] = v.content
	}

	for _, s := range sessions {
		var reqPtr *string
		if s.request != "" {
			reqCopy := s.request
			reqPtr = &reqCopy
		}
		_, err := tx.Exec(
			`INSERT INTO sessions (id, title, description, origin, request, target, created_at, opened_at) VALUES (?, ?, '', 'generated', ?, ?, ?, ?)`,
			s.id, s.title, reqPtr, s.target, s.createdAt, s.updatedAt,
		)
		if err != nil {
			return err
		}
		_, _ = tx.Exec(
			`INSERT INTO sessions_fts (session_id, title, request, content) VALUES (?, ?, ?, ?)`,
			s.id, s.title, s.request, latestContentBySession[s.id],
		)
	}

	for _, v := range versions {
		_, err := tx.Exec(
			`INSERT INTO prompt_versions (id, session_id, parent_id, seq, source, instruction, content, assumptions, incomplete, template_version, model, input_tokens, output_tokens, latency_ms, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?)`,
			v.id, v.sessionID, v.parentID, v.seq, v.source, v.instruction, v.content, v.assumptions, v.tmplVer, v.model, v.inTok, v.outTok, v.latMs, v.createdAt,
		)
		if err != nil {
			return err
		}
	}

	for _, lp := range libs {
		origin := "pasted"
		if lp.source == "generated" {
			origin = "generated"
		}
		colID := lp.colID
		if colID == "" {
			colID = SystemCollectionID
		}
		hash := NormalizeContentHash(lp.content)
		var hashPtr *string
		if origin == "pasted" {
			hashPtr = &hash
		}
		verID := NewID()
		_, err := tx.Exec(
			`INSERT INTO sessions (id, title, description, origin, request, target, collection_id, saved_at, pinned_version_id, content_hash, created_at, opened_at)
			 VALUES (?, ?, ?, ?, NULL, ?, ?, ?, ?, ?, ?, ?)`,
			lp.id, lp.title, lp.desc, origin, normalizeTarget(lp.target), colID, lp.createdAt, verID, hashPtr, lp.createdAt, lp.updatedAt,
		)
		if err != nil {
			continue
		}
		_, _ = tx.Exec(
			`INSERT INTO prompt_versions (id, session_id, parent_id, seq, source, content, incomplete, created_at)
			 VALUES (?, ?, NULL, 1, ?, ?, 0, ?)`,
			verID, lp.id, origin, lp.content, lp.createdAt,
		)
		_, _ = tx.Exec(
			`INSERT INTO sessions_fts (session_id, title, request, content) VALUES (?, ?, '', ?)`,
			lp.id, lp.title, lp.content,
		)
	}

	if _, err := tx.Exec(`PRAGMA foreign_keys = ON;`); err != nil {
		return err
	}
	return tx.Commit()
}
