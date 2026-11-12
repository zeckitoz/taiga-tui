package ui

import (
	"context"
	"errors"
	"slices"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/zeckitoz/taiga-tui/internal/board"
	"github.com/zeckitoz/taiga-tui/internal/config"
	"github.com/zeckitoz/taiga-tui/internal/domain"
	"github.com/zeckitoz/taiga-tui/internal/taiga"
)

// changeStatus moves the selected row of the board shown to the Status
// next to its own in direction d, or tells why it cannot.
func (a App) changeStatus(d board.Direction) (App, tea.Cmd) {
	return a.applyStatusChange(func(a App) (App, board.Effect, error) { return a.shownBoard().changeStatus(a, d) })
}

// pickStatus changes the selected row of the board shown to the Status
// with the given ID, or tells why it cannot.
func (a App) pickStatus(id int64) (App, tea.Cmd) {
	return a.applyStatusChange(func(a App) (App, board.Effect, error) { return a.shownBoard().pickStatus(a, id) })
}

// applyStatusChange makes a Status Change on the board shown through
// change. It is the one path of both the keys for the next or previous
// Status and the picker.
func (a App) applyStatusChange(change func(App) (App, board.Effect, error)) (App, tea.Cmd) {
	a, eff, err := change(a)
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
		a.sent = a.sent.Add(e)
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
	a.sent = a.sent.Answered(msg.save)
	a, eff, ok := boardOps(msg.on).statusSaved(a, msg)
	if !ok {
		if msg.err == nil {
			// The Team Member left the board: the change took effect on the
			// server anyway, and the next change needs its version.
			return a.withSavedItem(msg), nil
		}
		return a, nil
	}
	if errors.Is(msg.err, taiga.ErrSessionExpired) {
		return a.showLogin(), nil
	}
	// Off the board, e.g. in a picker, the board reloads when it opens again.
	if msg.err != nil && a.screen == screenBoard {
		a.status = append(a.status, saveFailure(msg.save.Ref, msg.err))
	}
	a.scrollBoard() // the status line takes rows from the list
	return a.runEffect(eff, msg.on)
}

// withSavedItem gives both boards the item of a successful save whose
// board ignored the result.
func (a App) withSavedItem(msg statusSavedMsg) App {
	if msg.save.Kind == board.TaskItem {
		return a.withTaskboard(func(tb board.Taskboard) board.Taskboard { return tb.ReplaceTask(msg.task) })
	}
	a.kanban = a.kanban.Replace(msg.story)
	return a.withTaskboard(func(tb board.Taskboard) board.Taskboard { return tb.ReplaceUserStory(msg.story) })
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

// changeKanbanStatus changes the Status of the selected User Story of the
// Kanban Board.
func (a App) changeKanbanStatus(d board.Direction) (App, board.Effect, error) {
	var eff board.Effect
	var err error
	a.kanban, eff, err = a.kanban.ChangeStatus(d, a.sent)
	return a, eff, err
}

// statusOptionsKanban returns the Statuses the selected User Story of the
// Kanban Board can be changed to.
func (a App) statusOptionsKanban() (board.StatusOptions, error) {
	return a.kanban.StatusOptions(a.sent)
}

// statusOptionsTaskboard returns the Statuses the selected row of the
// Taskboard can be changed to.
func (a App) statusOptionsTaskboard() (board.StatusOptions, error) {
	tb, ok := a.sprints.Taskboard()
	if !ok {
		return board.StatusOptions{}, nil
	}
	return tb.StatusOptions(a.sent)
}

// pickKanbanStatus moves the selected User Story of the Kanban Board to
// the Status with the given ID.
func (a App) pickKanbanStatus(id int64) (App, board.Effect, error) {
	var eff board.Effect
	var err error
	a.kanban, eff, err = a.kanban.PickStatus(id, a.sent)
	return a, eff, err
}

// pickTaskboardStatus moves the selected row of the Taskboard to the
// Status with the given ID.
func (a App) pickTaskboardStatus(id int64) (App, board.Effect, error) {
	tb, ok := a.sprints.Taskboard()
	if !ok {
		return a, nil, nil
	}
	tb, eff, err := tb.PickStatus(id, a.sent)
	a.sprints = a.sprints.WithTaskboard(tb)
	return a, eff, err
}

// openStatusPalette opens the palette in Status mode for the selected row
// of the board shown, or tells why it cannot be changed. It does nothing
// if the row has no Status.
func (a App) openStatusPalette() App {
	opts, err := a.shownBoard().statusOptions(a)
	if err != nil {
		a.status = append(a.status, statusMsg{text: err.Error(), hint: true})
		a.scrollBoard()
		return a
	}
	if len(opts.Statuses) == 0 {
		return a
	}
	entries := make([]paletteEntry, len(opts.Statuses))
	for i, s := range opts.Statuses {
		entries[i] = paletteEntry{kind: entryStatus, id: s.ID, label: "Status: " + plainText(s.Name), search: plainText(s.Name), current: s.ID == opts.Current}
	}
	a.paletteOpen, a.paletteStatus, a.pickedFor = true, true, opts
	a.helpOpen = false // the palette has its own footer
	a.palette = newPalette(entries, "Pick a status")
	a.palette.selected = slices.IndexFunc(entries, func(e paletteEntry) bool { return e.current })
	a.palette.setWidth(a.width)
	a.palette.scroll(a.paletteListHeight())
	return a
}

// changeTaskboardStatus changes the Status of the selected row of the
// Taskboard.
func (a App) changeTaskboardStatus(d board.Direction) (App, board.Effect, error) {
	tb, ok := a.sprints.Taskboard()
	if !ok {
		return a, nil, nil
	}
	tb, eff, err := tb.ChangeStatus(d, a.sent)
	a.sprints = a.sprints.WithTaskboard(tb)
	return a, eff, err
}

// kanbanStatusSaved feeds the result of a save into the Kanban Board.
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

// taskboardStatusSaved feeds the result of a save into the Taskboard shown.
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
