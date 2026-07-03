package credstore

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const currentFileVersion = 1

type kdfParams struct {
	Name      string `json:"name"`
	Time      uint32 `json:"time"`
	MemoryKiB uint32 `json:"memory_kib"`
	Threads   uint8  `json:"threads"`
}

type fileEntry struct {
	Salt   string    `json:"salt"`
	Lookup string    `json:"lookup"`
	Nonce  string    `json:"nonce"`
	Token  string    `json:"token"`
	KDF    kdfParams `json:"kdf"`
}

type credentialsFile struct {
	Version int         `json:"version"`
	Entries []fileEntry `json:"entries"`
}

type fileBackend struct {
	path string
}

func newFileBackend() (*fileBackend, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("locate home directory: %w", err)
	}

	return &fileBackend{path: filepath.Join(home, ".gl", "credentials.json")}, nil
}

func (f *fileBackend) set(domain, token string) error {
	credentials, err := f.load()
	if err != nil {
		return err
	}

	entry, err := encryptToken(domain, token)
	if err != nil {
		return err
	}

	if index := findEntry(credentials, domain); index >= 0 {
		credentials.Entries[index] = entry
	} else {
		credentials.Entries = append(credentials.Entries, entry)
	}

	return f.save(credentials)
}

func (f *fileBackend) get(domain string) (string, error) {
	credentials, err := f.load()
	if err != nil {
		return "", err
	}

	index := findEntry(credentials, domain)
	if index < 0 {
		return "", ErrNotFound
	}

	return decryptToken(domain, credentials.Entries[index])
}

func (f *fileBackend) delete(domain string) error {
	credentials, err := f.load()
	if err != nil {
		return err
	}

	index := findEntry(credentials, domain)
	if index < 0 {
		return ErrNotFound
	}

	credentials.Entries = append(credentials.Entries[:index], credentials.Entries[index+1:]...)

	return f.save(credentials)
}

func (f *fileBackend) has(domain string) (bool, error) {
	credentials, err := f.load()
	if err != nil {
		return false, err
	}

	return findEntry(credentials, domain) >= 0, nil
}

func (f *fileBackend) load() (credentialsFile, error) {
	raw, err := os.ReadFile(f.path)
	if errors.Is(err, fs.ErrNotExist) {
		return credentialsFile{Version: currentFileVersion}, nil
	}
	if err != nil {
		return credentialsFile{}, fmt.Errorf("read credentials file: %w", err)
	}

	var credentials credentialsFile
	if err := json.Unmarshal(raw, &credentials); err != nil {
		return credentialsFile{}, fmt.Errorf("%w: %v", ErrCorruptCredentials, err)
	}

	if credentials.Version > currentFileVersion {
		return credentialsFile{}, fmt.Errorf(
			"%w: version %d (upgrade gl or remove %s)",
			ErrUnsupportedVersion, credentials.Version, f.path,
		)
	}
	if credentials.Version < currentFileVersion {
		return credentialsFile{}, fmt.Errorf("%w: missing version", ErrCorruptCredentials)
	}

	return credentials, nil
}

func (f *fileBackend) save(credentials credentialsFile) error {
	credentials.Version = currentFileVersion

	directory := filepath.Dir(f.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create credentials directory: %w", err)
	}

	raw, err := json.MarshalIndent(credentials, "", "  ")
	if err != nil {
		return fmt.Errorf("encode credentials file: %w", err)
	}
	raw = append(raw, '\n')

	temp, err := os.CreateTemp(directory, "credentials-*.json")
	if err != nil {
		return fmt.Errorf("create temporary credentials file: %w", err)
	}
	tempPath := temp.Name()

	if err := writeAndClose(temp, raw); err != nil {
		_ = os.Remove(tempPath)

		return fmt.Errorf("write credentials file: %w", err)
	}

	if err := os.Rename(tempPath, f.path); err != nil {
		_ = os.Remove(tempPath)

		return fmt.Errorf("replace credentials file: %w", err)
	}

	return nil
}

func writeAndClose(file *os.File, raw []byte) error {
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()

		return err
	}
	if _, err := file.Write(raw); err != nil {
		_ = file.Close()

		return err
	}

	return file.Close()
}

func findEntry(credentials credentialsFile, domain string) int {
	for index, entry := range credentials.Entries {
		salt, err := base64.StdEncoding.DecodeString(entry.Salt)
		if err != nil || len(salt) != saltSize {
			continue
		}

		expected, err := base64.StdEncoding.DecodeString(entry.Lookup)
		if err != nil {
			continue
		}

		if subtle.ConstantTimeCompare(expected, lookupHash(salt, domain)) == 1 {
			return index
		}
	}

	return -1
}
