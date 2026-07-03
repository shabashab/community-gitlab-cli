// Package credstore persists GitLab credentials keyed by instance domain.
//
// Credentials are stored in the OS keychain when one is available and fall
// back to an encrypted JSON file under ~/.gl otherwise. The file backend
// never stores the domain or token in plaintext: entries are located through
// a salted domain hash and tokens are sealed with a key derived from the
// domain itself, so a credential is only recoverable by a caller that already
// knows the real domain (from git origin discovery or an explicit flag).
package credstore

import (
	"errors"
	"fmt"
)

// Backend identifies which storage mechanism holds a credential.
type Backend string

const (
	BackendKeyring Backend = "keyring"
	BackendFile    Backend = "file"
)

var (
	ErrNotFound           = errors.New("credential not found")
	ErrInvalidDomain      = errors.New("invalid credential domain")
	ErrCorruptCredentials = errors.New("corrupt credentials file")
	ErrUnsupportedVersion = errors.New("unsupported credentials file version")
)

// Store is the hybrid credential store combining the OS keychain with the
// encrypted file fallback.
type Store struct {
	keyring keyringBackend
	file    *fileBackend
	fileErr error
}

// New builds the hybrid store. A failure to locate the credentials file (no
// home directory) is deferred until the file backend is actually needed.
func New() *Store {
	file, err := newFileBackend()

	return &Store{
		file:    file,
		fileErr: err,
	}
}

// Set stores the token for domain, preferring the OS keychain and falling
// back to the encrypted file when the keychain is unavailable. The credential
// ends up single-homed: a successful write removes any stale copy from the
// other backend.
func (s *Store) Set(domain, token string) (Backend, error) {
	keyringErr := s.keyring.set(domain, token)
	if keyringErr == nil {
		if s.fileErr == nil {
			_ = s.file.delete(domain)
		}

		return BackendKeyring, nil
	}

	if s.fileErr != nil {
		return "", fmt.Errorf("store credential: keyring unavailable (%v): %w", keyringErr, s.fileErr)
	}
	if err := s.file.set(domain, token); err != nil {
		return "", fmt.Errorf("store credential: keyring unavailable (%v): %w", keyringErr, err)
	}

	return BackendFile, nil
}

// Get returns the stored token for domain, checking the keychain first and
// the file second so credentials remain reachable across environments (for
// example stored over SSH into the file, read later in a desktop session).
func (s *Store) Get(domain string) (string, Backend, error) {
	token, err := s.keyring.get(domain)
	if err == nil {
		return token, BackendKeyring, nil
	}

	if s.fileErr != nil {
		return "", "", ErrNotFound
	}

	token, err = s.file.get(domain)
	if err != nil {
		return "", "", err
	}

	return token, BackendFile, nil
}

// Delete removes the credential for domain from every backend holding it and
// reports which backends were affected. It returns ErrNotFound when no
// backend held an entry.
func (s *Store) Delete(domain string) ([]Backend, error) {
	var removed []Backend

	if err := s.keyring.delete(domain); err == nil {
		removed = append(removed, BackendKeyring)
	}

	var fileErr error
	if s.fileErr == nil {
		switch err := s.file.delete(domain); {
		case err == nil:
			removed = append(removed, BackendFile)
		case !errors.Is(err, ErrNotFound):
			fileErr = err
		}
	}

	if len(removed) == 0 {
		if fileErr != nil {
			return nil, fileErr
		}

		return nil, ErrNotFound
	}

	return removed, nil
}

// Status describes where a credential for a domain is stored and any
// per-backend problems encountered while probing.
type Status struct {
	Backends []Backend
	Warnings []string
}

// Status probes both backends for domain. Per-backend failures become
// warnings instead of errors so status reporting always succeeds.
func (s *Store) Status(domain string) Status {
	var status Status

	switch _, err := s.keyring.get(domain); {
	case err == nil:
		status.Backends = append(status.Backends, BackendKeyring)
	case !errors.Is(err, ErrNotFound):
		status.Warnings = append(status.Warnings, fmt.Sprintf("keyring: %v", err))
	}

	if s.fileErr != nil {
		status.Warnings = append(status.Warnings, fmt.Sprintf("file: %v", s.fileErr))

		return status
	}

	switch found, err := s.file.has(domain); {
	case err != nil:
		status.Warnings = append(status.Warnings, fmt.Sprintf("file: %v", err))
	case found:
		status.Backends = append(status.Backends, BackendFile)
	}

	return status
}
