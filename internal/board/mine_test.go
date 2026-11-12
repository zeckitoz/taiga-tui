package board_test

import (
	"slices"
	"testing"

	"github.com/zeckitoz/taiga-tui/internal/board"
	"github.com/zeckitoz/taiga-tui/internal/domain"
)

var mine = board.Taskboard.ToggleMine

func TestTaskboardMineHidesOthersAndKeepsUserStoriesAssignedToMe(t *testing.T) {
	// #15 is mine but its only Task is not, #12 has a Task of mine, #20 and
	// the Tasks without a User Story are not mine.
	want := []string{"US#15*", "US#12", "T#31"}

	tb := press(newTaskboard(), mine)

	if got := refs(tb); !slices.Equal(got, want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
	if !tb.Mine() || tb.Empty() {
		t.Errorf("mine %v, empty %v", tb.Mine(), tb.Empty())
	}
}

func TestTaskboardMineKeepsProgressOfAllTasks(t *testing.T) {
	rows := press(newTaskboard(), mine).Rows()

	if got, want := rows[1].Tasks, (domain.TaskProgress{Done: 1, Total: 2}); got != want {
		t.Errorf("progress of #12 %+v, want %+v", got, want)
	}
}

func TestTaskboardMineDoesNotSayNoTasksOfUserStoryWhoseTasksAreHidden(t *testing.T) {
	for _, r := range press(newTaskboard(), mine).Rows() {
		if r.Kind == board.RowNoTasks {
			t.Error("No tasks shown under a User Story that has Tasks")
		}
	}
}

func TestTaskboardMineToggledBackShowsEverything(t *testing.T) {
	tb := press(newTaskboard(), mine, mine)

	if got := refs(tb); len(got) != 10 || tb.Mine() {
		t.Errorf("got %v, mine %v", got, tb.Mine())
	}
}

func TestTaskboardMineMovesCursorToNextVisibleRow(t *testing.T) {
	tb := press(newTaskboard(), down, down, down) // #32, someone else's

	tb = tb.ToggleMine()

	if got := refs(tb); !slices.Equal(got, []string{"US#15", "US#12", "T#31*"}) {
		t.Errorf("got %v", got)
	}
}

func TestTaskboardMineMovesCursorToPreviousRowWhenNoneFollows(t *testing.T) {
	tb := newTaskboard()
	for range 9 {
		tb = tb.CursorDown() // #40, the last row
	}

	tb = tb.ToggleMine()

	if got := refs(tb); !slices.Equal(got, []string{"US#15", "US#12", "T#31*"}) {
		t.Errorf("got %v", got)
	}
}

func TestTaskboardMineLeavesNothingWhenNothingIsMine(t *testing.T) {
	c := domain.SprintContent{UserStories: sprintContent.UserStories[1:2], Tasks: sprintContent.Tasks[1:2]}
	tb := board.NewTaskboard(scrum, sprint, c, me).ToggleMine()

	if !tb.Empty() || !tb.Mine() || tb.SelectedIndex() != -1 {
		t.Errorf("empty %v, mine %v, selected %d", tb.Empty(), tb.Mine(), tb.SelectedIndex())
	}
	if tb = tb.ToggleMine(); tb.Empty() || tb.SelectedIndex() < 0 {
		t.Errorf("after showing all: empty %v, selected %d", tb.Empty(), tb.SelectedIndex())
	}
}

func TestTaskboardMineSurvivesReload(t *testing.T) {
	tb := press(newTaskboard(), mine, down, down).Reload(sprintContent)

	if got := refs(tb); !slices.Equal(got, []string{"US#15", "US#12", "T#31*"}) || !tb.Mine() {
		t.Errorf("after reload %v", got)
	}
}

func TestKanbanMineKeepsAllTabsWithFilteredCounts(t *testing.T) {
	k := board.NewKanban(project, stories, me).ToggleMine()

	var counts []int
	for _, tab := range k.Tabs() {
		counts = append(counts, tab.Count)
	}
	if !slices.Equal(counts, []int{1, 0, 0}) || len(k.Rows()) != 1 || !k.Mine() {
		t.Errorf("counts %v, rows %v", counts, k.Rows())
	}
}

func TestKanbanMineMovesCursorToNextVisibleRow(t *testing.T) {
	k := board.NewKanban(project, stories, me).CursorDown() // #12 "Second", not mine

	k = k.ToggleMine()

	if got := selected(k); got != 11 {
		t.Errorf("selected %d", got)
	}
}

func TestKanbanMineMakesBoardEmptyWhenNothingIsMine(t *testing.T) {
	k := board.NewKanban(project, stories[2:], me)
	if k.Empty() {
		t.Fatal("empty before the filter")
	}

	k = k.ToggleMine()

	if !k.Empty() || !k.Mine() || len(k.Tabs()) != 3 {
		t.Errorf("empty %v, tabs %v", k.Empty(), k.Tabs())
	}
	if k = k.ToggleMine(); k.Empty() {
		t.Error("empty after showing all")
	}
}

func TestKanbanReloadFollowsUserStoryMovedToAnotherStatus(t *testing.T) {
	k := board.NewKanban(project, stories, me).CursorDown() // #12
	moved := slices.Clone(stories)
	moved[2].Status = 2 // #12 is now in progress

	k = k.Reload(moved)

	if got := k.ActiveStatus().ID; got != 2 {
		t.Fatalf("active Status %d", got)
	}
	if got := selected(k); got != 12 {
		t.Errorf("selected %d", got)
	}
}

func TestKanbanRefreshingFromLoadToResult(t *testing.T) {
	k := board.NewKanban(project, stories, me)
	if k.Refreshing() {
		t.Fatal("refreshing at rest")
	}

	k, load := k.LoadUserStories()
	if !k.Refreshing() {
		t.Error("not refreshing while the load runs")
	}
	k, ok := k.UserStoriesLoaded(load, stories)
	if !ok || k.Refreshing() {
		t.Errorf("ok %v, refreshing %v after the result", ok, k.Refreshing())
	}

	k, load = k.LoadUserStories()
	if k, ok = k.UserStoriesFailed(load); !ok || k.Refreshing() {
		t.Errorf("ok %v, refreshing %v after the failure", ok, k.Refreshing())
	}
}

func TestKanbanFirstLoadIsNotARefresh(t *testing.T) {
	k, _ := board.UnloadedKanban(project, me).LoadUserStories()

	if k.Refreshing() {
		t.Error("the first load is shown on the loading screen")
	}
}

func TestKanbanCanceledLoadsDropTheirResults(t *testing.T) {
	k, load := board.NewKanban(project, stories, me).LoadUserStories()

	k = k.CancelLoads()

	if k.Refreshing() {
		t.Error("refreshing after cancel")
	}
	if _, ok := k.UserStoriesLoaded(load, nil); ok {
		t.Error("took the result of a canceled load")
	}
	if _, ok := k.UserStoriesFailed(load); ok {
		t.Error("took the failure of a canceled load")
	}
}

func TestStaleKanbanLoadIsDroppedAfterNewerOne(t *testing.T) {
	k := board.NewKanban(project, stories, me)
	k, first := k.LoadUserStories()
	k, second := k.LoadUserStories()

	if _, ok := k.UserStoriesLoaded(first, nil); ok {
		t.Error("took the result of the stale load")
	}
	if k, ok := k.UserStoriesLoaded(second, stories); !ok || len(k.Rows()) != 3 {
		t.Errorf("ok %v", ok)
	}
}

func TestSprintsRefreshingFromLoadToResult(t *testing.T) {
	s := board.NewSprints(scrum, me)
	s, load := s.LoadSprints()
	if s.Refreshing() {
		t.Fatal("first load of the Sprints is not a refresh")
	}
	s, _ = s.SprintsLoaded(load, []domain.Sprint{sprint})
	s, _ = s.TaskboardLoaded(openTaskboard(&s), sprintContent)

	s, sprintsLoad := s.LoadSprints()
	s, taskboardLoad := s.Open(sprint)
	if !s.Refreshing() {
		t.Fatal("not refreshing while both loads run")
	}
	s, _ = s.SprintsLoaded(sprintsLoad, []domain.Sprint{sprint})
	if !s.Refreshing() {
		t.Error("done while the Taskboard still loads")
	}
	s, _ = s.TaskboardLoaded(taskboardLoad, sprintContent)
	if s.Refreshing() {
		t.Error("still refreshing after both results")
	}
}

func openTaskboard(s *board.Sprints) board.LoadTaskboard {
	var load board.LoadTaskboard
	*s, load = s.Open(sprint)
	return load
}

func TestSprintsFailuresEndTheRefresh(t *testing.T) {
	s := board.NewSprints(scrum, me)
	s, _ = s.TaskboardLoaded(openTaskboard(&s), sprintContent)
	s, sl := s.LoadSprints()
	s, _ = s.SprintsLoaded(sl, []domain.Sprint{sprint})
	s, sl = s.LoadSprints()
	s, tl := s.Open(sprint)

	s, ok1 := s.SprintsFailed(sl)
	s, ok2 := s.TaskboardFailed(tl)

	if !ok1 || !ok2 || s.Refreshing() {
		t.Errorf("sprints %v, taskboard %v, refreshing %v", ok1, ok2, s.Refreshing())
	}
}

func TestSprintsCanceledLoadsDropTheirResults(t *testing.T) {
	s := board.NewSprints(scrum, me)
	s, _ = s.TaskboardLoaded(openTaskboard(&s), sprintContent)
	s, sl := s.LoadSprints()
	s, _ = s.SprintsLoaded(sl, []domain.Sprint{sprint})
	s, sl = s.LoadSprints()
	s, tl := s.Open(sprint)

	s = s.CancelLoads()

	if s.Refreshing() {
		t.Error("refreshing after cancel")
	}
	if _, ok := s.SprintsLoaded(sl, nil); ok {
		t.Error("took Sprints of a canceled load")
	}
	if _, ok := s.TaskboardLoaded(tl, domain.SprintContent{}); ok {
		t.Error("took the Taskboard of a canceled load")
	}
}

func TestOpeningAnotherSprintDropsTheEarlierLoad(t *testing.T) {
	s := board.NewSprints(scrum, me)
	s, first := s.Open(sprint)
	s, second := s.Open(domain.Sprint{ID: 6, Name: "Sprint 5"})

	if _, ok := s.TaskboardLoaded(first, sprintContent); ok {
		t.Error("took the result of the earlier Sprint")
	}
	if _, ok := s.TaskboardLoaded(second, sprintContent); !ok {
		t.Error("dropped the result of the latest Sprint")
	}
}
