package taiga_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestRefreshReportsUnreachableHost(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	_, err := taiga.NewAuthAPI(url).Refresh(t.Context(), "r")

	if !errors.Is(err, taiga.ErrUnreachable) {
		t.Errorf("got %v, want ErrUnreachable", err)
	}
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
