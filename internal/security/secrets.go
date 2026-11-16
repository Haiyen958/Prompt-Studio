package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const MasterKeySize = 32 // 256-bit key for AES-256-GCM

// LoadMasterKey reads the legacy v1 master key (hex or raw 32 bytes). A missing file
// returns an error wrapping os.ErrNotExist; it never creates one.
func LoadMasterKey(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(string(data))
	if decoded, decErr := hex.DecodeString(trimmed); decErr == nil && len(decoded) == MasterKeySize {
		_ = os.Chmod(path, 0600)
		return decoded, nil
	}
	if len(data) == MasterKeySize {
		_ = os.Chmod(path, 0600)
		return data, nil
	}
	return nil, fmt.Errorf("invalid master key length in %s", path)
}

// EnsureMasterKey loads the master key at path or creates a new one (0600).
// Only v1 test fixtures need creation; the v1 migrator uses LoadMasterKey.
func EnsureMasterKey(path string) ([]byte, error) {
	if key, err := LoadMasterKey(path); err == nil || !errors.Is(err, os.ErrNotExist) {
		return key, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	raw := make([]byte, MasterKeySize)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(raw)), 0600); err != nil {
		return nil, err
	}
	return raw, nil
}

// EncryptAESGCM encrypts plaintext using AES-256-GCM with the provided 32-byte key.
func EncryptAESGCM(masterKey []byte, plaintext string) (ciphertext []byte, nonce []byte, err error) {
	if len(masterKey) != MasterKeySize {
		return nil, nil, errors.New("master key must be 32 bytes")
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	ciphertext = gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return ciphertext, nonce, nil
}

// DecryptAESGCM decrypts AES-256-GCM ciphertext with the provided 32-byte key and nonce.
func DecryptAESGCM(masterKey, ciphertext, nonce []byte) (string, error) {
	if len(masterKey) != MasterKeySize {
		return "", errors.New("master key must be 32 bytes")
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(nonce) != gcm.NonceSize() {
		return "", errors.New("invalid nonce size")
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// GenerateSessionToken creates a cryptographically random hex token for local auth.
func GenerateSessionToken() (string, error) {
	b := make([]byte, 24)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
