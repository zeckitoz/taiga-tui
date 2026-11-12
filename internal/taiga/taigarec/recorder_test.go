package taigarec_test

import (
	"context"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zeckitoz/taiga-tui/internal/domain"
	"github.com/zeckitoz/taiga-tui/internal/taiga"
	"github.com/zeckitoz/taiga-tui/internal/taiga/taigarec"
)

// liveServer stands in for the live instance and answers with personal data.
func liveServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth":
			_, _ = w.Write([]byte(`{"id": 1234, "username": "jdoe", "full_name_display": "Jane Doe",
				"email": "jane.doe@uni.example", "auth_token": "` + accessJWT + `", "refresh": "` + refreshJWT + `"}`))
		case "/api/v1/users/me":
			_, _ = w.Write([]byte(`{"id": 1234, "username": "jdoe", "full_name_display": "Jane Doe", "bio": "Hi", "subject": "Password hint: s3cret"}`))
		default:
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"detail": "Given token not valid"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

type staticToken string

func (s staticToken) Token(context.Context) (string, error) { return string(s), nil }

// Refresh fails like a rejected refresh token: a staticToken cannot be renewed.
func (s staticToken) Refresh(context.Context, string) (string, error) {
	return "", taiga.ErrSessionExpired
}

func readDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files[e.Name()] = string(data)
	}
	return files
}

func TestRecorderPassesResponsesThroughAndSavesThemAnonymised(t *testing.T) {
	srv := liveServer(t)
	rec := taigarec.NewRecorder(http.DefaultTransport)
	auth := taiga.NewAuthAPI(srv.URL, taiga.WithTransport(rec))
	client := taiga.NewClient(srv.URL, staticToken("t"), taiga.WithTransport(rec))
	dir := t.TempDir()

	var login taiga.LoginResult
	err := rec.Record("login.json", func() (err error) {
		login, err = auth.Login(t.Context(), "jane.doe@uni.example", "pw")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var me domain.TeamMember
	err = rec.Record("me.json", func() (err error) {
		me, err = client.Me(t.Context())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := rec.Save(dir, taigarec.Anonymiser{}); err != nil {
		t.Fatal(err)
	}

	if login.Tokens.Access != accessJWT || me.FullName != "Jane Doe" {
		t.Errorf("client got anonymised data: %+v, %+v", login, me)
	}
	files := readDir(t, dir)
	wantMe := `{
  "bio": "redacted",
  "full_name_display": "Member 1",
  "id": 1234,
  "subject": "Password hint: s3cret",
  "username": "member-1"
}
`
	if files["me.json"] != wantMe {
		t.Errorf("me.json got\n%s\nwant\n%s", files["me.json"], wantMe)
	}
	if !strings.Contains(files["login.json"], `"auth_token": "redacted-access-token-1"`) {
		t.Errorf("login.json not anonymised:\n%s", files["login.json"])
	}
	if names := slices.Sorted(maps.Keys(files)); !slices.Equal(names, []string{"login.json", "me.json"}) {
		t.Errorf("files %v", names)
	}
}

func TestRecorderSavesNothingWhenAForbiddenStringRemains(t *testing.T) {
	srv := liveServer(t)
	rec := taigarec.NewRecorder(http.DefaultTransport)
	client := taiga.NewClient(srv.URL, staticToken("t"), taiga.WithTransport(rec))
	dir := t.TempDir()
	err := rec.Record("me.json", func() error {
		_, err := client.Me(t.Context())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// The password used for recording must never end up in a fixture.
	err = rec.Save(dir, taigarec.Anonymiser{}, "s3cret")

	if err == nil || !strings.Contains(err.Error(), "me.json") {
		t.Errorf("got %v, want an error naming me.json", err)
	}
	if files := readDir(t, dir); len(files) != 0 {
		t.Errorf("wrote %v", files)
	}
}

func TestRecorderReportsFailedCalls(t *testing.T) {
	srv := liveServer(t)
	rec := taigarec.NewRecorder(http.DefaultTransport)
	client := taiga.NewClient(srv.URL, staticToken("t"), taiga.WithTransport(rec))

	err := rec.Record("projects.json", func() error {
		_, err := client.Projects(t.Context(), 1234)
		return err
	})

	if err == nil || !strings.Contains(err.Error(), "projects.json") {
		t.Errorf("got %v, want an error naming projects.json", err)
	}
}

func TestRecorderRequiresExactlyOneRequestPerFixture(t *testing.T) {
	rec := taigarec.NewRecorder(http.DefaultTransport)

	err := rec.Record("me.json", func() error { return nil })

	if err == nil || !strings.Contains(err.Error(), "me.json") {
		t.Errorf("got %v, want an error naming me.json", err)
	}
}

func TestRecordEachKeepsOneFixturePerRequestInOrder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"path": "` + r.URL.Path + `"}`))
	}))
	t.Cleanup(srv.Close)
	rec := taigarec.NewRecorder(http.DefaultTransport)
	dir := t.TempDir()

	err := rec.RecordEach([]string{"first.json", "second.json"}, func() error {
		c := &http.Client{Transport: rec}
		for _, p := range []string{"/one", "/two"} {
			resp, err := c.Get(srv.URL + p)
			if err != nil {
				return err
			}
			_ = resp.Body.Close()
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := rec.Save(dir, taigarec.Anonymiser{}); err != nil {
		t.Fatal(err)
	}

	files := readDir(t, dir)
	if !strings.Contains(files["first.json"], "/one") || !strings.Contains(files["second.json"], "/two") {
		t.Errorf("files %v", files)
	}
}

func TestRecordEachRequiresOneRequestPerName(t *testing.T) {
	srv := liveServer(t)
	rec := taigarec.NewRecorder(http.DefaultTransport)
	client := taiga.NewClient(srv.URL, staticToken("t"), taiga.WithTransport(rec))

	err := rec.RecordEach([]string{"me.json", "more.json"}, func() error {
		_, err := client.Me(t.Context())
		return err
	})

	if err == nil || !strings.Contains(err.Error(), "sent 1 requests, want 2") {
		t.Errorf("got %v", err)
	}
}
