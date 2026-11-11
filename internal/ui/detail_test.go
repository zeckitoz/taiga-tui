package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/zeckitoz/taiga-tui/internal/domain"
	"github.com/zeckitoz/taiga-tui/internal/taiga"
)

func TestDetailViewIsRightOfListFrom110Columns(t *testing.T) {
	h := onKanban(t, 120, 40)

	h.key("j") // #12, with Assignees, Sprint, Tasks and a description

	h.golden("kanban_detail_right")
}

func TestDetailViewIsBelowListUnder110Columns(t *testing.T) {
	h := onKanban(t, 80, 30)

	h.key("j")

	h.golden("kanban_detail_below")
}

func TestDetailViewSwitchesSidesAt110Columns(t *testing.T) {
	h := onKanban(t, 110, 30)
	if !h.detailRight() {
		t.Errorf("not right at 110 columns:\n%s", h.plainView())
	}

	h.send(tea.WindowSizeMsg{Width: 109, Height: 30})

	if h.detailRight() {
		t.Errorf("still right at 109 columns:\n%s", h.plainView())
	}
}

func TestDetailViewReflowsOnResize(t *testing.T) {
	h := onKanban(t, 120, 40)
	h.key("j")

	h.send(tea.WindowSizeMsg{Width: 80, Height: 30})

	h.golden("kanban_detail_below")
}

func TestEnterTogglesDetailView(t *testing.T) {
	h := onKanban(t, 120, 40)

	h.key("enter")
	if view := h.plainView(); strings.Contains(view, "Assignees") || h.detailRight() {
		t.Errorf("detail view still open:\n%s", view)
	}
	h.key("enter")
	if view := h.plainView(); !strings.Contains(view, "Assignees") {
		t.Errorf("detail view not open again:\n%s", view)
	}
}

func TestClosedDetailViewLoadsNoDescriptions(t *testing.T) {
	h := onKanban(t, 120, 40)
	h.key("enter")
	before := h.taiga.Calls("UserStory")

	h.key("j")
	h.key("j")

	if n := h.taiga.Calls("UserStory") - before; n != 0 {
		t.Errorf("%d descriptions loaded while closed", n)
	}
	h.key("enter")
	if !strings.Contains(h.plainView(), "A wishlist keeps products for later.") {
		t.Errorf("description not loaded on opening:\n%s", h.plainView())
	}
}

func TestDescriptionShowsLoadingUntilItArrives(t *testing.T) {
	h := onKanban(t, 120, 40)

	cmd := h.sendOnly(keyMsg("j"))
	if view := h.plainView(); !strings.Contains(view, "Loading description…") {
		t.Errorf("no loading hint:\n%s", view)
	}
	h.runCmd(cmd)
	if view := h.plainView(); !strings.Contains(view, "Customers pay with Visa") {
		t.Errorf("description missing:\n%s", view)
	}
}

func TestEachDescriptionLoadsOnce(t *testing.T) {
	h := onKanban(t, 120, 40)

	h.key("j")
	h.key("k")
	h.key("j")

	if n := h.taiga.Calls("UserStory"); n != 2 {
		t.Errorf("%d loads, want 2", n)
	}
}

func TestFailedDescriptionLoadShowsReasonAndRetriesOnNextKey(t *testing.T) {
	h := onKanban(t, 120, 40)
	h.taiga.FailNext("UserStory", taiga.ErrTimeout)

	h.key("j")
	if view := h.plainView(); !strings.Contains(view, "Couldn't load description: timed out") {
		t.Errorf("no error:\n%s", view)
	}
	h.key("enter")
	h.key("enter")
	if view := h.plainView(); !strings.Contains(view, "Customers pay with Visa") {
		t.Errorf("reopening did not load the description again:\n%s", view)
	}
}

func TestUserStoryWithoutDescriptionSaysSo(t *testing.T) {
	h := onKanban(t, 120, 40)
	for range 4 {
		h.key("l") // Done: #3, in no Sprint
	}

	if view := h.plainView(); !strings.Contains(view, "No description") || !strings.Contains(view, "not in a sprint") {
		t.Errorf("view:\n%s", view)
	}
}

func TestDetailViewOfEmptyTabDescribesStatus(t *testing.T) {
	h := onKanban(t, 120, 40)

	h.key("l") // Ready, empty

	view := h.plainView()
	if !strings.Contains(view, "No user stories in Ready") || strings.Contains(view, "Assignees") {
		t.Errorf("view:\n%s", view)
	}
}

func TestDetailViewShowsWIPLimitOfEmptyStatus(t *testing.T) {
	api := newFake()
	api.SetStories(webShop.ID, webShopStories[0])
	h := resumed(t, api, aliceState(webShop.ID))
	h.send(tea.WindowSizeMsg{Width: 120, Height: 40})
	h.runCmd(h.app.Init())

	h.key("l")
	h.key("l") // In progress, empty

	if view := h.plainView(); !strings.Contains(view, "WIP limit 3") {
		t.Errorf("view:\n%s", view)
	}
}

func TestDetailViewFitsSmallestTerminal(t *testing.T) {
	h := onKanban(t, 60, 15)

	h.key("j")

	h.golden("kanban_detail_60x15")
}

// detailRight reports whether the detail view sits right of the list, which
// draws a vertical rule between them.
func (h *harness) detailRight() bool {
	lines := strings.Split(h.plainView(), "\n")
	return len(lines) > 3 && strings.Contains(lines[3], "│")
}

func TestRefreshErrorKeepsCursorVisible(t *testing.T) {
	api := newFake()
	var many []domain.UserStory
	for i := range 40 {
		many = append(many, domain.UserStory{ID: int64(1000 + i), Ref: 1000 + i, Title: fmt.Sprintf("Story %d", i), Status: 101, KanbanOrder: int64(i)})
	}
	api.SetStories(webShop.ID, many...)
	h := resumed(t, api, aliceState(webShop.ID))
	h.send(tea.WindowSizeMsg{Width: 120, Height: 30})
	h.runCmd(h.app.Init())
	for range 25 {
		h.key("j") // the last visible row
	}
	api.FailNext("Kanban", taiga.ErrTimeout)

	h.key("r")

	// once in the list and once in the detail view
	if view := h.plainView(); !strings.Contains(view, "Couldn't refresh") || strings.Count(view, "Story 25 ") != 2 {
		t.Errorf("cursor scrolled out of view:\n%s", view)
	}
}
