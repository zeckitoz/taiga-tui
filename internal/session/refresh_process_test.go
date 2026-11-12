package session_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zeckitoz/taiga-tui/internal/session"
	"github.com/zeckitoz/taiga-tui/internal/taiga"
)

// Environment of the helper process, which is this test binary run again.
const (
	envHelper = "TAIGA_TUI_TEST_REFRESH_HELPER"
	envURL    = "TAIGA_TUI_TEST_REFRESH_URL"
	envDir    = "TAIGA_TUI_TEST_REFRESH_DIR"
	envStale  = "TAIGA_TUI_TEST_REFRESH_STALE"

	// accessLine prefixes the line in which the helper reports its token.
	accessLine = "ACCESS="
)

// rotatingServer is an instance that rotates refresh tokens like Taiga: a
// refresh token works once, and the second use of it is rejected.
type rotatingServer struct {
	*httptest.Server
	mu      sync.Mutex
	current string
	calls   int
}

func newRotatingServer(t *testing.T, refresh string) *rotatingServer {
	t.Helper()
	rs := &rotatingServer{current: refresh}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Refresh string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		rs.mu.Lock()
		defer rs.mu.Unlock()
		if r.URL.Path != "/api/v1/auth/refresh" || body.Refresh != rs.current {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"detail": "Token is blocked"}`))
			return
		}
		rs.calls++
		// Long enough for the other process to arrive and wait for the lock.
		time.Sleep(150 * time.Millisecond)
		next := jwt(now.Add(8*24*time.Hour + time.Duration(rs.calls)*time.Second))
		rs.current = next
		_ = json.NewEncoder(w).Encode(map[string]string{"auth_token": jwt(now.Add(24 * time.Hour)), "refresh": next})
	}))
	t.Cleanup(rs.Close)
	return rs
}

// TestHelperRefresh is the body of the helper processes of
// TestConcurrentRefreshOfTwoProcessesCallsTheInstanceOnce, not a test.
func TestHelperRefresh(t *testing.T) {
	if os.Getenv(envHelper) == "" {
		t.Skip("only runs as a helper process")
	}
	m := session.NewManager(session.Options{
		Host: host,
		Auth: taiga.NewAuthAPI(os.Getenv(envURL)),
		// No keyring: the processes share their tokens through the file.
		Keyring: &memStore{data: map[string]string{}, getErr: errors.New("no keyring"), setErr: errors.New("no keyring")},
		File:    session.NewFileStore(os.Getenv(envDir)),
		Now:     func() time.Time { return now },
	})
	if !m.Resume("alice") {
		t.Fatal("no session to resume")
	}
	token, err := m.Refresh(t.Context(), os.Getenv(envStale))
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	t.Log(accessLine + token)
}

func TestConcurrentRefreshOfTwoProcessesCallsTheInstanceOnce(t *testing.T) {
	login := loginResult(now.Add(-time.Minute)) // the access token has expired
	dir := t.TempDir()
	file := session.NewFileStore(dir)
	for key, value := range map[string]string{host + "/alice/access": login.Tokens.Access, host + "/alice/refresh": login.Tokens.Refresh} {
		if err := file.Set(key, value); err != nil {
			t.Fatal(err)
		}
	}
	srv := newRotatingServer(t, login.Tokens.Refresh)

	type result struct {
		out []byte
		err error
	}
	results := make([]result, 2)
	var wg sync.WaitGroup
	for i := range results {
		cmd := exec.Command(os.Args[0], "-test.run=^TestHelperRefresh$", "-test.v")
		cmd.Env = append(os.Environ(), envHelper+"=1", envURL+"="+srv.URL, envDir+"="+dir, envStale+"="+login.Tokens.Access)
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i].out, results[i].err = cmd.CombinedOutput()
		}()
	}
	wg.Wait()

	var tokens []string
	for _, r := range results {
		if r.err != nil {
			t.Fatalf("helper failed: %v\n%s", r.err, r.out)
		}
		tokens = append(tokens, accessTokenIn(t, r.out))
	}
	if srv.calls != 1 {
		t.Errorf("%d calls of /auth/refresh, want exactly 1", srv.calls)
	}
	if tokens[0] != tokens[1] || tokens[0] == login.Tokens.Access {
		t.Errorf("processes ended with %q and %q, want the same renewed token", tokens[0], tokens[1])
	}
	if !session.NewManager(session.Options{Host: host, Auth: &fakeAuth{}, Keyring: newMemStore(), File: file, Now: func() time.Time { return now }}).Resume("alice") {
		t.Error("the stored session is not valid")
	}
	if stored, _ := file.Get(host + "/alice/access"); stored != tokens[0] {
		t.Errorf("stored access token %q, want %q", stored, tokens[0])
	}
}

func accessTokenIn(t *testing.T, out []byte) string {
	t.Helper()
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if _, token, ok := strings.Cut(sc.Text(), accessLine); ok {
			return strings.TrimSpace(token)
		}
	}
	t.Fatalf("helper reported no access token:\n%s", out)
	return ""
}
