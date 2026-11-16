package secrets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileStorePermissionsAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.json")
	store := NewFileStore(path)

	key, hint, has, err := store.Get()
	if err != nil || has || key != "" || hint != "" {
		t.Fatalf("expected empty store initially, got has=%v err=%v", has, err)
	}

	if err := store.Set("sk-live-secret-9988a1b2"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("expected 0600 permissions, got %04o", perm)
	}

	key, hint, has, err = store.Get()
	if err != nil || !has || key != "sk-live-secret-9988a1b2" {
		t.Fatalf("unexpected Get: has=%v key=%q err=%v", has, key, err)
	}
	if hint != "••••••••••a1b2" {
		t.Fatalf("unexpected hint: %q", hint)
	}

	if err := store.Delete(); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, _, has, _ = store.Get()
	if has {
		t.Fatalf("expected key deleted")
	}
}
