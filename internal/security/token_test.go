package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateSessionTokenIsStableAndPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "session.token")

	first, err := LoadOrCreateSessionToken(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !sessionTokenRe.MatchString(first) {
		t.Fatalf("unexpected token format %q", first)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("token file perm = %o, want 0600", perm)
	}

	second, err := LoadOrCreateSessionToken(path)
	if err != nil || second != first {
		t.Fatalf("reload: got %q err %v, want %q", second, err, first)
	}

	// A tampered file is replaced, never trusted.
	if err := os.WriteFile(path, []byte("short"), 0600); err != nil {
		t.Fatal(err)
	}
	third, err := LoadOrCreateSessionToken(path)
	if err != nil || third == "short" || !sessionTokenRe.MatchString(third) {
		t.Fatalf("tampered: got %q err %v", third, err)
	}
}
