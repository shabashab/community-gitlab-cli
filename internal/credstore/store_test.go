package credstore

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	return New()
}

func TestStorePrefersKeyring(t *testing.T) {
	keyring.MockInit()
	store := newTestStore(t)

	backend, err := store.Set("gitlab.example.com", "glpat-secret")
	if err != nil {
		t.Fatalf("Set error = %v", err)
	}
	if backend != BackendKeyring {
		t.Fatalf("Set backend = %q, want keyring", backend)
	}

	if found, err := store.file.has("gitlab.example.com"); err != nil || found {
		t.Fatalf("file backend has entry = %v, %v; want absent", found, err)
	}

	token, backend, err := store.Get("gitlab.example.com")
	if err != nil {
		t.Fatalf("Get error = %v", err)
	}
	if token != "glpat-secret" || backend != BackendKeyring {
		t.Fatalf("Get = %q from %q, want token from keyring", token, backend)
	}

	removed, err := store.Delete("gitlab.example.com")
	if err != nil {
		t.Fatalf("Delete error = %v", err)
	}
	if len(removed) != 1 || removed[0] != BackendKeyring {
		t.Fatalf("Delete removed = %v, want [keyring]", removed)
	}
}

func TestStoreFallsBackToFileWhenKeyringUnavailable(t *testing.T) {
	keyring.MockInitWithError(errors.New("no dbus"))
	store := newTestStore(t)

	backend, err := store.Set("gitlab.example.com", "glpat-secret")
	if err != nil {
		t.Fatalf("Set error = %v", err)
	}
	if backend != BackendFile {
		t.Fatalf("Set backend = %q, want file", backend)
	}

	token, backend, err := store.Get("gitlab.example.com")
	if err != nil {
		t.Fatalf("Get error = %v", err)
	}
	if token != "glpat-secret" || backend != BackendFile {
		t.Fatalf("Get = %q from %q, want token from file", token, backend)
	}

	status := store.Status("gitlab.example.com")
	if len(status.Backends) != 1 || status.Backends[0] != BackendFile {
		t.Fatalf("Status backends = %v, want [file]", status.Backends)
	}
	if len(status.Warnings) == 0 {
		t.Fatal("Status warnings empty, want keyring warning")
	}
}

func TestStoreGetFindsFileEntryDespiteWorkingKeyring(t *testing.T) {
	keyring.MockInit()
	store := newTestStore(t)

	if err := store.file.set("gitlab.example.com", "file-token"); err != nil {
		t.Fatalf("file set error = %v", err)
	}

	token, backend, err := store.Get("gitlab.example.com")
	if err != nil {
		t.Fatalf("Get error = %v", err)
	}
	if token != "file-token" || backend != BackendFile {
		t.Fatalf("Get = %q from %q, want file token", token, backend)
	}
}

func TestStoreSetRemovesStaleFileEntry(t *testing.T) {
	keyring.MockInit()
	store := newTestStore(t)

	if err := store.file.set("gitlab.example.com", "stale-token"); err != nil {
		t.Fatalf("file set error = %v", err)
	}

	if _, err := store.Set("gitlab.example.com", "fresh-token"); err != nil {
		t.Fatalf("Set error = %v", err)
	}

	if found, err := store.file.has("gitlab.example.com"); err != nil || found {
		t.Fatalf("file backend has stale entry = %v, %v; want removed", found, err)
	}
}

func TestStoreBackendEnvFileBypassesKeyring(t *testing.T) {
	keyring.MockInit()
	t.Setenv(BackendEnv, string(BackendFile))
	store := newTestStore(t)

	backend, err := store.Set("gitlab.example.com", "glpat-secret")
	if err != nil {
		t.Fatalf("Set error = %v", err)
	}
	if backend != BackendFile {
		t.Fatalf("Set backend = %q, want file", backend)
	}

	// The working (mocked) keyring must not have been touched.
	if _, err := (keyringBackend{}).get("gitlab.example.com"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("keyring get error = %v, want ErrNotFound", err)
	}

	token, backend, err := store.Get("gitlab.example.com")
	if err != nil {
		t.Fatalf("Get error = %v", err)
	}
	if token != "glpat-secret" || backend != BackendFile {
		t.Fatalf("Get = %q from %q, want token from file", token, backend)
	}

	status := store.Status("gitlab.example.com")
	if len(status.Backends) != 1 || status.Backends[0] != BackendFile {
		t.Fatalf("Status backends = %v, want [file]", status.Backends)
	}
	if len(status.Warnings) != 0 {
		t.Fatalf("Status warnings = %v, want none (disabled keyring is silent)", status.Warnings)
	}

	removed, err := store.Delete("gitlab.example.com")
	if err != nil {
		t.Fatalf("Delete error = %v", err)
	}
	if len(removed) != 1 || removed[0] != BackendFile {
		t.Fatalf("Delete removed = %v, want [file]", removed)
	}
}

func TestStoreBackendEnvUnrecognizedValueKeepsHybrid(t *testing.T) {
	keyring.MockInit()
	t.Setenv(BackendEnv, "keyring-please")
	store := newTestStore(t)

	backend, err := store.Set("gitlab.example.com", "glpat-secret")
	if err != nil {
		t.Fatalf("Set error = %v", err)
	}
	if backend != BackendKeyring {
		t.Fatalf("Set backend = %q, want keyring", backend)
	}
}

func TestStoreNotFound(t *testing.T) {
	keyring.MockInit()
	store := newTestStore(t)

	if _, _, err := store.Get("gitlab.example.com"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get error = %v, want ErrNotFound", err)
	}
	if _, err := store.Delete("gitlab.example.com"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete error = %v, want ErrNotFound", err)
	}

	status := store.Status("gitlab.example.com")
	if len(status.Backends) != 0 {
		t.Fatalf("Status backends = %v, want empty", status.Backends)
	}
}
