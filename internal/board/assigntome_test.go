package board_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/zeckitoz/taiga-tui/internal/board"
	"github.com/zeckitoz/taiga-tui/internal/domain"
)

// assignToMeTaskboard assigns the selected row to me and fails the test unless it
// asks to save it.
func assignToMeTaskboard(t *testing.T, tb board.Taskboard, confirmed board.ConfirmAssignToMe) (board.Taskboard, board.Save) {
	t.Helper()
	tb, eff, err := tb.AssignToMe(nothingSent, confirmed)
	save, ok := eff.(board.Save)
	if err != nil || !ok {
		t.Fatalf("AssignToMe: effect %#v, error %v", eff, err)
	}
	return tb, save
}

func TestKanbanAssignToMeKeepsOtherAssigneesAndPutsMeFirst(t *testing.T) {
	k := board.NewKanban(project, stories, me).CursorDown() // #3, assigned to Carla

	k, eff, err := k.AssignToMe(nothingSent)

	want := board.Save{Kind: board.UserStoryItem, ID: 12, Ref: 3, Generation: 1, Assignee: me, Assignees: []int64{me, 44}}
	if err != nil || !reflect.DeepEqual(eff, want) {
		t.Fatalf("got %#v, %v\nwant %#v", eff, err, want)
	}
	r := selectedRow(k)
	if !r.Mine || !r.Saving || len(r.Assignees) != 2 || r.Assignees[0].Name != "Alice Example" || r.Assignees[1].Name != "Carla de la Mar" {
		t.Errorf("row %+v, want me first and Carla kept, saving", r)
	}
}

func TestKanbanAssignToMeOfUnassignedUserStory(t *testing.T) {
	k := board.NewKanban(project, stories, me).CursorDown().CursorDown() // #1, nobody

	_, eff, err := k.AssignToMe(nothingSent)

	save, _ := eff.(board.Save)
	if err != nil || save.Assignee != me || !reflect.DeepEqual(save.Assignees, []int64{me}) || save.Version != 0 {
		t.Errorf("got %#v, %v", eff, err)
	}
}

func TestKanbanAssignToMeRejections(t *testing.T) {
	p := project
	p.Permissions.ModifyUserStory = false
	saving, _ := changeKanban(t, board.NewKanban(project, stories, me))
	tests := []struct {
		name string
		k    board.Kanban
		want string
	}{
		{"already assignee", board.NewKanban(project, stories, me), "Already assigned to you"},
		{"no permission", board.NewKanban(p, stories, me), "You don't have permission to change user stories"},
		{"saving", saving, "Still saving…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k, eff, err := tt.k.AssignToMe(nothingSent)
			if err == nil || err.Error() != tt.want || eff != nil {
				t.Errorf("got %#v, %v, want %q", eff, err, tt.want)
			}
			if !reflect.DeepEqual(k.Rows(), tt.k.Rows()) {
				t.Error("changed the board")
			}
		})
	}
	if !errors.Is(board.ErrAlreadyAssigned, board.ErrAlreadyAssigned) {
		t.Fatal()
	}
}

func TestKanbanAssignToMeUndoneOnFailureAndKeptOnSuccess(t *testing.T) {
	k := board.NewKanban(project, stories, me).CursorDown()
	k, eff, _ := k.AssignToMe(nothingSent)
	save := eff.(board.Save)

	undone, reload, ok := k.NotSaved(save, board.SaveConflict)
	if !ok || reload != (board.ReloadBoard{}) || selectedRow(undone).Mine || len(selectedRow(undone).Assignees) != 1 {
		t.Errorf("undone %+v, %#v, %v", selectedRow(undone), reload, ok)
	}
	saved, ok := k.Saved(save, domain.UserStory{ID: 12, Ref: 3, Title: "Second", Status: 1, KanbanOrder: 20, Assignees: []int64{me, 44}, Version: 1})
	if !ok || !selectedRow(saved).Mine || selectedRow(saved).Saving {
		t.Errorf("saved %+v, %v", selectedRow(saved), ok)
	}
}

func TestTaskboardAssignToMeOfUnassignedTask(t *testing.T) {
	tb := press(newTaskboard(), down, down, down, down, down, down, down, down) // T#40, nobody

	tb, save := assignToMeTaskboard(t, tb, board.ConfirmAssignToMe{})

	if save.Kind != board.TaskItem || save.ID != 904 || save.Ref != 40 || save.Assignee != me || save.Assignees != nil || save.Status != 0 {
		t.Errorf("save %+v", save)
	}
	if row := tb.Rows()[9]; !row.Selected || !row.Mine || !row.Saving || len(row.Assignees) != 1 {
		t.Errorf("row %+v", row)
	}
}

func TestTaskboardAssignToMeAsksBeforeReplacingAnotherAssignee(t *testing.T) {
	tb := press(newTaskboard(), down, down, down) // T#32, Bea (43)

	asked, eff, err := tb.AssignToMe(nothingSent, board.ConfirmAssignToMe{})

	if err != nil || eff != (board.ConfirmAssignToMe{ID: 902, Assignee: 43, Name: "bkoch"}) {
		t.Fatalf("got %#v, %v", eff, err)
	}
	if !reflect.DeepEqual(asked.Rows(), tb.Rows()) {
		t.Error("changed the board before the answer")
	}
	tb, save := assignToMeTaskboard(t, tb, eff.(board.ConfirmAssignToMe))
	if save.Assignee != me || save.ID != 902 {
		t.Errorf("save %+v", save)
	}
	if row := tb.Rows()[3]; !row.Mine || row.Assignees[0].Name != "Alice Example" {
		t.Errorf("row %+v", row)
	}
}

func TestTaskboardAssignToMeAsksAgainIfTheAssigneeChangedSinceTheQuestion(t *testing.T) {
	tb := press(newTaskboard(), down, down, down)
	stale := board.ConfirmAssignToMe{ID: 902, Assignee: 44, Name: "Carla de la Mar"}

	_, eff, err := tb.AssignToMe(nothingSent, stale)

	if err != nil || eff != (board.ConfirmAssignToMe{ID: 902, Assignee: 43, Name: "bkoch"}) {
		t.Errorf("got %#v, %v", eff, err)
	}
}

func TestTaskboardAssignToMeOfUserStoryKeepsOthersAndNeverAsks(t *testing.T) {
	tb := press(newTaskboard(), down, down) // US#12, Bea

	tb, save := assignToMeTaskboard(t, tb, board.ConfirmAssignToMe{})

	if save.Kind != board.UserStoryItem || save.ID != 501 || !reflect.DeepEqual(save.Assignees, []int64{me, 43}) || save.Assignee != me {
		t.Errorf("save %+v", save)
	}
	if d, _ := tb.Detail(); len(d.Assignees) != 2 || !d.Assignees[0].Mine {
		t.Errorf("detail %+v", d.Assignees)
	}
}

func TestTaskboardAssignToMeRejectionsAndNoOps(t *testing.T) {
	noPerm := scrum
	noPerm.Permissions.ModifyTask = false
	tests := []struct {
		name string
		tb   board.Taskboard
		want string
	}{
		{"own task", press(newTaskboard(), down, down, down, down), "Already assigned to you"},
		{"own user story", newTaskboard(), "Already assigned to you"},
		{"no permission", press(board.NewTaskboard(noPerm, sprint, sprintContent, me), down, down, down), "You don't have permission to change tasks"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, eff, err := tt.tb.AssignToMe(nothingSent, board.ConfirmAssignToMe{})
			if err == nil || err.Error() != tt.want || eff != nil {
				t.Errorf("got %#v, %v, want %q", eff, err, tt.want)
			}
		})
	}
	_, eff, err := press(newTaskboard(), down, down, down, down, down, down).AssignToMe(nothingSent, board.ConfirmAssignToMe{}) // group without User Story
	if eff != nil || err != nil {
		t.Errorf("group without User Story: %#v, %v", eff, err)
	}
}

func TestTaskboardAssignToMeIsLockedWhileSavingAndDoesNotReloadTheUserStory(t *testing.T) {
	tb := press(newTaskboard(), down, down, down, down, down, down, down, down)
	tb, save := assignToMeTaskboard(t, tb, board.ConfirmAssignToMe{})

	if _, _, err := tb.AssignToMe(nothingSent, board.ConfirmAssignToMe{}); !errors.Is(err, board.ErrStillSaving) {
		t.Errorf("got %v", err)
	}
	if _, _, err := tb.ChangeStatus(board.Next, nothingSent); !errors.Is(err, board.ErrStillSaving) {
		t.Errorf("status change while saving: %v", err)
	}
	done, eff, ok := tb.TaskSaved(save, domain.Task{ID: 904, Ref: 40, Status: 21, Assignee: me, TaskboardOrder: 2, Version: 1})
	if !ok || eff != nil || done.Saving() {
		t.Errorf("got %#v, %v, saving %v", eff, ok, done.Saving())
	}
}

func TestTaskboardAssignToMeUndoneOnFailure(t *testing.T) {
	tb := press(newTaskboard(), down, down, down, down, down, down, down, down)
	tb, save := assignToMeTaskboard(t, tb, board.ConfirmAssignToMe{})

	tb, eff, ok := tb.NotSaved(save, board.SaveFailed)

	if !ok || eff != nil || tb.Rows()[9].Mine || len(tb.Rows()[9].Assignees) != 0 {
		t.Errorf("row %+v, %#v, %v", tb.Rows()[9], eff, ok)
	}
}
