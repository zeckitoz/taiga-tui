package session_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/zeckitoz/taiga-tui/internal/session"
)

func TestKeyringStoreUsesServiceTaigaTUI(t *testing.T) {
	keyring.MockInit()
	s := session.NewKeyringStore()

	if err := s.Set("example.org/alice/access", "tok"); err != nil {
		t.Fatalf("set: %v", err)
	}

	if got, err := keyring.Get("taiga-tui", "example.org/alice/access"); err != nil || got != "tok" {
		t.Errorf("keyring holds %q, %v", got, err)
	}
	if got, err := s.Get("example.org/alice/access"); err != nil || got != "tok" {
		t.Errorf("get: %q, %v", got, err)
	}
	if err := s.Delete("example.org/alice/access"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.Get("example.org/alice/access"); !errors.Is(err, session.ErrNotFound) {
		t.Errorf("after delete: got %v, want ErrNotFound", err)
	}
	if err := s.Delete("example.org/alice/access"); err != nil {
		t.Errorf("deleting a missing entry must succeed, got %v", err)
	}
}

func TestKeyringStoreReportsErrors(t *testing.T) {
	broken := errors.New("no secret service")
	keyring.MockInitWithError(broken)
	s := session.NewKeyringStore()

	if _, err := s.Get("k"); !errors.Is(err, broken) {
		t.Errorf("get: got %v", err)
	}
	if err := s.Set("k", "v"); !errors.Is(err, broken) {
		t.Errorf("set: got %v", err)
	}
	if err := s.Delete("k"); !errors.Is(err, broken) {
		t.Errorf("delete: got %v", err)
	}
}

func TestWithTimeoutGivesUpOnHangingStore(t *testing.T) {
	hang := newHangingStore(t)
	s := session.WithTimeout(hang, 20*time.Millisecond)

	if _, err := s.Get("k"); !errors.Is(err, session.ErrTimeout) {
		t.Errorf("get: got %v, want ErrTimeout", err)
	}
	if err := s.Set("k", "v"); !errors.Is(err, session.ErrTimeout) {
		t.Errorf("set: got %v, want ErrTimeout", err)
	}
	if err := s.Delete("k"); !errors.Is(err, session.ErrTimeout) {
		t.Errorf("delete: got %v, want ErrTimeout", err)
	}
}

func TestWithTimeoutPassesResultsThrough(t *testing.T) {
	s := session.WithTimeout(newMemStore(), time.Second)

	if err := s.Set("k", "v"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Get("k"); err != nil || got != "v" {
		t.Errorf("get: %q, %v", got, err)
	}
	if _, err := s.Get("missing"); !errors.Is(err, session.ErrNotFound) {
		t.Errorf("get missing: %v", err)
	}
}

func TestFileStoreRoundTripWithPrivatePermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "taiga-tui")
	s := session.NewFileStore(dir)

	if _, err := s.Get("k"); !errors.Is(err, session.ErrNotFound) {
		t.Errorf("missing file: got %v, want ErrNotFound", err)
	}
	if err := s.Set("example.org/alice/access", "a"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := s.Set("example.org/alice/refresh", "r"); err != nil {
		t.Fatalf("set: %v", err)
	}
	for key, want := range map[string]string{"example.org/alice/access": "a", "example.org/alice/refresh": "r"} {
		if got, err := s.Get(key); err != nil || got != want {
			t.Errorf("%s: got %q, %v", key, got, err)
		}
	}

	if runtime.GOOS != "windows" {
		assertMode(t, dir, 0o700)
		assertMode(t, filepath.Join(dir, "credentials"), 0o600)
	}
}

func TestFileStoreDeleteRemovesEmptyFile(t *testing.T) {
	dir := t.TempDir()
	s := session.NewFileStore(dir)
	if err := s.Set("k", "v"); err != nil {
		t.Fatal(err)
	}

	if err := s.Delete("k"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.Delete("k"); err != nil {
		t.Fatalf("deleting a missing entry must succeed, got %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "credentials")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("credentials file should be gone, stat: %v", err)
	}
}

func TestFileStoreCorruptFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "credentials"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := session.NewFileStore(dir)

	if _, err := s.Get("k"); err == nil || errors.Is(err, session.ErrNotFound) {
		t.Errorf("get: got %v, want a corruption error", err)
	}
	if err := s.Set("k", "v"); err != nil {
		t.Fatalf("set must replace a corrupt file: %v", err)
	}
	if got, err := s.Get("k"); err != nil || got != "v" {
		t.Errorf("get after set: %q, %v", got, err)
	}
}

func assertMode(t *testing.T, name string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s: mode %o, want %o", name, got, want)
	}
}

// memStore is an in-memory CredentialStore that can be told to fail.
type memStore struct {
	data             map[string]string
	getErr, setErr   error
	deleteErr        error
	gets, sets, dels int
}

func newMemStore() *memStore { return &memStore{data: map[string]string{}} }

func (s *memStore) Get(key string) (string, error) {
	s.gets++
	if s.getErr != nil {
		return "", s.getErr
	}
	v, ok := s.data[key]
	if !ok {
		return "", session.ErrNotFound
	}
	return v, nil
}

func (s *memStore) Set(key, value string) error {
	s.sets++
	if s.setErr != nil {
		return s.setErr
	}
	s.data[key] = value
	return nil
}

func (s *memStore) Delete(key string) error {
	s.dels++
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.data, key)
	return nil
}

// hangingStore blocks every call until the test ends, like a keyring waiting
// for a system dialog nobody answers.
type hangingStore struct{ release chan struct{} }

func newHangingStore(t *testing.T) *hangingStore {
	s := &hangingStore{release: make(chan struct{})}
	t.Cleanup(func() { close(s.release) })
	return s
}

func (s *hangingStore) Get(string) (string, error) { <-s.release; return "", session.ErrNotFound }
func (s *hangingStore) Set(string, string) error   { <-s.release; return nil }
func (s *hangingStore) Delete(string) error        { <-s.release; return nil }
