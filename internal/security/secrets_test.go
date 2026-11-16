package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMasterKeyAndAESGCMRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	keyPath := filepath.Join(tmp, "master.key")

	key1, err := EnsureMasterKey(keyPath)
	if err != nil {
		t.Fatalf("EnsureMasterKey create failed: %v", err)
	}
	if len(key1) != MasterKeySize {
		t.Fatalf("expected %d bytes, got %d", MasterKeySize, len(key1))
	}
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat keyPath: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("expected 0600 permissions, got %o", perm)
	}

	key2, err := EnsureMasterKey(keyPath)
	if err != nil {
		t.Fatalf("EnsureMasterKey reload failed: %v", err)
	}
	if string(key1) != string(key2) {
		t.Fatal("reloaded master key does not match original")
	}

	secret := "sk-proj-super-secret-api-key-9876"
	ct, nonce, err := EncryptAESGCM(key1, secret)
	if err != nil {
		t.Fatalf("EncryptAESGCM failed: %v", err)
	}
	got, err := DecryptAESGCM(key2, ct, nonce)
	if err != nil {
		t.Fatalf("DecryptAESGCM failed: %v", err)
	}
	if got != secret {
		t.Fatalf("decrypted mismatch: got %q, want %q", got, secret)
	}

}
