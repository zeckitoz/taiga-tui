package board_test

import (
	"reflect"
	"testing"

	"github.com/zeckitoz/taiga-tui/internal/board"
	"github.com/zeckitoz/taiga-tui/internal/domain"
)

// projectWithNewIDs is the project after an admin added User Story
// Status 5 and Task Status 24 and added member 45.
func projectWithNewIDs() domain.Project {
	p := scrum
	p.UserStoryStatuses = append(append([]domain.Status{}, p.UserStoryStatuses...), domain.Status{ID: 5, Name: "Review"})
	p.TaskStatuses = append(append([]domain.Status{}, p.TaskStatuses...), domain.Status{ID: 24, Name: "Blocked"})
	p.Members = append(append([]domain.TeamMember{}, p.Members...), domain.TeamMember{ID: 45, FullName: "Dora Neu"})
	return p
}

var newStatusStories = []domain.UserStory{
	{ID: 10, Ref: 1, Title: "Known", Status: 1, KanbanOrder: 1},
	{ID: 14, Ref: 5, Title: "Reviewed", Status: 5, KanbanOrder: 2, Assignees: []int64{45}},
}

func loadedKanban(t *testing.T, p domain.Project, stories []domain.UserStory) board.Kanban {
	t.Helper()
	k, load := board.UnloadedKanban(p, me).LoadUserStories()
	k, ok := k.UserStoriesLoaded(load, stories)
	if !ok {
		t.Fatal("load dropped")
	}
	return k
}

func TestKanbanKnownIDsNeedNoProjectReload(t *testing.T) {
	k := loadedKanban(t, project, stories[:3])

	if _, eff := k.ReloadProjectOnUnknown(); eff != nil {
		t.Errorf("effect %v for known IDs", eff)
	}
}

func TestKanbanUnknownStatusReloadsProjectOncePerLoad(t *testing.T) {
	k := loadedKanban(t, project, newStatusStories)

	k, eff := k.ReloadProjectOnUnknown()
	if eff != (board.ReloadProject{}) {
		t.Fatalf("effect %v, want ReloadProject", eff)
	}
	if _, again := k.ReloadProjectOnUnknown(); again != nil {
		t.Errorf("second reload within the same load: %v", again)
	}

	k, load := k.LoadUserStories()
	k, _ = k.UserStoriesLoaded(load, newStatusStories)
	if _, eff := k.ReloadProjectOnUnknown(); eff == nil {
		t.Error("a new load did not allow a new reload")
	}
}

func TestKanbanUnknownMemberReloadsProject(t *testing.T) {
	k := loadedKanban(t, project, []domain.UserStory{{ID: 10, Status: 1, Assignees: []int64{45}}})

	if _, eff := k.ReloadProjectOnUnknown(); eff != (board.ReloadProject{}) {
		t.Errorf("effect %v, want ReloadProject", eff)
	}
}

func TestKanbanProjectReloadedResolvesUnknownStatus(t *testing.T) {
	k := loadedKanban(t, project, newStatusStories)
	k, _ = k.ReloadProjectOnUnknown()

	k = k.ProjectReloaded(projectWithNewIDs())

	names := []string{}
	for _, tab := range k.Tabs() {
		names = append(names, tab.Name)
	}
	if want := []string{"New", "In progress", "Done", "Review"}; !reflect.DeepEqual(names, want) {
		t.Errorf("tabs %v, want %v", names, want)
	}
	if _, eff := k.ReloadProjectOnUnknown(); eff != nil {
		t.Errorf("reloaded again after the project knows the IDs: %v", eff)
	}
}

func TestKanbanStillUnknownStatusGetsUnknownTab(t *testing.T) {
	k := loadedKanban(t, project, newStatusStories)
	k, _ = k.ReloadProjectOnUnknown()

	k = k.ProjectReloaded(project) // the admin's change is still not there

	tabs := k.Tabs()
	last := tabs[len(tabs)-1]
	if last.Name != "Unknown" || last.Count != 1 || len(tabs) != 4 {
		t.Fatalf("tabs %+v", tabs)
	}
	if _, eff := k.ReloadProjectOnUnknown(); eff != nil {
		t.Errorf("a second reload for the same load: %v", eff)
	}
	k = k.PrevTab() // wraps to the Unknown tab
	rows := k.Rows()
	if len(rows) != 1 || rows[0].ID != 14 || k.ActiveStatus().Name != "Unknown" {
		t.Errorf("rows %+v, active %+v", rows, k.ActiveStatus())
	}
	if d, ok := k.Detail(); !ok || d.Status.ID != 0 {
		t.Errorf("detail %+v, want the zero Status", d)
	}
}

func TestKanbanUnknownTabIsAbsentWithKnownStatuses(t *testing.T) {
	k := loadedKanban(t, project, stories[:3])
	for _, tab := range k.Tabs() {
		if tab.Name == "Unknown" {
			t.Errorf("unexpected Unknown tab in %+v", k.Tabs())
		}
	}
}

func TestKanbanArchivedStatusIsNotUnknown(t *testing.T) {
	k := loadedKanban(t, project, []domain.UserStory{{ID: 10, Status: 3}})

	if _, eff := k.ReloadProjectOnUnknown(); eff != nil {
		t.Errorf("effect %v for an archived Status", eff)
	}
}

func taskboardWith(t *testing.T, p domain.Project, c domain.SprintContent) board.Sprints {
	t.Helper()
	s, load := board.NewSprints(p, me).Open(sprint)
	s, ok := s.TaskboardLoaded(load, c)
	if !ok {
		t.Fatal("load dropped")
	}
	return s
}

func TestTaskboardUnknownTaskStatusReloadsProjectOncePerLoad(t *testing.T) {
	c := domain.SprintContent{Tasks: []domain.Task{{ID: 1, Status: 24}}}
	s := taskboardWith(t, scrum, c)

	s, eff := s.ReloadProjectOnUnknown()
	if eff != (board.ReloadProject{}) {
		t.Fatalf("effect %v, want ReloadProject", eff)
	}
	if _, again := s.ReloadProjectOnUnknown(); again != nil {
		t.Errorf("second reload in the same load: %v", again)
	}

	s = s.ProjectReloaded(projectWithNewIDs())
	if tb, _ := s.Taskboard(); tb.Rows()[0].Kind != board.RowUserStory || tb.Rows()[1].Status.Name != "Blocked" {
		t.Errorf("rows %+v", tb.Rows())
	}
}

func TestTaskboardKnownIDsNeedNoProjectReload(t *testing.T) {
	s := taskboardWith(t, scrum, domain.SprintContent{
		UserStories: []domain.UserStory{{ID: 501, Status: 2, Assignees: []int64{43}}},
		Tasks:       []domain.Task{{ID: 1, Status: 22, UserStory: 501, Assignee: 42}},
	})

	if _, eff := s.ReloadProjectOnUnknown(); eff != nil {
		t.Errorf("effect %v", eff)
	}
}

func TestTaskboardUnknownMemberAndUserStoryStatusReloadProject(t *testing.T) {
	for name, c := range map[string]domain.SprintContent{
		"task assignee":  {Tasks: []domain.Task{{ID: 1, Status: 22, Assignee: 99}}},
		"story assignee": {UserStories: []domain.UserStory{{ID: 5, Status: 1, Assignees: []int64{99}}}},
		"story status":   {UserStories: []domain.UserStory{{ID: 5, Status: 99}}},
	} {
		s := taskboardWith(t, scrum, c)
		if _, eff := s.ReloadProjectOnUnknown(); eff != (board.ReloadProject{}) {
			t.Errorf("%s: effect %v", name, eff)
		}
	}
}
