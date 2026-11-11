package taiga_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/zeckitoz/taiga-tui/internal/domain"
	"github.com/zeckitoz/taiga-tui/internal/taiga"
)

type staticToken string

func (s staticToken) Token(context.Context) (string, error) { return string(s), nil }

// fixtureServer answers each path with the fixture file named in routes and
// records the requests it receives.
type fixtureServer struct {
	*httptest.Server
	mu   sync.Mutex
	reqs []*http.Request
}

func newFixtureServer(t *testing.T, routes map[string]string) *fixtureServer {
	t.Helper()
	fs := &fixtureServer{}
	fs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs.mu.Lock()
		fs.reqs = append(fs.reqs, r)
		fs.mu.Unlock()
		name, ok := routes[r.URL.Path]
		if !ok {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.NotFound(w, r)
			return
		}
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Error(err)
		}
		_, _ = w.Write(data)
	}))
	t.Cleanup(fs.Close)
	return fs
}

func (fs *fixtureServer) only(t *testing.T) *http.Request {
	t.Helper()
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if len(fs.reqs) != 1 {
		t.Fatalf("got %d requests, want 1", len(fs.reqs))
	}
	return fs.reqs[0]
}

func TestMeSendsBearerTokenAndMapsTeamMember(t *testing.T) {
	srv := newFixtureServer(t, map[string]string{"/api/v1/users/me": "me.json"})

	got, err := taiga.NewClient(srv.URL, staticToken("access.jwt")).Me(t.Context())

	if err != nil {
		t.Fatal(err)
	}
	want := domain.TeamMember{ID: 368, Username: "member-3", FullName: "Member 3"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	req := srv.only(t)
	if a := req.Header.Get("Authorization"); a != "Bearer access.jwt" {
		t.Errorf("Authorization %q", a)
	}
}

func TestProjectsListsProjectsOfMemberWithoutPagination(t *testing.T) {
	srv := newFixtureServer(t, map[string]string{"/api/v1/projects": "projects.json"})

	got, err := taiga.NewClient(srv.URL, staticToken("t")).Projects(t.Context(), 368)

	if err != nil {
		t.Fatal(err)
	}
	want := []domain.ProjectSummary{
		{ID: 396, Name: "Project 3", Slug: "project-3", BacklogEnabled: true, KanbanEnabled: true},
		{ID: 326, Name: "Project 2", Slug: "project-2", BacklogEnabled: true, KanbanEnabled: true},
		{ID: 463, Name: "Project 4", Slug: "project-4", BacklogEnabled: true, KanbanEnabled: true},
		{ID: 478, Name: "Project 5", Slug: "project-5", BacklogEnabled: true, KanbanEnabled: true},
		{ID: 496, Name: "Test Project", Slug: "test-project", BacklogEnabled: true, KanbanEnabled: true},
		{ID: 258, Name: "Project 1", Slug: "project-1", BacklogEnabled: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
	req := srv.only(t)
	if m := req.URL.Query().Get("member"); m != "368" {
		t.Errorf("member=%q", m)
	}
	if p := req.Header.Get("x-disable-pagination"); p != "1" {
		t.Errorf("x-disable-pagination %q", p)
	}
}

func TestProjectLoadsBySlugWithStatusesAndMembers(t *testing.T) {
	srv := newFixtureServer(t, map[string]string{"/api/v1/projects/by_slug": "project_by_slug.json"})

	got, err := taiga.NewClient(srv.URL, staticToken("t")).Project(t.Context(), "test-project")

	if err != nil {
		t.Fatal(err)
	}
	want := domain.Project{
		ProjectSummary: domain.ProjectSummary{ID: 496, Name: "Test Project", Slug: "test-project", BacklogEnabled: true, KanbanEnabled: true},
		UserStoryStatuses: []domain.Status{
			{ID: 2949, Name: "New", Color: "#70728F"},
			{ID: 2950, Name: "Ready", Color: "#E44057"},
			{ID: 2951, Name: "In progress", Color: "#E47C40", WIPLimit: 3},
			{ID: 2952, Name: "Ready for test", Color: "#E4CE40"},
			{ID: 2953, Name: "Done", Color: "#A8E440", Closed: true},
			{ID: 2954, Name: "Archived", Color: "#A9AABC", Closed: true, Archived: true},
		},
		Members: []domain.TeamMember{
			{ID: 368, Username: "member-3", FullName: "Member 3"},
			{ID: 386, Username: "member-4", FullName: "Member 4"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
	if s := srv.only(t).URL.Query().Get("slug"); s != "test-project" {
		t.Errorf("slug=%q", s)
	}
}

func TestKanbanLoadsUserStoriesOfActiveStatuses(t *testing.T) {
	srv := newFixtureServer(t, map[string]string{"/api/v1/userstories": "userstories_kanban.json"})

	got, err := taiga.NewClient(srv.URL, staticToken("t")).Kanban(t.Context(), 496)

	if err != nil {
		t.Fatal(err)
	}
	want := []domain.UserStory{
		{
			ID: 10213, Ref: 1, Title: "A", Status: 2951, SprintName: "Sprint 1",
			Assignees: []int64{386, 368}, KanbanOrder: 1794413075166346, Version: 7,
			Tasks: domain.TaskProgress{Done: 1, Total: 2},
		},
		{ID: 10214, Ref: 2, Title: "B", Status: 2949, SprintName: "Sprint 1", KanbanOrder: 1794413084345230, Version: 1},
		{
			ID: 10215, Ref: 3, Title: "C", Status: 2952, SprintName: "Sprint 1",
			Assignees: []int64{386}, KanbanOrder: 1794413108582883, Version: 3,
			Tasks: domain.TaskProgress{Total: 1},
		},
		{ID: 10216, Ref: 4, Title: "D", Status: 2953, SprintName: "Sprint 1", Assignees: []int64{368}, KanbanOrder: 1794413124555477, Version: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
	req := srv.only(t)
	q := req.URL.Query()
	if q.Get("project") != "496" || q.Get("status__is_archived") != "false" || q.Get("include_tasks") != "1" {
		t.Errorf("query %v", q)
	}
	if p := req.Header.Get("x-disable-pagination"); p != "1" {
		t.Errorf("x-disable-pagination %q", p)
	}
}

func TestUserStoryLoadsRecordedUserStory(t *testing.T) {
	srv := newFixtureServer(t, map[string]string{"/api/v1/userstories/10213": "userstory.json"})

	got, err := taiga.NewClient(srv.URL, staticToken("t")).UserStory(t.Context(), 10213)

	if err != nil {
		t.Fatal(err)
	}
	// Unlike the list, a single User Story comes with "tasks": [], so its
	// task progress is unknown here. The recording redacts descriptions.
	want := domain.UserStory{
		ID: 10213, Ref: 1, Title: "A", Status: 2951, SprintName: "Sprint 1",
		Assignees: []int64{386, 368}, KanbanOrder: 1794413075166346, Version: 7,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

// The test project cannot provide these cases, so they are answered inline.

func TestProjectFallsBackToUsernameForMemberWithoutFullName(t *testing.T) {
	srv := newInlineServer(t, `{"id": 7, "name": "Web Shop", "slug": "web-shop", "us_statuses": null,
		"members": [{"id": 43, "username": "bkoch", "full_name_display": ""}]}`)

	got, err := taiga.NewClient(srv.URL, staticToken("t")).Project(t.Context(), "web-shop")

	if err != nil {
		t.Fatal(err)
	}
	want := []domain.TeamMember{{ID: 43, Username: "bkoch", FullName: "bkoch"}}
	if !reflect.DeepEqual(got.Members, want) {
		t.Errorf("got  %+v\nwant %+v", got.Members, want)
	}
}

func TestKanbanCountsNoTasksWhenTaigaSendsNull(t *testing.T) {
	srv := newInlineServer(t, `[{"id": 502, "ref": 15, "subject": "Product search", "status": 101,
		"assigned_to": null, "assigned_users": [], "kanban_order": 1, "version": 1, "tasks": null}]`)

	got, err := taiga.NewClient(srv.URL, staticToken("t")).Kanban(t.Context(), 7)

	if err != nil {
		t.Fatal(err)
	}
	want := []domain.UserStory{{ID: 502, Ref: 15, Title: "Product search", Status: 101, KanbanOrder: 1, Version: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestUserStoryLoadsOneUserStoryWithDescription(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"id": 501, "ref": 12, "subject": "Checkout with credit card", "status": 101,
			"milestone": null, "milestone_name": null, "assigned_to": 43, "assigned_users": [42, 43],
			"kanban_order": 20, "version": 4, "description": "Pay with *Visa*.\n\nNo PayPal yet."}`))
	}))
	t.Cleanup(srv.Close)

	got, err := taiga.NewClient(srv.URL, staticToken("t")).UserStory(t.Context(), 501)

	if err != nil {
		t.Fatal(err)
	}
	want := domain.UserStory{
		ID: 501, Ref: 12, Title: "Checkout with credit card", Status: 101,
		Assignees: []int64{43, 42}, KanbanOrder: 20, Version: 4,
		Description: "Pay with *Visa*.\n\nNo PayPal yet.",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
	if path != "/api/v1/userstories/501" {
		t.Errorf("path %q", path)
	}
}

// newInlineServer answers every request with body.
func newInlineServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestClientMapsFailuresToTypedErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   error
		server int // expected ServerError code
	}{
		{name: "conflict", status: 400, body: `{"version": "The version doesn't match with the current one"}`, want: taiga.ErrConflict},
		{name: "other 400", status: 400, body: `{"subject": ["required"]}`, want: taiga.ErrUnexpected},
		{name: "not found", status: 404, body: `{"_error_message": "No UserStory matches the given query."}`, want: taiga.ErrNotFound},
		{name: "session expired", status: 401, body: `{"detail": "Given token not valid"}`, want: taiga.ErrSessionExpired},
		{name: "forbidden", status: 403, body: `{}`, want: taiga.ErrUnexpected},
		{name: "server error", status: 502, body: `bad gateway`, server: 502},
		{name: "broken JSON", status: 200, body: `{"id": `, want: taiga.ErrUnexpected},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			_, err := taiga.NewClient(srv.URL, staticToken("t")).Me(t.Context())

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

func TestClientReportsUnreachableHost(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	_, err := taiga.NewClient(url, staticToken("t")).Me(t.Context())

	if !errors.Is(err, taiga.ErrUnreachable) {
		t.Errorf("got %v, want ErrUnreachable", err)
	}
}

func TestClientTimesOut(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)

	_, err := taiga.NewClient(srv.URL, staticToken("t"), taiga.WithTimeout(50*time.Millisecond)).Kanban(t.Context(), 7)

	if !errors.Is(err, taiga.ErrTimeout) {
		t.Errorf("got %v, want ErrTimeout", err)
	}
}

func TestClientPassesTokenSourceFailureOn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("request sent without token: %s", r.URL)
	}))
	defer srv.Close()

	_, err := taiga.NewClient(srv.URL, failingToken{}).Me(t.Context())

	if !errors.Is(err, taiga.ErrSessionExpired) {
		t.Errorf("got %v, want ErrSessionExpired", err)
	}
}

type failingToken struct{}

func (failingToken) Token(context.Context) (string, error) { return "", taiga.ErrSessionExpired }
