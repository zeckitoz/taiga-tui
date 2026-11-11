package ui

import (
	"fmt"
	"maps"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/zeckitoz/taiga-tui/internal/board"
	"github.com/zeckitoz/taiga-tui/internal/config"
	"github.com/zeckitoz/taiga-tui/internal/domain"
	"github.com/zeckitoz/taiga-tui/internal/taiga"
	"github.com/zeckitoz/taiga-tui/internal/taiga/taigafake"
)

// onTaskboard returns a harness showing the Taskboard of the Current Sprint
// of Web Shop at w×h.
func onTaskboard(t *testing.T, w, h int) *harness {
	t.Helper()
	st := aliceState(webShop.ID)
	st.LastBoard = config.BoardTaskboard
	hs := resumed(t, newFake(), st)
	hs.send(tea.WindowSizeMsg{Width: w, Height: h})
	hs.runCmd(hs.app.Init())
	if hs.app.screen != screenBoard || hs.app.board != config.BoardTaskboard {
		t.Fatalf("not on the Taskboard, screen %v:\n%s", hs.app.screen, hs.plainView())
	}
	return hs
}

// shownTaskboard returns the Taskboard shown, or the zero Taskboard.
func shownTaskboard(h *harness) board.Taskboard {
	tb, _ := h.app.sprints.Taskboard()
	return tb
}

// selectedTaskboardRef returns the ref of the selected Taskboard row.
func selectedTaskboardRef(h *harness) int {
	for _, r := range shownTaskboard(h).Rows() {
		if r.Selected {
			return r.Ref
		}
	}
	return -1
}

func TestTaskboardOfCurrentSprintWithDetailRight(t *testing.T) {
	h := onTaskboard(t, 120, 40)

	h.key("j") // #31, my Task with a description

	h.golden("taskboard_detail_right")
}

func TestTaskboardWithDetailBelow(t *testing.T) {
	h := onTaskboard(t, 80, 30)

	h.key("j")

	h.golden("taskboard_detail_below")
}

func TestTaskboardShowsInitialsOnNarrowList(t *testing.T) {
	h := onTaskboard(t, 68, 30)

	view := h.plainView()
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "#31") && !strings.HasSuffix(strings.TrimRight(line, " "), " AE") {
			t.Errorf("not initials: %q", line)
		}
	}
	for i, line := range strings.Split(view, "\n") {
		if w := ansi.StringWidth(line); w > 68 {
			t.Errorf("line %d is %d cells wide: %q", i, w, line)
		}
	}
}

func TestTaskboardColorsOfStatusLabelsMarkerAndCursor(t *testing.T) {
	h := onTaskboard(t, 100, 16)
	h.key("j")

	h.ansiGolden("taskboard")
}

func TestTaskboardUserStoryDetailCountsTasksPerStatus(t *testing.T) {
	h := onTaskboard(t, 120, 40)

	h.key("j")
	h.key("j")
	h.key("j") // #21

	h.golden("taskboard_user_story_detail")
}

func TestTaskboardNavigationCollapsesAndExpands(t *testing.T) {
	h := onTaskboard(t, 120, 40)

	h.key("down") // #31
	h.key("left") // #12
	if got := selectedTaskboardRef(h); got != 12 {
		t.Fatalf("left on a Task selected #%d", got)
	}
	h.key("h") // collapse #12
	if strings.Contains(h.plainView(), "Validate card number") {
		t.Errorf("not collapsed:\n%s", h.plainView())
	}
	h.key("l") // expand
	if !strings.Contains(h.plainView(), "Validate card number") {
		t.Errorf("not expanded:\n%s", h.plainView())
	}
	h.key("enter") // collapse
	if strings.Contains(h.plainView(), "Validate card number") {
		t.Errorf("enter did not collapse:\n%s", h.plainView())
	}
	if !strings.Contains(h.plainView(), "Tasks") {
		t.Error("enter on a User Story closed the detail view")
	}
	h.key("right")
	h.key("k")
	if got := selectedTaskboardRef(h); got != 12 {
		t.Errorf("k at the top selected #%d", got)
	}
}

func TestEnterOnTaskTogglesDetailView(t *testing.T) {
	h := onTaskboard(t, 120, 40)
	h.key("j")

	h.key("enter")
	if h.app.detailOpen || h.detailRight() {
		t.Errorf("detail view still open:\n%s", h.plainView())
	}
	h.key("enter")
	if !strings.Contains(h.plainView(), "Run the Luhn check") {
		t.Errorf("detail view not open again:\n%s", h.plainView())
	}
}

func TestTaskboardLoadsTaskDescriptionOnlyWhenShown(t *testing.T) {
	h := onTaskboard(t, 120, 40)
	if n := h.taiga.Calls("Task"); n != 0 {
		t.Errorf("%d Task loads on a User Story row", n)
	}

	h.key("j")

	if n := h.taiga.Calls("Task"); n != 1 {
		t.Errorf("%d Task loads, want 1", n)
	}
}

func TestTaskboardFailedDescriptionLoadIsShown(t *testing.T) {
	h := onTaskboard(t, 120, 40)
	h.taiga.FailNext("Task", taiga.ErrTimeout)

	h.key("j")

	if !strings.Contains(h.plainView(), "Couldn't load description: timed out") {
		t.Errorf("view:\n%s", h.plainView())
	}
}

func TestTaskboardScrollsToKeepCursorVisible(t *testing.T) {
	api := newFake()
	var tasks []domain.Task
	for i := range 40 {
		tasks = append(tasks, domain.Task{ID: int64(2000 + i), Ref: 2000 + i, Title: fmt.Sprintf("Task %d", i), Status: 201, UserStory: 501, Sprint: 304, TaskboardOrder: int64(i)})
	}
	api.SetTasks(webShop.ID, tasks...)
	h := onTaskboardWith(t, api, 100, 24)

	for range 30 {
		h.key("j")
	}

	view := h.plainView()
	if !strings.Contains(view, "Task 29") || strings.Contains(view, "Task 0 ") {
		t.Errorf("cursor not visible:\n%s", view)
	}
	if lines := strings.Count(view, "\n") + 1; lines != 24 {
		t.Errorf("view has %d lines, want 24", lines)
	}
}

func TestTaskboardRefreshKeepsSelectionAndCollapsing(t *testing.T) {
	h := onTaskboard(t, 120, 40)
	h.key("enter") // collapse #12
	h.key("j")     // #21
	h.key("j")     // #35
	h.taiga.SetTasks(webShop.ID, append([]domain.Task{
		{ID: 899, Ref: 99, Title: "Brand new", Status: 201, UserStory: 504, Sprint: 304},
	}, webShopTasks...)...)

	h.key("r")

	if got := selectedTaskboardRef(h); got != 35 {
		t.Errorf("selected #%d after refresh", got)
	}
	view := h.plainView()
	if !strings.Contains(view, "Brand new") || strings.Contains(view, "Validate card number") {
		t.Errorf("view:\n%s", view)
	}
}

func TestTaskboardFailedRefreshKeepsBoard(t *testing.T) {
	h := onTaskboard(t, 120, 40)
	h.taiga.FailNext("Taskboard", taiga.ErrTimeout)

	h.key("r")

	view := h.plainView()
	if h.app.screen != screenBoard || !strings.Contains(view, "Couldn't refresh: timed out") || !strings.Contains(view, "Order history") {
		t.Errorf("view:\n%s", view)
	}
}

func TestTaskboardLoadFailureShowsErrorPageAndRetries(t *testing.T) {
	api := newFake()
	api.FailNext("Taskboard", &taiga.ServerError{Code: 502})
	st := aliceState(webShop.ID)
	st.LastBoard = config.BoardTaskboard
	h := resumed(t, api, st)
	h.start()

	if view := h.plainView(); !strings.Contains(view, "Couldn't load Taskboard: server error (502)") {
		t.Fatalf("view:\n%s", view)
	}
	h.key("r")

	if h.app.screen != screenBoard || shownTaskboard(h).Empty() {
		t.Errorf("retry failed:\n%s", h.plainView())
	}
}

func TestSprintsLoadFailureShowsErrorPageAndRetries(t *testing.T) {
	api := newFake()
	api.FailNext("Sprints", taiga.ErrUnreachable)
	st := aliceState(webShop.ID)
	st.LastBoard = config.BoardTaskboard
	h := resumed(t, api, st)
	h.start()

	if view := h.plainView(); !strings.Contains(view, "Couldn't load Taskboard: no connection") {
		t.Fatalf("view:\n%s", view)
	}
	h.key("r")

	if h.app.screen != screenBoard || shownTaskboard(h).Sprint().ID != sprint4.ID {
		t.Errorf("retry failed:\n%s", h.plainView())
	}
}

func TestHeaderNamesSprintWithDateRangeAndPhase(t *testing.T) {
	tests := []struct {
		sprint domain.Sprint
		want   string
	}{
		{sprint4, "Web Shop › Taskboard › Sprint 4  03.11. - 16.11.2026  current"},
		{sprint5, "Web Shop › Taskboard › Sprint 5  17.11. - 30.11.2026  upcoming"},
		{sprint3, "Web Shop › Taskboard › Sprint 3  20.10. - 02.11.2026  closed"},
		{
			domain.Sprint{ID: 3, Name: "Hotfix", Start: domain.Date{Year: 2026, Month: 11, Day: 9}, Finish: domain.Date{Year: 2026, Month: 11, Day: 13}, Closed: true},
			"Web Shop › Taskboard › Hotfix  09.11. - 13.11.2026  current · closed",
		},
		{
			domain.Sprint{ID: 1, Name: "Winter", Start: domain.Date{Year: 2026, Month: 12, Day: 21}, Finish: domain.Date{Year: 2027, Month: 1, Day: 8}},
			"Web Shop › Taskboard › Winter  21.12.2026 - 08.01.2027  upcoming",
		},
		{
			domain.Sprint{ID: 2, Name: "Late", Start: domain.Date{Year: 2026, Month: 9, Day: 1}, Finish: domain.Date{Year: 2026, Month: 9, Day: 14}},
			"Web Shop › Taskboard › Late  01.09. - 14.09.2026 ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.sprint.Name, func(t *testing.T) {
			api := newFake()
			api.SetSprints(webShop.ID, tt.sprint, sprint4)
			st := aliceState(webShop.ID).WithSprint(webShop.ID, tt.sprint.ID)
			st.LastBoard = config.BoardTaskboard
			h := resumed(t, api, st)
			h.send(tea.WindowSizeMsg{Width: 120, Height: 20})
			h.runCmd(h.app.Init())
			if tt.sprint.ID != sprint4.ID {
				// Sprint 4 is current, so only the picker opens another one.
				h.key(":")
				h.typeText(tt.sprint.Name)
				h.key("enter")
			}

			if header := strings.Split(h.plainView(), "\n")[0]; !strings.HasPrefix(header, " "+tt.want) {
				t.Errorf("header %q", header)
			}
		})
	}
}

func TestEmptySprintShowsHintAndSwitchesSprint(t *testing.T) {
	api := newFake()
	api.SetStories(webShop.ID)
	api.SetTasks(webShop.ID)
	h := onTaskboardWith(t, api, 100, 24)

	h.golden("taskboard_no_user_stories")

	h.key(":")
	if h.app.screen != screenSprintPicker {
		t.Errorf("':' opened screen %v", h.app.screen)
	}
}

func TestProjectWithoutSprintsShowsHint(t *testing.T) {
	h := resumed(t, newFake(), aliceState(mobileApp.ID))
	h.start()

	h.golden("taskboard_no_sprints")

	h.taiga.SetSprints(mobileApp.ID, sprint4)
	h.key("r")
	if shownTaskboard(h).Sprint().ID != sprint4.ID {
		t.Errorf("refresh did not load the new Sprint:\n%s", h.plainView())
	}
}

func TestColonWithoutSprintsOpensProjectPicker(t *testing.T) {
	h := resumed(t, newFake(), aliceState(mobileApp.ID))
	h.start()

	h.key(":")

	if h.app.screen != screenPicker {
		t.Errorf("screen %v", h.app.screen)
	}
}

func TestSeveralCurrentSprintsOfferPickerWithClosedMarked(t *testing.T) {
	overlap := domain.Sprint{ID: 310, Name: "Hotfix week", Start: domain.Date{Year: 2026, Month: 11, Day: 9}, Finish: domain.Date{Year: 2026, Month: 11, Day: 13}, Closed: true}
	api := newFake()
	api.SetSprints(webShop.ID, sprint5, overlap, sprint4, sprint3)
	st := aliceState(webShop.ID)
	st.LastBoard = config.BoardTaskboard
	h := resumed(t, api, st)
	h.start()

	if h.app.screen != screenSprintPicker {
		t.Fatalf("screen %v:\n%s", h.app.screen, h.plainView())
	}
	h.golden("sprint_picker")

	h.key("down")
	h.key("enter")

	if h.app.screen != screenBoard || shownTaskboard(h).Sprint().ID != sprint4.ID {
		t.Errorf("did not open Sprint 4:\n%s", h.plainView())
	}
	if got := h.state.saved.Sprints; !maps.Equal(got, map[int64]int64{webShop.ID: sprint4.ID}) {
		t.Errorf("remembered %v", got)
	}
}

func TestRememberedSprintIsOpenedWithoutPicker(t *testing.T) {
	overlap := domain.Sprint{ID: 310, Name: "Hotfix week", Start: domain.Date{Year: 2026, Month: 11, Day: 9}, Finish: domain.Date{Year: 2026, Month: 11, Day: 13}}
	api := newFake()
	api.SetSprints(webShop.ID, overlap, sprint4)
	st := aliceState(webShop.ID).WithSprint(webShop.ID, sprint4.ID)
	st.LastBoard = config.BoardTaskboard
	h := resumed(t, api, st)

	h.start()

	if h.app.screen != screenBoard || shownTaskboard(h).Sprint().ID != sprint4.ID {
		t.Errorf("screen %v:\n%s", h.app.screen, h.plainView())
	}
}

func TestNoCurrentSprintOffersOpenSprints(t *testing.T) {
	api := newFake()
	api.SetSprints(webShop.ID, sprint5, sprint3)
	h := onSprintPickerWith(t, api)

	view := h.plainView()
	if !strings.Contains(view, "No sprint contains today") || !strings.Contains(view, "Sprint 5") || strings.Contains(view, "Sprint 3") {
		t.Errorf("view:\n%s", view)
	}
}

func TestNoOpenSprintOffersAllSprintsNewestFirst(t *testing.T) {
	old := sprint3
	old.ID, old.Name = 302, "Sprint 2"
	old.Start, old.Finish = domain.Date{Year: 2026, Month: 10, Day: 6}, domain.Date{Year: 2026, Month: 10, Day: 19}
	api := newFake()
	api.SetSprints(webShop.ID, old, sprint3)
	h := onSprintPickerWith(t, api)

	view := h.plainView()
	i, j := strings.Index(view, "Sprint 3"), strings.Index(view, "Sprint 2")
	if !strings.Contains(view, "No sprint is open") || i < 0 || j < i {
		t.Errorf("view:\n%s", view)
	}
}

func TestUnknownRememberedSprintIsDroppedSilently(t *testing.T) {
	st := aliceState(webShop.ID).WithSprint(webShop.ID, 999).WithSprint(8, 1)
	st.LastBoard = config.BoardTaskboard
	h := resumed(t, newFake(), st)

	h.start()

	if got := h.state.saved.Sprints; !maps.Equal(got, map[int64]int64{8: 1}) {
		t.Errorf("remembered %v", got)
	}
	if h.app.screen != screenBoard || len(h.app.status) != 0 {
		t.Errorf("screen %v, status %v", h.app.screen, h.app.status)
	}
}

func TestColonOnTaskboardSwitchesToAnySprint(t *testing.T) {
	h := onTaskboard(t, 100, 24)

	h.key(":")
	h.golden("sprint_picker_switch")
	h.key("down")
	h.key("down")
	h.key("enter")

	if shownTaskboard(h).Sprint().ID != sprint3.ID {
		t.Errorf("Sprint %+v", shownTaskboard(h).Sprint())
	}
	if got := h.state.saved.Sprints[webShop.ID]; got != sprint3.ID {
		t.Errorf("remembered %d", got)
	}
}

func TestEscInSprintPickerReturnsToTaskboard(t *testing.T) {
	h := onTaskboard(t, 100, 24)
	h.key(":")

	h.key("esc")

	if h.quit || h.app.screen != screenBoard || shownTaskboard(h).Sprint().ID != sprint4.ID {
		t.Errorf("quit %v, screen %v", h.quit, h.app.screen)
	}
}

func TestEscInSprintPickerAtStartQuits(t *testing.T) {
	api := newFake()
	api.SetSprints(webShop.ID, sprint5, sprint3)
	h := onSprintPickerWith(t, api)

	h.key("esc")

	if !h.quit {
		t.Error("esc did not quit")
	}
}

func TestBSwitchesBoardsAndRemembersLastBoard(t *testing.T) {
	h := onKanban(t, 100, 24)

	h.key("b")
	if h.app.board != config.BoardTaskboard || h.state.saved.LastBoard != config.BoardTaskboard {
		t.Fatalf("board %q, saved %q", h.app.board, h.state.saved.LastBoard)
	}
	if !strings.Contains(h.plainView(), "Web Shop › Taskboard › Sprint 4") {
		t.Errorf("view:\n%s", h.plainView())
	}

	h.key("b")
	if h.app.board != config.BoardKanban || h.state.saved.LastBoard != config.BoardKanban {
		t.Errorf("board %q, saved %q", h.app.board, h.state.saved.LastBoard)
	}
}

func TestBDoesNothingWhenOtherBoardIsDisabled(t *testing.T) {
	h := resumed(t, newFake(), aliceState(mobileApp.ID))
	h.start()

	h.key("b")

	if h.app.board != config.BoardTaskboard {
		t.Errorf("board %q", h.app.board)
	}
}

func TestStartOpensLastUsedBoardIfEnabled(t *testing.T) {
	tests := []struct {
		name    string
		project domain.Project
		last    config.Board
		want    config.Board
	}{
		{"Kanban by default", webShop, "", config.BoardKanban},
		{"last Taskboard", webShop, config.BoardTaskboard, config.BoardTaskboard},
		{"Kanban disabled", mobileApp, config.BoardKanban, config.BoardTaskboard},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := aliceState(tt.project.ID)
			st.LastBoard = tt.last
			h := resumed(t, newFake(), st)

			h.start()

			if h.app.board != tt.want || h.app.state.LastBoard != tt.want {
				t.Errorf("board %q, last board %q, want %q", h.app.board, h.app.state.LastBoard, tt.want)
			}
			if changed := tt.last != tt.want; changed != (h.state.saves > 0) || changed && h.state.saved.LastBoard != tt.want {
				t.Errorf("%d saves of %q for a change from %q", h.state.saves, h.state.saved.LastBoard, tt.last)
			}
		})
	}
}

func TestPickedProjectOpensKanbanIfEnabledOtherwiseTaskboard(t *testing.T) {
	st := aliceState(0)
	st.LastBoard = config.BoardTaskboard
	h := resumed(t, newFake(), st)
	h.start()

	h.typeText("web")
	h.key("enter")

	if h.app.board != config.BoardKanban {
		t.Errorf("board %q", h.app.board)
	}
}

// onTaskboardWith returns a harness showing the Taskboard of Web Shop on api.
func onTaskboardWith(t *testing.T, api *taigafake.Fake, w, h int) *harness {
	t.Helper()
	st := aliceState(webShop.ID)
	st.LastBoard = config.BoardTaskboard
	hs := resumed(t, api, st)
	hs.send(tea.WindowSizeMsg{Width: w, Height: h})
	hs.runCmd(hs.app.Init())
	if hs.app.screen != screenBoard {
		t.Fatalf("not on the board, screen %v:\n%s", hs.app.screen, hs.plainView())
	}
	return hs
}

// onSprintPickerWith returns a harness showing the Sprint picker of Web
// Shop at start.
func onSprintPickerWith(t *testing.T, api *taigafake.Fake) *harness {
	t.Helper()
	st := aliceState(webShop.ID)
	st.LastBoard = config.BoardTaskboard
	h := resumed(t, api, st)
	h.start()
	if h.app.screen != screenSprintPicker {
		t.Fatalf("screen %v:\n%s", h.app.screen, h.plainView())
	}
	return h
}

func TestSprintSwitcherStartsOnSprintShown(t *testing.T) {
	h := onTaskboard(t, 100, 24)

	h.key(":")

	if id, _ := h.app.sprintPicker.chosen(); id != sprint4.ID {
		t.Errorf("selected Sprint %d", id)
	}
}

func TestNarrowHeaderLeavesOutDateRangeAndKeepsName(t *testing.T) {
	h := onTaskboard(t, 60, 15)

	header := strings.Split(h.plainView(), "\n")[0]
	if want := " Web Shop › Taskboard › Sprint 4  current"; !strings.HasPrefix(header, want) || !strings.HasSuffix(header, "Alice Example ") {
		t.Errorf("header %q", header)
	}
}

func TestNoSprintsFooterOffersKanbanWhenEnabled(t *testing.T) {
	api := newFake()
	api.SetSprints(webShop.ID)

	h := onTaskboardWith(t, api, 100, 24)

	lines := strings.Split(h.plainView(), "\n")
	if foot := lines[len(lines)-1]; !strings.Contains(foot, "b kanban") {
		t.Errorf("footer %q", foot)
	}
}
