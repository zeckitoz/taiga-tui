package board_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/zeckitoz/taiga-tui/internal/board"
	"github.com/zeckitoz/taiga-tui/internal/domain"
)

// nothingSent is the Sent of an App that has no save waiting.
var nothingSent = board.Sent{}

// changeKanban moves the selected User Story to the Status with the given ID
// and fails the test unless it asks to save it.
func changeKanban(t *testing.T, k board.Kanban, status int64) (board.Kanban, board.Save) {
	t.Helper()
	k, eff, err := k.PickStatus(status, nothingSent)
	save, ok := eff.(board.Save)
	if err != nil || !ok {
		t.Fatalf("PickStatus: effect %#v, error %v", eff, err)
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

	k, save := changeKanban(t, k, 2)

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

func TestKanbanStatusChangeRefusesStatusesTheUserStoryCannotHave(t *testing.T) {
	k := board.NewKanban(project, stories, me)

	for name, id := range map[string]int64{"archived": 3, "of another project": 99} {
		if _, eff, err := k.PickStatus(id, nothingSent); !errors.Is(err, board.ErrUnknownStatus) || eff != nil {
			t.Errorf("%s: %v, %#v", name, err, eff)
		}
	}
	if err := board.ErrUnknownStatus; err.Error() != "Unknown status" {
		t.Errorf("text %q", err)
	}
}

func TestKanbanRejectsChangesWhileSaving(t *testing.T) {
	k, _ := changeKanban(t, board.NewKanban(project, stories, me), 2)

	_, eff, err := k.PickStatus(4, nothingSent)

	if !errors.Is(err, board.ErrStillSaving) || err.Error() != "Still saving…" || eff != nil {
		t.Errorf("got %v, %#v", err, eff)
	}
}

func TestKanbanRejectsChangesWithoutPermission(t *testing.T) {
	p := project
	p.Permissions.ModifyUserStory = false

	k, eff, err := board.NewKanban(p, stories, me).PickStatus(2, nothingSent)

	if err == nil || err.Error() != "You don't have permission to change user stories" || eff != nil {
		t.Errorf("got %v, %#v", err, eff)
	}
	if k.ActiveStatus().Name != "New" || selectedRow(k).Saving {
		t.Error("changed the board")
	}
}

func TestKanbanTakesSavedUserStoryAndUnlocksIt(t *testing.T) {
	k, save := changeKanban(t, board.NewKanban(project, stories, me), 2)

	k, ok := k.Saved(save, domain.UserStory{ID: 11, Ref: 2, Title: "First (saved)", Status: 2, Version: 1, KanbanOrder: 10})

	if !ok {
		t.Fatal("result dropped")
	}
	if r := selectedRow(k); r.ID != 11 || r.Saving || r.Title != "First (saved)" {
		t.Errorf("selected %+v", r)
	}
	_, again := changeKanban(t, k, 4)
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
		k, save := changeKanban(t, board.NewKanban(project, stories, me), 2)

		k, eff, ok := k.NotSaved(save, tt.failure)

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
		if _, _, err := k.PickStatus(2, nothingSent); err != nil {
			t.Errorf("%v: still locked: %v", tt.failure, err)
		}
	}
}

func TestKanbanKeepsChangeThatIsSavingAcrossReload(t *testing.T) {
	k, save := changeKanban(t, board.NewKanban(project, stories, me), 2)

	k = k.Reload(stories) // still the old Status on the server

	if r := selectedRow(k); r.ID != 11 || !r.Saving || k.ActiveStatus().Name != "In progress" {
		t.Errorf("selected %+v in %q", r, k.ActiveStatus().Name)
	}
	if _, ok := k.Saved(save, domain.UserStory{ID: 11, Status: 2, Version: 1}); !ok {
		t.Error("result dropped after reload")
	}
}

func TestKanbanIgnoresResultsOfDroppedSaves(t *testing.T) {
	k, save := changeKanban(t, board.NewKanban(project, stories, me), 2)

	k = k.DropSaves()

	if k.Saving() {
		t.Error("still saving")
	}
	if _, ok := k.Saved(save, domain.UserStory{ID: 11, Status: 2, Version: 1}); ok {
		t.Error("took a saved result")
	}
	if _, eff, ok := k.NotSaved(save, board.SaveConflict); ok || eff != nil {
		t.Error("took a failed result")
	}
}

func TestKanbanStatusChangeNeverChangesTheReceiver(t *testing.T) {
	k := board.NewKanban(project, stories, me)

	changed, save := changeKanban(t, k, 2)
	_, _ = changed.Saved(save, domain.UserStory{ID: 11, Status: 2})

	if k.ActiveStatus().Name != "New" || selectedRow(k).Saving || k.Saving() || !changed.Saving() {
		t.Error("receiver changed")
	}
}

func TestKanbanStatusChangeOnEmptyTabDoesNothing(t *testing.T) {
	k, eff, err := board.NewKanban(project, stories, me).PrevTab().PickStatus(2, nothingSent)

	if eff != nil || err != nil || k.Saving() {
		t.Errorf("got %#v, %v", eff, err)
	}
}

// changeTaskboard changes the Status of the selected row to the one with the
// given ID and fails the test
// unless it asks to save it.
func changeTaskboard(t *testing.T, tb board.Taskboard, status int64) (board.Taskboard, board.Save) {
	t.Helper()
	tb, eff, err := tb.PickStatus(status, nothingSent)
	save, ok := eff.(board.Save)
	if err != nil || !ok {
		t.Fatalf("PickStatus: effect %#v, error %v", eff, err)
	}
	return tb, save
}

func TestTaskboardTaskStatusChangeKeepsRowAndChangesLabel(t *testing.T) {
	tb := press(newTaskboard(), down, down, down, down) // T#31, In progress

	tb, save := changeTaskboard(t, tb, 23)

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

	tb, save := changeTaskboard(t, tb, 4)

	if save.Kind != board.UserStoryItem || save.ID != 501 || save.Status != 4 {
		t.Errorf("save %+v", save)
	}
	if d, _ := tb.Detail(); d.Status.Name != "Done" {
		t.Errorf("detail status %q", d.Status.Name)
	}
	if row := tb.Rows()[2]; !row.Selected || !row.Saving || row.Status.Name != "Done" {
		t.Errorf("row %+v", row)
	}
	if _, ok := tb.UserStorySaved(save, domain.UserStory{ID: 501, Ref: 12, Title: "Checkout", Status: 4, SprintOrder: 2, Version: 1}); !ok {
		t.Error("result dropped")
	}
}

func TestTaskboardGroupWithoutUserStoryHasNoStatus(t *testing.T) {
	tb := press(newTaskboard(), down, down, down, down, down, down) // the group without User Story

	_, eff, err := tb.PickStatus(22, nothingSent)

	if eff != nil || err != nil {
		t.Errorf("got %#v, %v", eff, err)
	}
}

func TestTaskboardRejectsTaskChangesWithoutPermission(t *testing.T) {
	p := scrum
	p.Permissions.ModifyTask = false
	tb := press(board.NewTaskboard(p, sprint, sprintContent, me), down)

	_, _, err := tb.PickStatus(22, nothingSent)

	if err == nil || err.Error() != "You don't have permission to change tasks" {
		t.Errorf("got %v", err)
	}
	if _, _, err := press(tb, up).PickStatus(2, nothingSent); err != nil {
		t.Errorf("User Story: %v", err)
	}
}

func TestTaskboardSavedTaskReloadsItsUserStory(t *testing.T) {
	tb, save := changeTaskboard(t, press(newTaskboard(), down, down, down, down), 23)

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
	tb, save := changeTaskboard(t, tb, 22)

	_, eff, ok := tb.TaskSaved(save, domain.Task{ID: 904, Ref: 40, Status: 22, Version: 1})

	if !ok || eff != nil {
		t.Errorf("ok %v, effect %#v", ok, eff)
	}
}

func TestTaskboardUndoesChangeThatWasNotSaved(t *testing.T) {
	tb, save := changeTaskboard(t, press(newTaskboard(), down, down, down, down), 23)

	tb, eff, ok := tb.NotSaved(save, board.SaveConflict)

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
	tb, save := changeTaskboard(t, press(tb, down), 22)
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

func TestItemStaysLockedUntilItsSaveIsAnsweredEvenAfterLeavingTheBoard(t *testing.T) {
	_, save := changeKanban(t, board.NewKanban(project, stories, me), 2)
	sent := nothingSent.Add(save)
	back := board.NewKanban(project, stories, me) // the board loaded again, #2 selected

	if _, _, err := back.PickStatus(2, sent); !errors.Is(err, board.ErrStillSaving) {
		t.Errorf("Kanban: %v, want ErrStillSaving", err)
	}
	content := domain.SprintContent{UserStories: []domain.UserStory{{ID: 11, Ref: 2, Status: 1}}}
	tb := board.NewTaskboard(project, sprint, content, me)
	if _, _, err := tb.PickStatus(2, sent); !errors.Is(err, board.ErrStillSaving) {
		t.Errorf("Taskboard: %v, want ErrStillSaving", err)
	}

	sent = sent.Answered(save)
	_, eff, err := tb.PickStatus(2, sent)
	again, ok := eff.(board.Save)
	if err != nil || !ok {
		t.Fatalf("after the answer: %#v, %v", eff, err)
	}
	if again.Generation <= save.Generation {
		t.Errorf("generation %d repeats %d", again.Generation, save.Generation)
	}
}

func TestAnswerOfAnEarlierSaveKeepsTheLaterOneLocked(t *testing.T) {
	first := board.Save{Kind: board.UserStoryItem, ID: 11, Generation: 1}
	later := board.Save{Kind: board.UserStoryItem, ID: 11, Generation: 2}
	sent := nothingSent.Add(first).Answered(first).Add(later)

	sent = sent.Answered(first)

	k := board.NewKanban(project, stories, me)
	if _, _, err := k.PickStatus(2, sent); !errors.Is(err, board.ErrStillSaving) {
		t.Errorf("got %v, want ErrStillSaving", err)
	}
}

func TestBoardsTakeItemsSavedWhileTheyWereLeft(t *testing.T) {
	k := board.NewKanban(project, stories, me).Replace(domain.UserStory{ID: 11, Ref: 2, Title: "First", Status: 2, Version: 3})
	tb := newTaskboard().
		ReplaceTask(domain.Task{ID: 903, Ref: 33, Title: "Form", Status: 22, UserStory: 502, Version: 4}).
		ReplaceUserStory(domain.UserStory{ID: 502, Ref: 15, Title: "Login", Status: 1, SprintOrder: 1, Version: 5})

	if k.ActiveStatus().Name != "New" || k.Tabs()[1].Count != 2 {
		t.Errorf("Kanban tabs %+v", k.Tabs())
	}
	_, save := changeKanban(t, k.NextTab(), 4)
	if save.ID != 11 || save.Version != 3 {
		t.Errorf("Kanban save %+v, want the saved version", save)
	}
	_, task := changeTaskboard(t, press(tb, down), 23)
	_, story := changeTaskboard(t, tb, 2)
	if task.Version != 4 || task.Status != 23 || story.Version != 5 || story.Status != 2 {
		t.Errorf("Taskboard saves %+v and %+v", task, story)
	}
	if k := board.NewKanban(project, stories, me).Replace(domain.UserStory{ID: 99, Status: 1}); k.Tabs()[0].Count != 3 {
		t.Error("took a User Story the board does not show")
	}
}

func TestTaskboardUserStoryRowRevertsStatusWhenSaveFails(t *testing.T) {
	tb := press(newTaskboard(), down, down) // US#12, In progress
	tb, save := changeTaskboard(t, tb, 4)

	tb, _, ok := tb.NotSaved(save, board.SaveFailed)

	if row := tb.Rows()[2]; !ok || row.Saving || row.Status.Name != "In progress" {
		t.Errorf("row %+v", row)
	}
}

func TestKanbanOffersTheActiveStatusesOfTheSelectedUserStory(t *testing.T) {
	opts, err := board.NewKanban(project, stories, me).StatusOptions(nothingSent)

	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range opts.Statuses {
		names = append(names, s.Name)
	}
	if want := []string{"New", "In progress", "Done"}; !slices.Equal(names, want) || opts.Current != 1 {
		t.Errorf("statuses %v, current %d", names, opts.Current)
	}
}

func TestKanbanMovesToAnyStatusAndSavesIt(t *testing.T) {
	versioned := append([]domain.UserStory{}, stories...)
	versioned[1].Version = 6 // #2 First
	k := board.NewKanban(project, versioned, me)

	k, eff, err := k.PickStatus(4, nothingSent)

	save, ok := eff.(board.Save)
	if err != nil || !ok {
		t.Fatalf("effect %#v, error %v", eff, err)
	}
	if save.Kind != board.UserStoryItem || save.ID != 11 || save.Status != 4 || save.Version != 6 || save.Generation != 1 {
		t.Errorf("save %+v", save)
	}
	if got := k.ActiveStatus().Name; got != "Done" || !selectedRow(k).Saving {
		t.Errorf("cursor in %q, selected %+v", got, selectedRow(k))
	}
}

func TestPickStatusRefusesWhatTheNeighbourKeysRefuse(t *testing.T) {
	noPerm := project
	noPerm.Permissions.ModifyUserStory = false
	saving, _ := changeKanban(t, board.NewKanban(project, stories, me), 2)

	for name, tt := range map[string]struct {
		k    board.Kanban
		want error
	}{
		"saving":     {saving, board.ErrStillSaving},
		"unknown":    {board.NewKanban(project, stories, me), board.ErrUnknownStatus},
		"archived":   {board.NewKanban(project, stories, me), board.ErrUnknownStatus},
		"permission": {board.NewKanban(noPerm, stories, me), nil},
	} {
		id := int64(4)
		switch name {
		case "unknown":
			id = 99
		case "archived":
			id = 3 // Old
		}
		_, eff, err := tt.k.PickStatus(id, nothingSent)
		_, optErr := tt.k.StatusOptions(nothingSent)
		if eff != nil || err == nil || (name != "unknown" && name != "archived" && optErr == nil) {
			t.Errorf("%s: effect %#v, errors %v, %v", name, eff, err, optErr)
		}
		if tt.want != nil && !errors.Is(err, tt.want) {
			t.Errorf("%s: got %v, want %v", name, err, tt.want)
		}
		if name == "permission" && err.Error() != "You don't have permission to change user stories" {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestTaskboardOffersTheStatusesOfTheKindOfTheSelectedRow(t *testing.T) {
	task := press(newTaskboard(), down) // a Task
	story := newTaskboard()             // its User Story

	taskOpts, err := task.StatusOptions(nothingSent)
	if err != nil {
		t.Fatal(err)
	}
	storyOpts, err := story.StatusOptions(nothingSent)
	if err != nil {
		t.Fatal(err)
	}

	if taskOpts.Kind != board.TaskItem || storyOpts.Kind != board.UserStoryItem {
		t.Errorf("kinds %v, %v", taskOpts.Kind, storyOpts.Kind)
	}
	if len(taskOpts.Statuses) != len(scrum.TaskStatuses) || len(storyOpts.Statuses) == 0 {
		t.Errorf("task statuses %d, story statuses %d", len(taskOpts.Statuses), len(storyOpts.Statuses))
	}
	_, eff, err := task.PickStatus(scrum.TaskStatuses[len(scrum.TaskStatuses)-1].ID, nothingSent)
	if save, ok := eff.(board.Save); err != nil || !ok || save.Kind != board.TaskItem {
		t.Errorf("effect %#v, error %v", eff, err)
	}
}

func TestTaskboardGroupWithoutUserStoryHasNoStatusToPick(t *testing.T) {
	tb := press(newTaskboard(), down, down, down, down, down, down)

	opts, err := tb.StatusOptions(nothingSent)
	_, eff, pickErr := tb.PickStatus(1, nothingSent)

	if len(opts.Statuses) != 0 || err != nil || eff != nil || pickErr != nil {
		t.Errorf("got %+v, %v, %#v, %v", opts, err, eff, pickErr)
	}
}
