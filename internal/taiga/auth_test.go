package taiga_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/zeckitoz/taiga-tui/internal/domain"
	"github.com/zeckitoz/taiga-tui/internal/taiga"
)

func TestLoginSendsNormalLoginAndReturnsMemberAndTokens(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/auth" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content type %q", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		writeFixture(t, w, "login.json")
	}))
	defer srv.Close()

	res, err := taiga.NewAuthAPI(srv.URL).Login(t.Context(), "alice@example.org", "s3cret")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]string{"type": "normal", "username": "alice@example.org", "password": "s3cret"}
	if len(got) != len(want) || got["type"] != want["type"] || got["username"] != want["username"] || got["password"] != want["password"] {
		t.Errorf("request body %v, want %v", got, want)
	}
	wantRes := taiga.LoginResult{
		Member: domain.TeamMember{ID: 368, Username: "member-3", FullName: "Member 3"},
		Tokens: taiga.Tokens{Access: "redacted-access-token-1", Refresh: "redacted-refresh-token-1"},
	}
	if res != wantRes {
		t.Errorf("got  %+v\nwant %+v", res, wantRes)
	}
}

func TestLoginMapsFailuresToTypedErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		want    error
		wantMsg string
	}{
		{name: "wrong credentials", status: 401, body: `{"_error_message": "No active account found with the given credentials"}`, want: taiga.ErrInvalidCredentials},
		{name: "rejected input", status: 400, body: `{"username": ["This field may not be blank."]}`, want: taiga.ErrInvalidCredentials},
		{name: "server error", status: 502, body: `bad gateway`, wantMsg: "server error (502)"},
		{name: "broken JSON", status: 200, body: `{"id": `, want: taiga.ErrUnexpected},
		{name: "missing tokens", status: 200, body: `{"id": 42, "username": "alice"}`, want: taiga.ErrUnexpected},
		{name: "unexpected status", status: 404, body: `not found`, want: taiga.ErrUnexpected},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			_, err := taiga.NewAuthAPI(srv.URL).Login(t.Context(), "alice", "pw")

			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Errorf("got %v, want %v", err, tt.want)
			}
			if tt.wantMsg != "" {
				serr, ok := errors.AsType[*taiga.ServerError](err)
				if !ok || serr.Code != tt.status || serr.Error() != tt.wantMsg {
					t.Errorf("got %v, want ServerError %q", err, tt.wantMsg)
				}
			}
		})
	}
}

func TestLoginRejectionCarriesTaigasReason(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantMsg string
	}{
		{name: "error message", status: 401, body: `{"_error_message": "No active account found with the given credentials", "_error_type": "taiga.base.exceptions.WrongArguments"}`,
			wantMsg: "invalid username or password: No active account found with the given credentials"},
		{name: "detail", status: 400, body: `{"detail": "JSON parse error"}`,
			wantMsg: "invalid username or password: JSON parse error"},
		{name: "error message before detail", status: 400, body: `{"detail": "second", "_error_message": "first"}`,
			wantMsg: "invalid username or password: first"},
		{name: "first field error", status: 400, body: `{"username": ["This field may not be blank."], "password": ["This field may not be blank."]}`,
			wantMsg: "invalid username or password: username: This field may not be blank."},
		{name: "field error as string", status: 400, body: `{"type": "Invalid login type"}`,
			wantMsg: "invalid username or password: type: Invalid login type"},
		{name: "none", status: 400, body: `{"_error_type": "x", "count": 3}`, wantMsg: "invalid username or password"},
		{name: "empty object", status: 401, body: `{}`, wantMsg: "invalid username or password"},
		{name: "not JSON", status: 401, body: `<html>Unauthorized</html>`, wantMsg: "invalid username or password"},
		{name: "empty body", status: 401, body: ``, wantMsg: "invalid username or password"},
		{name: "multi-line reason", status: 400, body: `{"_error_message": "  first line\n\tsecond   line  "}`,
			wantMsg: "invalid username or password: first line second line"},
		{name: "password echoed", status: 400, body: `{"_error_message": "wrong password p4ss-w0rd!"}`,
			wantMsg: "invalid username or password: wrong password [redacted]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := rejectingServer(t, tt.status, tt.body)

			_, err := taiga.NewAuthAPI(srv.URL).Login(t.Context(), "alice", "p4ss-w0rd!")

			if !errors.Is(err, taiga.ErrInvalidCredentials) {
				t.Errorf("got %v, want ErrInvalidCredentials", err)
			}
			if err == nil || err.Error() != tt.wantMsg {
				t.Errorf("got %q, want %q", err, tt.wantMsg)
			}
			rerr, ok := errors.AsType[*taiga.RejectedError](err)
			if !ok || rerr.Code != tt.status {
				t.Errorf("got %v, want RejectedError with code %d", err, tt.status)
			}
		})
	}
}

func TestRejectionReasonIsOneLineOfAtMost200Characters(t *testing.T) {
	long := strings.Repeat("ä", 150) + "\n" + strings.Repeat("b", 150)
	srv := rejectingServer(t, 400, `{"_error_message": "`+strings.ReplaceAll(long, "\n", `\n`)+`"}`)

	_, err := taiga.NewAuthAPI(srv.URL).Login(t.Context(), "alice", "pw")

	rerr, ok := errors.AsType[*taiga.RejectedError](err)
	if !ok {
		t.Fatalf("got %v, want RejectedError", err)
	}
	want := strings.Repeat("ä", 150) + " " + strings.Repeat("b", 48) + "…"
	if rerr.Reason != want {
		t.Errorf("got reason %q (%d characters), want %q", rerr.Reason, utf8.RuneCountInString(rerr.Reason), want)
	}
}

func TestLoginReportsUnreachableHost(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	_, err := taiga.NewAuthAPI(url).Login(t.Context(), "alice", "pw")

	if !errors.Is(err, taiga.ErrUnreachable) {
		t.Errorf("got %v, want ErrUnreachable", err)
	}
}

func TestLoginTimesOut(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)

	_, err := taiga.NewAuthAPI(srv.URL, taiga.WithTimeout(50*time.Millisecond)).Login(t.Context(), "alice", "pw")

	if !errors.Is(err, taiga.ErrTimeout) {
		t.Errorf("got %v, want ErrTimeout", err)
	}
}

func TestRefreshSendsRefreshTokenAndReturnsRotatedTokens(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/auth/refresh" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content type %q", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		writeFixture(t, w, "refresh.json")
	}))
	defer srv.Close()

	tokens, err := taiga.NewAuthAPI(srv.URL).Refresh(t.Context(), "old-refresh")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got["refresh"] != "old-refresh" {
		t.Errorf("request body %v", got)
	}
	want := taiga.Tokens{Access: "redacted-access-token-2", Refresh: "redacted-refresh-token-2"}
	if tokens != want {
		t.Errorf("got %+v, want %+v", tokens, want)
	}
}

func TestRefreshMapsFailuresToTypedErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   error
		server int
	}{
		{name: "refresh token rejected", status: 401, body: `{"detail": "Token is invalid or expired", "code": "token_not_valid"}`, want: taiga.ErrSessionExpired},
		{name: "refresh token missing", status: 400, body: `{"refresh": ["This field may not be blank."]}`, want: taiga.ErrSessionExpired},
		{name: "server error", status: 503, body: `unavailable`, server: 503},
		{name: "broken JSON", status: 200, body: `{"auth_token": `, want: taiga.ErrUnexpected},
		{name: "missing tokens", status: 200, body: `{"auth_token": "a"}`, want: taiga.ErrUnexpected},
		{name: "unexpected status", status: 404, body: `not found`, want: taiga.ErrUnexpected},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			_, err := taiga.NewAuthAPI(srv.URL).Refresh(t.Context(), "r")

			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Errorf("got %v, want %v", err, tt.want)
			}
			if tt.server != 0 {
				serr, ok := errors.AsType[*taiga.ServerError](err)
				if !ok || serr.Code != tt.server {
					t.Errorf("got %v, want ServerError %d", err, tt.server)
				}
			}
		})
	}
}

func TestRefreshRejectionCarriesTaigasReason(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantMsg string
	}{
		{name: "detail", status: 401, body: `{"detail": "Token is invalid or expired", "code": "token_not_valid"}`,
			wantMsg: "session expired: Token is invalid or expired"},
		{name: "field error", status: 400, body: `{"refresh": ["This field may not be blank."]}`,
			wantMsg: "session expired: refresh: This field may not be blank."},
		{name: "not JSON", status: 401, body: `nope`, wantMsg: "session expired"},
		{name: "token echoed", status: 401, body: `{"detail": "Token old.refresh.jwt is blacklisted"}`,
			wantMsg: "session expired: Token [redacted] is blacklisted"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := rejectingServer(t, tt.status, tt.body)

			_, err := taiga.NewAuthAPI(srv.URL).Refresh(t.Context(), "old.refresh.jwt")

			if !errors.Is(err, taiga.ErrSessionExpired) {
				t.Errorf("got %v, want ErrSessionExpired", err)
			}
			if err == nil || err.Error() != tt.wantMsg {
				t.Errorf("got %q, want %q", err, tt.wantMsg)
			}
		})
	}
}

func TestRefreshReportsUnreachableHost(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	_, err := taiga.NewAuthAPI(url).Refresh(t.Context(), "r")

	if !errors.Is(err, taiga.ErrUnreachable) {
		t.Errorf("got %v, want ErrUnreachable", err)
	}
}

// rejectingServer answers every request with status and body.
func rejectingServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// writeFixture answers with the fixture file name from testdata.
func writeFixture(t *testing.T, w http.ResponseWriter, name string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Error(err)
	}
	_, _ = w.Write(data)
}
