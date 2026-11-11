package ui

import (
	"context"
	"errors"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/zeckitoz/taiga-tui/internal/board"
	"github.com/zeckitoz/taiga-tui/internal/config"
	"github.com/zeckitoz/taiga-tui/internal/domain"
	"github.com/zeckitoz/taiga-tui/internal/taiga"
)

// changeStatus moves the selected row of the board shown to the next (dir
// 1) or previous (dir -1) Status, or tells why it cannot.
func (a App) changeStatus(dir int) (App, tea.Cmd) {
	a, eff, err := a.shownBoard().changeStatus(a, dir)
	if err != nil {
		a.status = append(a.status, statusMsg{text: err.Error(), hint: true})
	}
	a.scrollBoard()
	a, cmd := a.runEffect(eff, a.board)
	a, detail := a.loadDetail()
	return a, tea.Batch(cmd, detail)
}

// runEffect translates an effect of the board on into a command.
func (a App) runEffect(eff board.Effect, on config.Board) (App, tea.Cmd) {
	switch e := eff.(type) {
	case board.SaveStatus:
		return a, saveStatus(a.deps.Taiga, e, on)
	case board.ReloadBoard:
		if a.screen == screenBoard && a.board == on {
			return boardOps(on).reload(a)
		}
	case board.ReloadUserStory:
		api := a.deps.Taiga
		return a, func() tea.Msg {
			s, err := api.UserStory(context.Background(), e.ID)
			return userStoryReloadedMsg{load: e, story: s, err: err}
		}
	}
	return a, nil
}

// saveStatus returns the command that saves the Status Change of save made
// on the board on. Like every PATCH it is never canceled, because the
// change takes effect on the server anyway.
func saveStatus(api Taiga, save board.SaveStatus, on config.Board) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		msg := statusSavedMsg{on: on, save: save}
		if save.Kind == board.TaskItem {
			msg.task, msg.err = api.PatchTask(ctx, save.ID, save.Version, domain.TaskPatch{Status: save.Status})
		} else {
			msg.story, msg.err = api.PatchUserStory(ctx, save.ID, save.Version, domain.UserStoryPatch{Status: save.Status})
		}
		return msg
	}
}

// statusSaved feeds the result of a save back into its board and shows why
// a change was undone. The board ignores results that no longer count,
// e.g. after the Team Member left it.
func (a App) statusSaved(msg statusSavedMsg) (tea.Model, tea.Cmd) {
	a, eff, ok := boardOps(msg.on).statusSaved(a, msg)
	if !ok {
		return a, nil
	}
	if errors.Is(msg.err, taiga.ErrSessionExpired) {
		return a.showLogin(), nil
	}
	if msg.err != nil {
		a.status = append(a.status, saveFailure(msg.save.Ref, msg.err))
	}
	a.scrollBoard() // the status line takes rows from the list
	return a.runEffect(eff, msg.on)
}

// saveFailureOf classifies the error of a save for the board, false if the
// save succeeded.
func saveFailureOf(err error) (board.SaveFailure, bool) {
	switch {
	case err == nil:
		return 0, false
	case errors.Is(err, taiga.ErrConflict):
		return board.SaveConflict, true
	case errors.Is(err, taiga.ErrNotFound):
		return board.SaveNotFound, true
	}
	return board.SaveFailed, true
}

// saveFailure is the status line entry for a save of the item with the
// given ref that failed with err. It stays until the next action.
func saveFailure(ref int, err error) statusMsg {
	item := "#" + strconv.Itoa(ref)
	switch f, _ := saveFailureOf(err); f {
	case board.SaveConflict:
		return statusMsg{text: item + " was changed by someone else. Reloaded, please try again.", hint: true}
	case board.SaveNotFound:
		return statusMsg{text: item + " no longer exists. Reloaded.", hint: true}
	}
	return statusMsg{text: "Couldn't save " + item + ": " + reason(err), isErr: true, hint: true}
}

func (a App) changeKanbanStatus(dir int) (App, board.Effect, error) {
	var eff board.Effect
	var err error
	a.kanban, eff, err = a.kanban.ChangeStatus(dir)
	return a, eff, err
}

func (a App) changeTaskboardStatus(dir int) (App, board.Effect, error) {
	tb, ok := a.sprints.Taskboard()
	if !ok {
		return a, nil, nil
	}
	tb, eff, err := tb.ChangeStatus(dir)
	a.sprints = a.sprints.WithTaskboard(tb)
	return a, eff, err
}

func (a App) kanbanStatusSaved(msg statusSavedMsg) (App, board.Effect, bool) {
	var eff board.Effect
	ok := false
	if f, failed := saveFailureOf(msg.err); failed {
		a.kanban, eff, ok = a.kanban.StatusNotSaved(msg.save, f)
	} else {
		a.kanban, ok = a.kanban.StatusSaved(msg.save, msg.story)
	}
	return a, eff, ok
}

func (a App) taskboardStatusSaved(msg statusSavedMsg) (App, board.Effect, bool) {
	tb, has := a.sprints.Taskboard()
	if !has {
		return a, nil, false
	}
	var eff board.Effect
	ok := false
	f, failed := saveFailureOf(msg.err)
	switch {
	case failed:
		tb, eff, ok = tb.StatusNotSaved(msg.save, f)
	case msg.save.Kind == board.TaskItem:
		tb, eff, ok = tb.TaskSaved(msg.save, msg.task)
	default:
		tb, ok = tb.UserStorySaved(msg.save, msg.story)
	}
	a.sprints = a.sprints.WithTaskboard(tb)
	return a, eff, ok
}

// reloadTaskboard reloads the Taskboard shown.
func (a App) reloadTaskboard() (App, tea.Cmd) {
	if tb, ok := a.sprints.Taskboard(); ok {
		return a.loadTaskboard(tb.Sprint())
	}
	return a, nil
}
