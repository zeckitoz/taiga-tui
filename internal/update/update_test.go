package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func serve(t *testing.T, status int, body string) *Checker {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("request is authenticated: %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return &Checker{URL: srv.URL, Client: srv.Client()}
}

func TestLatestReturnsTagOfLatestRelease(t *testing.T) {
	c := serve(t, http.StatusOK, `{"tag_name":"v0.4.0","prerelease":false,"draft":false}`)

	got, err := c.Latest(context.Background())

	if err != nil || got != "v0.4.0" {
		t.Errorf("Latest = %q, %v; want v0.4.0", got, err)
	}
}

func TestLatestFailsOnBadResponses(t *testing.T) {
	tests := map[string]*Checker{
		"server error": serve(t, http.StatusInternalServerError, `{}`),
		"rate limited": serve(t, http.StatusForbidden, `{"message":"rate limit"}`),
		"not json":     serve(t, http.StatusOK, `<html>`),
		"no tag":       serve(t, http.StatusOK, `{}`),
		"pre-release":  serve(t, http.StatusOK, `{"tag_name":"v1.0.0-rc.1","prerelease":true}`),
		"draft":        serve(t, http.StatusOK, `{"tag_name":"v1.0.0","draft":true}`),
		"not semver":   serve(t, http.StatusOK, `{"tag_name":"nightly"}`),
	}
	for name, c := range tests {
		if got, err := c.Latest(context.Background()); err == nil {
			t.Errorf("%s: Latest = %q, want an error", name, got)
		}
	}
}

func TestLatestTimesOutAfterFiveSeconds(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-block }))
	t.Cleanup(func() { close(block); srv.Close() })
	c := &Checker{URL: srv.URL, Client: srv.Client()}

	if got := c.timeout(); got != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", got)
	}
	c.Timeout = time.Millisecond
	if _, err := c.Latest(context.Background()); err == nil {
		t.Error("want an error when the server does not answer")
	}
}

func TestNewer(t *testing.T) {
	tests := []struct {
		current, found string
		want           bool
	}{
		{"v0.3.0", "v0.4.0", true},
		{"v0.3.0", "v0.3.1", true},
		{"v0.9.0", "v0.10.0", true},
		{"v0.3.0", "v1.0.0", true},
		{"v0.3.0", "v0.3.0", false},
		{"v0.4.0", "v0.3.0", false},
		{"v0.4.0-rc.1", "v0.4.0", true},
		{"v0.3.0", "v0.4.0-rc.1", false},
		{"dev", "v0.4.0", false},
		{"(devel)", "v0.4.0", false},
		{"", "v0.4.0", false},
		{"v0.0.0-20261111-abcdef", "v0.4.0", false},
		{"v0.3.0", "", false},
		{"v0.3.0", "garbage", false},
	}
	for _, tt := range tests {
		if got := Newer(tt.current, tt.found); got != tt.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", tt.current, tt.found, got, tt.want)
		}
	}
}

func TestDue(t *testing.T) {
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		last time.Time
		want bool
	}{
		{"never checked", time.Time{}, true},
		{"25 hours ago", now.Add(-25 * time.Hour), true},
		{"exactly a day ago", now.Add(-24 * time.Hour), true},
		{"23 hours ago", now.Add(-23 * time.Hour), false},
		{"clock went back", now.Add(time.Hour), true},
	}
	for _, tt := range tests {
		if got := Due(tt.last, now); got != tt.want {
			t.Errorf("%s: Due = %v, want %v", tt.name, got, tt.want)
		}
	}
}
