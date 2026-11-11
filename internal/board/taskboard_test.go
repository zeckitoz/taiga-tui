package board_test

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/zeckitoz/taiga-tui/internal/board"
	"github.com/zeckitoz/taiga-tui/internal/domain"
)

var scrum = func() domain.Project {
	p := project
	p.TaskStatuses = []domain.Status{
		{ID: 21, Name: "New", Color: "#70728F"},
		{ID: 22, Name: "In progress", Color: "#E47C40"},
		{ID: 23, Name: "Closed", Color: "#A8E440", Closed: true},
	}
	return p
}()

var sprint = domain.Sprint{ID: 5, Name: "Sprint 4"}

// sprintContent has, in Sprint order: #15 with one Task, #12 with two, #20
// without Tasks, and two Tasks without a User Story in the Sprint.
var sprintContent = domain.SprintContent{
	UserStories: []domain.UserStory{
		{ID: 501, Ref: 12, Title: "Checkout", Status: 2, SprintOrder: 2, Assignees: []int64{43}},
		{ID: 503, Ref: 20, Title: "Search", Status: 1, SprintOrder: 3},
		{ID: 502, Ref: 15, Title: "Login", Status: 4, SprintOrder: 1, Assignees: []int64{42}},
	},
	Tasks: []domain.Task{
		{ID: 901, Ref: 31, Title: "Card number", Status: 22, UserStory: 501, Assignee: 42, TaskboardOrder: 2},
		{ID: 902, Ref: 32, Title: "3-D Secure", Status: 23, UserStory: 501, Assignee: 43, TaskboardOrder: 1},
		{ID: 903, Ref: 33, Title: "Form", Status: 23, UserStory: 502, TaskboardOrder: 1},
		{ID: 904, Ref: 40, Title: "Docs", Status: 21, TaskboardOrder: 2},
		{ID: 905, Ref: 41, Title: "Orphan", Status: 99, UserStory: 777, Assignee: 99, TaskboardOrder: 1},
	},
}

func newTaskboard() board.Taskboard {
	return board.NewTaskboard(scrum, sprint, sprintContent, me)
}

// refs lists the visible rows as "US#ref", "T#ref", "none" for a "No tasks"
// row and "US-" for the group of Tasks without User Story, with a "*" for
// the selected row.
func refs(tb board.Taskboard) []string {
	var out []string
	for _, r := range tb.Rows() {
		var s string
		switch r.Kind {
		case board.RowUserStory:
			s = "US-"
			if r.ID != 0 {
				s = "US#" + itoa(r.Ref)
			}
		case board.RowTask:
			s = "T#" + itoa(r.Ref)
		case board.RowNoTasks:
			s = "none"
		}
		if r.Selected {
			s += "*"
		}
		out = append(out, s)
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

func press(tb board.Taskboard, keys ...func(board.Taskboard) board.Taskboard) board.Taskboard {
	for _, k := range keys {
		tb = k(tb)
	}
	return tb
}

var (
	down   = board.Taskboard.CursorDown
	up     = board.Taskboard.CursorUp
	left   = board.Taskboard.Left
	right  = board.Taskboard.Right
	toggle = board.Taskboard.Toggle
)

func TestTaskboardGroupsTasksByUserStoryInSprintOrder(t *testing.T) {
	want := []string{"US#15*", "T#33", "US#12", "T#32", "T#31", "US#20", "none", "US-", "T#41", "T#40"}

	if got := refs(newTaskboard()); !slices.Equal(got, want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
}

func TestTaskboardWithoutTasksOutsideUserStoriesHasNoExtraGroup(t *testing.T) {
	c := domain.SprintContent{UserStories: sprintContent.UserStories[:1], Tasks: sprintContent.Tasks[:2]}

	want := []string{"US#12*", "T#32", "T#31"}
	if got := refs(board.NewTaskboard(scrum, sprint, c, me)); !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTaskboardRowsCarryProgressStatusAndAssignee(t *testing.T) {
	rows := newTaskboard().Rows()

	story := rows[2] // #12
	wantStory := board.TaskboardRow{
		Kind: board.RowUserStory, ID: 501, Ref: 12, Title: "Checkout", Expanded: true,
		Tasks: domain.TaskProgress{Done: 1, Total: 2},
	}
	if !reflect.DeepEqual(story, wantStory) {
		t.Errorf("User Story row\ngot  %+v\nwant %+v", story, wantStory)
	}
	task := rows[4] // #31, mine
	wantTask := board.TaskboardRow{
		Kind: board.RowTask, ID: 901, Ref: 31, Title: "Card number",
		Status:    scrum.TaskStatuses[1],
		Assignees: []board.Assignee{{Name: "Alice Example", Initials: "AE", Mine: true}},
		Mine:      true,
	}
	if !reflect.DeepEqual(task, wantTask) {
		t.Errorf("Task row\ngot  %+v\nwant %+v", task, wantTask)
	}
	orphan := rows[8] // #41: unknown Status and member
	if orphan.Status.ID != 0 || len(orphan.Assignees) != 1 || orphan.Assignees[0].Name != "Unknown member" || orphan.Mine {
		t.Errorf("orphan row %+v", orphan)
	}
	if none := rows[7]; none.Tasks != (domain.TaskProgress{Total: 2}) {
		t.Errorf("group without User Story %+v", none)
	}
}

func TestTaskboardCursorSkipsNoTasksRowsAndStopsAtEdges(t *testing.T) {
	tb := press(newTaskboard(), up)
	if got := refs(tb)[0]; got != "US#15*" {
		t.Errorf("up at the top: %v", refs(tb))
	}

	tb = press(tb, down, down, down, down, down)
	if got := refs(tb); got[5] != "US#20*" {
		t.Errorf("after five downs: %v", got)
	}
	tb = press(tb, down)
	if got := refs(tb); got[7] != "US-*" {
		t.Errorf("down skips the No tasks row: %v", got)
	}
	tb = press(tb, down, down, down)
	if got := refs(tb); got[9] != "T#40*" {
		t.Errorf("down at the bottom: %v", got)
	}
}

func TestLeftOnTaskJumpsToUserStoryThenCollapsesIt(t *testing.T) {
	tb := press(newTaskboard(), down, down, down, down) // #31

	tb = press(tb, left)
	if got := refs(tb); got[2] != "US#12*" {
		t.Errorf("left on a Task: %v", got)
	}
	tb = press(tb, left)
	want := []string{"US#15", "T#33", "US#12*", "US#20", "none", "US-", "T#41", "T#40"}
	if got := refs(tb); !slices.Equal(got, want) {
		t.Errorf("left on a User Story\ngot  %v\nwant %v", got, want)
	}
	if tb.Rows()[2].Expanded {
		t.Error("collapsed User Story still expanded")
	}
	if got := refs(press(tb, left)); !slices.Equal(got, want) {
		t.Errorf("left on a collapsed User Story: %v", got)
	}
}

func TestRightExpandsCollapsedUserStoryOnly(t *testing.T) {
	tb := press(newTaskboard(), left)
	if len(tb.Rows()) != 9 {
		t.Fatalf("not collapsed: %v", refs(tb))
	}

	tb = press(tb, right)
	if got := refs(tb); !slices.Equal(got, refs(newTaskboard())) {
		t.Errorf("right: %v", got)
	}
	if got := refs(press(tb, down, right)); got[1] != "T#33*" || len(got) != 10 {
		t.Errorf("right on a Task: %v", got)
	}
}

func TestToggleCollapsesAndExpandsUserStory(t *testing.T) {
	tb := press(newTaskboard(), down, down, toggle)
	if got := refs(tb); len(got) != 8 || got[2] != "US#12*" {
		t.Errorf("toggle once: %v", got)
	}
	if got := refs(press(tb, toggle)); !slices.Equal(got, refs(press(newTaskboard(), down, down))) {
		t.Errorf("toggle twice: %v", got)
	}
}

func TestToggleOnTaskChangesNothing(t *testing.T) {
	tb := press(newTaskboard(), down)

	if got := refs(press(tb, toggle)); !slices.Equal(got, refs(tb)) {
		t.Errorf("got %v", got)
	}
}

func TestGroupWithoutUserStoryCollapses(t *testing.T) {
	tb := press(newTaskboard(), down, down, down, down, down, down, left)

	if got := refs(tb); len(got) != 8 || got[7] != "US-*" {
		t.Errorf("got %v", got)
	}
}

func TestTaskboardMethodsNeverChangeTheReceiver(t *testing.T) {
	tb := press(newTaskboard(), down)
	before := refs(tb)

	_ = press(tb, down, left, left, toggle, right)

	if got := refs(tb); !slices.Equal(got, before) {
		t.Errorf("receiver changed to %v", got)
	}
}

func TestTaskboardReloadKeepsCollapsedUserStoriesAndSelectedTask(t *testing.T) {
	tb := press(newTaskboard(), left, down, down, down) // #15 collapsed, #31 selected
	c := sprintContent
	c.Tasks = append([]domain.Task{{ID: 906, Ref: 50, Title: "New", Status: 21, UserStory: 501}}, c.Tasks...)

	tb = tb.Reload(c)

	want := []string{"US#15", "US#12", "T#50", "T#32", "T#31*", "US#20", "none", "US-", "T#41", "T#40"}
	if got := refs(tb); !slices.Equal(got, want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
}

func TestTaskboardReloadSelectsNeighbourWhenSelectedTaskVanished(t *testing.T) {
	tb := press(newTaskboard(), down, down, down) // #32
	c := sprintContent
	c.Tasks = slices.DeleteFunc(slices.Clone(c.Tasks), func(t domain.Task) bool { return t.ID == 902 })

	tb = tb.Reload(c)

	if got := refs(tb); got[3] != "T#31*" {
		t.Errorf("got %v", got)
	}
}

func TestEmptyTaskboard(t *testing.T) {
	tb := board.NewTaskboard(scrum, sprint, domain.SprintContent{}, me)

	if !tb.Empty() || len(tb.Rows()) != 0 || tb.SelectedIndex() != -1 {
		t.Errorf("empty: %v, rows %v, selected %d", tb.Empty(), tb.Rows(), tb.SelectedIndex())
	}
	if _, ok := tb.Detail(); ok {
		t.Error("detail on an empty Taskboard")
	}
	if got := refs(press(tb, down, left, right, toggle)); len(got) != 0 {
		t.Errorf("keys on an empty Taskboard: %v", got)
	}
	if newTaskboard().Empty() {
		t.Error("Taskboard with User Stories is empty")
	}
}

func TestTaskboardSelectedIndexMatchesSelectedRow(t *testing.T) {
	tb := press(newTaskboard(), down, down, down)

	if i := tb.SelectedIndex(); !tb.Rows()[i].Selected {
		t.Errorf("index %d is not the selected row", i)
	}
	if !tb.TaskSelected() || press(tb, left).TaskSelected() {
		t.Error("TaskSelected wrong")
	}
}

func TestTaskDetailNamesStatusAssigneeAndUserStory(t *testing.T) {
	tb := press(newTaskboard(), down, down, down, down) // #31

	d, ok := tb.Detail()

	want := board.TaskboardDetail{
		Kind: board.RowTask, ID: 901, Ref: 31, Title: "Card number",
		Status:         scrum.TaskStatuses[1],
		Assignees:      []board.Assignee{{Name: "Alice Example", Initials: "AE", Mine: true}},
		UserStoryRef:   12,
		UserStoryTitle: "Checkout",
		Description:    board.Description{State: board.DescriptionLoading},
	}
	if !ok || !reflect.DeepEqual(d, want) {
		t.Errorf("got  %+v\nwant %+v", d, want)
	}
}

func TestTaskWithoutUserStoryHasNoneInDetail(t *testing.T) {
	tb := press(newTaskboard(), down, down, down, down, down, down, down) // #41

	if d, _ := tb.Detail(); d.Ref != 41 || d.UserStoryRef != 0 || d.UserStoryTitle != "" {
		t.Errorf("got %+v", d)
	}
}

func TestUserStoryDetailCountsTasksPerStatusInTaigaOrder(t *testing.T) {
	tb := press(newTaskboard(), down, down) // #12

	d, ok := tb.Detail()

	want := board.TaskboardDetail{
		Kind: board.RowUserStory, ID: 501, Ref: 12, Title: "Checkout",
		Status:    project.UserStoryStatuses[1],
		Assignees: []board.Assignee{{Name: "bkoch", Initials: "BK"}},
		Tasks:     domain.TaskProgress{Done: 1, Total: 2},
		TaskCounts: []board.StatusCount{
			{Status: scrum.TaskStatuses[1], Count: 1},
			{Status: scrum.TaskStatuses[2], Count: 1},
		},
	}
	if !ok || !reflect.DeepEqual(d, want) {
		t.Errorf("got  %+v\nwant %+v", d, want)
	}
}

func TestGroupWithoutUserStoryCountsUnknownStatusesLast(t *testing.T) {
	tb := press(newTaskboard(), down, down, down, down, down, down) // the group

	d, _ := tb.Detail()

	want := []board.StatusCount{{Status: scrum.TaskStatuses[0], Count: 1}, {Count: 1}}
	if d.ID != 0 || !reflect.DeepEqual(d.TaskCounts, want) {
		t.Errorf("got %+v", d)
	}
}

func TestTaskboardLoadsDescriptionOfSelectedTaskOnly(t *testing.T) {
	tb := newTaskboard()
	if _, _, ok := tb.LoadDetail(); ok {
		t.Error("loads a description for a User Story")
	}

	tb = press(tb, down)
	tb, load, ok := tb.LoadDetail()
	if !ok || load.ID != 903 {
		t.Fatalf("load %+v, %v", load, ok)
	}
	if _, _, again := tb.LoadDetail(); again {
		t.Error("loads the description twice")
	}

	tb = tb.TaskLoaded(load, domain.Task{ID: 903, Description: "Fields and labels."})
	if d, _ := tb.Detail(); d.Description != (board.Description{State: board.DescriptionLoaded, Text: "Fields and labels."}) {
		t.Errorf("detail %+v", d.Description)
	}
}

func TestTaskboardDescriptionFailureIsRetriedAndStaleResultsDropped(t *testing.T) {
	tb := press(newTaskboard(), down)
	tb, load, _ := tb.LoadDetail()
	boom := errors.New("boom")

	tb = tb.TaskFailed(load, boom)
	if d, _ := tb.Detail(); d.Description.State != board.DescriptionFailed || !errors.Is(d.Description.Err, boom) {
		t.Errorf("detail %+v", d.Description)
	}
	tb, retry, ok := tb.LoadDetail()
	if !ok {
		t.Fatal("failed load not retried")
	}

	tb = tb.Reload(sprintContent)
	tb = tb.TaskLoaded(retry, domain.Task{ID: 903, Description: "stale"})
	if d, _ := tb.Detail(); d.Description.State == board.DescriptionLoaded {
		t.Errorf("stale result taken: %+v", d.Description)
	}
}
