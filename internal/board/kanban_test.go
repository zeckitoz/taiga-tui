package board_test

import (
	"reflect"
	"testing"

	"github.com/zeckitoz/taiga-tui/internal/board"
	"github.com/zeckitoz/taiga-tui/internal/domain"
)

const me = 42

var project = domain.Project{
	ProjectSummary: domain.ProjectSummary{ID: 7, Name: "Web Shop", KanbanEnabled: true},
	UserStoryStatuses: []domain.Status{
		{ID: 1, Name: "New", Color: "#70728F"},
		{ID: 2, Name: "In progress", Color: "#E47C40", WIPLimit: 3},
		{ID: 3, Name: "Old", Color: "#5C3566", Archived: true},
		{ID: 4, Name: "Done", Color: "#A8E440", Closed: true},
	},
	Members: []domain.TeamMember{
		{ID: 42, Username: "alice", FullName: "Alice Example"},
		{ID: 43, Username: "bkoch", FullName: "bkoch"},
		{ID: 44, Username: "cm", FullName: "Carla de la Mar"},
	},
}

var stories = []domain.UserStory{
	{ID: 10, Ref: 1, Title: "Third", Status: 1, KanbanOrder: 30},
	{ID: 11, Ref: 2, Title: "First", Status: 1, KanbanOrder: 10, Assignees: []int64{43, 42}, Tasks: domain.TaskProgress{Done: 1, Total: 2}},
	{ID: 12, Ref: 3, Title: "Second", Status: 1, KanbanOrder: 20, Assignees: []int64{44}},
	{ID: 13, Ref: 4, Title: "Working", Status: 2, KanbanOrder: 5, Assignees: []int64{99}},
}

func TestTabsFollowTaigaOrderWithoutArchivedStatuses(t *testing.T) {
	k := board.NewKanban(project, stories, me)

	want := []board.Tab{
		{Name: "New", Color: "#70728F", Count: 3, Active: true},
		{Name: "In progress", Color: "#E47C40", Count: 1, WIPLimit: 3},
		{Name: "Done", Color: "#A8E440", Count: 0},
	}
	if got := k.Tabs(); !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestRowsOfActiveTabAreSortedByKanbanOrder(t *testing.T) {
	k := board.NewKanban(project, stories, me)

	want := []board.Row{
		{
			ID: 11, Ref: 2, Title: "First", Mine: true, Selected: true,
			Assignees: []board.Assignee{{Initials: "BK"}, {Initials: "AE", Mine: true}},
			Tasks:     domain.TaskProgress{Done: 1, Total: 2},
		},
		{ID: 12, Ref: 3, Title: "Second", Assignees: []board.Assignee{{Initials: "CM"}}},
		{ID: 10, Ref: 1, Title: "Third"},
	}
	if got := k.Rows(); !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestUnknownMemberHasPlaceholderInitials(t *testing.T) {
	k := board.NewKanban(project, stories, me).NextTab()

	rows := k.Rows()
	if len(rows) != 1 || !reflect.DeepEqual(rows[0].Assignees, []board.Assignee{{Initials: "??"}}) {
		t.Errorf("got %+v", rows)
	}
}

func TestCursorMovesWithinTabAndStopsAtEdges(t *testing.T) {
	k := board.NewKanban(project, stories, me)

	k = k.CursorUp()
	if got := selected(k); got != 11 {
		t.Errorf("up at top: selected %d", got)
	}
	k = k.CursorDown().CursorDown().CursorDown()
	if got := selected(k); got != 10 {
		t.Errorf("down past bottom: selected %d", got)
	}
}

func TestTabsWrapAroundAndRememberCursor(t *testing.T) {
	k := board.NewKanban(project, stories, me).CursorDown()

	k = k.PrevTab()
	if got := k.ActiveStatus().Name; got != "Done" {
		t.Errorf("prev from first tab: %q", got)
	}
	k = k.NextTab()
	if got := k.ActiveStatus().Name; got != "New" {
		t.Errorf("next from last tab: %q", got)
	}
	if got := selected(k); got != 12 {
		t.Errorf("cursor not remembered: selected %d", got)
	}
}

func TestEmptyTabAndEmptyBoard(t *testing.T) {
	k := board.NewKanban(project, stories, me).PrevTab()
	if len(k.Rows()) != 0 || k.Empty() {
		t.Errorf("Done tab: rows %v, empty board %v", k.Rows(), k.Empty())
	}
	if rows := k.CursorDown().Rows(); len(rows) != 0 {
		t.Errorf("cursor on an empty tab: %v", rows)
	}

	if !board.NewKanban(project, nil, me).Empty() {
		t.Error("board without User Stories is not empty")
	}
}

func TestBoardWithoutStatusesHasNoTabs(t *testing.T) {
	p := project
	p.UserStoryStatuses = nil

	k := board.NewKanban(p, nil, me).NextTab().CursorDown()

	if len(k.Tabs()) != 0 || len(k.Rows()) != 0 || !k.Empty() {
		t.Errorf("tabs %v rows %v", k.Tabs(), k.Rows())
	}
}

func TestReloadKeepsTabAndSelectedUserStory(t *testing.T) {
	k := board.NewKanban(project, stories, me).CursorDown()
	moved := append([]domain.UserStory{{ID: 14, Ref: 5, Title: "New first", Status: 1, KanbanOrder: 1}}, stories...)

	k = k.Reload(moved)

	if got := selected(k); got != 12 {
		t.Errorf("selected %d after reload, want 12", got)
	}
	if got := k.Tabs()[0].Count; got != 4 {
		t.Errorf("count %d", got)
	}
}

func TestReloadSelectsFirstRowWhenSelectedUserStoryVanished(t *testing.T) {
	k := board.NewKanban(project, stories, me).CursorDown()

	k = k.Reload(stories[:2])

	if got := selected(k); got != 11 {
		t.Errorf("selected %d", got)
	}
}

func selected(k board.Kanban) int64 {
	for _, r := range k.Rows() {
		if r.Selected {
			return r.ID
		}
	}
	return 0
}

func TestSelectedIndexMatchesSelectedRow(t *testing.T) {
	k := board.NewKanban(project, stories, me).CursorDown()

	if got := k.SelectedIndex(); got != 1 || !k.Rows()[got].Selected {
		t.Errorf("SelectedIndex %d", got)
	}
	if got := k.PrevTab().SelectedIndex(); got != -1 {
		t.Errorf("empty tab: %d", got)
	}
}
