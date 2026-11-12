package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/zeckitoz/taiga-tui/internal/domain"
	"github.com/zeckitoz/taiga-tui/internal/taiga"
)

// lineWith returns the line of the view that contains s.
func lineWith(h *harness, s string) string {
	for _, l := range strings.Split(h.plainView(), "\n") {
		if strings.Contains(l, s) {
			return l
		}
	}
	return ""
}

// pickStatus changes the Status of the selected row with the status picker.
func pickStatus(h *harness, name string) {
	h.t.Helper()
	h.key("s")
	h.typeText(name)
	h.key("enter")
}

// pickStatusOnly is pickStatus, but returns the command that saves the
// change instead of running it.
func (h *harness) pickStatusOnly(name string) tea.Cmd {
	h.t.Helper()
	h.key("s")
	h.typeText(name)
	return h.sendOnly(keyMsg("enter"))
}

// storedStory returns the User Story as the fake instance holds it.
func storedStory(t *testing.T, h *harness, id int64) domain.UserStory {
	t.Helper()
	s, err := h.taiga.UserStory(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPickingAStatusChangesTheUserStoryAndTheCursorFollows(t *testing.T) {
	h := onKanban(t, 100, 24) // #15 Product search selected in New

	pickStatus(h, "ready")

	if got := h.app.kanban.ActiveStatus().Name; got != "Ready" || selectedRef(h) != 15 {
		t.Errorf("cursor on #%d in %q, want #15 in Ready", selectedRef(h), got)
	}
	if got := storedStory(t, h, 502); got.Status != 102 || got.Version != 1 {
		t.Errorf("stored %+v", got)
	}
	pickStatus(h, "new")
	if got := storedStory(t, h, 502); got.Status != 101 || got.Version != 2 {
		t.Errorf("not saved with the new version: %+v", got)
	}
	if got := h.app.kanban.ActiveStatus().Name; got != "New" {
		t.Errorf("back in %q", got)
	}
}

func TestShiftArrowsAndHLDoNothingOnTheKanbanBoard(t *testing.T) {
	for _, key := range []string{"shift+left", "shift+right", "H", "L"} {
		t.Run(key, func(t *testing.T) {
			h := onKanban(t, 100, 24) // #15 selected in New
			view := h.plainView()

			h.key(key)

			if h.plainView() != view || h.taiga.Calls("PatchUserStory") != 0 {
				t.Errorf("%s changed the board:\n%s", key, h.plainView())
			}
		})
	}
}

func TestSavingRowShowsSpinnerAndRefusesFurtherChanges(t *testing.T) {
	h := onKanban(t, 100, 24)

	save := h.pickStatusOnly("ready")

	if line := lineWith(h, "#15"); !strings.Contains(line, spinnerFrames[0]) {
		t.Errorf("no spinner: %q", line)
	}
	h.key("s")
	if !strings.Contains(h.plainView(), "Still saving…") {
		t.Errorf("view:\n%s", h.plainView())
	}
	h.runCmd(save)
	if line := lineWith(h, "#15"); strings.Contains(line, spinnerFrames[0]) {
		t.Errorf("spinner after the save: %q", line)
	}
	if h.app.saving.active {
		t.Error("spinner still runs")
	}
	if n := h.taiga.Calls("PatchUserStory"); n != 1 {
		t.Errorf("%d requests, want 1", n)
	}
}

func TestStatusChangeWithoutPermissionDoesNothing(t *testing.T) {
	api := newFake()
	p := webShop
	p.Permissions = domain.Permissions{}
	api.SetProject(p)
	h := resumed(t, api, aliceState(webShop.ID))
	h.start()

	h.key("s")

	if !strings.Contains(h.plainView(), "You don't have permission to change user stories") {
		t.Errorf("view:\n%s", h.plainView())
	}
	if h.app.kanban.ActiveStatus().Name != "New" || api.Calls("PatchUserStory") != 0 {
		t.Error("changed the Status")
	}
}

// TestConflictUndoesChangeReloadsBoardAndTellsWhy is the end-to-end test of
// the chain taiga → board → ui.
func TestConflictUndoesChangeReloadsBoardAndTellsWhy(t *testing.T) {
	h := onKanban(t, 100, 24)
	// A colleague changes #15 after the board loaded, which bumps its version.
	changed := append([]domain.UserStory{}, webShopStories...)
	changed[1].Title, changed[1].Version = "Product search with filters", 1
	h.taiga.SetStories(webShop.ID, changed...)

	pickStatus(h, "ready")

	if n := h.taiga.Calls("Kanban"); n != 2 {
		t.Errorf("Kanban loaded %d times, want a reload", n)
	}
	if got := h.app.kanban.ActiveStatus().Name; got != "New" || selectedRef(h) != 15 {
		t.Errorf("cursor on #%d in %q, want #15 back in New", selectedRef(h), got)
	}
	if line := lineWith(h, "#15"); !strings.Contains(line, "Product search with filters") {
		t.Errorf("colleague's change not reloaded: %q", line)
	}
	want := "#15 was changed by someone else. Reloaded, please try again."
	if !strings.Contains(h.plainView(), want) {
		t.Errorf("view:\n%s", h.plainView())
	}
	if len(h.app.status) != 1 || h.app.status[0].isErr {
		t.Errorf("status %+v, want one warning", h.app.status)
	}
}

func TestFailedSaveUndoesChangeAndShowsErrorUntilNextAction(t *testing.T) {
	h := onKanban(t, 100, 24)
	h.taiga.FailNext("PatchUserStory", taiga.ErrUnreachable)

	pickStatus(h, "ready")

	if !strings.Contains(h.plainView(), "Couldn't save #15: no connection") || !h.app.status[0].isErr {
		t.Errorf("status %+v", h.app.status)
	}
	if h.app.kanban.ActiveStatus().Name != "New" || h.taiga.Calls("Kanban") != 1 {
		t.Error("not undone, or reloaded")
	}
	h.key("j")
	if strings.Contains(h.plainView(), "Couldn't save") {
		t.Error("error stays after the next action")
	}
}

func TestExpiredSessionDuringSaveShowsLogin(t *testing.T) {
	h := onKanban(t, 100, 24)
	h.taiga.FailNext("PatchUserStory", taiga.ErrSessionExpired)

	pickStatus(h, "ready")

	if h.app.screen != screenLogin {
		t.Errorf("screen %v", h.app.screen)
	}
}

func TestSaveResultAfterBoardSwitchIsIgnored(t *testing.T) {
	h := onKanban(t, 100, 24)
	h.taiga.FailNext("PatchUserStory", taiga.ErrConflict)
	save := h.pickStatusOnly("ready")
	h.key(":")
	h.typeText("taskboard")
	h.key("enter")
	kanbanLoads := h.taiga.Calls("Kanban")

	h.runCmd(save)

	if len(h.app.status) != 0 || h.taiga.Calls("Kanban") != kanbanLoads {
		t.Errorf("result was not ignored: status %+v", h.app.status)
	}
	if h.app.kanban.Saving() || h.app.saving.active {
		t.Error("still saving after leaving the board")
	}
}

func TestTaskStatusChangeKeepsRowAndReloadsUserStory(t *testing.T) {
	h := onTaskboard(t, 120, 40)
	h.key("j") // #31 In progress
	userStoryLoads := h.taiga.Calls("UserStory")

	pickStatus(h, "ready for test")

	if selectedTaskboardRef(h) != 31 {
		t.Errorf("cursor on #%d", selectedTaskboardRef(h))
	}
	if line := lineWith(h, "▍ #31"); !strings.Contains(line, "Ready for test") {
		t.Errorf("label not changed: %q", line)
	}
	if task, _ := h.taiga.Task(t.Context(), 801); task.Status != 203 {
		t.Errorf("stored %+v", task)
	}
	if n := h.taiga.Calls("UserStory"); n != userStoryLoads+1 {
		t.Errorf("User Story loaded %d times, want once more", n-userStoryLoads)
	}
}

func TestUserStoryStatusChangeOnTaskboard(t *testing.T) {
	h := onTaskboard(t, 120, 40) // #12 New

	pickStatus(h, "ready")

	if got := storedStory(t, h, 501); got.Status != 102 {
		t.Errorf("stored %+v", got)
	}
	if d, _ := shownTaskboard(h).Detail(); d.Ref != 12 || d.Status.Name != "Ready" {
		t.Errorf("detail %+v", d)
	}
}

func TestTaskStatusChangeWithoutPermission(t *testing.T) {
	api := newFake()
	p := webShop
	p.Permissions.ModifyTask = false
	api.SetProject(p)
	st := aliceState(webShop.ID)
	st.LastBoard = "taskboard"
	h := resumed(t, api, st)
	h.start()
	h.key("j")

	h.key("s")

	if !strings.Contains(h.plainView(), "You don't have permission to change tasks") {
		t.Errorf("view:\n%s", h.plainView())
	}
}

func TestSaveFailureTexts(t *testing.T) {
	tests := []struct {
		err   error
		want  string
		isErr bool
	}{
		{taiga.ErrConflict, "#42 was changed by someone else. Reloaded, please try again.", false},
		{taiga.ErrNotFound, "#42 no longer exists. Reloaded.", false},
		{taiga.ErrUnreachable, "Couldn't save #42: no connection", true},
		{taiga.ErrTimeout, "Couldn't save #42: timed out", true},
		{&taiga.ServerError{Code: 500}, "Couldn't save #42: server error (500)", true},
		{errors.New("boom"), "Couldn't save #42: unexpected response from server", true},
	}
	for _, tt := range tests {
		got := saveFailure(42, tt.err)
		if got.text != tt.want || got.isErr != tt.isErr || !got.hint {
			t.Errorf("saveFailure(%v) = %+v, want %q, error %v", tt.err, got, tt.want, tt.isErr)
		}
	}
}

func TestFootersAndHelpNameStatusChange(t *testing.T) {
	h := onKanban(t, 120, 24)
	if !strings.Contains(h.plainView(), "s status") || strings.Contains(h.plainView(), "⇧") {
		t.Errorf("Kanban footer:\n%s", h.plainView())
	}
	h.key("?")
	if strings.Contains(h.plainView(), "previous / next status") || strings.Contains(h.plainView(), "⇧") {
		t.Errorf("Kanban help:\n%s", h.plainView())
	}

	tb := onTaskboard(t, 120, 40)
	if !strings.Contains(tb.plainView(), "s status") || strings.Contains(tb.plainView(), "⇧") {
		t.Errorf("Taskboard footer:\n%s", tb.plainView())
	}
}

func TestSavingRowsGolden(t *testing.T) {
	h := onKanban(t, 100, 12)
	h.pickStatusOnly("ready")
	h.golden("kanban_saving")

	tb := onTaskboard(t, 100, 14)
	tb.pickStatusOnly("ready") // #12
	tb.key("j")
	tb.pickStatusOnly("ready for test") // #31
	tb.golden("taskboard_saving")
}

func TestKanbanFooterOffersStatusChangeOnlyWithUserStory(t *testing.T) {
	h := onKanban(t, 120, 24)

	h.key("l") // the empty Ready tab

	if strings.Contains(h.plainView(), "s status") {
		t.Errorf("view:\n%s", h.plainView())
	}
}

func TestItemStaysLockedAcrossBoardSwitchUntilItsSaveIsAnswered(t *testing.T) {
	h := onKanban(t, 100, 40)
	save := h.pickStatusOnly("ready") // #15 Product search
	h.key(":")
	h.typeText("taskboard")
	h.key("enter")
	for range 7 {
		h.key("j")
	}
	if got := selectedTaskboardRef(h); got != 15 {
		t.Fatalf("selected #%d, want the User Story #15", got)
	}

	h.key("s")

	if !strings.Contains(h.plainView(), "Still saving…") {
		t.Errorf("view:\n%s", h.plainView())
	}
	h.runCmd(save)
	pickStatus(h, "in progress")
	if n := h.taiga.Calls("PatchUserStory"); n != 2 {
		t.Errorf("%d requests, want the second one after the answer", n)
	}
	if got := storedStory(t, h, 502); got.Status != 103 || got.Version != 2 {
		t.Errorf("stored %+v", got)
	}
}
