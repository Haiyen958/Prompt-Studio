package secrets

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type filePayload struct {
	APIKey string `json:"apiKey,omitempty"`
}

// Store persists the provider API key exclusively in a local 0600 file
// (~/.prompt-studio/secrets.json) and never in SQLite or logs.
type Store struct {
	mu   sync.RWMutex
	path string
}

func NewFileStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Path() string {
	return s.path
}

// KeyHint returns a masked representation showing only the last 4 characters.
func KeyHint(apiKey string) string {
	trimmed := strings.TrimSpace(apiKey)
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= 4 {
		return "••••••••••" + string(runes)
	}
	return "••••••••••" + string(runes[len(runes)-4:])
}

func (s *Store) Get() (apiKey string, hint string, hasKey bool, err error) {
	if s == nil || s.path == "" {
		return "", "", false, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	raw, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", false, nil
		}
		return "", "", false, err
	}

	var p filePayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", "", false, err
	}
	key := strings.TrimSpace(p.APIKey)
	if key == "" {
		return "", "", false, nil
	}
	return key, KeyHint(key), true, nil
}

func (s *Store) Set(apiKey string) error {
	if s == nil || s.path == "" {
		return nil
	}
	trimmed := strings.TrimSpace(apiKey)
	if trimmed == "" {
		return s.Delete()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(filePayload{APIKey: trimmed}, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, s.path)
}

func (s *Store) Delete() error {
	if s == nil || s.path == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
