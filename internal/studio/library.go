package studio

import (
	"strings"
	"unicode/utf8"

	"prompt-studio/internal/storage"
)

// Library, history, collection, version and backup operations.
// Handlers call these instead of the repository so that every business rule has one
// home (this package) and storage stays a SQL/transaction layer.

func (s *Service) ListSessions(f storage.SessionFilter) ([]storage.Session, error) {
	return s.repo.ListSessions(f)
}

// OpenSession returns the session with all versions and records it as opened.
func (s *Service) OpenSession(id string) (*storage.SessionDetail, error) {
	return s.repo.GetSessionDetail(id, true)
}

func (s *Service) UpdateSession(id string, patch storage.SessionPatch) (*storage.Session, error) {
	if patch.Title != nil {
		trimmed := strings.TrimSpace(*patch.Title)
		if trimmed == "" {
			return nil, ErrSessionTitleEmpty
		}
		if utf8.RuneCountInString(trimmed) > MaxSessionTitleChars {
			return nil, ErrSessionTitleTooLong
		}
		patch.Title = &trimmed
	}
	return s.repo.UpdateSession(id, patch)
}

// SaveSession saves to the Library and pins versionID (latest when empty).
func (s *Service) SaveSession(id, collectionID, title, versionID string) (*storage.Session, error) {
	if trimmed := strings.TrimSpace(title); utf8.RuneCountInString(trimmed) > MaxSessionTitleChars {
		return nil, ErrSessionTitleTooLong
	}
	return s.repo.SaveSessionToLibrary(id, collectionID, title, versionID)
}

func (s *Service) UnsaveSession(id string) error { return s.repo.UnsaveSession(id) }

func (s *Service) DeleteSession(id string) error { return s.repo.DeleteSession(id) }

func (s *Service) GetVersion(id string) (*storage.PromptVersion, *storage.Session, error) {
	return s.repo.GetVersion(strings.TrimSpace(id))
}

func (s *Service) ListCollections() ([]storage.Collection, int, error) {
	return s.repo.ListCollections()
}

func (s *Service) CreateCollection(name string) (*storage.Collection, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrCollectionName
	}
	col, err := s.repo.CreateCollection(name)
	return col, collectionNameErr(name, err)
}

func (s *Service) RenameCollection(id, name string) (*storage.Collection, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrCollectionName
	}
	col, err := s.repo.RenameCollection(id, name)
	return col, collectionNameErr(name, err)
}

func (s *Service) DeleteCollection(id string) error { return s.repo.DeleteCollection(id) }

// collectionNameErr turns the UNIQUE(name) violation into a user-facing input error.
func collectionNameErr(name string, err error) error {
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return invalidInput("collection “%s” đã tồn tại", name)
	}
	return err
}

func (s *Service) Export() (*storage.BackupBundle, error) { return s.repo.ExportAll() }

func (s *Service) Import(b *storage.BackupBundle) error { return s.repo.ImportAll(b) }

// Settings returns the raw settings table plus the API key hint (never the key).
func (s *Service) Settings() (kv map[string]string, keyHint string, hasKey bool, err error) {
	kv, err = s.repo.GetSettings()
	if err != nil {
		return nil, "", false, err
	}
	_, keyHint, hasKey, err = s.repo.GetAPIKey()
	return kv, keyHint, hasKey, err
}

func (s *Service) DeleteAPIKey() error { return s.repo.DeleteAPIKey() }

// SessionExists returns storage.ErrNotFound for an unknown session id.
func (s *Service) SessionExists(id string) (bool, error) {
	if _, err := s.repo.GetSessionDetail(id, false); err != nil {
		return false, err
	}
	return true, nil
}
