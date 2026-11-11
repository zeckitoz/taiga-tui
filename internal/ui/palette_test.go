package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/zeckitoz/taiga-tui/internal/config"
	"github.com/zeckitoz/taiga-tui/internal/taiga"
)

func TestColonAndCtrlKOpenPaletteAndEscClosesIt(t *testing.T) {
	for _, open := range []string{":", "ctrl+k"} {
		t.Run(open, func(t *testing.T) {
			h := onTaskboard(t, 120, 30)

			h.key(open)
			if !h.app.paletteOpen || !strings.Contains(h.plainView(), "Go to Kanban Board") {
				t.Fatalf("palette not open:\n%s", h.plainView())
			}

			h.key("esc")
			if h.app.paletteOpen || h.quit || strings.Contains(h.plainView(), "Go to Kanban Board") {
				t.Errorf("palette still open, quit %v:\n%s", h.quit, h.plainView())
			}
		})
	}
}

func TestPaletteListsBoardsSprintsAndProjectsWithCurrentMarked(t *testing.T) {
	h := onTaskboard(t, 120, 30)

	h.key(":")

	view := h.plainView()
	for _, want := range []string{
		"● Go to Taskboard",
		"  Go to Kanban Board",
		"  Sprint: Sprint 5     17.11. - 30.11.2026  upcoming",
		"● Sprint: Sprint 4     03.11. - 16.11.2026  current",
		"  Sprint: Sprint 3     20.10. - 02.11.2026  closed",
		"  Project: Archive     (no boards enabled)",
		"  Project: mobile app  Taskboard",
		"● Project: Web Shop ★  Taskboard · Kanban Board",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q", want)
		}
	}
	h.golden("palette")
}

func TestPaletteSwitchesBoardsAndRemembersLastBoard(t *testing.T) {
	h := onKanban(t, 100, 24)

	h.key(":")
	h.typeText("taskboard")
	h.key("enter")
	if h.app.paletteOpen || h.app.board != config.BoardTaskboard || h.state.saved.LastBoard != config.BoardTaskboard {
		t.Fatalf("board %q, saved %q", h.app.board, h.state.saved.LastBoard)
	}
	if !strings.Contains(h.plainView(), "Web Shop › Taskboard › Sprint 4") {
		t.Errorf("view:\n%s", h.plainView())
	}

	h.key("ctrl+k")
	h.typeText("kanban")
	h.key("enter")
	if h.app.board != config.BoardKanban || h.state.saved.LastBoard != config.BoardKanban {
		t.Errorf("board %q, saved %q", h.app.board, h.state.saved.LastBoard)
	}
}

func TestChoosingCurrentEntryOnlyClosesPalette(t *testing.T) {
	h := onTaskboard(t, 100, 24)
	saves := h.state.saves

	h.key(":")
	h.key("enter") // Go to Taskboard

	if h.app.paletteOpen || h.app.screen != screenBoard || h.state.saves != saves {
		t.Errorf("palette %v, screen %v, %d saves", h.app.paletteOpen, h.app.screen, h.state.saves-saves)
	}
}

func TestChoosingSprintOpensItsTaskboard(t *testing.T) {
	h := onKanban(t, 100, 24)

	h.key(":")
	h.typeText("sprint 3")
	h.key("enter")

	if h.app.board != config.BoardTaskboard || shownTaskboard(h).Sprint().ID != sprint3.ID {
		t.Fatalf("board %q, Sprint %+v:\n%s", h.app.board, shownTaskboard(h).Sprint(), h.plainView())
	}
	if h.state.saved.Sprints[webShop.ID] != sprint3.ID || h.state.saved.LastBoard != config.BoardTaskboard {
		t.Errorf("saved %+v", h.state.saved)
	}
	if !strings.Contains(h.plainView(), "Web Shop › Taskboard › Sprint 3") {
		t.Errorf("view:\n%s", h.plainView())
	}
}

func TestPaletteOnKanbanLoadsSprintsWithoutLeavingKanban(t *testing.T) {
	h := onKanban(t, 100, 24)

	h.key(":")

	if h.app.board != config.BoardKanban || !h.app.paletteOpen || !strings.Contains(h.plainView(), "Sprint: Sprint 5") {
		t.Errorf("board %q:\n%s", h.app.board, h.plainView())
	}
}

func TestDisabledBoardIsGreyedOutWithHintAndNotSelectable(t *testing.T) {
	h := resumed(t, newFake(), aliceState(mobileApp.ID))
	h.start()

	h.key(":")
	h.typeText("kanban")
	h.golden("palette_disabled_board")
	h.key("enter")

	if !h.app.paletteOpen || h.app.board != config.BoardTaskboard {
		t.Errorf("palette %v, board %q", h.app.paletteOpen, h.app.board)
	}
	view := h.plainView()
	if !strings.Contains(words(view), "Go to Kanban Board (disabled in this project)") || !strings.Contains(view, "Kanban Board is disabled in mobile app, use the Taskboard") {
		t.Errorf("view:\n%s", view)
	}
}

func TestSwitchingProjectKeepsBoardAndSavesDefaultProject(t *testing.T) {
	h := onTaskboard(t, 100, 24)

	h.key(":")
	h.typeText("mobile")
	h.key("enter")

	if h.app.project.ID != mobileApp.ID || h.app.board != config.BoardTaskboard || h.state.saved.DefaultProject != mobileApp.ID {
		t.Errorf("project %d, board %q, saved %+v", h.app.project.ID, h.app.board, h.state.saved)
	}
	if strings.Contains(h.plainView(), "disabled") {
		t.Errorf("unexpected hint:\n%s", h.plainView())
	}
}

func TestSwitchingProjectFallsBackToOtherBoardWithHint(t *testing.T) {
	h := onKanban(t, 100, 24)

	h.key(":")
	h.typeText("mobile")
	h.key("enter")

	if h.app.board != config.BoardTaskboard || h.state.saved.LastBoard != config.BoardTaskboard || h.state.saved.DefaultProject != mobileApp.ID {
		t.Errorf("board %q, saved %+v", h.app.board, h.state.saved)
	}
	if !strings.Contains(h.plainView(), "Kanban is disabled in mobile app, showing Taskboard") {
		t.Errorf("hint missing:\n%s", h.plainView())
	}
}

func TestProjectWithoutBoardsIsNotSelectable(t *testing.T) {
	h := onKanban(t, 100, 24)

	h.key(":")
	h.typeText("archive")
	h.key("enter")

	if !h.app.paletteOpen || h.app.project.ID != webShop.ID {
		t.Errorf("palette %v, project %d", h.app.paletteOpen, h.app.project.ID)
	}
}

func TestFailedSaveOnBoardSwitchIsShownWithoutQuitting(t *testing.T) {
	h := onKanban(t, 100, 24)
	h.state.err = errors.New("disk full")

	h.key(":")
	h.typeText("taskboard")
	h.key("enter")

	if h.quit || h.app.board != config.BoardTaskboard || !strings.Contains(h.plainView(), "Couldn't save settings: disk full") {
		t.Errorf("quit %v, board %q:\n%s", h.quit, h.app.board, h.plainView())
	}
}

func TestBNoLongerSwitchesBoards(t *testing.T) {
	h := onKanban(t, 100, 24)

	h.key("b")

	if h.app.board != config.BoardKanban {
		t.Errorf("board %q", h.app.board)
	}
}

func TestQuestionMarkExpandsFooterIntoFullHelp(t *testing.T) {
	h := onKanban(t, 100, 24)
	lastLine := func() string {
		lines := strings.Split(h.plainView(), "\n")
		return lines[len(lines)-1]
	}
	if got := lastLine(); !strings.HasSuffix(strings.TrimSpace(got), ": menu · ? help") {
		t.Errorf("footer %q", got)
	}

	h.key("?")
	if !strings.Contains(h.plainView(), "quit") || h.app.paletteOpen {
		t.Errorf("no full help:\n%s", h.plainView())
	}
	h.golden("full_help")
	if lines := strings.Split(h.plainView(), "\n"); len(lines) != 24 {
		t.Errorf("%d lines", len(lines))
	}

	h.key("?")
	if !strings.HasSuffix(strings.TrimSpace(lastLine()), ": menu · ? help") {
		t.Errorf("help still open:\n%s", h.plainView())
	}
}

func TestPaletteFitsSmallestTerminal(t *testing.T) {
	h := onTaskboard(t, 60, 15)
	h.key(":")

	for range 10 {
		h.key("down")
	}

	if e, _ := h.app.palette.chosen(); e.label != "Project: Web Shop ★" {
		t.Errorf("selected %q", e.label)
	}
	h.golden("palette_60x15")
}

func TestFailedSprintLoadForPaletteKeepsKanban(t *testing.T) {
	h := onKanban(t, 100, 24)
	h.taiga.FailNext("Sprints", taiga.ErrTimeout)

	h.key(":")

	view := h.plainView()
	if h.app.screen != screenBoard || !h.app.paletteOpen || !strings.Contains(view, "Couldn't load sprints: "+taiga.ErrTimeout.Error()) {
		t.Errorf("screen %v:\n%s", h.app.screen, view)
	}
}
