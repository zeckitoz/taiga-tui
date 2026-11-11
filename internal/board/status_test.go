package board_test

import (
	"errors"
	"testing"

	"github.com/zeckitoz/taiga-tui/internal/board"
	"github.com/zeckitoz/taiga-tui/internal/domain"
)

const (
	next = 1
	prev = -1
)

// changeKanban moves the selected User Story to the next Status and fails
// the test unless it asks to save it.
func changeKanban(t *testing.T, k board.Kanban) (board.Kanban, board.SaveStatus) {
	t.Helper()
	k, eff, err := k.ChangeStatus(next)
	save, ok := eff.(board.SaveStatus)
	if err != nil || !ok {
		t.Fatalf("ChangeStatus: effect %#v, error %v", eff, err)
	}
	return k, save
}

func selectedRow(k board.Kanban) board.Row {
	for _, r := range k.Rows() {
		if r.Selected {
			return r
		}
	}
	return board.Row{}
}

func TestKanbanStatusChangeIsOptimisticAndCursorFollows(t *testing.T) {
	versioned := append([]domain.UserStory{}, stories...)
	versioned[1].Version = 6 // #2 First
	k := board.NewKanban(project, versioned, me)

	k, save := changeKanban(t, k)

	if save.Kind != board.UserStoryItem || save.ID != 11 || save.Ref != 2 || save.Status != 2 || save.Version != 6 {
		t.Errorf("save %+v", save)
	}
	if got := k.ActiveStatus().Name; got != "In progress" {
		t.Errorf("active tab %q, want the new Status", got)
	}
	if r := selectedRow(k); r.ID != 11 || !r.Saving {
		t.Errorf("selected %+v, want #2 saving", r)
	}
	if got := []int{k.Tabs()[0].Count, k.Tabs()[1].Count}; got[0] != 2 || got[1] != 2 {
		t.Errorf("counts %v", got)
	}
	if d, _ := k.Detail(); d.Status.Name != "In progress" {
		t.Errorf("detail status %q", d.Status.Name)
	}
}

func TestKanbanStatusChangeSkipsArchivedStatusesAndStopsAtEdges(t *testing.T) {
	k := board.NewKanban(project, stories, me)

	if _, eff, err := k.ChangeStatus(prev); !errors.Is(err, board.ErrFirstStatus) || eff != nil {
		t.Errorf("first status: %v, %#v", err, eff)
	}
	if err := board.ErrFirstStatus; err.Error() != "Already in first status" {
		t.Errorf("text %q", err)
	}
	k = k.NextTab() // #4 Working in In progress
	k, save := changeKanban(t, k)
	if save.Status != 4 {
		t.Errorf("saves Status %d, want Done after the archived Old", save.Status)
	}
	k, _ = k.StatusSaved(save, domain.UserStory{ID: 13, Ref: 4, Title: "Working", Status: 4, Version: 1})
	if _, _, err := k.ChangeStatus(next); !errors.Is(err, board.ErrLastStatus) || err.Error() != "Already in last status" {
		t.Errorf("last status: %v", err)
	}
}

func TestKanbanRejectsChangesWhileSaving(t *testing.T) {
	k, _ := changeKanban(t, board.NewKanban(project, stories, me))

	_, eff, err := k.ChangeStatus(next)

	if !errors.Is(err, board.ErrStillSaving) || err.Error() != "Still saving…" || eff != nil {
		t.Errorf("got %v, %#v", err, eff)
	}
}

func TestKanbanRejectsChangesWithoutPermission(t *testing.T) {
	p := project
	p.Permissions.ModifyUserStory = false

	k, eff, err := board.NewKanban(p, stories, me).ChangeStatus(next)

	if err == nil || err.Error() != "You don't have permission to change user stories" || eff != nil {
		t.Errorf("got %v, %#v", err, eff)
	}
	if k.ActiveStatus().Name != "New" || selectedRow(k).Saving {
		t.Error("changed the board")
	}
}

func TestKanbanTakesSavedUserStoryAndUnlocksIt(t *testing.T) {
	k, save := changeKanban(t, board.NewKanban(project, stories, me))

	k, ok := k.StatusSaved(save, domain.UserStory{ID: 11, Ref: 2, Title: "First (saved)", Status: 2, Version: 1, KanbanOrder: 10})

	if !ok {
		t.Fatal("result dropped")
	}
	if r := selectedRow(k); r.ID != 11 || r.Saving || r.Title != "First (saved)" {
		t.Errorf("selected %+v", r)
	}
	_, again := changeKanban(t, k)
	if again.Version != 1 || again.Status != 4 {
		t.Errorf("next save %+v, want the saved version", again)
	}
}

func TestKanbanUndoesChangeThatWasNotSaved(t *testing.T) {
	for _, tt := range []struct {
		failure board.SaveFailure
		reload  bool
	}{
		{board.SaveConflict, true},
		{board.SaveNotFound, true},
		{board.SaveFailed, false},
	} {
		k, save := changeKanban(t, board.NewKanban(project, stories, me))

		k, eff, ok := k.StatusNotSaved(save, tt.failure)

		if !ok {
			t.Fatalf("%v: result dropped", tt.failure)
		}
		if _, reload := eff.(board.ReloadBoard); reload != tt.reload {
			t.Errorf("%v: effect %#v", tt.failure, eff)
		}
		if got := k.ActiveStatus().Name; got != "New" {
			t.Errorf("%v: cursor stayed in %q instead of following #2 back", tt.failure, got)
		}
		if r := selectedRow(k); r.ID != 11 || r.Saving {
			t.Errorf("%v: selected %+v", tt.failure, r)
		}
		if _, _, err := k.ChangeStatus(next); err != nil {
			t.Errorf("%v: still locked: %v", tt.failure, err)
		}
	}
}

func TestKanbanKeepsChangeThatIsSavingAcrossReload(t *testing.T) {
	k, save := changeKanban(t, board.NewKanban(project, stories, me))

	k = k.Reload(stories) // still the old Status on the server

	if r := selectedRow(k); r.ID != 11 || !r.Saving || k.ActiveStatus().Name != "In progress" {
		t.Errorf("selected %+v in %q", r, k.ActiveStatus().Name)
	}
	if _, ok := k.StatusSaved(save, domain.UserStory{ID: 11, Status: 2, Version: 1}); !ok {
		t.Error("result dropped after reload")
	}
}

func TestKanbanIgnoresResultsOfDroppedSaves(t *testing.T) {
	k, save := changeKanban(t, board.NewKanban(project, stories, me))

	k = k.DropSaves()

	if k.Saving() {
		t.Error("still saving")
	}
	if _, ok := k.StatusSaved(save, domain.UserStory{ID: 11, Status: 2, Version: 1}); ok {
		t.Error("took a saved result")
	}
	if _, eff, ok := k.StatusNotSaved(save, board.SaveConflict); ok || eff != nil {
		t.Error("took a failed result")
	}
}

func TestKanbanStatusChangeNeverChangesTheReceiver(t *testing.T) {
	k := board.NewKanban(project, stories, me)

	changed, save := changeKanban(t, k)
	_, _ = changed.StatusSaved(save, domain.UserStory{ID: 11, Status: 2})

	if k.ActiveStatus().Name != "New" || selectedRow(k).Saving || k.Saving() || !changed.Saving() {
		t.Error("receiver changed")
	}
}

func TestKanbanStatusChangeOnEmptyTabDoesNothing(t *testing.T) {
	k, eff, err := board.NewKanban(project, stories, me).PrevTab().ChangeStatus(next)

	if eff != nil || err != nil || k.Saving() {
		t.Errorf("got %#v, %v", eff, err)
	}
}

// changeTaskboard changes the Status of the selected row and fails the test
// unless it asks to save it.
func changeTaskboard(t *testing.T, tb board.Taskboard, dir int) (board.Taskboard, board.SaveStatus) {
	t.Helper()
	tb, eff, err := tb.ChangeStatus(dir)
	save, ok := eff.(board.SaveStatus)
	if err != nil || !ok {
		t.Fatalf("ChangeStatus: effect %#v, error %v", eff, err)
	}
	return tb, save
}

func TestTaskboardTaskStatusChangeKeepsRowAndChangesLabel(t *testing.T) {
	tb := press(newTaskboard(), down, down, down, down) // T#31, In progress

	tb, save := changeTaskboard(t, tb, next)

	if save.Kind != board.TaskItem || save.ID != 901 || save.Ref != 31 || save.Status != 23 {
		t.Errorf("save %+v", save)
	}
	row := tb.Rows()[4]
	if !row.Selected || row.ID != 901 || row.Status.Name != "Closed" || !row.Saving {
		t.Errorf("row %+v", row)
	}
	if got := tb.Rows()[2].Tasks; got != (domain.TaskProgress{Done: 2, Total: 2}) {
		t.Errorf("progress of #12 %+v", got)
	}
}

func TestTaskboardUserStoryStatusChangeUsesUserStoryStatuses(t *testing.T) {
	tb := press(newTaskboard(), down, down) // US#12, In progress

	tb, save := changeTaskboard(t, tb, next)

	if save.Kind != board.UserStoryItem || save.ID != 501 || save.Status != 4 {
		t.Errorf("save %+v", save)
	}
	if d, _ := tb.Detail(); d.Status.Name != "Done" {
		t.Errorf("detail status %q", d.Status.Name)
	}
	if row := tb.Rows()[2]; !row.Selected || !row.Saving {
		t.Errorf("row %+v", row)
	}
	if _, ok := tb.UserStorySaved(save, domain.UserStory{ID: 501, Ref: 12, Title: "Checkout", Status: 4, SprintOrder: 2, Version: 1}); !ok {
		t.Error("result dropped")
	}
}

func TestTaskboardGroupWithoutUserStoryHasNoStatus(t *testing.T) {
	tb := press(newTaskboard(), down, down, down, down, down, down) // the group without User Story

	_, eff, err := tb.ChangeStatus(next)

	if eff != nil || err != nil {
		t.Errorf("got %#v, %v", eff, err)
	}
}

func TestTaskboardRejectsTaskChangesWithoutPermission(t *testing.T) {
	p := scrum
	p.Permissions.ModifyTask = false
	tb := press(board.NewTaskboard(p, sprint, sprintContent, me), down)

	_, _, err := tb.ChangeStatus(next)

	if err == nil || err.Error() != "You don't have permission to change tasks" {
		t.Errorf("got %v", err)
	}
	if _, _, err := press(tb, up).ChangeStatus(prev); err != nil {
		t.Errorf("User Story: %v", err)
	}
}

func TestTaskboardSavedTaskReloadsItsUserStory(t *testing.T) {
	tb, save := changeTaskboard(t, press(newTaskboard(), down, down, down, down), next)

	tb, eff, ok := tb.TaskSaved(save, domain.Task{ID: 901, Ref: 31, Title: "Card number", Status: 23, UserStory: 501, Assignee: 42, TaskboardOrder: 2, Version: 1})

	reload, isReload := eff.(board.ReloadUserStory)
	if !ok || !isReload || reload.ID != 501 || reload.Sprint != sprint.ID {
		t.Fatalf("ok %v, effect %#v", ok, eff)
	}
	if tb.Rows()[4].Saving {
		t.Error("still saving")
	}
	tb = tb.UserStoryReloaded(reload, domain.UserStory{ID: 501, Ref: 12, Title: "Checkout", Status: 4, SprintOrder: 2})
	if d, _ := press(tb, up, up).Detail(); d.Ref != 12 || d.Status.Name != "Done" {
		t.Errorf("User Story not reloaded: %+v", d)
	}
	other := reload
	other.Sprint = 99
	if d, _ := press(tb.UserStoryReloaded(other, domain.UserStory{ID: 501, Status: 1}), up, up).Detail(); d.Status.Name != "Done" {
		t.Error("took a User Story reloaded for another Sprint")
	}
}

func TestTaskboardSavedTaskWithoutUserStoryInSprintReloadsNothing(t *testing.T) {
	tb := press(newTaskboard(), down, down, down, down, down, down, down, down) // T#40 without User Story
	tb, save := changeTaskboard(t, tb, next)

	_, eff, ok := tb.TaskSaved(save, domain.Task{ID: 904, Ref: 40, Status: 22, Version: 1})

	if !ok || eff != nil {
		t.Errorf("ok %v, effect %#v", ok, eff)
	}
}

func TestTaskboardUndoesChangeThatWasNotSaved(t *testing.T) {
	tb, save := changeTaskboard(t, press(newTaskboard(), down, down, down, down), next)

	tb, eff, ok := tb.StatusNotSaved(save, board.SaveConflict)

	if _, reload := eff.(board.ReloadBoard); !ok || !reload {
		t.Errorf("ok %v, effect %#v", ok, eff)
	}
	if row := tb.Rows()[4]; row.Status.Name != "In progress" || row.Saving || !row.Selected {
		t.Errorf("row %+v", row)
	}
}

func TestOpeningOtherSprintDropsSavesOfTaskboardShown(t *testing.T) {
	s := board.NewSprints(scrum, me)
	s, load := s.Open(sprint)
	s, _ = s.TaskboardLoaded(load, sprintContent)
	tb, _ := s.Taskboard()
	tb, save := changeTaskboard(t, press(tb, down), prev)
	s = s.WithTaskboard(tb)

	s, other := s.Open(sprint2)

	tb, _ = s.Taskboard()
	if tb.Saving() {
		t.Error("still saving")
	}
	if _, _, ok := tb.TaskSaved(save, domain.Task{ID: 903, Status: 22}); ok {
		t.Error("took a result after the Sprint switch")
	}
	s, _ = s.TaskboardLoaded(other, sprintContent)
	tb, _ = s.Taskboard()
	if _, _, ok := tb.TaskSaved(save, domain.Task{ID: 903, Status: 22}); ok {
		t.Error("the new Taskboard took a result of the old one")
	}
}
