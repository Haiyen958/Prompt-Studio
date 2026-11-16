package storage

import (
	"encoding/json"
	"errors"
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

	for k, v := range bundle.Settings {
		if k == "api_key" || k == "apiKey" {
			continue // Never import secrets into SQLite settings
		}
		_, _ = tx.Exec(
			`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
			k, v,
		)
	}

	for _, c := range bundle.Collections {
		if c.IsSystem || c.ID == SystemCollectionID {
			continue
		}
		_, err := tx.Exec(
			`INSERT INTO collections (id, name, is_system, sort_order) VALUES (?, ?, 0, ?)
			 ON CONFLICT(id) DO UPDATE SET name = excluded.name, sort_order = excluded.sort_order`,
			c.ID, c.Name, c.SortOrder,
		)
		if err != nil {
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
		if s.CollectionID != "" {
			cp := s.CollectionID
			colPtr = &cp
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
