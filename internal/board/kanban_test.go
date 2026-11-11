package board_test

import (
	"errors"
	"reflect"
	"slices"
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
			Assignees: []board.Assignee{{Name: "bkoch", Initials: "BK"}, {Name: "Alice Example", Initials: "AE", Mine: true}},
			Tasks:     domain.TaskProgress{Done: 1, Total: 2},
		},
		{ID: 12, Ref: 3, Title: "Second", Assignees: []board.Assignee{{Name: "Carla de la Mar", Initials: "CM"}}},
		{ID: 10, Ref: 1, Title: "Third"},
	}
	if got := k.Rows(); !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestUnknownMemberHasPlaceholderNameAndInitials(t *testing.T) {
	k := board.NewKanban(project, stories, me).NextTab()

	rows := k.Rows()
	if len(rows) != 1 || !reflect.DeepEqual(rows[0].Assignees, []board.Assignee{{Name: "Unknown member", Initials: "??"}}) {
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

func TestDetailDescribesSelectedUserStory(t *testing.T) {
	withSprint := slices.Clone(stories)
	withSprint[1].SprintName = "Sprint 4"
	k := board.NewKanban(project, withSprint, me)

	got, ok := k.Detail()

	want := board.Detail{
		ID: 11, Ref: 2, Title: "First",
		Status:     project.UserStoryStatuses[0],
		Assignees:  []board.Assignee{{Name: "bkoch", Initials: "BK"}, {Name: "Alice Example", Initials: "AE", Mine: true}},
		SprintName: "Sprint 4",
		Tasks:      domain.TaskProgress{Done: 1, Total: 2},
	}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestNoDetailOnEmptyTab(t *testing.T) {
	k := board.NewKanban(project, stories, me).PrevTab()

	if d, ok := k.Detail(); ok {
		t.Errorf("got %+v", d)
	}
}

func TestLoadDetailRequestsDescriptionOfSelectedUserStoryOnce(t *testing.T) {
	k := board.NewKanban(project, stories, me)

	k, load, ok := k.LoadDetail()
	if !ok || load.ID != 11 {
		t.Fatalf("got %+v, %v", load, ok)
	}
	if d, _ := k.Detail(); d.Description.State != board.DescriptionLoading {
		t.Errorf("description %+v while loading", d.Description)
	}
	if _, again, ok := k.LoadDetail(); ok {
		t.Errorf("requested again while loading: %+v", again)
	}
	if _, _, ok := k.PrevTab().LoadDetail(); ok {
		t.Error("requested a description on an empty tab")
	}
}

func TestLoadedDescriptionAppearsInDetail(t *testing.T) {
	k, load, _ := board.NewKanban(project, stories, me).LoadDetail()

	k = k.UserStoryLoaded(load, domain.UserStory{ID: 11, Description: "Pay by card."})

	d, _ := k.Detail()
	if want := (board.Description{State: board.DescriptionLoaded, Text: "Pay by card."}); d.Description != want {
		t.Errorf("got %+v", d.Description)
	}
	if _, _, ok := k.LoadDetail(); ok {
		t.Error("requested a loaded description again")
	}
}

func TestFailedDescriptionLoadIsReportedAndRetried(t *testing.T) {
	k, load, _ := board.NewKanban(project, stories, me).LoadDetail()
	boom := errors.New("timed out")

	k = k.UserStoryFailed(load, boom)

	d, _ := k.Detail()
	if d.Description.State != board.DescriptionFailed || !errors.Is(d.Description.Err, boom) {
		t.Errorf("got %+v", d.Description)
	}
	if _, retry, ok := k.LoadDetail(); !ok || retry.ID != 11 {
		t.Errorf("failed load not retried: %+v, %v", retry, ok)
	}
}

func TestReloadRequestsDescriptionAgainAndDropsStaleResults(t *testing.T) {
	k, old, _ := board.NewKanban(project, stories, me).LoadDetail()
	k = k.UserStoryLoaded(old, domain.UserStory{ID: 11, Description: "Old text."})

	k = k.Reload(stories)
	k, fresh, ok := k.LoadDetail()
	if !ok || fresh.ID != 11 {
		t.Fatalf("not requested again after reload: %+v, %v", fresh, ok)
	}
	if d, _ := k.Detail(); d.Description.State != board.DescriptionLoaded || d.Description.Text != "Old text." {
		t.Errorf("old text not kept while reloading: %+v", d.Description)
	}
	k = k.UserStoryLoaded(old, domain.UserStory{ID: 11, Description: "Stale."})
	k = k.UserStoryFailed(old, errors.New("stale"))
	if d, _ := k.Detail(); d.Description.Text != "Old text." || d.Description.Err != nil {
		t.Errorf("stale result applied: %+v", d.Description)
	}
	k = k.UserStoryLoaded(fresh, domain.UserStory{ID: 11, Description: "New text."})
	if d, _ := k.Detail(); d.Description.Text != "New text." {
		t.Errorf("fresh result not applied: %+v", d.Description)
	}
}

func TestLoadDetailNeverChangesTheReceiver(t *testing.T) {
	k := board.NewKanban(project, stories, me)

	_, _, _ = k.LoadDetail()

	if _, _, ok := k.LoadDetail(); !ok {
		t.Error("the first LoadDetail changed the receiver")
	}
}
