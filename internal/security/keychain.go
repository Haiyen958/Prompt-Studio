package security

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

var ErrKeychainUnavailable = errors.New("os keychain unavailable")

// Keychain abstracts OS credential storage with graceful fallback when headless.
type Keychain interface {
	Set(service, account, secret string) error
	Get(service, account string) (string, error)
	Delete(service, account string) error
}

// SystemKeychain uses libsecret's `secret-tool` on Linux when a D-Bus session is active,
// returning ErrKeychainUnavailable on headless systems so SQLite AES-256-GCM fallback is used.
type SystemKeychain struct {
	Disabled bool
}

func (k SystemKeychain) available() bool {
	if k.Disabled {
		return false
	}
	_, err := exec.LookPath("secret-tool")
	return err == nil
}

func (k SystemKeychain) Set(service, account, secret string) error {
	if !k.available() {
		return ErrKeychainUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "secret-tool", "store", "--label=Prompt Studio API Key", "service", service, "account", account)
	cmd.Stdin = strings.NewReader(secret)
	if err := cmd.Run(); err != nil {
		return ErrKeychainUnavailable
	}
	return nil
}

func (k SystemKeychain) Get(service, account string) (string, error) {
	if !k.available() {
		return "", ErrKeychainUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "secret-tool", "lookup", "service", service, "account", account)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", ErrKeychainUnavailable
	}
	val := strings.TrimSpace(out.String())
	if val == "" {
		return "", ErrKeychainUnavailable
	}
	return val, nil
}

func (k SystemKeychain) Delete(service, account string) error {
	if !k.available() {
		return ErrKeychainUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "secret-tool", "clear", "service", service, "account", account)
	if err := cmd.Run(); err != nil {
		return ErrKeychainUnavailable
	}
	return nil
}
