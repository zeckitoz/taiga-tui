//go:build live

package taiga_test

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/zeckitoz/taiga-tui/internal/config"
	"github.com/zeckitoz/taiga-tui/internal/domain"
	"github.com/zeckitoz/taiga-tui/internal/taiga"
)

// api is the authenticated part of the Taiga API the smoke test exercises,
// which is the interface of the UI.
type api interface {
	Me(ctx context.Context) (domain.TeamMember, error)
	Projects(ctx context.Context, memberID int64) ([]domain.ProjectSummary, error)
	Project(ctx context.Context, slug string) (domain.Project, error)
	Kanban(ctx context.Context, projectID int64) ([]domain.UserStory, error)
	UserStory(ctx context.Context, id int64) (domain.UserStory, error)
	Sprints(ctx context.Context, projectID int64) ([]domain.Sprint, error)
	Taskboard(ctx context.Context, projectID, sprintID int64) (domain.SprintContent, error)
	Task(ctx context.Context, id int64) (domain.Task, error)
	PatchTask(ctx context.Context, id int64, version int, p domain.TaskPatch) (domain.Task, error)
	PatchUserStory(ctx context.Context, id int64, version int, p domain.UserStoryPatch) (domain.UserStory, error)
}

// TestLiveSmoke is the smoke test a maintainer runs against the real
// instance before every release. It runs only with the build tag live:
//
//	TAIGA_TUI_LIVE_USER=… TAIGA_TUI_LIVE_PASSWORD=… TAIGA_TUI_LIVE_PROJECT=<slug> \
//	  go test -tags live ./internal/taiga -run TestLiveSmoke -v
//
// It uses the environment variables of the recorder and is skipped without
// them. It changes one User Story and one Task of the test project, and puts
// them back in a cleanup, also if a check fails. CI never sets the tag. See
// docs/test-project.md for the test project.
func TestLiveSmoke(t *testing.T) {
	user, password, slug := liveSmokeEnv(t)
	instance := config.DefaultInstanceURL
	if u := os.Getenv("TAIGA_TUI_LIVE_URL"); u != "" {
		instance = u
	}
	ctx := t.Context()

	auth := taiga.NewAuthAPI(instance)
	login, err := auth.Login(ctx, user, password)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	tokens, err := auth.Refresh(ctx, login.Tokens.Refresh)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	smoke(t, taiga.NewClient(instance, staticToken(tokens.Access)), slug)

	_, err = taiga.NewClient(instance, staticToken(tokens.Access+"x")).Me(ctx)
	if !errors.Is(err, taiga.ErrSessionExpired) {
		t.Errorf("Me with a broken token: got %v, want ErrSessionExpired", err)
	}
}

func liveSmokeEnv(t *testing.T) (user, password, slug string) {
	t.Helper()
	vars := []string{"TAIGA_TUI_LIVE_USER", "TAIGA_TUI_LIVE_PASSWORD", "TAIGA_TUI_LIVE_PROJECT"}
	values := make([]string, len(vars))
	for i, name := range vars {
		values[i] = os.Getenv(name)
		if values[i] == "" {
			t.Skipf("the live smoke test needs %s", name)
		}
	}
	return values[0], values[1], values[2]
}

// smoke runs every call of the Taiga interface against the project with the
// given slug.
func smoke(t *testing.T, c api, slug string) {
	t.Helper()
	ctx := t.Context()

	me, err := c.Me(ctx)
	if err != nil || me.ID == 0 {
		t.Fatalf("Me: %+v, %v", me, err)
	}
	projects, err := c.Projects(ctx, me.ID)
	if err != nil {
		t.Fatalf("Projects: %v", err)
	}
	if !slices.ContainsFunc(projects, func(p domain.ProjectSummary) bool { return p.Slug == slug }) {
		t.Fatalf("Projects: the test project is not among the %d projects of the Team Member", len(projects))
	}
	project, err := c.Project(ctx, slug)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if len(project.UserStoryStatuses) < 2 || len(project.TaskStatuses) < 2 {
		t.Fatalf("Project: needs at least two User Story and two Task Statuses, got %d and %d",
			len(project.UserStoryStatuses), len(project.TaskStatuses))
	}
	if !project.Permissions.ModifyUserStory || !project.Permissions.ModifyTask {
		t.Fatalf("Project: the Team Member may not modify items: %+v", project.Permissions)
	}

	stories, err := c.Kanban(ctx, project.ID)
	if err != nil {
		t.Fatalf("Kanban: %v", err)
	}
	sprints, err := c.Sprints(ctx, project.ID)
	if err != nil {
		t.Fatalf("Sprints: %v", err)
	}
	current := domain.ResolveCurrentSprint(sprints, domain.DateOf(time.Now()), 0)
	if current.Kind != domain.One {
		t.Fatal("Sprints: the test project needs exactly one Sprint containing today")
	}
	content, err := c.Taskboard(ctx, project.ID, current.Sprint.ID)
	if err != nil {
		t.Fatalf("Taskboard: %v", err)
	}

	t.Run("UserStory", func(t *testing.T) { smokeUserStory(t, c, project, me, stories) })
	t.Run("Task", func(t *testing.T) { smokeTask(t, c, project, me, content.Tasks) })
}

func smokeUserStory(t *testing.T, c api, project domain.Project, me domain.TeamMember, stories []domain.UserStory) {
	ctx := t.Context()
	// Taiga's PATCH cannot empty the Assignees, so the story needs an
	// Assignee other than me to put back.
	i := slices.IndexFunc(stories, func(s domain.UserStory) bool {
		return len(s.Assignees) > 0 && !s.AssignedTo(me.ID) && openStatus(project.UserStoryStatuses, s.Status) != 0
	})
	if i < 0 {
		t.Fatal("Kanban: needs a User Story with an Assignee other than the Team Member, in a Status with another open Status to move to")
	}
	id := stories[i].ID
	orig, err := c.UserStory(ctx, id)
	if err != nil {
		t.Fatalf("UserStory: %v", err)
	}
	if orig.ID != id || orig.Title == "" {
		t.Fatalf("UserStory: %+v", orig)
	}
	t.Cleanup(func() { restoreUserStory(t, c, orig) })

	// Status Change there and back.
	other := openStatus(project.UserStoryStatuses, orig.Status)
	moved, err := c.PatchUserStory(ctx, id, orig.Version, domain.UserStoryPatch{Status: other})
	if err != nil || moved.Status != other {
		t.Fatalf("Status Change: %+v, %v", moved, err)
	}
	back, err := c.PatchUserStory(ctx, id, moved.Version, domain.UserStoryPatch{Status: orig.Status})
	if err != nil || back.Status != orig.Status {
		t.Fatalf("Status Change back: %+v, %v", back, err)
	}

	// Assign to me there and back.
	assignees := append(slices.Clone(back.Assignees), me.ID)
	taken, err := c.PatchUserStory(ctx, id, back.Version, domain.UserStoryPatch{Assignee: back.Assignees[0], Assignees: assignees})
	if err != nil || !taken.AssignedTo(me.ID) {
		t.Fatalf("Assign to me: %+v, %v", taken, err)
	}
	given, err := c.PatchUserStory(ctx, id, taken.Version, domain.UserStoryPatch{Assignee: orig.Assignees[0], Assignees: orig.Assignees})
	if err != nil || given.AssignedTo(me.ID) {
		t.Fatalf("Assign to me back: %+v, %v", given, err)
	}

	// A provoked conflict: back.Version is outdated by now.
	_, err = c.PatchUserStory(ctx, id, back.Version, domain.UserStoryPatch{Status: other})
	if !errors.Is(err, taiga.ErrConflict) {
		t.Errorf("patching with an outdated version: got %v, want ErrConflict", err)
	}
}

func smokeTask(t *testing.T, c api, project domain.Project, me domain.TeamMember, tasks []domain.Task) {
	ctx := t.Context()
	// Taiga's PATCH cannot empty the Assignee, so the Task needs an Assignee
	// other than me to put back.
	i := slices.IndexFunc(tasks, func(task domain.Task) bool {
		return task.Assignee != 0 && task.Assignee != me.ID && openStatus(project.TaskStatuses, task.Status) != 0
	})
	if i < 0 {
		t.Fatal("Taskboard: needs a Task with an Assignee other than the Team Member")
	}
	id := tasks[i].ID
	orig, err := c.Task(ctx, id)
	if err != nil {
		t.Fatalf("Task: %v", err)
	}
	if orig.ID != id || orig.Title == "" {
		t.Fatalf("Task: %+v", orig)
	}
	t.Cleanup(func() { restoreTask(t, c, orig) })

	other := openStatus(project.TaskStatuses, orig.Status)
	moved, err := c.PatchTask(ctx, id, orig.Version, domain.TaskPatch{Status: other})
	if err != nil || moved.Status != other {
		t.Fatalf("Status Change: %+v, %v", moved, err)
	}
	back, err := c.PatchTask(ctx, id, moved.Version, domain.TaskPatch{Status: orig.Status})
	if err != nil || back.Status != orig.Status {
		t.Fatalf("Status Change back: %+v, %v", back, err)
	}

	taken, err := c.PatchTask(ctx, id, back.Version, domain.TaskPatch{Assignee: me.ID})
	if err != nil || taken.Assignee != me.ID {
		t.Fatalf("Assign to me: %+v, %v", taken, err)
	}
	given, err := c.PatchTask(ctx, id, taken.Version, domain.TaskPatch{Assignee: orig.Assignee})
	if err != nil || given.Assignee != orig.Assignee {
		t.Fatalf("Assign to me back: %+v, %v", given, err)
	}

	_, err = c.PatchTask(ctx, id, back.Version, domain.TaskPatch{Status: other})
	if !errors.Is(err, taiga.ErrConflict) {
		t.Errorf("patching with an outdated version: got %v, want ErrConflict", err)
	}
}

// openStatus returns the ID of a Status other than current that is neither
// closed nor archived, so that moving there has no side effects beyond the
// Status. It returns 0 if there is none.
func openStatus(statuses []domain.Status, current int64) int64 {
	for _, s := range statuses {
		if s.ID != current && !s.Closed && !s.Archived {
			return s.ID
		}
	}
	return 0
}

// restoreUserStory puts the User Story back as it was before the test, so the
// test project stays as it is even if a check failed halfway.
func restoreUserStory(t *testing.T, c api, orig domain.UserStory) {
	t.Helper()
	ctx := context.WithoutCancel(t.Context())
	cur, err := c.UserStory(ctx, orig.ID)
	if err != nil {
		t.Errorf("restoring the User Story: %v", err)
		return
	}
	if cur.Status != orig.Status || !slices.Equal(cur.Assignees, orig.Assignees) {
		cur, err = c.PatchUserStory(ctx, orig.ID, cur.Version, domain.UserStoryPatch{
			Status: orig.Status, Assignee: orig.Assignees[0], Assignees: orig.Assignees,
		})
		if err != nil {
			t.Errorf("restoring the User Story: %v", err)
			return
		}
	}
	if cur.Status != orig.Status || !slices.Equal(cur.Assignees, orig.Assignees) {
		t.Errorf("the User Story was not restored: got status %d and Assignees %v, want %d and %v",
			cur.Status, cur.Assignees, orig.Status, orig.Assignees)
	}
}

func restoreTask(t *testing.T, c api, orig domain.Task) {
	t.Helper()
	ctx := context.WithoutCancel(t.Context())
	cur, err := c.Task(ctx, orig.ID)
	if err != nil {
		t.Errorf("restoring the Task: %v", err)
		return
	}
	if cur.Status != orig.Status || cur.Assignee != orig.Assignee {
		cur, err = c.PatchTask(ctx, orig.ID, cur.Version, domain.TaskPatch{Status: orig.Status, Assignee: orig.Assignee})
		if err != nil {
			t.Errorf("restoring the Task: %v", err)
			return
		}
	}
	if cur.Status != orig.Status || cur.Assignee != orig.Assignee {
		t.Errorf("the Task was not restored: got status %d and Assignee %d, want %d and %d",
			cur.Status, cur.Assignee, orig.Status, orig.Assignee)
	}
}
