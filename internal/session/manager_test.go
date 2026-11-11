package session_test

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zeckitoz/taiga-tui/internal/domain"
	"github.com/zeckitoz/taiga-tui/internal/session"
	"github.com/zeckitoz/taiga-tui/internal/taiga"
)

var (
	now   = time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	alice = domain.TeamMember{ID: 42, Username: "alice", FullName: "Alice Example"}
)

const host = "taiga.example.org"

func TestAuthenticatePassesThroughTheAuthAPI(t *testing.T) {
	auth := &fakeAuth{result: loginResult(now.Add(24 * time.Hour))}
	m := newManager(t, auth, newMemStore(), t.TempDir())

	res, err := m.Authenticate(t.Context(), "alice@example.org", "pw")

	if err != nil || res.Member != alice {
		t.Fatalf("got %+v, %v", res, err)
	}
	if auth.username != "alice@example.org" || auth.password != "pw" {
		t.Errorf("auth called with %q/%q", auth.username, auth.password)
	}

	auth.err = taiga.ErrInvalidCredentials
	if _, err := m.Authenticate(t.Context(), "alice", "bad"); !errors.Is(err, taiga.ErrInvalidCredentials) {
		t.Errorf("got %v", err)
	}
}

func TestSaveStoresTokensAsSeparateKeyringEntries(t *testing.T) {
	kr := newMemStore()
	dir := t.TempDir()
	m := newManager(t, &fakeAuth{}, kr, dir)
	res := loginResult(now.Add(time.Hour))

	saved, err := m.Save(res)

	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if saved.UsedFile {
		t.Error("tokens went to the file although the keyring works")
	}
	if kr.data[host+"/alice/access"] != res.Tokens.Access || kr.data[host+"/alice/refresh"] != res.Tokens.Refresh {
		t.Errorf("keyring holds %v", kr.data)
	}
	if _, err := os.Stat(filepath.Join(dir, "credentials")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("no credentials file expected, stat: %v", err)
	}
}

func TestSaveFallsBackToFileWhenKeyringWriteFails(t *testing.T) {
	kr := newMemStore()
	kr.setErr = errors.New("no secret service")
	dir := t.TempDir()
	m := newManager(t, &fakeAuth{}, kr, dir)
	res := loginResult(now.Add(time.Hour))

	saved, err := m.Save(res)

	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if !saved.UsedFile || saved.File != filepath.Join(dir, "credentials") {
		t.Errorf("got %+v, want the file fallback", saved)
	}
	file := session.NewFileStore(dir)
	if got, _ := file.Get(host + "/alice/refresh"); got != res.Tokens.Refresh {
		t.Errorf("refresh token in file: %q", got)
	}
	if kr.sets != 1 {
		t.Errorf("keyring must be marked unavailable after the first failure, got %d writes", kr.sets)
	}
}

func TestSaveFallsBackToFileWhenKeyringHangs(t *testing.T) {
	kr := session.WithTimeout(newHangingStore(t), 20*time.Millisecond)
	dir := t.TempDir()
	m := newManager(t, &fakeAuth{}, kr, dir)

	saved, err := m.Save(loginResult(now.Add(time.Hour)))

	if err != nil || !saved.UsedFile {
		t.Fatalf("got %+v, %v; want the file fallback", saved, err)
	}
}

func TestSaveRemovesStaleTokensFromFileWhenKeyringWorks(t *testing.T) {
	dir := t.TempDir()
	file := session.NewFileStore(dir)
	if err := file.Set(host+"/alice/access", "old"); err != nil {
		t.Fatal(err)
	}
	m := newManager(t, &fakeAuth{}, newMemStore(), dir)

	if _, err := m.Save(loginResult(now.Add(time.Hour))); err != nil {
		t.Fatal(err)
	}

	if _, err := file.Get(host + "/alice/access"); !errors.Is(err, session.ErrNotFound) {
		t.Errorf("stale file token should be gone, got %v", err)
	}
}

func TestResume(t *testing.T) {
	tests := []struct {
		name   string
		access string
		want   bool
	}{
		{name: "unexpired token", access: jwt(now.Add(time.Minute)), want: true},
		{name: "token expiring now", access: jwt(now)},
		{name: "expired token", access: jwt(now.Add(-time.Minute))},
		{name: "not a JWT", access: "garbage"},
		{name: "payload without exp", access: "h." + base64.RawURLEncoding.EncodeToString([]byte(`{"user_id":42}`)) + ".s"},
		{name: "no token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kr := newMemStore()
			if tt.access != "" {
				kr.data[host+"/alice/access"] = tt.access
			}
			m := newManager(t, &fakeAuth{}, kr, t.TempDir())

			if got := m.Resume("alice"); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResumeWithoutKnownUsername(t *testing.T) {
	m := newManager(t, &fakeAuth{}, newMemStore(), t.TempDir())

	if m.Resume("") {
		t.Error("resumed without a username")
	}
}

func TestResumeReadsFileWhenKeyringFails(t *testing.T) {
	for name, kr := range map[string]session.CredentialStore{
		"keyring error":   &memStore{data: map[string]string{}, getErr: errors.New("locked")},
		"keyring timeout": session.WithTimeout(newHangingStore(t), 20*time.Millisecond),
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := session.NewFileStore(dir).Set(host+"/alice/access", jwt(now.Add(time.Hour))); err != nil {
				t.Fatal(err)
			}
			m := newManager(t, &fakeAuth{}, kr, dir)

			if !m.Resume("alice") {
				t.Error("token from the credentials file was not used")
			}
		})
	}
}

func TestResumeReadsFileWhenKeyringHasNoEntry(t *testing.T) {
	dir := t.TempDir()
	if err := session.NewFileStore(dir).Set(host+"/alice/access", jwt(now.Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	m := newManager(t, &fakeAuth{}, newMemStore(), dir)

	if !m.Resume("alice") {
		t.Error("token from the credentials file was not used")
	}
}

func TestResumeWithCorruptCredentialsFileFailsQuietly(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "credentials"), []byte("\x00garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	kr := &memStore{data: map[string]string{}, getErr: errors.New("locked")}
	m := newManager(t, &fakeAuth{}, kr, dir)

	if m.Resume("alice") {
		t.Error("resumed from a corrupt file")
	}
}

func TestLogoutClearsKeyringAndFile(t *testing.T) {
	dir := t.TempDir()
	file := session.NewFileStore(dir)
	kr := newMemStore()
	for _, s := range []session.CredentialStore{kr, file} {
		for _, kind := range []string{"access", "refresh"} {
			if err := s.Set(host+"/alice/"+kind, "tok"); err != nil {
				t.Fatal(err)
			}
		}
	}
	kr.data[host+"/bob/access"] = "other"
	m := newManager(t, &fakeAuth{}, kr, dir)

	if err := m.Logout("alice"); err != nil {
		t.Fatalf("logout: %v", err)
	}

	if len(kr.data) != 1 || kr.data[host+"/bob/access"] != "other" {
		t.Errorf("keyring holds %v, want only bob's token", kr.data)
	}
	if _, err := os.Stat(filepath.Join(dir, "credentials")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("credentials file should be gone, stat: %v", err)
	}
}

func TestLogoutStillClearsFileWhenKeyringFails(t *testing.T) {
	dir := t.TempDir()
	if err := session.NewFileStore(dir).Set(host+"/alice/access", "tok"); err != nil {
		t.Fatal(err)
	}
	kr := &memStore{data: map[string]string{}, deleteErr: errors.New("locked")}
	m := newManager(t, &fakeAuth{}, kr, dir)

	err := m.Logout("alice")

	if err == nil {
		t.Error("a keyring failure must be reported")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "credentials")); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("credentials file should be gone, stat: %v", statErr)
	}
}

func newManager(t *testing.T, auth *fakeAuth, kr session.CredentialStore, dir string) *session.Manager {
	t.Helper()
	return session.NewManager(session.Options{
		Host:    host,
		Auth:    auth,
		Keyring: kr,
		File:    session.NewFileStore(dir),
		Now:     func() time.Time { return now },
	})
}

func loginResult(exp time.Time) taiga.LoginResult {
	return taiga.LoginResult{
		Member: alice,
		Tokens: taiga.Tokens{Access: jwt(exp), Refresh: jwt(exp.Add(7 * 24 * time.Hour))},
	}
}

// jwt builds an unsigned token whose payload carries exp, like Taiga's.
func jwt(exp time.Time) string {
	enc := base64.RawURLEncoding
	payload := fmt.Sprintf(`{"token_type":"access","exp":%d,"user_id":42}`, exp.Unix())
	return enc.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`)) + "." + enc.EncodeToString([]byte(payload)) + ".sig"
}

type fakeAuth struct {
	result             taiga.LoginResult
	err                error
	username, password string
}

func (f *fakeAuth) Login(_ context.Context, username, password string) (taiga.LoginResult, error) {
	f.username, f.password = username, password
	return f.result, f.err
}
