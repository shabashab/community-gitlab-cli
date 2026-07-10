package credstore

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

// KeyringService is the service name used for OS keychain entries.
const KeyringService = "community-gitlab-cli"

type keyringBackend struct{}

func (keyringBackend) set(domain, token string) error {
	if err := keyring.Set(KeyringService, domain, token); err != nil {
		return fmt.Errorf("keyring set: %w", err)
	}

	return nil
}

func (keyringBackend) get(domain string) (string, error) {
	token, err := keyring.Get(KeyringService, domain)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("keyring get: %w", err)
	}

	return token, nil
}

func (keyringBackend) delete(domain string) error {
	err := keyring.Delete(KeyringService, domain)
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("keyring delete: %w", err)
	}

	return nil
}

// disabledKeyringBackend replaces the OS keychain when BackendEnv selects the
// file backend. Writes fail so Store.Set falls through to the file, and reads
// report not-found so lookups and status probes skip the keychain without
// warnings.
type disabledKeyringBackend struct{}

var errKeyringDisabled = fmt.Errorf("keyring disabled by %s=%s", BackendEnv, BackendFile)

func (disabledKeyringBackend) set(string, string) error { return errKeyringDisabled }

func (disabledKeyringBackend) get(string) (string, error) { return "", ErrNotFound }

func (disabledKeyringBackend) delete(string) error { return ErrNotFound }
