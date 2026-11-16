package security

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// sessionTokenRe matches tokens produced by GenerateSessionToken (24 random bytes, hex).
var sessionTokenRe = regexp.MustCompile(`^[0-9a-f]{48}$`)

// LoadOrCreateSessionToken returns the local session token stored at path, creating it
// (0600) on first run. A stable token lets the user open the startup URL once and keep
// using the app across restarts through the long-lived cookie.
// A missing, unreadable-format or tampered file is replaced with a fresh token.
func LoadOrCreateSessionToken(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err == nil {
		tok := strings.TrimSpace(string(raw))
		if sessionTokenRe.MatchString(tok) {
			return tok, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	tok, err := GenerateSessionToken()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(tok+"\n"), 0600); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp, 0600); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return tok, nil
}
