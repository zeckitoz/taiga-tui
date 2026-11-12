package taigafake_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/zeckitoz/taiga-tui/internal/domain"
	"github.com/zeckitoz/taiga-tui/internal/taiga"
	"github.com/zeckitoz/taiga-tui/internal/taiga/taigafake"
)

var alice = domain.TeamMember{ID: 42, Username: "alice", FullName: "Alice Example"}

func newFake() *taigafake.Fake {
	f := taigafake.New(alice)
	f.AddProject(domain.Project{
		ProjectSummary: domain.ProjectSummary{ID: 7, Name: "Web Shop", Slug: "web-shop", KanbanEnabled: true},
		UserStoryStatuses: []domain.Status{
			{ID: 1, Name: "New"},
			{ID: 2, Name: "Old", Archived: true},
		},
	},
		domain.UserStory{ID: 10, Status: 1, Description: "Pay by card."},
		domain.UserStory{ID: 11, Status: 2},
	)
	return f
}

func TestKanbanLeavesOutArchivedStatusesLikeTaiga(t *testing.T) {
	got, err := newFake().Kanban(t.Context(), 7)

	if err != nil || len(got) != 1 || got[0].ID != 10 {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestOnlyASingleUserStoryCarriesItsDescriptionLikeTaiga(t *testing.T) {
	f := newFake()

	list, err := f.Kanban(t.Context(), 7)
	if err != nil || list[0].Description != "" {
		t.Errorf("Kanban: %+v, %v", list, err)
	}
	if s, err := f.UserStory(t.Context(), 10); err != nil || s.Description != "Pay by card." {
		t.Errorf("UserStory: %+v, %v", s, err)
	}
	if _, err := f.UserStory(t.Context(), 99); !errors.Is(err, taiga.ErrNotFound) {
		t.Errorf("unknown User Story: %v, want ErrNotFound", err)
	}
}

func TestProjectBySlug(t *testing.T) {
	f := newFake()

	if p, err := f.Project(t.Context(), "web-shop"); err != nil || p.ID != 7 {
		t.Errorf("got %+v, %v", p, err)
	}
	if _, err := f.Project(t.Context(), "gone"); !errors.Is(err, taiga.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestFailNextFailsOnlyTheNextCallOfThatMethod(t *testing.T) {
	f := newFake()
	f.FailNext("Kanban", taiga.ErrTimeout)

	if _, err := f.Me(t.Context()); err != nil {
		t.Errorf("Me: %v", err)
	}
	if _, err := f.Kanban(t.Context(), 7); !errors.Is(err, taiga.ErrTimeout) {
		t.Errorf("first Kanban: %v", err)
	}
	if _, err := f.Kanban(t.Context(), 7); err != nil {
		t.Errorf("second Kanban: %v", err)
	}
	if n := f.Calls("Kanban"); n != 2 {
		t.Errorf("calls %d", n)
	}
}

func TestHoldDelaysCallsUntilReleased(t *testing.T) {
	f := newFake()
	release := f.Hold("Me")
	done := make(chan error)

	go func() {
		_, err := f.Me(context.Background())
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("call returned while held: %v", err)
	default:
	}
	release()

	if err := <-done; err != nil {
		t.Error(err)
	}
}

func TestHeldCallEndsWithItsContext(t *testing.T) {
	f := newFake()
	f.Hold("Projects")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := f.Projects(ctx, 42); !errors.Is(err, context.Canceled) {
		t.Errorf("got %v", err)
	}
}

func TestSprintsOfProject(t *testing.T) {
	f := newFake()
	s := domain.Sprint{ID: 3, Name: "Sprint 1"}
	f.SetSprints(7, s)

	if got, err := f.Sprints(t.Context(), 7); err != nil || len(got) != 1 || got[0] != s {
		t.Errorf("got %+v, %v", got, err)
	}
	if got, err := f.Sprints(t.Context(), 8); err != nil || len(got) != 0 {
		t.Errorf("other project: %+v, %v", got, err)
	}
}

func TestTaskboardHoldsUserStoriesAndTasksOfSprintWithoutDescriptions(t *testing.T) {
	f := newFake()
	f.SetStories(7,
		domain.UserStory{ID: 10, Sprint: 3, Description: "Pay by card."},
		domain.UserStory{ID: 11, Sprint: 4},
		domain.UserStory{ID: 12, Status: 2, Sprint: 3},
	)
	f.SetTasks(7,
		domain.Task{ID: 20, UserStory: 10, Sprint: 3, Description: "Luhn check."},
		domain.Task{ID: 21, Sprint: 4},
		domain.Task{ID: 22, Sprint: 3},
	)

	got, err := f.Taskboard(t.Context(), 7, 3)

	if err != nil {
		t.Fatal(err)
	}
	ids := func(n int, id func(int) int64) []int64 {
		out := make([]int64, n)
		for i := range out {
			out[i] = id(i)
		}
		return out
	}
	// Unlike the Kanban Board, the Taskboard keeps archived Statuses.
	if s := ids(len(got.UserStories), func(i int) int64 { return got.UserStories[i].ID }); !slices.Equal(s, []int64{10, 12}) {
		t.Errorf("User Stories %v", s)
	}
	if s := ids(len(got.Tasks), func(i int) int64 { return got.Tasks[i].ID }); !slices.Equal(s, []int64{20, 22}) {
		t.Errorf("Tasks %v", s)
	}
	if got.UserStories[0].Description != "" || got.Tasks[0].Description != "" {
		t.Errorf("descriptions in lists: %+v", got)
	}
	if other, err := f.Taskboard(t.Context(), 8, 3); err != nil || len(other.UserStories)+len(other.Tasks) != 0 {
		t.Errorf("Taskboard of the Sprint in another project: %+v, %v", other, err)
	}
	if task, err := f.Task(t.Context(), 20); err != nil || task.Description != "Luhn check." {
		t.Errorf("Task: %+v, %v", task, err)
	}
	if _, err := f.Task(t.Context(), 99); !errors.Is(err, taiga.ErrNotFound) {
		t.Errorf("unknown Task: %v, want ErrNotFound", err)
	}
}

func TestPatchUserStoryChangesStatusAndBumpsVersionLikeTaiga(t *testing.T) {
	f := newFake()

	got, err := f.PatchUserStory(t.Context(), 10, 0, domain.UserStoryPatch{Status: 2})

	if err != nil || got.Status != 2 || got.Version != 1 || got.Description != "Pay by card." {
		t.Errorf("got %+v, %v", got, err)
	}
	if s, _ := f.UserStory(t.Context(), 10); s.Status != 2 || s.Version != 1 {
		t.Errorf("stored %+v", s)
	}
	if _, err := f.PatchUserStory(t.Context(), 10, 0, domain.UserStoryPatch{Status: 1}); !errors.Is(err, taiga.ErrConflict) {
		t.Errorf("outdated version: %v, want ErrConflict", err)
	}
	if _, err := f.PatchUserStory(t.Context(), 99, 0, domain.UserStoryPatch{Status: 1}); !errors.Is(err, taiga.ErrNotFound) {
		t.Errorf("unknown User Story: %v, want ErrNotFound", err)
	}
}

func TestPatchTaskChangesStatusAndBumpsVersionLikeTaiga(t *testing.T) {
	f := newFake()
	f.SetTasks(7, domain.Task{ID: 20, Status: 1, Version: 4})

	got, err := f.PatchTask(t.Context(), 20, 4, domain.TaskPatch{Status: 3})

	if err != nil || got.Status != 3 || got.Version != 5 {
		t.Errorf("got %+v, %v", got, err)
	}
	if task, _ := f.Task(t.Context(), 20); task.Status != 3 || task.Version != 5 {
		t.Errorf("stored %+v", task)
	}
	if _, err := f.PatchTask(t.Context(), 20, 4, domain.TaskPatch{Status: 1}); !errors.Is(err, taiga.ErrConflict) {
		t.Errorf("outdated version: %v, want ErrConflict", err)
	}
	if _, err := f.PatchTask(t.Context(), 99, 0, domain.TaskPatch{Status: 1}); !errors.Is(err, taiga.ErrNotFound) {
		t.Errorf("unknown Task: %v, want ErrNotFound", err)
	}
}

func TestPatchTakeOverChangesAssigneesAndKeepsStatusLikeTaiga(t *testing.T) {
	f := newFake()
	f.SetTasks(7, domain.Task{ID: 20, Status: 1, Assignee: 43})

	task, terr := f.PatchTask(t.Context(), 20, 0, domain.TaskPatch{Assignee: 42})
	story, serr := f.PatchUserStory(t.Context(), 10, 0, domain.UserStoryPatch{Assignee: 42, Assignees: []int64{42, 43}})

	if terr != nil || task.Assignee != 42 || task.Status != 1 || task.Version != 1 {
		t.Errorf("task %+v, %v", task, terr)
	}
	if serr != nil || !slices.Equal(story.Assignees, []int64{42, 43}) || story.Status != 1 || story.Version != 1 {
		t.Errorf("story %+v, %v", story, serr)
	}
}

func TestPatchesNeverChangeTheDataPassedIn(t *testing.T) {
	stories := []domain.UserStory{{ID: 10, Status: 1}}
	tasks := []domain.Task{{ID: 20, Status: 1}}
	f := taigafake.New(alice)
	f.AddProject(domain.Project{ProjectSummary: domain.ProjectSummary{ID: 7}}, stories...)
	f.SetTasks(7, tasks...)

	_, _ = f.PatchUserStory(t.Context(), 10, 0, domain.UserStoryPatch{Status: 2})
	_, _ = f.PatchTask(t.Context(), 20, 0, domain.TaskPatch{Status: 2})

	if stories[0].Status != 1 || tasks[0].Status != 1 {
		t.Errorf("changed %+v and %+v", stories[0], tasks[0])
	}
}

func TestSetProjectReplacesProjectAndKeepsItsUserStories(t *testing.T) {
	f := newFake()

	f.SetProject(domain.Project{ProjectSummary: domain.ProjectSummary{ID: 7, Slug: "web-shop", Name: "Renamed"}})

	if p, _ := f.Project(t.Context(), "web-shop"); p.Name != "Renamed" {
		t.Errorf("got %+v", p)
	}
	if list, _ := f.Kanban(t.Context(), 7); len(list) != 2 {
		t.Errorf("User Stories %+v", list)
	}
}
