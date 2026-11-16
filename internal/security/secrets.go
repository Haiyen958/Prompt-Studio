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

// EnsureMasterKey loads an existing 32-byte master key from path or generates
// a new one with 0600 permissions.
func EnsureMasterKey(path string) ([]byte, error) {
	if data, err := os.ReadFile(path); err == nil {
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
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	raw := make([]byte, MasterKeySize)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return nil, err
	}
	encoded := []byte(hex.EncodeToString(raw))
	if err := os.WriteFile(path, encoded, 0600); err != nil {
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

// KeyHint returns the last 4 characters of an API key (e.g. "••••a1b2").
// It never returns the full key.
func KeyHint(apiKey string) string {
	trimmed := strings.TrimSpace(apiKey)
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= 4 {
		return "••••" + string(runes)
	}
	return "••••" + string(runes[len(runes)-4:])
}

// GenerateSessionToken creates a cryptographically random hex token for local auth.
func GenerateSessionToken() (string, error) {
	b := make([]byte, 24)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
