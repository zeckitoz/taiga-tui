package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/zeckitoz/taiga-tui/internal/atomicfile"
)

// KeyringService is the service name of all keyring entries of the app.
const KeyringService = "taiga-tui"

// KeyringTimeout is the time a keyring access may take before the keyring is
// treated as unavailable.
const KeyringTimeout = 5 * time.Second

// CredentialsFileName is the name of the fallback token file inside the config directory.
const CredentialsFileName = "credentials"

var (
	// ErrNotFound means the store holds no entry for the key.
	ErrNotFound = errors.New("credential not found")
	// ErrTimeout means the store did not answer within its timeout.
	ErrTimeout = errors.New("credential store did not answer in time")
)

// CredentialStore keeps secrets by key.
type CredentialStore interface {
	// Get returns the secret stored under key, or ErrNotFound.
	Get(key string) (string, error)
	// Set stores value under key, replacing an existing entry.
	Set(key, value string) error
	// Delete removes the entry under key. A missing entry is not an error.
	Delete(key string) error
}

// keyringStore is the system keyring: macOS Keychain, Secret Service on
// Linux, Windows Credential Manager.
type keyringStore struct{}

// NewKeyringStore returns the system keyring, using service KeyringService.
// Calls can block, so wrap it with WithTimeout.
func NewKeyringStore() CredentialStore { return keyringStore{} }

func (keyringStore) Get(key string) (string, error) {
	v, err := keyring.Get(KeyringService, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return v, err
}

func (keyringStore) Set(key, value string) error {
	return keyring.Set(KeyringService, key, value)
}

func (keyringStore) Delete(key string) error {
	if err := keyring.Delete(KeyringService, key); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

// WithTimeout returns a store that gives up on calls to s after d with
// ErrTimeout. A call that timed out keeps running in the background.
func WithTimeout(s CredentialStore, d time.Duration) CredentialStore {
	return timeoutStore{s: s, d: d}
}

type timeoutStore struct {
	s CredentialStore
	d time.Duration
}

func (t timeoutStore) Get(key string) (string, error) {
	return withTimeout(t.d, func() (string, error) { return t.s.Get(key) })
}

func (t timeoutStore) Set(key, value string) error {
	_, err := withTimeout(t.d, func() (struct{}, error) { return struct{}{}, t.s.Set(key, value) })
	return err
}

func (t timeoutStore) Delete(key string) error {
	_, err := withTimeout(t.d, func() (struct{}, error) { return struct{}{}, t.s.Delete(key) })
	return err
}

// withTimeout calls f and gives up after d. The results travel through a
// buffered channel, so a call that finishes late writes nothing the caller reads.
func withTimeout[T any](d time.Duration, f func() (T, error)) (T, error) {
	type result struct {
		v   T
		err error
	}
	done := make(chan result, 1)
	go func() {
		v, err := f()
		done <- result{v, err}
	}()
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case r := <-done:
		return r.v, r.err
	case <-timer.C:
		var zero T
		return zero, ErrTimeout
	}
}

// FileStore keeps secrets in a JSON file with mode 0600, the fallback for
// systems without a working keyring.
type FileStore struct {
	path string
	mu   sync.Mutex
}

// NewFileStore returns the credentials file in the config directory dir.
func NewFileStore(dir string) *FileStore {
	return &FileStore{path: filepath.Join(dir, CredentialsFileName)}
}

// Path returns the file name.
func (s *FileStore) Path() string { return s.path }

// Get returns the secret stored under key. A corrupt file is an error other
// than ErrNotFound.
func (s *FileStore) Get(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.read()
	if err != nil {
		return "", err
	}
	v, ok := entries[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

// Set stores value under key. A corrupt file is replaced.
func (s *FileStore) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.read()
	if err != nil {
		entries = map[string]string{}
	}
	entries[key] = value
	return s.write(entries)
}

// Delete removes the entry under key. The file is removed when it becomes
// empty or is corrupt.
func (s *FileStore) Delete(key string) error {
	return s.deleteWhere(func(k string) bool { return k == key })
}

// DeletePrefix removes every entry whose key starts with prefix.
func (s *FileStore) DeletePrefix(prefix string) error {
	return s.deleteWhere(func(k string) bool { return strings.HasPrefix(k, prefix) })
}

func (s *FileStore) deleteWhere(match func(key string) bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.read()
	if err != nil {
		return s.remove()
	}
	n := len(entries)
	maps.DeleteFunc(entries, func(k, _ string) bool { return match(k) })
	switch {
	case len(entries) == n:
		return nil
	case len(entries) == 0:
		return s.remove()
	default:
		return s.write(entries)
	}
}

func (s *FileStore) read() (map[string]string, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var entries map[string]string
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("%s is corrupt: %w", s.path, err)
	}
	if entries == nil {
		entries = map[string]string{}
	}
	return entries, nil
}

func (s *FileStore) write(entries map[string]string) error {
	return atomicfile.WriteJSON(s.path, entries)
}

func (s *FileStore) remove() error {
	if err := os.Remove(s.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
