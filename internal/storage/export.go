package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

type BackupBundle struct {
	Version     int               `json:"version"`
	ExportedAt  int64             `json:"exportedAt"`
	Settings    map[string]string `json:"settings"`
	Collections []Collection      `json:"collections"`
	Sessions    []SessionDetail   `json:"sessions"`
}

// ExportAll serializes all non-secret settings, collections, sessions, and prompt_versions
// into a portable JSON-ready BackupBundle (API keys are never included).
func (r *Repo) ExportAll() (*BackupBundle, error) {
	settings, err := r.GetSettings()
	if err != nil {
		return nil, err
	}
	cols, _, err := r.ListCollections()
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Query(`SELECT id FROM sessions ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()

	var sessions []SessionDetail
	for _, id := range ids {
		detail, err := r.GetSessionDetail(id, false)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, *detail)
	}

	return &BackupBundle{
		Version:     2,
		ExportedAt:  time.Now().UnixMilli(),
		Settings:    settings,
		Collections: cols,
		Sessions:    sessions,
	}, nil
}

// ImportAll restores collections, sessions, prompt_versions, and non-secret settings
// inside a single SQLite transaction, preserving idempotence via UPSERT.
func (r *Repo) ImportAll(bundle *BackupBundle) error {
	if bundle == nil {
		return errors.New("backup payload is empty")
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Load existing collections inside tx so duplicate names from another machine map to
	// the local collection ID instead of failing UNIQUE(collections.name).
	existingCols, knownColIDs, err := loadCollectionsTx(tx)
	if err != nil {
		return err
	}
	colRemap := map[string]string{SystemCollectionID: SystemCollectionID}

	for _, c := range bundle.Collections {
		cid := strings.TrimSpace(c.ID)
		cname := strings.TrimSpace(c.Name)
		if cid == "" {
			continue
		}
		if c.IsSystem || cid == SystemCollectionID || strings.EqualFold(cname, SystemCollectionName) {
			colRemap[cid] = SystemCollectionID
			continue
		}
		if cname == "" {
			continue
		}
		if knownColIDs[cid] {
			// Same ID already exists locally; update name only if no other collection owns it.
			nameTaken := false
			for _, ex := range existingCols {
				if ex.ID != cid && strings.EqualFold(ex.Name, cname) {
					nameTaken = true
					break
				}
			}
			if nameTaken {
				if _, err := tx.Exec(`UPDATE collections SET sort_order = ? WHERE id = ? AND is_system = 0`, c.SortOrder, cid); err != nil {
					return err
				}
			} else {
				if _, err := tx.Exec(`UPDATE collections SET name = ?, sort_order = ? WHERE id = ? AND is_system = 0`, cname, c.SortOrder, cid); err != nil {
					return err
				}
				for i := range existingCols {
					if existingCols[i].ID == cid {
						existingCols[i].Name = cname
					}
				}
			}
			colRemap[cid] = cid
			continue
		}

		matchedID := ""
		for _, ex := range existingCols {
			if strings.EqualFold(ex.Name, cname) {
				matchedID = ex.ID
				break
			}
		}
		if matchedID != "" {
			colRemap[cid] = matchedID
			continue
		}

		if _, err := tx.Exec(
			`INSERT INTO collections (id, name, is_system, sort_order) VALUES (?, ?, 0, ?)`,
			cid, cname, c.SortOrder,
		); err != nil {
			return err
		}
		existingCols = append(existingCols, Collection{ID: cid, Name: cname})
		knownColIDs[cid] = true
		colRemap[cid] = cid
	}

	for k, v := range sanitizeImportedSettings(bundle.Settings, colRemap, knownColIDs) {
		if _, err := tx.Exec(
			`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
			k, v,
		); err != nil {
			return err
		}
	}

	for _, sd := range bundle.Sessions {
		s := sd.Session
		if s.ID == "" {
			continue
		}
		var reqPtr, colPtr, pinPtr, hashPtr *string
		if s.Request != "" {
			cp := s.Request
			reqPtr = &cp
		}
		if rawCol := strings.TrimSpace(s.CollectionID); rawCol != "" {
			mapped := colRemap[rawCol]
			if mapped == "" && knownColIDs[rawCol] {
				mapped = rawCol
			}
			if mapped == "" && s.SavedAt != nil {
				mapped = SystemCollectionID
			}
			if mapped != "" {
				colPtr = &mapped
			}
		} else if s.SavedAt != nil {
			sys := SystemCollectionID
			colPtr = &sys
		}
		if s.PinnedVersionID != "" {
			cp := s.PinnedVersionID
			pinPtr = &cp
		}
		if s.ContentHash != "" {
			cp := s.ContentHash
			hashPtr = &cp
		}

		_, err := tx.Exec(
			`INSERT INTO sessions
			 (id, title, description, origin, request, target, collection_id, saved_at, pinned_version_id, content_hash, created_at, opened_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(id) DO UPDATE SET
			   title = excluded.title,
			   description = excluded.description,
			   collection_id = excluded.collection_id,
			   saved_at = excluded.saved_at,
			   pinned_version_id = excluded.pinned_version_id,
			   opened_at = excluded.opened_at`,
			s.ID, s.Title, s.Description, defaultStr(s.Origin, "generated"), reqPtr, normalizeTarget(s.Target),
			colPtr, s.SavedAt, pinPtr, hashPtr, s.CreatedAt, s.OpenedAt,
		)
		if err != nil {
			return err
		}

		for _, v := range sd.Versions {
			var parentPtr, instrPtr, assumpPtr, targetPtr *string
			if v.Target != "" {
				cp := normalizeTarget(v.Target)
				targetPtr = &cp
			}
			if v.ParentID != "" {
				cp := v.ParentID
				parentPtr = &cp
			}
			if v.Instruction != "" {
				cp := v.Instruction
				instrPtr = &cp
			}
			if len(v.Assumptions) > 0 {
				if b, mErr := json.Marshal(v.Assumptions); mErr == nil {
					str := string(b)
					assumpPtr = &str
				}
			}
			inc := 0
			if v.Incomplete {
				inc = 1
			}
			_, err := tx.Exec(
				`INSERT OR IGNORE INTO prompt_versions
				 (id, session_id, parent_id, seq, source, instruction, content, assumptions, incomplete, target, template_version, model, input_tokens, output_tokens, latency_ms, created_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				v.ID, s.ID, parentPtr, v.Seq, defaultStr(v.Source, "generate"), instrPtr, v.Content, assumpPtr, inc, targetPtr,
				v.TemplateVersion, v.Model, v.InputTokens, v.OutputTokens, v.LatencyMs, v.CreatedAt,
			)
			if err != nil {
				return err
			}
		}

		_ = syncSessionFTSTx(tx, s.ID)
	}

	return tx.Commit()
}

func defaultStr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func loadCollectionsTx(tx *sql.Tx) ([]Collection, map[string]bool, error) {
	rows, err := tx.Query(`SELECT id, name, is_system, sort_order FROM collections`)
	if err != nil {
		return nil, nil, err
	}
	var out []Collection
	known := make(map[string]bool)
	for rows.Next() {
		var c Collection
		var sys int
		if err := rows.Scan(&c.ID, &c.Name, &sys, &c.SortOrder); err != nil {
			rows.Close()
			return nil, nil, err
		}
		c.IsSystem = sys != 0
		out = append(out, c)
		known[c.ID] = true
	}
	if err := rows.Close(); err != nil {
		return nil, nil, err
	}
	return out, known, rows.Err()
}

// sanitizeImportedSettings keeps only known non-secret setting keys whose values satisfy
// the application's accepted ranges, remapping last_collection_id when needed.
func sanitizeImportedSettings(raw map[string]string, colRemap map[string]string, knownCols map[string]bool) map[string]string {
	out := make(map[string]string)
	for k, v := range raw {
		val := strings.TrimSpace(v)
		switch k {
		case "provider":
			low := strings.ToLower(val)
			if low == "openai" || low == "anthropic" || low == "openai-compatible" || low == "mock" {
				out[k] = low
			}
		case "base_url":
			out[k] = val
		case "model":
			if val != "" {
				out[k] = val
			}
		case "temperature":
			if f, err := strconv.ParseFloat(val, 64); err == nil && f >= 0.0 && f <= 2.0 {
				out[k] = strconv.FormatFloat(f, 'f', 2, 64)
			}
		case "max_tokens":
			if n, err := strconv.Atoi(val); err == nil && n >= 128 && n <= 16384 {
				out[k] = strconv.Itoa(n)
			}
		case "timeout_sec":
			if n, err := strconv.Atoi(val); err == nil && n >= 5 && n <= 300 {
				out[k] = strconv.Itoa(n)
			}
		case "theme":
			if val == "dark" || val == "light" || val == "system" {
				out[k] = val
			}
		case "accent":
			low := strings.ToLower(val)
			if low == "crimson" || low == "indigo" || low == "ocean" || low == "emerald" || low == "violet" || low == "amber" {
				out[k] = low
			}
		case "prompt_language":
			if val == "match" || val == "vi" || val == "en" {
				out[k] = val
			}
		case "default_target":
			if val == "chat" || val == "api" || val == "agent" {
				out[k] = val
			}
		case "last_collection_id":
			mapped := colRemap[val]
			if mapped == "" && knownCols[val] {
				mapped = val
			}
			if mapped != "" {
				out[k] = mapped
			}
		}
	}
	return out
}
