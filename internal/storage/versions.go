package storage

// Immutable prompt versions and the FTS index sync.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const versionColumns = `id, session_id, parent_id, seq, source, instruction, content, assumptions, incomplete, target, template_version, model, input_tokens, output_tokens, latency_ms, created_at`

type rowScanner interface {
	Scan(dest ...any) error
}

// scanVersion reads one prompt_versions row selected with versionColumns.
// sessionTarget fills Target for rows created before per-version targets existed.
func scanVersion(sc rowScanner, sessionTarget string) (PromptVersion, error) {
	var v PromptVersion
	var parentNull, instrNull, assumpNull, targetNull, tmplNull, modelNull sql.NullString
	var incompleteInt int
	if err := sc.Scan(
		&v.ID, &v.SessionID, &parentNull, &v.Seq, &v.Source, &instrNull, &v.Content, &assumpNull, &incompleteInt,
		&targetNull, &tmplNull, &modelNull, &v.InputTokens, &v.OutputTokens, &v.LatencyMs, &v.CreatedAt,
	); err != nil {
		return v, err
	}
	v.ParentID = parentNull.String
	v.Instruction = instrNull.String
	v.TemplateVersion = tmplNull.String
	v.Model = modelNull.String
	v.Incomplete = incompleteInt != 0
	v.Target = targetNull.String
	if v.Target == "" {
		v.Target = sessionTarget
	}
	if assumpNull.Valid && assumpNull.String != "" {
		_ = json.Unmarshal([]byte(assumpNull.String), &v.Assumptions)
	}
	return v, nil
}

// CreateVersion appends a new immutable version row and syncs FTS in the same transaction.
func (r *Repo) CreateVersion(v PromptVersion) (*PromptVersion, error) {
	now := time.Now().UnixMilli()
	if v.ID == "" {
		v.ID = NewID()
	}
	if v.CreatedAt == 0 {
		v.CreatedAt = now
	}

	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var nextSeq int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(seq), 0) + 1 FROM prompt_versions WHERE session_id = ?`, v.SessionID).Scan(&nextSeq); err != nil {
		return nil, err
	}
	v.Seq = nextSeq

	if strings.TrimSpace(v.Target) == "" {
		if err := tx.QueryRow(`SELECT target FROM sessions WHERE id = ?`, v.SessionID).Scan(&v.Target); err != nil {
			return nil, err
		}
	}
	v.Target = normalizeTarget(v.Target)

	var parentPtr *string
	if strings.TrimSpace(v.ParentID) != "" {
		p := v.ParentID
		parentPtr = &p
	}
	var instrPtr *string
	if strings.TrimSpace(v.Instruction) != "" {
		inst := v.Instruction
		instrPtr = &inst
	}
	var assumpPtr *string
	if len(v.Assumptions) > 0 {
		b, err := json.Marshal(v.Assumptions)
		if err != nil {
			return nil, err
		}
		s := string(b)
		assumpPtr = &s
	}
	incompleteInt := 0
	if v.Incomplete {
		incompleteInt = 1
	}

	_, err = tx.Exec(
		`INSERT INTO prompt_versions
		 (id, session_id, parent_id, seq, source, instruction, content, assumptions, incomplete, target, template_version, model, input_tokens, output_tokens, latency_ms, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.ID, v.SessionID, parentPtr, v.Seq, v.Source, instrPtr, v.Content, assumpPtr, incompleteInt, v.Target,
		v.TemplateVersion, v.Model, v.InputTokens, v.OutputTokens, v.LatencyMs, v.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(`UPDATE sessions SET opened_at = ? WHERE id = ?`, now, v.SessionID); err != nil {
		return nil, err
	}

	if err := syncSessionFTSTx(tx, v.SessionID); err != nil {
		// Non-fatal if FTS5 disabled
		_ = err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &v, nil
}

func syncSessionFTSTx(tx *sql.Tx, sessionID string) error {
	var title string
	var reqNull, pinnedNull sql.NullString
	if err := tx.QueryRow(`SELECT title, request, pinned_version_id FROM sessions WHERE id = ?`, sessionID).Scan(&title, &reqNull, &pinnedNull); err != nil {
		return err
	}

	var content string
	if pinnedNull.Valid && pinnedNull.String != "" {
		_ = tx.QueryRow(`SELECT content FROM prompt_versions WHERE id = ? AND session_id = ?`, pinnedNull.String, sessionID).Scan(&content)
	}
	if content == "" {
		_ = tx.QueryRow(`SELECT content FROM prompt_versions WHERE session_id = ? ORDER BY seq DESC LIMIT 1`, sessionID).Scan(&content)
	}

	if _, err := tx.Exec(`DELETE FROM sessions_fts WHERE session_id = ?`, sessionID); err != nil {
		return err
	}
	_, err := tx.Exec(
		`INSERT INTO sessions_fts (session_id, title, request, content) VALUES (?, ?, ?, ?)`,
		sessionID, title, reqNull.String, content,
	)
	return err
}

func (r *Repo) GetVersion(versionID string) (*PromptVersion, *Session, error) {
	v, err := scanVersion(r.db.QueryRow(`SELECT `+versionColumns+` FROM prompt_versions WHERE id = ?`, versionID), "")
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}

	sess, err := r.getSessionRow(v.SessionID)
	if err != nil {
		return nil, nil, err
	}
	if v.Target == "" {
		v.Target = sess.Target
	}
	return &v, sess, nil
}

func versionBelongsTx(tx *sql.Tx, sessionID, versionID string) error {
	var owner string
	err := tx.QueryRow(`SELECT session_id FROM prompt_versions WHERE id = ?`, versionID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && owner != sessionID) {
		return ErrVersionMismatch
	}
	return err
}
