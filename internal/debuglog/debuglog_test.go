package debuglog_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zeckitoz/taiga-tui/internal/debuglog"
)

func TestTransportLogsRequestsAndResponsesWithoutSecrets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Add("Set-Cookie", "session=cookie-secret")
		_, _ = io.WriteString(w, `{"id": 42, "username": "alice", "auth_token": "access-secret", "refresh": "refresh-secret"}`)
	}))
	defer srv.Close()
	var log bytes.Buffer
	client := &http.Client{Transport: debuglog.NewTransport(http.DefaultTransport, &log)}

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/auth", strings.NewReader(
		`{"type": "normal", "username": "alice", "password": "pw-secret", "nested": {"refresh": "refresh-in"}}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer bearer-secret")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	got := log.String()
	for _, secret := range []string{"bearer-secret", "pw-secret", "access-secret", "refresh-secret", "refresh-in", "cookie-secret"} {
		if strings.Contains(got, secret) {
			t.Errorf("log contains %q:\n%s", secret, got)
		}
	}
	for _, want := range []string{"POST " + srv.URL + "/api/v1/auth", `"username":"alice"`, `"id":42`, "200 OK", "Authorization: [redacted]"} {
		if !strings.Contains(got, want) {
			t.Errorf("log lacks %q:\n%s", want, got)
		}
	}
	if !strings.Contains(string(body), "access-secret") {
		t.Errorf("caller got a changed response: %s", body)
	}
}

func TestTransportLogsFailedRequests(t *testing.T) {
	var log bytes.Buffer
	client := &http.Client{Transport: debuglog.NewTransport(failing{}, &log)}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://taiga.example.org/api/v1/users/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("want an error")
	}
	if got := log.String(); !strings.Contains(got, "GET https://taiga.example.org/api/v1/users/me") || !strings.Contains(got, "unexpected EOF") {
		t.Errorf("log:\n%s", got)
	}
}

type failing struct{}

func (failing) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, io.ErrUnexpectedEOF
}
