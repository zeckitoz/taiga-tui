package taigafake_test

import (
	"context"
	"errors"
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
		domain.UserStory{ID: 10, Status: 1},
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
