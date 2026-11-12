package board_test

import (
	"slices"
	"testing"

	"github.com/zeckitoz/taiga-tui/internal/board"
	"github.com/zeckitoz/taiga-tui/internal/domain"
)

var (
	sprint2 = domain.Sprint{ID: 2, Name: "Sprint 2"}
	sprint3 = domain.Sprint{ID: 3, Name: "Sprint 3"}
)

func TestSprintsLoadIsFirstAndThenRefresh(t *testing.T) {
	s := board.NewSprints(scrum, me)
	if s.Listed() {
		t.Fatal("listed before loading")
	}

	s, load := s.LoadSprints()
	if load.Refresh || load.ProjectID != scrum.ID {
		t.Errorf("first load %+v", load)
	}
	s, ok := s.SprintsLoaded(load, []domain.Sprint{sprint3, sprint2})
	if !ok || !s.Listed() || !slices.Equal(s.List(), []domain.Sprint{sprint3, sprint2}) {
		t.Fatalf("loaded %v, listed %v, list %v", ok, s.Listed(), s.List())
	}

	if _, again := s.LoadSprints(); !again.Refresh {
		t.Error("second load is no refresh")
	}
}

func TestSprintsDropsResultsOfEarlierLoadsAndOtherProjects(t *testing.T) {
	s := board.NewSprints(scrum, me)
	s, first := s.LoadSprints()
	s, second := s.LoadSprints()

	if _, ok := s.SprintsLoaded(first, []domain.Sprint{sprint2}); ok {
		t.Error("took the result of an earlier load")
	}
	if s.SprintsCurrent(first) || !s.SprintsCurrent(second) {
		t.Error("SprintsCurrent wrong")
	}
	other := second
	other.ProjectID = 99
	if _, ok := s.SprintsLoaded(other, []domain.Sprint{sprint2}); ok {
		t.Error("took the result of another project")
	}
	if _, ok := (board.Sprints{}).SprintsLoaded(second, nil); ok {
		t.Error("the zero Sprints took a result")
	}
}

func TestSprintsKnows(t *testing.T) {
	s := board.NewSprints(scrum, me)
	s, load := s.LoadSprints()
	s, _ = s.SprintsLoaded(load, []domain.Sprint{sprint3})

	if !s.Knows(3) || s.Knows(2) {
		t.Error("Knows wrong")
	}
}

func TestOpenSprintLoadsItsTaskboard(t *testing.T) {
	s := board.NewSprints(scrum, me)

	s, load := s.Open(sprint)

	if load.Refresh || load.Sprint != sprint || load.ProjectID != scrum.ID {
		t.Errorf("load %+v", load)
	}
	if _, ok := s.Taskboard(); ok {
		t.Error("Taskboard before it loaded")
	}
	s, ok := s.TaskboardLoaded(load, sprintContent)
	tb, has := s.Taskboard()
	if !ok || !has || tb.Sprint() != sprint || !slices.Equal(refs(tb), refs(newTaskboard())) {
		t.Errorf("loaded %v, has %v, rows %v", ok, has, refs(tb))
	}
}

func TestOpeningSameSprintAgainRefreshesAndKeepsTaskboardState(t *testing.T) {
	s := board.NewSprints(scrum, me)
	s, load := s.Open(sprint)
	s, _ = s.TaskboardLoaded(load, sprintContent)
	tb, _ := s.Taskboard()
	s = s.WithTaskboard(press(tb, toggle, down)) // #15 collapsed, #12 selected

	s, refresh := s.Open(sprint)
	if !refresh.Refresh {
		t.Fatal("no refresh")
	}
	s, _ = s.TaskboardLoaded(refresh, sprintContent)

	tb, _ = s.Taskboard()
	if got := refs(tb); got[0] != "US#15" || got[1] != "US#12*" {
		t.Errorf("state lost: %v", got)
	}
}

func TestOpeningOtherSprintReplacesTaskboard(t *testing.T) {
	s := board.NewSprints(scrum, me)
	s, load := s.Open(sprint)
	s, _ = s.TaskboardLoaded(load, sprintContent)
	tb, _ := s.Taskboard()
	s = s.WithTaskboard(press(tb, toggle))

	s, other := s.Open(sprint2)
	if other.Refresh {
		t.Error("another Sprint is no refresh")
	}
	if tb, _ := s.Taskboard(); tb.Sprint() != sprint {
		t.Error("the shown Taskboard changed before the other one loaded")
	}
	s, _ = s.TaskboardLoaded(other, sprintContent)

	tb, _ = s.Taskboard()
	if tb.Sprint() != sprint2 || refs(tb)[1] != "T#33" {
		t.Errorf("not a new Taskboard: %v, %v", tb.Sprint(), refs(tb))
	}
}

func TestTaskboardResultOfEarlierOpenIsDropped(t *testing.T) {
	s := board.NewSprints(scrum, me)
	s, first := s.Open(sprint)
	s, second := s.Open(sprint2)

	if _, ok := s.TaskboardLoaded(first, sprintContent); ok {
		t.Error("took the result of an earlier load")
	}
	if s.TaskboardCurrent(first) || !s.TaskboardCurrent(second) {
		t.Error("TaskboardCurrent wrong")
	}
}

func TestTaskResultOfAnotherSprintIsDropped(t *testing.T) {
	tb := press(newTaskboard(), down)
	tb, load, _ := tb.LoadDetail()
	other := board.NewTaskboard(scrum, sprint2, sprintContent, me)
	other = press(other, down)
	other, _, _ = other.LoadDetail()

	other = other.TaskLoaded(load, domain.Task{ID: 903, Description: "wrong Sprint"})

	if d, _ := other.Detail(); d.Description.State == board.DescriptionLoaded {
		t.Errorf("took a result for Sprint %d: %+v", load.Sprint, d.Description)
	}
	tb = tb.TaskLoaded(load, domain.Task{ID: 903, Description: "ok"})
	if d, _ := tb.Detail(); d.Description.Text != "ok" {
		t.Errorf("dropped the result for its own Sprint: %+v", d.Description)
	}
}

func TestUserStoryResultOfAnotherProjectIsDropped(t *testing.T) {
	k := board.NewKanban(project, stories, me)
	k, load, _ := k.LoadDetail()
	other := project
	other.ID = 99
	o := board.NewKanban(other, stories, me)
	o, _, _ = o.LoadDetail()

	o = o.UserStoryLoaded(load, domain.UserStory{ID: load.ID, Description: "wrong project"})

	if d, _ := o.Detail(); d.Description.State == board.DescriptionLoaded {
		t.Errorf("took a result for project %d: %+v", load.ProjectID, d.Description)
	}
	k = k.UserStoryLoaded(load, domain.UserStory{ID: load.ID, Description: "ok"})
	if d, _ := k.Detail(); d.Description.Text != "ok" {
		t.Errorf("dropped the result for its own project: %+v", d.Description)
	}
}
