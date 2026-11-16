package storage

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"

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
