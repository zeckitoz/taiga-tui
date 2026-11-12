package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/zeckitoz/taiga-tui/internal/domain"
	"github.com/zeckitoz/taiga-tui/internal/taiga"
)

// onKanban returns a harness showing the Kanban Board of Web Shop at w×h.
func onKanban(t *testing.T, w, h int) *harness {
	t.Helper()
	hs := resumed(t, newFake(), aliceState(webShop.ID))
	hs.send(tea.WindowSizeMsg{Width: w, Height: h})
	hs.runCmd(hs.app.Init())
	if hs.app.screen != screenBoard {
		t.Fatalf("not on the board, screen %v", hs.app.screen)
	}
	return hs
}

func selectedRef(h *harness) int {
	for _, r := range h.app.kanban.Rows() {
		if r.Selected {
			return r.Ref
		}
	}
	return 0
}

func TestKanbanBoardShowsTabsAndUserStoriesOfFirstStatus(t *testing.T) {
	h := onKanban(t, 100, 24)

	h.golden("kanban")
}

func TestKanbanBoardShortensInactiveTabsWhenNarrow(t *testing.T) {
	h := onKanban(t, 60, 15)

	for i, line := range strings.Split(h.plainView(), "\n") {
		if w := ansi.StringWidth(line); w > 60 {
			t.Errorf("line %d is %d cells wide: %q", i, w, line)
		}
	}
	h.golden("kanban_60x15")
}

func TestKanbanColorsOfStatusTabsAndCursor(t *testing.T) {
	h := onKanban(t, 100, 12)

	h.ansiGolden("kanban")
}

func TestArrowKeysAndHJKLSwitchTabsAndMoveCursor(t *testing.T) {
	for _, keys := range [][4]string{{"left", "right", "up", "down"}, {"h", "l", "k", "j"}} {
		t.Run(keys[0], func(t *testing.T) {
			h := onKanban(t, 100, 24)
			prev, next, up, down := keys[0], keys[1], keys[2], keys[3]

			h.key(down)
			h.key(down)
			if got := selectedRef(h); got != 108 {
				t.Errorf("after two downs: #%d", got)
			}
			h.key(up)
			if got := selectedRef(h); got != 12 {
				t.Errorf("after up: #%d", got)
			}
			h.key(next)
			h.key(next)
			if got := h.app.kanban.ActiveStatus().Name; got != "In progress" {
				t.Errorf("after two nexts: %q", got)
			}
			h.key(prev)
			if got := h.app.kanban.ActiveStatus().Name; got != "Ready" {
				t.Errorf("after prev: %q", got)
			}
		})
	}
}

func TestInProgressTabShowsWIPLimitAndMyUserStory(t *testing.T) {
	h := onKanban(t, 100, 24)

	h.key("l")
	h.key("l")

	h.golden("kanban_in_progress")
}

func TestEmptyStatusTabShowsHint(t *testing.T) {
	h := onKanban(t, 100, 24)

	h.key("l")

	if view := h.plainView(); !strings.Contains(view, "No user stories in Ready") || !strings.Contains(view, "Ready 0") {
		t.Errorf("view:\n%s", view)
	}
}

func TestEmptyKanbanBoardShowsHintAndRefreshes(t *testing.T) {
	api := newFake()
	api.SetStories(webShop.ID)
	h := resumed(t, api, aliceState(webShop.ID))
	h.start()

	h.golden("kanban_empty")

	api.SetStories(webShop.ID, webShopStories...)
	h.key("r")
	if got := h.app.kanban.Tabs()[0].Count; got != 3 {
		t.Errorf("refresh did not reload, New has %d", got)
	}
}

func TestRefreshKeepsSelectedUserStory(t *testing.T) {
	h := onKanban(t, 100, 24)
	h.key("j")
	h.taiga.SetStories(webShop.ID, append([]domain.UserStory{
		{ID: 599, Ref: 99, Title: "Brand new", Status: 101, KanbanOrder: 1},
	}, webShopStories...)...)

	h.key("r")

	if got := selectedRef(h); got != 12 {
		t.Errorf("selected #%d after refresh", got)
	}
	if !strings.Contains(h.plainView(), "Brand new") {
		t.Errorf("new User Story missing:\n%s", h.plainView())
	}
}

func TestFailedRefreshKeepsBoardAndShowsError(t *testing.T) {
	h := onKanban(t, 100, 24)
	h.taiga.FailNext("Kanban", taiga.ErrTimeout)

	h.key("r")

	view := h.plainView()
	if h.app.screen != screenBoard || !strings.Contains(view, "Couldn't refresh: timed out") || !strings.Contains(view, "Product search") {
		t.Errorf("view:\n%s", view)
	}
}

func TestKanbanLoadFailureShowsErrorPageAndRetries(t *testing.T) {
	api := newFake()
	api.FailNext("Kanban", &taiga.ServerError{Code: 502})
	h := resumed(t, api, aliceState(webShop.ID))
	h.start()

	if view := h.plainView(); !strings.Contains(view, "server error (502)") {
		t.Fatalf("view:\n%s", view)
	}
	h.key("r")

	if h.app.screen != screenBoard || len(h.app.kanban.Rows()) != 3 {
		t.Errorf("retry failed, screen %v", h.app.screen)
	}
}

func TestKanbanListScrollsToKeepCursorVisible(t *testing.T) {
	api := newFake()
	var many []domain.UserStory
	for i := range 40 {
		many = append(many, domain.UserStory{ID: int64(1000 + i), Ref: 1000 + i, Title: fmt.Sprintf("Story %d", i), Status: 101, KanbanOrder: int64(i)})
	}
	api.SetStories(webShop.ID, many...)
	h := resumed(t, api, aliceState(webShop.ID))
	h.start()

	for range 30 {
		h.key("j")
	}

	view := h.plainView()
	if !strings.Contains(view, "Story 30") || strings.Contains(view, "Story 0 ") {
		t.Errorf("cursor not visible:\n%s", view)
	}
	if lines := strings.Count(view, "\n") + 1; lines != 24 {
		t.Errorf("view has %d lines, want 24", lines)
	}
}

func TestEmptyStatusTabGolden(t *testing.T) {
	h := onKanban(t, 100, 12)

	h.key("l")

	h.golden("kanban_empty_tab")
}

func TestFailedRefreshErrorClearsOnNextAction(t *testing.T) {
	h := onKanban(t, 100, 24)
	h.taiga.FailNext("Kanban", taiga.ErrTimeout)
	h.taiga.FailNext("Kanban", taiga.ErrTimeout)
	h.key("r")
	h.key("r")

	if n := strings.Count(h.plainView(), "Couldn't refresh"); n != 1 {
		t.Errorf("%d refresh errors shown, want 1", n)
	}
	h.key("j")
	if strings.Contains(h.plainView(), "Couldn't refresh") {
		t.Error("error still shown after the next action")
	}
}

func TestKanbanLoadFailureNamesTheBoard(t *testing.T) {
	api := newFake()
	api.FailNext("Kanban", taiga.ErrNotFound)
	h := resumed(t, api, aliceState(webShop.ID))

	h.start()

	if view := h.plainView(); !strings.Contains(view, "Couldn't load Kanban Board: unexpected response from server") {
		t.Errorf("view:\n%s", view)
	}
}

func TestKanbanFooterAndHelpShowTheEnterGlyph(t *testing.T) {
	h := onKanban(t, 120, 24)
	if view := h.plainView(); !strings.Contains(view, "↵ detail") || strings.Contains(view, "enter detail") {
		t.Errorf("footer:\n%s", view)
	}
	h.key("?")
	if !strings.Contains(h.plainView(), "↵ enter") {
		t.Errorf("full help:\n%s", h.plainView())
	}
}
