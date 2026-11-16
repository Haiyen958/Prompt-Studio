package storage

// Sessions: history, Library save/unsave, search and patches.

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

func (r *Repo) CreateSession(origin, title, description, request, target string) (*Session, error) {
	now := time.Now().UnixMilli()
	id := NewID()
	target = normalizeTarget(target)
	if origin == "" {
		origin = "generated"
	}
	if strings.TrimSpace(title) == "" {
		title = deriveTitle(request)
	}

	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var reqPtr *string
	if origin != "pasted" && strings.TrimSpace(request) != "" {
		reqCopy := request
		reqPtr = &reqCopy
	}

	_, err = tx.Exec(
		`INSERT INTO sessions (id, title, description, origin, request, target, created_at, opened_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, title, description, origin, reqPtr, target, now, now,
	)
	if err != nil {
		return nil, err
	}

	_, _ = tx.Exec(
		`INSERT INTO sessions_fts (session_id, title, request, content) VALUES (?, ?, ?, '')`,
		id, title, request,
	)

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &Session{
		ID:          id,
		Title:       title,
		Description: description,
		Origin:      origin,
		Request:     request,
		Target:      target,
		CreatedAt:   now,
		OpenedAt:    now,
	}, nil
}

func (r *Repo) CreatePastedSession(title, description, content, collectionID, target string) (*Session, *PromptVersion, string, error) {
	hash := NormalizeContentHash(content)
	var existingID string
	err := r.db.QueryRow(
		`SELECT id FROM sessions WHERE content_hash = ? AND saved_at IS NOT NULL LIMIT 1`,
		hash,
	).Scan(&existingID)
	if err == nil && existingID != "" {
		return nil, nil, existingID, ErrDuplicateSession
	}

	now := time.Now().UnixMilli()
	sessID := NewID()
	verID := NewID()
	target = normalizeTarget(target)

	if strings.TrimSpace(title) == "" {
		title = deriveTitleFromFirstLine(content)
	}
	colID := r.resolveCollectionID(collectionID)

	tx, err := r.db.Begin()
	if err != nil {
		return nil, nil, "", err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec(
		`INSERT INTO sessions (id, title, description, origin, request, target, collection_id, saved_at, pinned_version_id, content_hash, created_at, opened_at)
		 VALUES (?, ?, ?, 'pasted', NULL, ?, ?, ?, NULL, ?, ?, ?)`,
		sessID, title, strings.TrimSpace(description), target, colID, now, hash, now, now,
	)
	if err != nil {
		return nil, nil, "", err
	}

	_, err = tx.Exec(
		`INSERT INTO prompt_versions (id, session_id, parent_id, seq, source, content, incomplete, target, created_at)
		 VALUES (?, ?, NULL, 1, 'pasted', ?, 0, ?, ?)`,
		verID, sessID, content, target, now,
	)
	if err != nil {
		return nil, nil, "", err
	}

	_, _ = tx.Exec(
		`INSERT INTO settings (key, value) VALUES ('last_collection_id', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		colID,
	)
	_, _ = tx.Exec(
		`INSERT INTO sessions_fts (session_id, title, request, content) VALUES (?, ?, '', ?)`,
		sessID, title, content,
	)

	if err := tx.Commit(); err != nil {
		return nil, nil, "", err
	}

	savedAt := now
	sess := &Session{
		ID:             sessID,
		Title:          title,
		Description:    strings.TrimSpace(description),
		Origin:         "pasted",
		Target:         target,
		CollectionID:   colID,
		SavedAt:        &savedAt,
		ContentHash:    hash,
		CreatedAt:      now,
		OpenedAt:       now,
		Preview:        makePreview(content),
		VariablesCount: countDistinctVariables(content),
	}
	ver := &PromptVersion{
		ID:        verID,
		SessionID: sessID,
		Seq:       1,
		Source:    "pasted",
		Content:   content,
		Target:    target,
		CreatedAt: now,
	}
	return sess, ver, "", nil
}

func (r *Repo) getSessionRow(sessionID string) (*Session, error) {
	var s Session
	var reqNull, colIDNull, colNameNull, pinnedNull, hashNull sql.NullString
	var savedNull sql.NullInt64

	err := r.db.QueryRow(
		`SELECT s.id, s.title, s.description, s.origin, s.request, s.target,
		        s.collection_id, c.name, s.saved_at, s.pinned_version_id, s.content_hash,
		        s.created_at, s.opened_at
		 FROM sessions s
		 LEFT JOIN collections c ON c.id = s.collection_id
		 WHERE s.id = ?`,
		sessionID,
	).Scan(
		&s.ID, &s.Title, &s.Description, &s.Origin, &reqNull, &s.Target,
		&colIDNull, &colNameNull, &savedNull, &pinnedNull, &hashNull,
		&s.CreatedAt, &s.OpenedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	s.Request = reqNull.String
	s.CollectionID = colIDNull.String
	s.CollectionName = colNameNull.String
	s.PinnedVersionID = pinnedNull.String
	s.ContentHash = hashNull.String
	if savedNull.Valid {
		val := savedNull.Int64
		s.SavedAt = &val
	}
	return &s, nil
}

func (r *Repo) GetSessionDetail(sessionID string, markOpened bool) (*SessionDetail, error) {
	if markOpened {
		now := time.Now().UnixMilli()
		_, _ = r.db.Exec(`UPDATE sessions SET opened_at = ? WHERE id = ?`, now, sessionID)
	}

	sess, err := r.getSessionRow(sessionID)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Query(
		`SELECT `+versionColumns+` FROM prompt_versions WHERE session_id = ? ORDER BY seq ASC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []PromptVersion
	var activeContent string
	for rows.Next() {
		v, err := scanVersion(rows, sess.Target)
		if err != nil {
			return nil, err
		}
		if sess.PinnedVersionID != "" && v.ID == sess.PinnedVersionID {
			activeContent = v.Content
		} else if sess.PinnedVersionID == "" {
			activeContent = v.Content
		}
		versions = append(versions, v)
	}
	sess.Preview = makePreview(activeContent)
	sess.VariablesCount = countDistinctVariables(activeContent)

	return &SessionDetail{
		Session:  *sess,
		Versions: versions,
	}, rows.Err()
}

// expandVietnameseDVariants generates 'd'/'đ' token variants (up to 2 positions → max 4
// variants) because SQLite's `unicode61 remove_diacritics 2` strips vowel accents (`ị`→`i`)
// but treats stroked `đ`/`Đ` as a distinct base letter from `d`/`D`. Expanding at query time
// works immediately on existing databases without rewriting `sessions_fts`.
func expandVietnameseDVariants(tok string) []string {
	runes := []rune(strings.ToLower(tok))
	var dPos []int
	for i, r := range runes {
		if r == 'd' || r == 'đ' {
			dPos = append(dPos, i)
			if len(dPos) == 2 {
				break
			}
		}
	}
	if len(dPos) == 0 {
		return []string{`"` + tok + `"*`}
	}
	variants := []string{string(runes)}
	for _, pos := range dPos {
		var next []string
		for _, cur := range variants {
			r := []rune(cur)
			r[pos] = 'd'
			next = append(next, string(r))
			r[pos] = 'đ'
			next = append(next, string(r))
		}
		variants = next
	}
	for i, v := range variants {
		variants[i] = `"` + v + `"*`
	}
	return variants
}

func buildFTSQuery(raw string) string {
	var clauses []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			vars := expandVietnameseDVariants(cur.String())
			if len(vars) == 1 {
				clauses = append(clauses, vars[0])
			} else {
				clauses = append(clauses, "("+strings.Join(vars, " OR ")+")")
			}
			cur.Reset()
		}
	}
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return strings.Join(clauses, " AND ")
}

func (r *Repo) ListSessions(f SessionFilter) ([]Session, error) {
	var where []string
	var args []any

	if f.SavedOnly {
		where = append(where, "s.saved_at IS NOT NULL")
	}
	if f.CollectionID != "" && f.CollectionID != "all" {
		where = append(where, "s.collection_id = ?")
		args = append(args, f.CollectionID)
	}

	q := strings.TrimSpace(f.Query)
	if q != "" {
		ftsExpr := buildFTSQuery(q)
		if ftsExpr != "" {
			where = append(where, "s.id IN (SELECT session_id FROM sessions_fts WHERE sessions_fts MATCH ?)")
			args = append(args, ftsExpr)
		}
	}

	orderBy := "s.opened_at DESC"
	switch f.Sort {
	case "saved":
		orderBy = "COALESCE(s.saved_at, s.created_at) DESC"
	case "title":
		orderBy = "s.title COLLATE NOCASE ASC"
	}

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}

	querySQL := fmt.Sprintf(`
		SELECT s.id, s.title, s.description, s.origin, s.request, s.target,
		       s.collection_id, c.name, s.saved_at, s.pinned_version_id, s.content_hash,
		       s.created_at, s.opened_at,
		       COALESCE(
		         (SELECT pv.content FROM prompt_versions pv WHERE pv.id = s.pinned_version_id),
		         (SELECT pv.content FROM prompt_versions pv WHERE pv.session_id = s.id ORDER BY pv.seq DESC LIMIT 1),
		         ''
		       ) AS active_content
		FROM sessions s
		LEFT JOIN collections c ON c.id = s.collection_id
		%s
		ORDER BY %s, s.id DESC
		LIMIT ? OFFSET ?
	`, whereSQL, orderBy)

	limit := f.Limit
	if limit <= 0 {
		limit = 200
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}
	args = append(args, limit, offset)

	rows, err := r.db.Query(querySQL, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Session
	for rows.Next() {
		var s Session
		var reqNull, colIDNull, colNameNull, pinnedNull, hashNull sql.NullString
		var savedNull sql.NullInt64
		var activeContent string

		if err := rows.Scan(
			&s.ID, &s.Title, &s.Description, &s.Origin, &reqNull, &s.Target,
			&colIDNull, &colNameNull, &savedNull, &pinnedNull, &hashNull,
			&s.CreatedAt, &s.OpenedAt, &activeContent,
		); err != nil {
			return nil, err
		}
		s.Request = reqNull.String
		s.CollectionID = colIDNull.String
		s.CollectionName = colNameNull.String
		s.PinnedVersionID = pinnedNull.String
		s.ContentHash = hashNull.String
		if savedNull.Valid {
			val := savedNull.Int64
			s.SavedAt = &val
		}
		s.Preview = makePreview(activeContent)
		s.VariablesCount = countDistinctVariables(activeContent)
		out = append(out, s)
	}
	return out, rows.Err()
}

type SessionPatch struct {
	Title           *string
	Description     *string
	CollectionID    *string
	PinnedVersionID *string
	Target          *string
}

func (r *Repo) UpdateSession(id string, patch SessionPatch) (*Session, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var exists string
	if err := tx.QueryRow(`SELECT id FROM sessions WHERE id = ?`, id).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}

	if patch.Title != nil && strings.TrimSpace(*patch.Title) != "" {
		if _, err := tx.Exec(`UPDATE sessions SET title = ? WHERE id = ?`, strings.TrimSpace(*patch.Title), id); err != nil {
			return nil, err
		}
	}
	if patch.Description != nil {
		if _, err := tx.Exec(`UPDATE sessions SET description = ? WHERE id = ?`, strings.TrimSpace(*patch.Description), id); err != nil {
			return nil, err
		}
	}
	if patch.Target != nil {
		if _, err := tx.Exec(`UPDATE sessions SET target = ? WHERE id = ?`, normalizeTarget(*patch.Target), id); err != nil {
			return nil, err
		}
	}
	if patch.CollectionID != nil {
		colID := resolveCollectionIDWith(tx, *patch.CollectionID)
		if _, err := tx.Exec(`UPDATE sessions SET collection_id = ? WHERE id = ?`, colID, id); err != nil {
			return nil, err
		}
		_, _ = tx.Exec(
			`INSERT INTO settings (key, value) VALUES ('last_collection_id', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
			colID,
		)
	}
	if patch.PinnedVersionID != nil {
		pin := strings.TrimSpace(*patch.PinnedVersionID)
		if pin == "" {
			if _, err := tx.Exec(`UPDATE sessions SET pinned_version_id = NULL WHERE id = ?`, id); err != nil {
				return nil, err
			}
		} else {
			if err := versionBelongsTx(tx, id, pin); err != nil {
				return nil, err
			}
			if _, err := tx.Exec(`UPDATE sessions SET pinned_version_id = ? WHERE id = ?`, pin, id); err != nil {
				return nil, err
			}
		}
	}

	_ = syncSessionFTSTx(tx, id)
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.getSessionRow(id)
}

// SaveSessionToLibrary saves the session and pins versionID (the version the user is
// looking at) so later experiments never silently change what the Library shows.
// An empty versionID pins the latest version.
func (r *Repo) SaveSessionToLibrary(id, collectionID, titleOverride, versionID string) (*Session, error) {
	colID := r.resolveCollectionID(collectionID)
	now := time.Now().UnixMilli()

	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var curSaved sql.NullInt64
	if err := tx.QueryRow(`SELECT saved_at FROM sessions WHERE id = ?`, id).Scan(&curSaved); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}

	savedTs := now
	if curSaved.Valid && curSaved.Int64 > 0 {
		savedTs = curSaved.Int64
	}

	pin := strings.TrimSpace(versionID)
	if pin != "" {
		if err := versionBelongsTx(tx, id, pin); err != nil {
			return nil, err
		}
	} else {
		_ = tx.QueryRow(`SELECT id FROM prompt_versions WHERE session_id = ? ORDER BY seq DESC LIMIT 1`, id).Scan(&pin)
	}
	var pinPtr *string
	if pin != "" {
		pinPtr = &pin
	}
	if _, err := tx.Exec(`UPDATE sessions SET pinned_version_id = ? WHERE id = ?`, pinPtr, id); err != nil {
		return nil, err
	}

	if strings.TrimSpace(titleOverride) != "" {
		if _, err := tx.Exec(
			`UPDATE sessions SET collection_id = ?, saved_at = ?, title = ? WHERE id = ?`,
			colID, savedTs, strings.TrimSpace(titleOverride), id,
		); err != nil {
			return nil, err
		}
	} else {
		if _, err := tx.Exec(
			`UPDATE sessions SET collection_id = ?, saved_at = ? WHERE id = ?`,
			colID, savedTs, id,
		); err != nil {
			return nil, err
		}
	}

	_, _ = tx.Exec(
		`INSERT INTO settings (key, value) VALUES ('last_collection_id', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		colID,
	)
	_ = syncSessionFTSTx(tx, id)

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.getSessionRow(id)
}

func (r *Repo) UnsaveSession(id string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(`UPDATE sessions SET saved_at = NULL, collection_id = NULL, pinned_version_id = NULL WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	_ = syncSessionFTSTx(tx, id)
	return tx.Commit()
}

func (r *Repo) DeleteSession(id string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	_, _ = tx.Exec(`DELETE FROM sessions_fts WHERE session_id = ?`, id)
	return tx.Commit()
}

// rowQuerier is satisfied by both *sql.DB and *sql.Tx.
type rowQuerier interface {
	QueryRow(query string, args ...any) *sql.Row
}

func (r *Repo) resolveCollectionID(preferred string) string {
	return resolveCollectionIDWith(r.db, preferred)
}

// resolveCollectionIDWith must receive the open *sql.Tx when called inside a transaction:
// the pool has a single connection, so querying r.db there would deadlock.
func resolveCollectionIDWith(q rowQuerier, preferred string) string {
	cand := strings.TrimSpace(preferred)
	if cand == "" {
		_ = q.QueryRow(`SELECT value FROM settings WHERE key = 'last_collection_id'`).Scan(&cand)
	}
	if cand != "" {
		var found string
		if err := q.QueryRow(`SELECT id FROM collections WHERE id = ?`, cand).Scan(&found); err == nil && found != "" {
			return found
		}
	}
	return SystemCollectionID
}

func deriveTitle(text string) string {
	clean := strings.Join(strings.Fields(text), " ")
	if clean == "" {
		return "Phiên mới"
	}
	runes := []rune(clean)
	if len(runes) > 60 {
		return string(runes[:60]) + "…"
	}
	return clean
}

func deriveTitleFromFirstLine(content string) string {
	for _, line := range strings.Split(content, "\n") {
		clean := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#*- "))
		if clean != "" && clean != "===USER===" {
			runes := []rune(clean)
			if len(runes) > 70 {
				return string(runes[:70]) + "…"
			}
			return clean
		}
	}
	return "Prompt đã dán"
}

// normalizeTarget mirrors ai.NormalizeTarget (storage does not import ai).
func normalizeTarget(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "api", "agent", "chat":
		return strings.ToLower(strings.TrimSpace(t))
	default:
		return "chat"
	}
}
