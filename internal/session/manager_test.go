package session_test

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
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
	if saved.File != "" {
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
	if saved.File != filepath.Join(dir, "credentials") {
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

	if err != nil || saved.File == "" {
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

	refreshed    taiga.Tokens
	refreshErr   error
	refreshCalls []string // the refresh tokens sent
}

func (f *fakeAuth) Refresh(_ context.Context, refresh string) (taiga.Tokens, error) {
	f.refreshCalls = append(f.refreshCalls, refresh)
	return f.refreshed, f.refreshErr
}

func (f *fakeAuth) Login(_ context.Context, username, password string) (taiga.LoginResult, error) {
	f.username, f.password = username, password
	return f.result, f.err
}

func TestResumePrefersFileOverStaleKeyringAfterFallback(t *testing.T) {
	kr := newMemStore()
	kr.data[host+"/alice/access"] = jwt(now.Add(-time.Hour)) // from an older login
	dir := t.TempDir()
	m := newManager(t, &fakeAuth{}, kr, dir)
	kr.setErr = errors.New("keyring locked")
	if _, err := m.Save(loginResult(now.Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	kr.setErr = nil

	if !newManager(t, &fakeAuth{}, kr, dir).Resume("alice") {
		t.Error("the stale keyring token won over the fresh one in the file")
	}
}

func TestLogoutWithoutUsernameClearsAllFileTokensOfHost(t *testing.T) {
	dir := t.TempDir()
	file := session.NewFileStore(dir)
	for _, key := range []string{host + "/alice/access", host + "/bob/refresh", "other.example.org/alice/access"} {
		if err := file.Set(key, "tok"); err != nil {
			t.Fatal(err)
		}
	}
	m := newManager(t, &fakeAuth{}, newMemStore(), dir)

	if err := m.Logout(""); err != nil {
		t.Fatalf("logout: %v", err)
	}

	if _, err := file.Get("other.example.org/alice/access"); err != nil {
		t.Errorf("token of another instance was deleted: %v", err)
	}
	for _, key := range []string{host + "/alice/access", host + "/bob/refresh"} {
		if _, err := file.Get(key); !errors.Is(err, session.ErrNotFound) {
			t.Errorf("%s still stored: %v", key, err)
		}
	}
}

func TestLogoutGivesUpOnHangingKeyringAfterOneTimeout(t *testing.T) {
	hang := &countingHang{hangingStore: newHangingStore(t)}
	m := newManager(t, &fakeAuth{}, session.WithTimeout(hang, 20*time.Millisecond), t.TempDir())

	if err := m.Logout("alice"); !errors.Is(err, session.ErrTimeout) {
		t.Errorf("got %v, want ErrTimeout", err)
	}
	if hang.calls.Load() != 1 {
		t.Errorf("keyring called %d times, want 1", hang.calls.Load())
	}
}

type countingHang struct {
	*hangingStore
	calls atomic.Int32
}

func (c *countingHang) Delete(key string) error {
	c.calls.Add(1)
	return c.hangingStore.Delete(key)
}

func TestTokenHandsOutAccessTokenOfResumedSession(t *testing.T) {
	kr := newMemStore()
	access := jwt(now.Add(time.Hour))
	kr.data[host+"/alice/access"] = access
	m := newManager(t, &fakeAuth{}, kr, t.TempDir())

	if _, err := m.Token(t.Context()); !errors.Is(err, taiga.ErrSessionExpired) {
		t.Errorf("before resume: got %v, want ErrSessionExpired", err)
	}
	m.Resume("alice")
	got, err := m.Token(t.Context())

	if err != nil || got != access {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestTokenHandsOutAccessTokenOfSavedLogin(t *testing.T) {
	m := newManager(t, &fakeAuth{}, newMemStore(), t.TempDir())
	res := loginResult(now.Add(time.Hour))
	if _, err := m.Save(res); err != nil {
		t.Fatal(err)
	}

	got, err := m.Token(t.Context())

	if err != nil || got != res.Tokens.Access {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestTokenReportsExpiredSession(t *testing.T) {
	m := newManager(t, &fakeAuth{refreshErr: taiga.ErrSessionExpired}, newMemStore(), t.TempDir())
	if _, err := m.Save(loginResult(now)); err != nil {
		t.Fatal(err)
	}

	if _, err := m.Token(t.Context()); !errors.Is(err, taiga.ErrSessionExpired) {
		t.Errorf("got %v, want ErrSessionExpired", err)
	}
}

// rotated is what the instance answers to a refresh.
func rotated() taiga.Tokens {
	return taiga.Tokens{Access: jwt(now.Add(24 * time.Hour)), Refresh: jwt(now.Add(8 * 24 * time.Hour))}
}

func TestTokenRefreshesAnExpiredAccessToken(t *testing.T) {
	auth := &fakeAuth{refreshed: rotated()}
	kr := newMemStore()
	m := newManager(t, auth, kr, t.TempDir())
	login := loginResult(now.Add(-time.Minute))
	if _, err := m.Save(login); err != nil {
		t.Fatal(err)
	}

	got, err := m.Token(t.Context())

	if err != nil || got != auth.refreshed.Access {
		t.Fatalf("got %q, %v", got, err)
	}
	if len(auth.refreshCalls) != 1 || auth.refreshCalls[0] != login.Tokens.Refresh {
		t.Errorf("refreshed with %q, want the stored refresh token once", auth.refreshCalls)
	}
	if kr.data[host+"/alice/access"] != auth.refreshed.Access || kr.data[host+"/alice/refresh"] != auth.refreshed.Refresh {
		t.Errorf("the new tokens were not stored: %v", kr.data)
	}
}

func TestTokenDoesNotRefreshAnUnexpiredAccessToken(t *testing.T) {
	auth := &fakeAuth{refreshed: rotated()}
	m := newManager(t, auth, newMemStore(), t.TempDir())
	if _, err := m.Save(loginResult(now.Add(time.Minute))); err != nil {
		t.Fatal(err)
	}

	if _, err := m.Token(t.Context()); err != nil || len(auth.refreshCalls) != 0 {
		t.Errorf("got %v after %d refreshes", err, len(auth.refreshCalls))
	}
}

func TestResumeWithExpiredAccessTokenNeedsAnUnexpiredRefreshToken(t *testing.T) {
	for name, tt := range map[string]struct {
		refresh string
		want    bool
	}{
		"refresh token valid":   {refresh: jwt(now.Add(time.Hour)), want: true},
		"refresh token expired": {refresh: jwt(now.Add(-time.Hour))},
		"no refresh token":      {},
	} {
		t.Run(name, func(t *testing.T) {
			kr := newMemStore()
			kr.data[host+"/alice/access"] = jwt(now.Add(-time.Hour))
			if tt.refresh != "" {
				kr.data[host+"/alice/refresh"] = tt.refresh
			}
			m := newManager(t, &fakeAuth{}, kr, t.TempDir())

			if got := m.Resume("alice"); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRefreshSkipsTheCallWhenAnotherInstanceAlreadyRotatedTheTokens(t *testing.T) {
	auth := &fakeAuth{refreshed: rotated()}
	kr := newMemStore()
	m := newManager(t, auth, kr, t.TempDir())
	stale := loginResult(now.Add(-time.Minute))
	if _, err := m.Save(stale); err != nil {
		t.Fatal(err)
	}
	// The other instance stored fresh tokens after this one read its token.
	fresh := rotated()
	kr.data[host+"/alice/access"] = fresh.Access
	kr.data[host+"/alice/refresh"] = fresh.Refresh

	got, err := m.Refresh(t.Context(), stale.Tokens.Access)

	if err != nil || got != fresh.Access {
		t.Errorf("got %q, %v, want the stored token", got, err)
	}
	if len(auth.refreshCalls) != 0 {
		t.Errorf("refreshed although the tokens were rotated: %q", auth.refreshCalls)
	}
}

func TestRefreshRefreshesWhenTheStoredTokenIsTheRejectedOne(t *testing.T) {
	// The server rejected an access token that has not expired yet.
	auth := &fakeAuth{refreshed: rotated()}
	m := newManager(t, auth, newMemStore(), t.TempDir())
	login := loginResult(now.Add(time.Hour))
	if _, err := m.Save(login); err != nil {
		t.Fatal(err)
	}

	got, err := m.Refresh(t.Context(), login.Tokens.Access)

	if err != nil || got != auth.refreshed.Access {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestRefreshFailures(t *testing.T) {
	unreachable := fmt.Errorf("%w: down", taiga.ErrUnreachable)
	tests := []struct {
		name       string
		refreshErr error
		noRefresh  bool
		want       error
	}{
		{name: "rejected refresh token", refreshErr: taiga.ErrSessionExpired, want: taiga.ErrSessionExpired},
		{name: "no refresh token stored", noRefresh: true, want: taiga.ErrSessionExpired},
		{name: "instance unreachable", refreshErr: unreachable, want: taiga.ErrUnreachable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := &fakeAuth{refreshErr: tt.refreshErr}
			kr := newMemStore()
			m := newManager(t, auth, kr, t.TempDir())
			login := loginResult(now.Add(-time.Minute))
			if _, err := m.Save(login); err != nil {
				t.Fatal(err)
			}
			if tt.noRefresh {
				delete(kr.data, host+"/alice/refresh")
			}

			_, err := m.Refresh(t.Context(), login.Tokens.Access)

			if !errors.Is(err, tt.want) {
				t.Errorf("got %v, want %v", err, tt.want)
			}
			if kr.data[host+"/alice/access"] != login.Tokens.Access && !tt.noRefresh {
				t.Error("the stored tokens changed although the refresh failed")
			}
		})
	}
}

func TestRefreshWithoutActiveSessionReportsExpiredSession(t *testing.T) {
	m := newManager(t, &fakeAuth{}, newMemStore(), t.TempDir())

	if _, err := m.Refresh(t.Context(), "stale"); !errors.Is(err, taiga.ErrSessionExpired) {
		t.Errorf("got %v, want ErrSessionExpired", err)
	}
}

func TestRefreshStoresTheNewTokensInTheFileWhenTheKeyringFails(t *testing.T) {
	auth := &fakeAuth{refreshed: rotated()}
	dir := t.TempDir()
	kr := newMemStore()
	m := newManager(t, auth, kr, dir)
	login := loginResult(now.Add(-time.Minute))
	if _, err := m.Save(login); err != nil {
		t.Fatal(err)
	}
	kr.setErr = errors.New("keyring locked")

	if _, err := m.Refresh(t.Context(), login.Tokens.Access); err != nil {
		t.Fatal(err)
	}

	if got, _ := session.NewFileStore(dir).Get(host + "/alice/refresh"); got != auth.refreshed.Refresh {
		t.Errorf("refresh token in file: %q", got)
	}
}
