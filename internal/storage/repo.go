package storage

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"prompt-studio/internal/secrets"
)

var (
	ErrNotFound          = errors.New("record not found")
	ErrDuplicateSession  = errors.New("duplicate pasted prompt")
	ErrProtectedResource = errors.New("system collection cannot be modified or deleted")
	ErrVersionMismatch   = errors.New("version does not belong to this session")
)

var varCountRe = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

type AssumptionRecord struct {
	Label        string   `json:"label"`
	Value        string   `json:"value"`
	Alternatives []string `json:"alternatives,omitempty"`
}

type Collection struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsSystem  bool   `json:"isSystem"`
	SortOrder int    `json:"sortOrder"`
	Count     int    `json:"count"`
}

type Session struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	Origin          string `json:"origin"` // "generated" | "pasted"
	Request         string `json:"request,omitempty"`
	Target          string `json:"target"` // "chat" | "api" | "agent"
	CollectionID    string `json:"collectionId,omitempty"`
	CollectionName  string `json:"collectionName,omitempty"`
	SavedAt         *int64 `json:"savedAt,omitempty"`
	PinnedVersionID string `json:"pinnedVersionId,omitempty"`
	ContentHash     string `json:"contentHash,omitempty"`
	CreatedAt       int64  `json:"createdAt"`
	OpenedAt        int64  `json:"openedAt"`
	Preview         string `json:"preview,omitempty"`
	VariablesCount  int    `json:"variablesCount"`
}

type PromptVersion struct {
	ID              string             `json:"id"`
	SessionID       string             `json:"sessionId"`
	ParentID        string             `json:"parentId,omitempty"`
	Seq             int                `json:"seq"`
	Source          string             `json:"source"` // generate | regenerate | improve | assumption | manual | pasted
	Instruction     string             `json:"instruction,omitempty"`
	Content         string             `json:"content"`
	Assumptions     []AssumptionRecord `json:"assumptions,omitempty"`
	Incomplete      bool               `json:"incomplete"`
	Target          string             `json:"target"` // chat | api | agent (per version; falls back to the session target)
	TemplateVersion string             `json:"templateVersion,omitempty"`
	Model           string             `json:"model,omitempty"`
	InputTokens     int                `json:"inputTokens"`
	OutputTokens    int                `json:"outputTokens"`
	LatencyMs       int                `json:"latencyMs"`
	CreatedAt       int64              `json:"createdAt"`
}

type SessionDetail struct {
	Session  Session         `json:"session"`
	Versions []PromptVersion `json:"versions"`
}

type SessionFilter struct {
	Query        string
	SavedOnly    bool
	CollectionID string
	Sort         string // "recent" | "saved" | "title"
	Limit        int    // 0 = default (200)
	Offset       int
}

type Repo struct {
	db      *sql.DB
	secrets *secrets.Store
}

func NewRepo(db *sql.DB, secretStore *secrets.Store) *Repo {
	return &Repo{
		db:      db,
		secrets: secretStore,
	}
}

func (r *Repo) DB() *sql.DB {
	return r.db
}

// NormalizeContentHash collapses all runs of whitespace into single spaces and returns SHA-256 hex.
func NormalizeContentHash(content string) string {
	fields := strings.Fields(content)
	normalized := strings.Join(fields, " ")
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func countDistinctVariables(content string) int {
	matches := varCountRe.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return 0
	}
	seen := make(map[string]struct{}, len(matches))
	for _, m := range matches {
		if len(m) > 1 {
			seen[m[1]] = struct{}{}
		}
	}
	return len(seen)
}

func makePreview(content string) string {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	var kept []string
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" || t == "===USER===" {
			continue
		}
		kept = append(kept, t)
		if len(kept) >= 2 {
			break
		}
	}
	joined := strings.Join(kept, " ")
	runes := []rune(joined)
	if len(runes) > 220 {
		return string(runes[:220]) + "…"
	}
	return joined
}

// --- Settings & Secrets ---

func (r *Repo) GetSettings() (map[string]string, error) {
	rows, err := r.db.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (r *Repo) SaveSettings(kv map[string]string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for k, v := range kv {
		if _, err := tx.Exec(
			`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
			k, v,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repo) GetAPIKey() (string, string, bool, error) {
	if r.secrets == nil {
		return "", "", false, nil
	}
	return r.secrets.Get()
}

func (r *Repo) SetAPIKey(apiKey string) error {
	if r.secrets == nil {
		return nil
	}
	return r.secrets.Set(apiKey)
}

func (r *Repo) DeleteAPIKey() error {
	if r.secrets == nil {
		return nil
	}
	return r.secrets.Delete()
}

// --- Sessions & Versions ---

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

func buildFTSQuery(raw string) string {
	var tokens []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, `"`+cur.String()+`"*`)
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
	return strings.Join(tokens, " AND ")
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

func versionBelongsTx(tx *sql.Tx, sessionID, versionID string) error {
	var owner string
	err := tx.QueryRow(`SELECT session_id FROM prompt_versions WHERE id = ?`, versionID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && owner != sessionID) {
		return ErrVersionMismatch
	}
	return err
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
	res, err := r.db.Exec(`UPDATE sessions SET saved_at = NULL, collection_id = NULL, pinned_version_id = NULL WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
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

// --- Collections ---

func (r *Repo) ListCollections() ([]Collection, int, error) {
	rows, err := r.db.Query(`
		SELECT c.id, c.name, c.is_system, c.sort_order,
		       COUNT(s.id) AS cnt
		FROM collections c
		LEFT JOIN sessions s ON s.collection_id = c.id AND s.saved_at IS NOT NULL
		GROUP BY c.id
		ORDER BY c.is_system ASC, c.sort_order ASC, c.name COLLATE NOCASE ASC
	`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var cols []Collection
	totalSaved := 0
	for rows.Next() {
		var c Collection
		var sysInt int
		if err := rows.Scan(&c.ID, &c.Name, &sysInt, &c.SortOrder, &c.Count); err != nil {
			return nil, 0, err
		}
		c.IsSystem = sysInt != 0
		totalSaved += c.Count
		cols = append(cols, c)
	}
	return cols, totalSaved, rows.Err()
}

func (r *Repo) CreateCollection(name string) (*Collection, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, errors.New("tên collection không được để trống")
	}
	id := NewID()
	var maxOrder int
	_ = r.db.QueryRow(`SELECT COALESCE(MAX(sort_order), 0) + 1 FROM collections WHERE is_system = 0`).Scan(&maxOrder)

	_, err := r.db.Exec(
		`INSERT INTO collections (id, name, is_system, sort_order) VALUES (?, ?, 0, ?)`,
		id, trimmed, maxOrder,
	)
	if err != nil {
		return nil, err
	}
	return &Collection{
		ID:        id,
		Name:      trimmed,
		IsSystem:  false,
		SortOrder: maxOrder,
		Count:     0,
	}, nil
}

func (r *Repo) RenameCollection(id, name string) (*Collection, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, errors.New("tên collection không được để trống")
	}
	var sysInt int
	if err := r.db.QueryRow(`SELECT is_system FROM collections WHERE id = ?`, id).Scan(&sysInt); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	if sysInt != 0 || id == SystemCollectionID {
		return nil, ErrProtectedResource
	}

	if _, err := r.db.Exec(`UPDATE collections SET name = ? WHERE id = ?`, trimmed, id); err != nil {
		return nil, err
	}
	cols, _, err := r.ListCollections()
	if err != nil {
		return nil, err
	}
	for _, c := range cols {
		if c.ID == id {
			return &c, nil
		}
	}
	return nil, ErrNotFound
}

func (r *Repo) DeleteCollection(id string) error {
	var sysInt int
	if err := r.db.QueryRow(`SELECT is_system FROM collections WHERE id = ?`, id).Scan(&sysInt); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if sysInt != 0 || id == SystemCollectionID {
		return ErrProtectedResource
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`UPDATE sessions SET collection_id = ? WHERE collection_id = ?`, SystemCollectionID, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE settings SET value = ? WHERE key = 'last_collection_id' AND value = ?`, SystemCollectionID, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM collections WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
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
