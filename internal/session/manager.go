// Package session logs a Team Member in and out and keeps their tokens in the
// system keyring, falling back to a private file when the keyring fails.
package session

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/zeckitoz/taiga-tui/internal/taiga"
)

// Authenticator is the part of taiga.AuthAPI the Manager needs.
type Authenticator interface {
	Login(ctx context.Context, username, password string) (taiga.LoginResult, error)
}

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

	mu          sync.Mutex
	keyring     CredentialStore
	keyringDown bool // set on the first keyring failure, for the rest of the run
	file        *FileStore
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
	username := res.Member.Username
	entries := []struct{ key, value string }{
		{m.key(username, accessToken), res.Tokens.Access},
		{m.key(username, refreshToken), res.Tokens.Refresh},
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
			return Saved{}, nil
		}
		m.keyringDown = true
	}

	for _, e := range entries {
		if err := m.file.Set(e.key, e.value); err != nil {
			return Saved{}, err
		}
	}
	return Saved{File: m.file.Path()}, nil
}

// Resume reports whether an unexpired access token is stored for username.
func (m *Manager) Resume(username string) bool {
	if username == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	token, err := m.get(m.key(username, accessToken))
	if err != nil {
		return false
	}
	exp, ok := expiry(token)
	return ok && m.now().Before(exp)
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
