package credstore

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func newTestFileBackend(t *testing.T) *fileBackend {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	backend, err := newFileBackend()
	if err != nil {
		t.Fatalf("newFileBackend error = %v", err)
	}

	return backend
}

func TestFileBackendSetGetRoundtrip(t *testing.T) {
	backend := newTestFileBackend(t)

	if err := backend.set("gitlab.example.com", "glpat-secret"); err != nil {
		t.Fatalf("set error = %v", err)
	}

	token, err := backend.get("gitlab.example.com")
	if err != nil {
		t.Fatalf("get error = %v", err)
	}
	if token != "glpat-secret" {
		t.Fatalf("token = %q, want stored token", token)
	}
}

func TestFileBackendMultipleDomainsCoexist(t *testing.T) {
	backend := newTestFileBackend(t)

	if err := backend.set("gitlab.example.com", "token-one"); err != nil {
		t.Fatalf("set error = %v", err)
	}
	if err := backend.set("gitlab.other.com", "token-two"); err != nil {
		t.Fatalf("set error = %v", err)
	}

	if token, err := backend.get("gitlab.example.com"); err != nil || token != "token-one" {
		t.Fatalf("get first domain = %q, %v", token, err)
	}
	if token, err := backend.get("gitlab.other.com"); err != nil || token != "token-two" {
		t.Fatalf("get second domain = %q, %v", token, err)
	}
}

func TestFileBackendSetReplacesExistingEntry(t *testing.T) {
	backend := newTestFileBackend(t)

	if err := backend.set("gitlab.example.com", "old-token"); err != nil {
		t.Fatalf("set error = %v", err)
	}
	if err := backend.set("gitlab.example.com", "new-token"); err != nil {
		t.Fatalf("set error = %v", err)
	}

	credentials, err := backend.load()
	if err != nil {
		t.Fatalf("load error = %v", err)
	}
	if len(credentials.Entries) != 1 {
		t.Fatalf("entries = %d, want 1 after replace", len(credentials.Entries))
	}

	if token, err := backend.get("gitlab.example.com"); err != nil || token != "new-token" {
		t.Fatalf("get after replace = %q, %v", token, err)
	}
}

func TestFileBackendNotFound(t *testing.T) {
	backend := newTestFileBackend(t)

	if _, err := backend.get("gitlab.example.com"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get error = %v, want ErrNotFound", err)
	}
	if err := backend.delete("gitlab.example.com"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete error = %v, want ErrNotFound", err)
	}
}

func TestFileBackendDeleteRemovesEntry(t *testing.T) {
	backend := newTestFileBackend(t)

	if err := backend.set("gitlab.example.com", "glpat-secret"); err != nil {
		t.Fatalf("set error = %v", err)
	}
	if err := backend.delete("gitlab.example.com"); err != nil {
		t.Fatalf("delete error = %v", err)
	}
	if err := backend.delete("gitlab.example.com"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete error = %v, want ErrNotFound", err)
	}
}

func TestFileBackendPermissions(t *testing.T) {
	backend := newTestFileBackend(t)

	if err := backend.set("gitlab.example.com", "glpat-secret"); err != nil {
		t.Fatalf("set error = %v", err)
	}

	directoryInfo, err := os.Stat(filepath.Dir(backend.path))
	if err != nil {
		t.Fatalf("stat directory: %v", err)
	}
	if perm := directoryInfo.Mode().Perm(); perm != 0o700 {
		t.Fatalf("directory permissions = %o, want 0700", perm)
	}

	fileInfo, err := os.Stat(backend.path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if perm := fileInfo.Mode().Perm(); perm != 0o600 {
		t.Fatalf("file permissions = %o, want 0600", perm)
	}
}

func TestFileBackendStoresNoPlaintext(t *testing.T) {
	backend := newTestFileBackend(t)

	if err := backend.set("gitlab.example.com", "glpat-secret"); err != nil {
		t.Fatalf("set error = %v", err)
	}

	raw, err := os.ReadFile(backend.path)
	if err != nil {
		t.Fatalf("read credentials file: %v", err)
	}

	if bytes.Contains(raw, []byte("gitlab.example.com")) {
		t.Fatal("credentials file contains plaintext domain")
	}
	if bytes.Contains(raw, []byte("glpat-secret")) {
		t.Fatal("credentials file contains plaintext token")
	}
}

func TestFileBackendCorruptFile(t *testing.T) {
	backend := newTestFileBackend(t)

	if err := os.MkdirAll(filepath.Dir(backend.path), 0o700); err != nil {
		t.Fatalf("create directory: %v", err)
	}
	if err := os.WriteFile(backend.path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}

	if _, err := backend.get("gitlab.example.com"); !errors.Is(err, ErrCorruptCredentials) {
		t.Fatalf("get error = %v, want ErrCorruptCredentials", err)
	}
}

func TestFileBackendUnsupportedVersion(t *testing.T) {
	backend := newTestFileBackend(t)

	if err := os.MkdirAll(filepath.Dir(backend.path), 0o700); err != nil {
		t.Fatalf("create directory: %v", err)
	}
	if err := os.WriteFile(backend.path, []byte(`{"version":2,"entries":[]}`), 0o600); err != nil {
		t.Fatalf("write future version file: %v", err)
	}

	if _, err := backend.get("gitlab.example.com"); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("get error = %v, want ErrUnsupportedVersion", err)
	}
}

func TestFileBackendMissingFileActsEmpty(t *testing.T) {
	backend := newTestFileBackend(t)

	found, err := backend.has("gitlab.example.com")
	if err != nil {
		t.Fatalf("has error = %v", err)
	}
	if found {
		t.Fatal("has = true for missing file")
	}
}
