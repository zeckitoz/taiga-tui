package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/zeckitoz/taiga-tui/internal/config"
	"github.com/zeckitoz/taiga-tui/internal/domain"
	"github.com/zeckitoz/taiga-tui/internal/taiga"
)

// failedKanban returns a harness on a Kanban Board whose load failed.
func failedKanban(t *testing.T, err error) *harness {
	t.Helper()
	api := newFake()
	api.FailNext("Kanban", err)
	h := resumed(t, api, aliceState(webShop.ID))
	h.send(tea.WindowSizeMsg{Width: 80, Height: 20})
	h.runCmd(h.app.Init())
	return h
}

func TestBoardLoadFailureWithoutDataShowsErrorInBoardAndRetries(t *testing.T) {
	h := failedKanban(t, &taiga.ServerError{Code: 502})

	view := h.plainView()
	if h.app.screen != screenBoard || !strings.Contains(view, "Couldn't load Kanban Board: server error (502)") {
		t.Fatalf("screen %v:\n%s", h.app.screen, view)
	}
	if !strings.Contains(view, "r retry · : menu · q quit") || !strings.Contains(view, "Web Shop") {
		t.Errorf("footer or header missing:\n%s", view)
	}
	h.golden("board_load_failed")

	h.key("r")

	if h.app.failedBoard != nil || !h.app.kanban.Loaded() || strings.Contains(h.plainView(), "Couldn't load") {
		t.Errorf("retry failed:\n%s", h.plainView())
	}
	if got := h.taiga.Calls("Kanban"); got != 2 {
		t.Errorf("%d Kanban calls, want 2 (no automatic retry)", got)
	}
}

func TestPaletteStaysUsableOnFailedBoard(t *testing.T) {
	h := failedKanban(t, taiga.ErrUnreachable)

	h.key(":")
	if !h.app.paletteOpen {
		t.Fatalf("palette did not open:\n%s", h.plainView())
	}
	for paletteLabel(h) != "Go to Taskboard" {
		h.key("down")
	}
	h.key("enter")

	if h.app.board != config.BoardTaskboard || h.app.failedBoard != nil || strings.Contains(h.plainView(), "Couldn't load") {
		t.Errorf("did not leave the failed board:\n%s", h.plainView())
	}
}

func TestFailedBoardIgnoresKeysOfTheBoard(t *testing.T) {
	h := failedKanban(t, taiga.ErrTimeout)

	h.key("s")
	h.key("a")

	if h.app.failedBoard == nil || h.app.paletteOpen {
		t.Errorf("state changed:\n%s", h.plainView())
	}
	h.key("q")
}

func TestExpiredSessionOnBoardLoadShowsLoginNotError(t *testing.T) {
	h := failedKanban(t, taiga.ErrSessionExpired)

	if h.app.screen != screenLogin || h.app.failedBoard != nil {
		t.Errorf("screen %v:\n%s", h.app.screen, h.plainView())
	}
}

func TestEveryFailureReasonIsShort(t *testing.T) {
	for err, want := range map[error]string{
		taiga.ErrUnreachable:          "no connection",
		taiga.ErrTimeout:              "timed out",
		&taiga.ServerError{Code: 502}: "server error (502)",
		taiga.ErrUnexpected:           "unexpected response from server",
	} {
		h := failedKanban(t, err)
		if !strings.Contains(h.plainView(), "Couldn't load Kanban Board: "+want) {
			t.Errorf("%v:\n%s", err, h.plainView())
		}
	}
}

func TestTaskboardLoadFailureShowsErrorInBoard(t *testing.T) {
	api := newFake()
	api.FailNext("Taskboard", taiga.ErrTimeout)
	st := aliceState(webShop.ID)
	st.LastBoard = config.BoardTaskboard
	h := resumed(t, api, st)
	h.send(tea.WindowSizeMsg{Width: 80, Height: 20})
	h.runCmd(h.app.Init())

	if h.app.screen != screenBoard || !strings.Contains(h.plainView(), "Couldn't load Taskboard: timed out") {
		t.Fatalf("screen %v:\n%s", h.app.screen, h.plainView())
	}
	h.key("r")
	if shownTaskboard(h).Sprint().ID != sprint4.ID {
		t.Errorf("retry failed:\n%s", h.plainView())
	}
}

func TestFailedProjectSwitchKeepsPickerOpenWithErrorBelowAndEnterRetries(t *testing.T) {
	for _, from := range []string{"picker", "palette"} {
		t.Run(from, func(t *testing.T) {
			h := onKanban(t, 80, 20)
			h.taiga.FailNext("Project", taiga.ErrTimeout)
			if from == "picker" {
				h = resumed(t, h.taiga, aliceState(0))
				h.send(tea.WindowSizeMsg{Width: 80, Height: 20})
				h.runCmd(h.app.Init())
				h.key("down")
				h.key("enter")
			} else {
				h.key(":")
				for paletteLabel(h) != "Project: mobile app" {
					h.key("down")
				}
				h.key("enter")
			}

			view := h.plainView()
			if h.app.screen != screenPicker || !strings.Contains(view, "Couldn't load ") || !strings.Contains(view, "timed out") {
				t.Fatalf("screen %v:\n%s", h.app.screen, view)
			}
			h.send(tea.WindowSizeMsg{Width: 80, Height: 20}) // anything but a key keeps the error
			if !strings.Contains(h.plainView(), "timed out") {
				t.Errorf("error gone before enter:\n%s", h.plainView())
			}
			h.key("enter")
			if h.app.screen != screenBoard {
				t.Errorf("enter did not retry, screen %v:\n%s", h.app.screen, h.plainView())
			}
		})
	}
}

func paletteLabel(h *harness) string {
	e, _ := h.app.palette.chosen()
	return e.label
}

func TestUnknownStatusReloadsProjectOnceAndShowsNewStatus(t *testing.T) {
	h := onKanban(t, 100, 24)
	changed := webShop
	changed.UserStoryStatuses = append(append([]domain.Status{}, webShop.UserStoryStatuses...), domain.Status{ID: 107, Name: "Review"})
	h.taiga.SetProject(changed)
	h.taiga.SetStories(webShop.ID, append([]domain.UserStory{{ID: 599, Ref: 99, Title: "Reviewed", Status: 107, KanbanOrder: 1}}, webShopStories...)...)
	before := h.taiga.Calls("Project")

	h.key("r")

	if got := h.taiga.Calls("Project") - before; got != 1 {
		t.Errorf("%d project reloads, want 1", got)
	}
	tabs := h.app.kanban.Tabs()
	if tabs[len(tabs)-1].Name != "Review" || tabs[len(tabs)-1].Count != 1 {
		t.Errorf("tabs %+v", tabs)
	}
}

func TestStillUnknownStatusShowsUnknownTab(t *testing.T) {
	h := onKanban(t, 100, 24)
	h.taiga.SetStories(webShop.ID, append([]domain.UserStory{{ID: 599, Ref: 99, Title: "Mystery", Status: 107, KanbanOrder: 1}}, webShopStories...)...)
	before := h.taiga.Calls("Project")

	h.key("r")

	if got := h.taiga.Calls("Project") - before; got != 1 {
		t.Errorf("%d project reloads, want 1", got)
	}
	tabs := h.app.kanban.Tabs()
	if last := tabs[len(tabs)-1]; last.Name != "Unknown" || last.Count != 1 {
		t.Fatalf("tabs %+v", tabs)
	}
	h.key("h") // wraps around to the Unknown tab
	if !strings.Contains(h.plainView(), "Mystery") {
		t.Errorf("view:\n%s", h.plainView())
	}
	h.golden("kanban_unknown_status")
}

func TestFailedRefreshKeepsDataAndShowsRedErrorInStatusLine(t *testing.T) {
	h := onKanban(t, 80, 20)
	h.taiga.FailNext("Kanban", &taiga.ServerError{Code: 502})

	h.key("r")

	if !h.app.status[0].isErr || !strings.Contains(h.plainView(), "Couldn't refresh: server error (502)") {
		t.Fatalf("view:\n%s", h.plainView())
	}
	h.golden("kanban_refresh_failed")
	h.key("j") // the data stays usable
	if h.app.failedBoard != nil {
		t.Error("refresh failure turned into a board error")
	}
}

func TestTaskboardShowsUnknownStatusAndMemberAfterTheProjectReload(t *testing.T) {
	api := newFake()
	api.SetTasks(webShop.ID, append([]domain.Task{
		{ID: 899, Ref: 99, Title: "Mystery task", Status: 299, UserStory: 501, Sprint: 304, Assignee: 99, TaskboardOrder: 0},
	}, webShopTasks...)...)
	h := onTaskboardWith(t, api, 100, 24)

	if got := api.Calls("Project"); got != 2 {
		t.Errorf("%d Project calls, want 2 (start and one reload)", got)
	}
	view := h.plainView()
	if !strings.Contains(view, "Mystery task") {
		t.Fatalf("view:\n%s", view)
	}
	h.key("down")
	h.key("down")
	if !strings.Contains(h.plainView(), "Unknown member") || !strings.Contains(h.plainView(), "Unknown status") {
		t.Errorf("view:\n%s", h.plainView())
	}
	h.golden("taskboard_unknown_status_and_member")
}
