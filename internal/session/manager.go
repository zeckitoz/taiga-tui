// Package session logs a Team Member in and out and keeps their tokens in the
// system keyring, falling back to a private file when the keyring fails.
package session

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"

	"github.com/zeckitoz/taiga-tui/internal/taiga"
)

// Authenticator is the part of taiga.AuthAPI the Manager needs.
type Authenticator interface {
	Login(ctx context.Context, username, password string) (taiga.LoginResult, error)
	Refresh(ctx context.Context, refresh string) (taiga.Tokens, error)
}

// RefreshLockName is the name of the lock file inside the config directory
// that serializes the token refreshes of all instances of the app.
const RefreshLockName = "refresh.lock"

// lockPollInterval is how often a refresh retries while another instance
// holds the lock.
const lockPollInterval = 20 * time.Millisecond

// Options configures a Manager.
type Options struct {
	// Host names the instance in the token keys, e.g. "taiga.example.org".
	Host string
	Auth Authenticator
	// Keyring is the preferred store, usually NewKeyringStore wrapped with WithTimeout.
	Keyring CredentialStore
	// File is the fallback store.
	File *FileStore
	// Now is the clock used to check token expiry.
	Now func() time.Time
}

// Manager owns the session of one Team Member on one instance.
type Manager struct {
	host string
	auth Authenticator
	now  func() time.Time

	// refreshMu orders the refreshes of this process; the lock file does so
	// across processes.
	refreshMu sync.Mutex

	mu          sync.Mutex
	keyring     CredentialStore
	keyringDown bool // set on the first keyring failure, for the rest of the run
	file        *FileStore
	// username is the Team Member whose session is active, set by Resume and Save.
	username string
}

// NewManager returns a Manager.
func NewManager(o Options) *Manager {
	return &Manager{host: o.Host, auth: o.Auth, now: o.Now, keyring: o.Keyring, file: o.File}
}

// Authenticate logs in with a username or email address and a password. It
// does not store anything; pass the result to Save.
func (m *Manager) Authenticate(ctx context.Context, login, password string) (taiga.LoginResult, error) {
	return m.auth.Login(ctx, login, password)
}

// Saved tells where Save stored the tokens.
type Saved struct {
	// File is the credentials file if the keyring failed and the tokens went
	// there, else empty.
	File string
}

// The two tokens of a login, stored as separate entries.
const (
	accessToken  = "access"
	refreshToken = "refresh"
)

// Save stores the tokens of a login as separate entries.
func (m *Manager) Save(res taiga.LoginResult) (Saved, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.store(res.Member.Username, res.Tokens)
}

// store writes the tokens of username to the keyring, or to the file if the
// keyring fails, and makes username the active session. The caller holds mu.
func (m *Manager) store(username string, tokens taiga.Tokens) (Saved, error) {
	entries := []struct{ key, value string }{
		{m.key(username, accessToken), tokens.Access},
		{m.key(username, refreshToken), tokens.Refresh},
	}

	if !m.keyringDown {
		var err error
		for _, e := range entries {
			if err = m.keyring.Set(e.key, e.value); err != nil {
				break
			}
		}
		if err == nil {
			// Older tokens in the fallback file are stale now.
			for _, e := range entries {
				_ = m.file.Delete(e.key)
			}
			m.username = username
			return Saved{}, nil
		}
		m.keyringDown = true
	}

	for _, e := range entries {
		if err := m.file.Set(e.key, e.value); err != nil {
			return Saved{}, err
		}
	}
	m.username = username
	return Saved{File: m.file.Path()}, nil
}

// Resume reports whether a session can be continued for username: an
// unexpired access token is stored, or an unexpired refresh token that renews it.
func (m *Manager) Resume(username string) bool {
	if username == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_, accessOK := m.validToken(username, accessToken)
	_, refreshOK := m.validToken(username, refreshToken)
	if !accessOK && !refreshOK {
		return false
	}
	m.username = username
	return true
}

// Token returns the access token of the active session, which Resume or Save
// started. If it has expired it is renewed first. It implements
// taiga.TokenSource.
func (m *Manager) Token(ctx context.Context) (string, error) {
	m.mu.Lock()
	token, ok := m.validToken(m.username, accessToken)
	stale, _ := m.get(m.key(m.username, accessToken))
	m.mu.Unlock()
	if ok {
		return token, nil
	}
	return m.Refresh(ctx, stale)
}

// Refresh returns a new access token after stale was rejected or has expired.
// It implements taiga.TokenSource.
//
// Taiga rotates refresh tokens, so refreshing the same token twice would log
// the session out. Refresh therefore holds the refresh lock, re-reads the
// stored tokens, and returns the stored access token without calling Taiga if
// another instance rotated the tokens in the meantime. A refresh token that
// Taiga rejects, or none at all, yields taiga.ErrSessionExpired.
func (m *Manager) Refresh(ctx context.Context, stale string) (string, error) {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	unlock, err := m.lockRefresh(ctx)
	if err != nil {
		return "", err
	}
	defer unlock()

	m.mu.Lock()
	username := m.username
	access, accessOK := m.validToken(username, accessToken)
	refresh, refreshErr := m.get(m.key(username, refreshToken))
	m.mu.Unlock()
	if accessOK && access != stale {
		return access, nil
	}
	if username == "" || refreshErr != nil || refresh == "" {
		return "", taiga.ErrSessionExpired
	}

	tokens, err := m.auth.Refresh(ctx, refresh)
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.store(username, tokens); err != nil {
		return "", fmt.Errorf("storing the renewed login: %w", err)
	}
	return tokens.Access, nil
}

// lockRefresh takes the lock file shared by all instances of the app on this
// instance's config directory and returns the function that releases it.
func (m *Manager) lockRefresh(ctx context.Context) (func(), error) {
	dir := filepath.Dir(m.file.Path())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	lock := flock.New(filepath.Join(dir, RefreshLockName))
	if _, err := lock.TryLockContext(ctx, lockPollInterval); err != nil {
		return nil, err
	}
	return func() { _ = lock.Unlock() }, nil
}

// validToken returns the stored token of the given kind if it has not
// expired.
func (m *Manager) validToken(username, kind string) (string, bool) {
	if username == "" {
		return "", false
	}
	token, err := m.get(m.key(username, kind))
	if err != nil {
		return "", false
	}
	exp, ok := expiry(token)
	return token, ok && m.now().Before(exp)
}

// Logout deletes the tokens of username from the keyring and every token of
// the instance from the file. Without a username, as after a lost state.json,
// only the file can be cleared, because keyring entries cannot be listed. The
// keyring is given up after its first failure; every failure is reported.
func (m *Manager) Logout(username string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var errs []error
	if username != "" {
		for _, kind := range []string{accessToken, refreshToken} {
			if m.keyringDown {
				break
			}
			if err := m.keyring.Delete(m.key(username, kind)); err != nil {
				m.keyringDown = true
				errs = append(errs, err)
			}
		}
	}
	if err := m.file.DeletePrefix(m.host + "/"); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// get reads key from the file first: it only holds tokens while the keyring
// fails, so its entries are newer than any left in the keyring. Then it
// reads the keyring.
func (m *Manager) get(key string) (string, error) {
	if v, err := m.file.Get(key); err == nil {
		return v, nil
	}
	if m.keyringDown {
		return "", ErrNotFound
	}
	v, err := m.keyring.Get(key)
	if err != nil && !errors.Is(err, ErrNotFound) {
		m.keyringDown = true
	}
	return v, err
}

func (m *Manager) key(username, kind string) string {
	return m.host + "/" + username + "/" + kind
}

// expiry reads the exp claim of a JWT without checking its signature.
func expiry(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
		return time.Time{}, false
	}
	return time.Unix(claims.Exp, 0), true
}
