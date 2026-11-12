package taiga_test

import (
	"context"
	"errors"
	"io"
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

// Refresh fails like a rejected refresh token: a staticToken cannot be renewed.
func (s staticToken) Refresh(context.Context, string) (string, error) {
	return "", taiga.ErrSessionExpired
}

// fixtureServer answers each path with the fixture file named in routes and
// records the requests it receives.
func newFixtureServer(t *testing.T, routes map[string]string) *testServer {
	t.Helper()
	bodies := map[string]string{}
	for path, name := range routes {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		bodies[path] = string(data)
	}
	return newTestServer(t, bodies)
}

// testServer answers each path with its body and records the requests it
// receives.
type testServer struct {
	*httptest.Server
	mu   sync.Mutex
	reqs []*http.Request
}

func newTestServer(t *testing.T, bodies map[string]string) *testServer {
	t.Helper()
	ts := &testServer{}
	ts.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.mu.Lock()
		ts.reqs = append(ts.reqs, r)
		ts.mu.Unlock()
		body, ok := bodies[r.URL.Path]
		if !ok {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)
	return ts
}

func (ts *testServer) requests() []*http.Request {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.reqs
}

func (ts *testServer) only(t *testing.T) *http.Request {
	t.Helper()
	reqs := ts.requests()
	if len(reqs) != 1 {
		t.Fatalf("got %d requests, want 1", len(reqs))
	}
	return reqs[0]
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
		TaskStatuses: []domain.Status{
			{ID: 2449, Name: "New", Color: "#70728F"},
			{ID: 2450, Name: "In progress", Color: "#E47C40"},
			{ID: 2451, Name: "Ready for test", Color: "#E4CE40"},
			{ID: 2452, Name: "Closed", Color: "#A8E440", Closed: true},
			{ID: 2453, Name: "Needs Info", Color: "#5178D3"},
		},
		Members: []domain.TeamMember{
			{ID: 368, Username: "member-3", FullName: "Member 3"},
			{ID: 386, Username: "member-4", FullName: "Member 4"},
		},
		Permissions: domain.Permissions{ModifyTask: true, ModifyUserStory: true},
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
			ID: 10213, Ref: 1, Title: "A", Status: 2951, Sprint: 1947, SprintName: "Sprint 1", SprintOrder: 1,
			Assignees: []int64{386, 368}, KanbanOrder: 1794413075166346, Version: 8,
			Tasks: domain.TaskProgress{Done: 1, Total: 2},
		},
		{ID: 10214, Ref: 2, Title: "B", Status: 2949, Sprint: 1947, SprintName: "Sprint 1", SprintOrder: 2, KanbanOrder: 1794413084345230, Version: 1},
		{
			ID: 10215, Ref: 3, Title: "C", Status: 2952, Sprint: 1947, SprintName: "Sprint 1", SprintOrder: 3,
			Assignees: []int64{386}, KanbanOrder: 1794413108582883, Version: 3,
			Tasks: domain.TaskProgress{Total: 1},
		},
		{ID: 10216, Ref: 4, Title: "D", Status: 2953, Sprint: 1947, SprintName: "Sprint 1", SprintOrder: 4, Assignees: []int64{368}, KanbanOrder: 1794413124555477, Version: 1},
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
		ID: 10213, Ref: 1, Title: "A", Status: 2951, Sprint: 1947, SprintName: "Sprint 1", SprintOrder: 1,
		Assignees: []int64{386, 368}, KanbanOrder: 1794413075166346, Version: 8,
		Description: "redacted",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestSprintsLoadsRecordedSprints(t *testing.T) {
	srv := newFixtureServer(t, map[string]string{"/api/v1/milestones": "milestones.json"})

	got, err := taiga.NewClient(srv.URL, staticToken("t")).Sprints(t.Context(), 496)

	if err != nil {
		t.Fatal(err)
	}
	want := []domain.Sprint{{
		ID: 1947, Name: "Sprint 1",
		Start: domain.Date{Year: 2026, Month: 11, Day: 11}, Finish: domain.Date{Year: 2026, Month: 11, Day: 25},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
	if p := srv.only(t).URL.Query().Get("project"); p != "496" {
		t.Errorf("project=%q", p)
	}
}

func TestTaskboardLoadsRecordedSprint(t *testing.T) {
	srv := newFixtureServer(t, map[string]string{
		"/api/v1/userstories": "userstories_sprint.json",
		"/api/v1/tasks":       "tasks_sprint.json",
	})

	got, err := taiga.NewClient(srv.URL, staticToken("t")).Taskboard(t.Context(), 496, 1947)

	if err != nil {
		t.Fatal(err)
	}
	// Unlike the Kanban Board, the Taskboard keeps E in "Archived". The list
	// of a Sprint leaves out the Tasks of each User Story.
	story := func(id int64, ref int, title string, status int64, order, kanban int64, version int, assignees ...int64) domain.UserStory {
		return domain.UserStory{
			ID: id, Ref: ref, Title: title, Status: status, Sprint: 1947, SprintName: "Sprint 1",
			SprintOrder: order, KanbanOrder: kanban, Version: version, Assignees: assignees,
		}
	}
	want := domain.SprintContent{
		UserStories: []domain.UserStory{
			story(10213, 1, "A", 2951, 1, 1794413075166346, 8, 386, 368),
			story(10214, 2, "B", 2949, 2, 1794413084345230, 1),
			story(10215, 3, "C", 2952, 3, 1794413108582883, 3, 386),
			story(10216, 4, "D", 2953, 4, 1794413124555477, 1, 368),
			story(10217, 5, "E", 2954, 5, 1794413135196546, 1, 368),
		},
		Tasks: []domain.Task{
			{ID: 9891, Ref: 6, Title: "1", Status: 2452, UserStory: 10213, Sprint: 1947, Assignee: 368, TaskboardOrder: 1794413287265, Version: 1},
			{ID: 9892, Ref: 7, Title: "2", Status: 2449, UserStory: 10213, Sprint: 1947, TaskboardOrder: 1794413310301, Version: 1},
			{ID: 9893, Ref: 8, Title: "3", Status: 2451, UserStory: 10215, Sprint: 1947, Assignee: 386, Version: 2},
			{ID: 9894, Ref: 9, Title: "4", Status: 2453, UserStory: 10217, Sprint: 1947, Assignee: 368, Version: 3},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
	for _, req := range srv.requests() {
		if q := req.URL.Query(); q.Get("project") != "496" || q.Get("milestone") != "1947" {
			t.Errorf("%s: query %v", req.URL.Path, q)
		}
		if p := req.Header.Get("x-disable-pagination"); p != "1" {
			t.Errorf("%s: x-disable-pagination %q", req.URL.Path, p)
		}
	}
}

func TestTaskLoadsRecordedTask(t *testing.T) {
	srv := newFixtureServer(t, map[string]string{"/api/v1/tasks/9891": "task.json"})

	got, err := taiga.NewClient(srv.URL, staticToken("t")).Task(t.Context(), 9891)

	if err != nil {
		t.Fatal(err)
	}
	// The recorded Task has no description.
	want := domain.Task{
		ID: 9891, Ref: 6, Title: "1", Status: 2452, UserStory: 10213, Sprint: 1947,
		Assignee: 368, TaskboardOrder: 1794413287265, Version: 1,
	}
	if got != want {
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

func TestSprintsMapsClosedAndDateRanges(t *testing.T) {
	srv := newInlineServer(t, `[
		{"id": 12, "name": "Sprint 5", "estimated_start": "2026-11-17", "estimated_finish": "2026-11-30", "closed": false, "user_stories": []},
		{"id": 11, "name": "Sprint 4", "estimated_start": "2026-11-03", "estimated_finish": "2026-11-16", "closed": true, "user_stories": []}]`)

	got, err := taiga.NewClient(srv.URL, staticToken("t")).Sprints(t.Context(), 7)

	if err != nil {
		t.Fatal(err)
	}
	want := []domain.Sprint{
		{ID: 12, Name: "Sprint 5", Start: domain.Date{Year: 2026, Month: 11, Day: 17}, Finish: domain.Date{Year: 2026, Month: 11, Day: 30}},
		{ID: 11, Name: "Sprint 4", Start: domain.Date{Year: 2026, Month: 11, Day: 3}, Finish: domain.Date{Year: 2026, Month: 11, Day: 16}, Closed: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestSprintsRejectsMalformedDates(t *testing.T) {
	srv := newInlineServer(t, `[{"id": 12, "name": "Sprint 5", "estimated_start": "17.11.2026", "estimated_finish": "2026-11-30"}]`)

	_, err := taiga.NewClient(srv.URL, staticToken("t")).Sprints(t.Context(), 7)

	if !errors.Is(err, taiga.ErrUnexpected) {
		t.Errorf("got %v, want ErrUnexpected", err)
	}
}

func TestTaskboardMapsTaskWithoutUserStoryOrAssignee(t *testing.T) {
	srv := newTestServer(t, map[string]string{
		"/api/v1/userstories": `[{"id": 501, "ref": 12, "subject": "Checkout", "status": 101, "milestone": 11,
			"milestone_name": "Sprint 4", "sprint_order": 2, "assigned_to": 43, "assigned_users": [42, 43],
			"kanban_order": 20, "version": 4}]`,
		"/api/v1/tasks": `[
			{"id": 901, "ref": 31, "subject": "Validate card number", "status": 201, "user_story": 501,
			 "milestone": 11, "assigned_to": 42, "taskboard_order": 5, "version": 2},
			{"id": 902, "ref": 40, "subject": "Update docs", "status": 202, "user_story": null,
			 "milestone": 11, "assigned_to": null, "taskboard_order": 1, "version": 1}]`,
	})

	got, err := taiga.NewClient(srv.URL, staticToken("t")).Taskboard(t.Context(), 7, 11)

	if err != nil {
		t.Fatal(err)
	}
	want := domain.SprintContent{
		UserStories: []domain.UserStory{{
			ID: 501, Ref: 12, Title: "Checkout", Status: 101, Sprint: 11, SprintName: "Sprint 4", SprintOrder: 2,
			Assignees: []int64{43, 42}, KanbanOrder: 20, Version: 4,
		}},
		Tasks: []domain.Task{
			{ID: 901, Ref: 31, Title: "Validate card number", Status: 201, UserStory: 501, Sprint: 11, Assignee: 42, TaskboardOrder: 5, Version: 2},
			{ID: 902, Ref: 40, Title: "Update docs", Status: 202, Sprint: 11, TaskboardOrder: 1, Version: 1},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
	for _, req := range srv.requests() {
		if q := req.URL.Query(); q.Get("project") != "7" || q.Get("milestone") != "11" {
			t.Errorf("%s: query %v", req.URL.Path, q)
		}
		if p := req.Header.Get("x-disable-pagination"); p != "1" {
			t.Errorf("%s: x-disable-pagination %q", req.URL.Path, p)
		}
	}
}

func TestTaskboardFailsIfTasksFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/tasks" {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(srv.Close)

	_, err := taiga.NewClient(srv.URL, staticToken("t")).Taskboard(t.Context(), 7, 11)

	if serr, ok := errors.AsType[*taiga.ServerError](err); !ok || serr.Code != 502 {
		t.Errorf("got %v, want ServerError 502", err)
	}
}

func TestTaskCarriesItsDescription(t *testing.T) {
	srv := newTestServer(t, map[string]string{
		"/api/v1/tasks/901": `{"id": 901, "ref": 31, "subject": "Validate card number", "status": 201,
			"user_story": 501, "milestone": 11, "assigned_to": 42, "taskboard_order": 5, "version": 2,
			"description": "Luhn check."}`,
	})

	got, err := taiga.NewClient(srv.URL, staticToken("t")).Task(t.Context(), 901)

	if err != nil {
		t.Fatal(err)
	}
	want := domain.Task{
		ID: 901, Ref: 31, Title: "Validate card number", Status: 201, UserStory: 501, Sprint: 11,
		Assignee: 42, TaskboardOrder: 5, Version: 2, Description: "Luhn check.",
	}
	if got != want {
		t.Errorf("got  %+v\nwant %+v", got, want)
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

func (failingToken) Refresh(context.Context, string) (string, error) {
	return "", taiga.ErrSessionExpired
}

// rotatingToken hands out "old", then "new" after a refresh, and records the
// stale tokens it is asked to refresh.
type rotatingToken struct {
	mu         sync.Mutex
	current    string
	stale      []string
	refreshErr error
}

func (r *rotatingToken) Token(context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.current, nil
}

func (r *rotatingToken) Refresh(_ context.Context, stale string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stale = append(r.stale, stale)
	if r.refreshErr != nil {
		return "", r.refreshErr
	}
	r.current = "new"
	return r.current, nil
}

// bearerServer accepts only the bearer token ok and answers 401 to any other,
// and records the method, token and body of every request.
type bearerServer struct {
	*httptest.Server
	mu   sync.Mutex
	seen []string
}

func newBearerServer(t *testing.T, ok string) *bearerServer {
	t.Helper()
	bs := &bearerServer{}
	bs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bs.mu.Lock()
		bs.seen = append(bs.seen, r.Method+" "+r.Header.Get("Authorization")+" "+string(body))
		bs.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer "+ok {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"detail": "Given token not valid"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id": 42, "username": "alice", "version": 1}`))
	}))
	t.Cleanup(bs.Close)
	return bs
}

func TestClientRefreshesOnceOn401AndRepeatsTheRequest(t *testing.T) {
	srv := newBearerServer(t, "new")
	tokens := &rotatingToken{current: "old"}

	got, err := taiga.NewClient(srv.URL, tokens).Me(t.Context())

	if err != nil || got.ID != 42 {
		t.Fatalf("got %+v, %v", got, err)
	}
	if !reflect.DeepEqual(tokens.stale, []string{"old"}) {
		t.Errorf("refreshed with %v, want the stale token once", tokens.stale)
	}
	want := []string{"GET Bearer old ", "GET Bearer new "}
	if !reflect.DeepEqual(srv.seen, want) {
		t.Errorf("requests %q, want %q", srv.seen, want)
	}
}

func TestClientRepeatsTheBodyOfAPatchAfterRefresh(t *testing.T) {
	srv := newBearerServer(t, "new")
	tokens := &rotatingToken{current: "old"}

	_, err := taiga.NewClient(srv.URL, tokens).PatchTask(t.Context(), 9891, 3, domain.TaskPatch{Status: 2452})

	if err != nil {
		t.Fatal(err)
	}
	const body = `{"status":2452,"version":3}`
	want := []string{"PATCH Bearer old " + body, "PATCH Bearer new " + body}
	if !reflect.DeepEqual(srv.seen, want) {
		t.Errorf("requests %q, want %q", srv.seen, want)
	}
}

func TestClientReportsExpiredSessionWhenRefreshIsRejected(t *testing.T) {
	srv := newBearerServer(t, "new")
	tokens := &rotatingToken{current: "old", refreshErr: taiga.ErrSessionExpired}

	_, err := taiga.NewClient(srv.URL, tokens).Me(t.Context())

	if !errors.Is(err, taiga.ErrSessionExpired) {
		t.Errorf("got %v, want ErrSessionExpired", err)
	}
	if len(srv.seen) != 1 {
		t.Errorf("request repeated after a failed refresh: %q", srv.seen)
	}
}

func TestClientKeepsRefreshFailureThatIsNotAnExpiredSession(t *testing.T) {
	srv := newBearerServer(t, "new")
	tokens := &rotatingToken{current: "old", refreshErr: taiga.ErrUnreachable}

	_, err := taiga.NewClient(srv.URL, tokens).Me(t.Context())

	if !errors.Is(err, taiga.ErrUnreachable) {
		t.Errorf("got %v, want ErrUnreachable", err)
	}
}

func TestClientRefreshesAndRepeatsOnlyOnce(t *testing.T) {
	srv := newBearerServer(t, "never")
	tokens := &rotatingToken{current: "old"}

	_, err := taiga.NewClient(srv.URL, tokens).Me(t.Context())

	if !errors.Is(err, taiga.ErrSessionExpired) {
		t.Errorf("got %v, want ErrSessionExpired", err)
	}
	if len(tokens.stale) != 1 || len(srv.seen) != 2 {
		t.Errorf("%d refreshes and %d requests, want 1 and 2", len(tokens.stale), len(srv.seen))
	}
}

func TestProjectWithoutModifyPermissions(t *testing.T) {
	srv := newInlineServer(t, `{"id": 1, "my_permissions": ["view_us", "view_tasks"]}`)

	got, err := taiga.NewClient(srv.URL, staticToken("t")).Project(t.Context(), "p")

	if err != nil {
		t.Fatal(err)
	}
	if got.Permissions != (domain.Permissions{}) {
		t.Errorf("permissions %+v", got.Permissions)
	}
}

// patchServer answers a PATCH of path with the fixture file and records the
// method, content type and body of the request.
type patchServer struct {
	*httptest.Server
	method, contentType, body string
}

func newPatchServer(t *testing.T, path, fixture string) *patchServer {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatal(err)
	}
	ps := &patchServer{}
	ps.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		body, _ := io.ReadAll(r.Body)
		ps.method, ps.contentType, ps.body = r.Method, r.Header.Get("Content-Type"), string(body)
		_, _ = w.Write(data)
	}))
	t.Cleanup(ps.Close)
	return ps
}

func TestPatchTaskSendsOnlyStatusAndVersion(t *testing.T) {
	srv := newPatchServer(t, "/api/v1/tasks/9891", "task.json")

	got, err := taiga.NewClient(srv.URL, staticToken("t")).PatchTask(t.Context(), 9891, 0, domain.TaskPatch{Status: 2452})

	if err != nil {
		t.Fatal(err)
	}
	if srv.method != http.MethodPatch || srv.contentType != "application/json" {
		t.Errorf("%s with %q", srv.method, srv.contentType)
	}
	if srv.body != `{"status":2452,"version":0}` {
		t.Errorf("body %s", srv.body)
	}
	if got.ID != 9891 || got.Status != 2452 || got.Version != 1 {
		t.Errorf("got %+v", got)
	}
}

func TestPatchUserStorySendsOnlyStatusAndVersion(t *testing.T) {
	srv := newPatchServer(t, "/api/v1/userstories/10213", "userstory.json")

	got, err := taiga.NewClient(srv.URL, staticToken("t")).PatchUserStory(t.Context(), 10213, 7, domain.UserStoryPatch{Status: 2951})

	if err != nil {
		t.Fatal(err)
	}
	if srv.method != http.MethodPatch || srv.contentType != "application/json" {
		t.Errorf("%s with %q", srv.method, srv.contentType)
	}
	if srv.body != `{"status":2951,"version":7}` {
		t.Errorf("body %s", srv.body)
	}
	if got.ID != 10213 || got.Status != 2951 || got.Version != 8 {
		t.Errorf("got %+v", got)
	}
}

func TestPatchTaskAssignToMeSendsOnlyAssigneeAndVersion(t *testing.T) {
	srv := newPatchServer(t, "/api/v1/tasks/9891", "task.json")

	_, err := taiga.NewClient(srv.URL, staticToken("t")).PatchTask(t.Context(), 9891, 3, domain.TaskPatch{Assignee: 42})

	if err != nil || srv.body != `{"assigned_to":42,"version":3}` {
		t.Errorf("body %s, %v", srv.body, err)
	}
}

func TestPatchUserStoryAssignToMeSendsMainAssigneeAndAllAssignees(t *testing.T) {
	srv := newPatchServer(t, "/api/v1/userstories/10213", "userstory.json")
	patch := domain.UserStoryPatch{Assignee: 42, Assignees: []int64{42, 43}}

	_, err := taiga.NewClient(srv.URL, staticToken("t")).PatchUserStory(t.Context(), 10213, 7, patch)

	if err != nil || srv.body != `{"assigned_to":42,"assigned_users":[42,43],"version":7}` {
		t.Errorf("body %s, %v", srv.body, err)
	}
}

func TestPatchReportsVersionConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"version": "The version doesn't match with the current one"}`))
	}))
	defer srv.Close()
	c := taiga.NewClient(srv.URL, staticToken("t"))

	_, terr := c.PatchTask(t.Context(), 1, 3, domain.TaskPatch{Status: 2})
	_, uerr := c.PatchUserStory(t.Context(), 1, 3, domain.UserStoryPatch{Status: 2})

	if !errors.Is(terr, taiga.ErrConflict) || !errors.Is(uerr, taiga.ErrConflict) {
		t.Errorf("got %v and %v, want ErrConflict", terr, uerr)
	}
}
