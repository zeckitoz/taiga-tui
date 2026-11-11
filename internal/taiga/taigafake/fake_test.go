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

	got, err := f.Taskboard(t.Context(), 3)

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
	if task, err := f.Task(t.Context(), 20); err != nil || task.Description != "Luhn check." {
		t.Errorf("Task: %+v, %v", task, err)
	}
	if _, err := f.Task(t.Context(), 99); !errors.Is(err, taiga.ErrNotFound) {
		t.Errorf("unknown Task: %v, want ErrNotFound", err)
	}
}
