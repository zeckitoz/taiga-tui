package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/zeckitoz/taiga-tui/internal/config"
	"github.com/zeckitoz/taiga-tui/internal/domain"
	"github.com/zeckitoz/taiga-tui/internal/taiga"
)

func headerOf(h *harness) string { return strings.Split(h.plainView(), "\n")[0] }

func TestKanbanMineFiltersCountsAndShowsItInTheHeader(t *testing.T) {
	h := onKanban(t, 100, 24)

	h.key("m")

	if header := headerOf(h); !strings.Contains(header, "mine  Alice Example") {
		t.Errorf("header %q", header)
	}
	view := h.plainView()
	for _, want := range []string{"New 1", "Ready 0", "In progress 1/3", "Checkout with credit card", "m show all"} {
		if !strings.Contains(view, want) {
			t.Errorf("%q missing:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Product search") {
		t.Errorf("someone else's User Story shown:\n%s", view)
	}
	h.golden("kanban_mine")
	h.key("m")
	if strings.Contains(headerOf(h), "mine") || !strings.Contains(h.plainView(), "Product search") {
		t.Errorf("filter not off:\n%s", h.plainView())
	}
}

func TestKanbanMineKeepsEmptyTabHint(t *testing.T) {
	h := onKanban(t, 100, 24)
	h.key("m")

	h.key("l") // Ready

	if view := h.plainView(); !strings.Contains(view, "No user stories in Ready") {
		t.Errorf("view:\n%s", view)
	}
}

func TestKanbanMineWithNothingAssignedShowsHintAndShowAll(t *testing.T) {
	api := newFake()
	api.SetStories(webShop.ID, domain.UserStory{ID: 502, Ref: 15, Title: "Product search", Status: 101})
	h := resumed(t, api, aliceState(webShop.ID))
	h.start()

	h.key("m")

	view := h.plainView()
	if !strings.Contains(view, "Nothing assigned to you") || !strings.Contains(view, "m show all") {
		t.Errorf("view:\n%s", view)
	}
	h.golden("kanban_mine_empty")
	h.key("m")
	if !strings.Contains(h.plainView(), "Product search") {
		t.Errorf("m did not show all:\n%s", h.plainView())
	}
}

func TestTaskboardMineHidesOthersAndTogglesBack(t *testing.T) {
	h := onTaskboard(t, 120, 40)

	h.key("m")

	view := h.plainView()
	if !strings.Contains(headerOf(h), "mine  Alice Example") {
		t.Errorf("header %q", headerOf(h))
	}
	for _, gone := range []string{"Redirect to 3-D Secure", "Order detail page", "Update privacy policy", "#15 Product search"} {
		if strings.Contains(view, gone) {
			t.Errorf("%q still shown:\n%s", gone, view)
		}
	}
	for _, kept := range []string{"Validate card number", "List past orders", "Reorder button", "#12 Checkout", "#21 Order history", "1/2", "3/3"} {
		if !strings.Contains(view, kept) {
			t.Errorf("%q missing:\n%s", kept, view)
		}
	}
	h.golden("taskboard_mine")
	h.key("m")
	if strings.Contains(headerOf(h), "mine") || !strings.Contains(h.plainView(), "Redirect to 3-D Secure") {
		t.Errorf("filter not off:\n%s", h.plainView())
	}
}

func TestTaskboardMineMovesCursorToNextVisibleRow(t *testing.T) {
	h := onTaskboard(t, 120, 40)
	h.key("j")
	h.key("j") // #32, Bea's Task

	h.key("m")

	if got := selectedTaskboardRef(h); got != 21 {
		t.Errorf("selected #%d, want #21, the next visible row", got)
	}
}

func TestTaskboardMineWithNothingAssignedShowsHintAndShowAll(t *testing.T) {
	api := newFake()
	api.SetStories(webShop.ID, domain.UserStory{ID: 502, Ref: 15, Title: "Product search", Status: 101, Sprint: 304, SprintOrder: 1})
	api.SetTasks(webShop.ID, domain.Task{ID: 802, Ref: 32, Title: "Someone else's", Status: 204, UserStory: 502, Sprint: 304, Assignee: 43})
	h := onTaskboardWith(t, api)

	h.key("m")

	view := h.plainView()
	if !strings.Contains(view, "Nothing assigned to you") || !strings.Contains(view, "m show all") {
		t.Errorf("view:\n%s", view)
	}
	h.golden("taskboard_mine_empty")
	h.key("m")
	if !strings.Contains(h.plainView(), "Someone else's") {
		t.Errorf("m did not show all:\n%s", h.plainView())
	}
}

func TestMineIsOffAtStartAndNeverPersisted(t *testing.T) {
	h := onKanban(t, 100, 24)
	saves := h.state.saves

	h.key("m")

	if h.state.saves != saves {
		t.Error("the filter was saved")
	}
	h = onKanban(t, 100, 24)
	if strings.Contains(headerOf(h), "mine") {
		t.Error("filter on at start")
	}
}

func TestMineStaysOnWhenSwitchingBoards(t *testing.T) {
	h := onKanban(t, 100, 24)
	h.key("m")

	h.key(":")
	h.typeText("taskboard")
	h.key("enter")

	if !strings.Contains(headerOf(h), "mine") || strings.Contains(h.plainView(), "Redirect to 3-D Secure") {
		t.Errorf("filter lost on the Taskboard:\n%s", h.plainView())
	}
}

func TestFootersOfBothBoardsOfferMine(t *testing.T) {
	h := onKanban(t, 140, 24)
	if !strings.Contains(h.plainView(), "s status · m mine · r refresh") {
		t.Errorf("Kanban footer:\n%s", h.plainView())
	}
	h = onTaskboard(t, 140, 24)
	if !strings.Contains(h.plainView(), "s status · m mine · r refresh") {
		t.Errorf("Taskboard footer:\n%s", h.plainView())
	}
}

func TestRefreshShowsSpinnerInHeaderWhileOldDataStaysVisible(t *testing.T) {
	h := onKanban(t, 100, 24)

	cmd := h.sendOnly(keyMsg("r")) // the request is in flight

	if !h.app.refreshing() || !strings.Contains(headerOf(h), spinnerFrames[0]) {
		t.Errorf("no spinner in the header %q", headerOf(h))
	}
	if !strings.Contains(h.plainView(), "Product search") {
		t.Errorf("old data not visible:\n%s", h.plainView())
	}
	h.runCmd(cmd)
	if h.app.refreshing() || strings.Contains(headerOf(h), spinnerFrames[0]) {
		t.Errorf("spinner stays after the result: %q", headerOf(h))
	}
}

func TestTaskboardRefreshShowsSpinnerUntilBothLoadsAreDone(t *testing.T) {
	h := onTaskboard(t, 120, 40)

	cmd := h.sendOnly(keyMsg("r"))

	if !h.app.refreshing() {
		t.Fatal("not refreshing")
	}
	h.runCmd(cmd)
	if h.app.refreshing() {
		t.Errorf("still refreshing: %q", headerOf(h))
	}
}

func TestFailedRefreshStopsSpinner(t *testing.T) {
	h := onKanban(t, 100, 24)
	h.taiga.FailNext("Kanban", taiga.ErrTimeout)

	h.key("r")

	if h.app.refreshing() {
		t.Error("spinner stays after a failed refresh")
	}
}

func TestLeavingTheBoardCancelsItsLoadAndDropsTheResult(t *testing.T) {
	h := onKanban(t, 100, 24)
	release := h.taiga.Hold("Kanban")
	defer release()
	cmd := h.sendOnly(keyMsg("r"))
	done := make(chan struct{})
	var result tea.Msg
	go func() { result = cmd(); close(done) }()

	h.key(":")
	h.typeText("taskboard")
	h.key("enter")
	<-done // the request ended by itself, without release

	h.send(result)
	if h.app.board != config.BoardTaskboard || h.app.screen != screenBoard || strings.Contains(h.plainView(), "Couldn't") {
		t.Errorf("late result disturbed the Taskboard:\n%s", h.plainView())
	}
}
