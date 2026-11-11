package board

import (
	"cmp"
	"maps"
	"slices"

	"github.com/zeckitoz/taiga-tui/internal/domain"
)

// Taskboard is the Taskboard of one Sprint: its Tasks grouped by User Story
// in Sprint order, with the Tasks without a User Story in the Sprint as the
// last group. Groups collapse and expand; this is never persisted. Its
// methods return a new Taskboard and never change the receiver.
type Taskboard struct {
	project domain.Project
	me      int64
	sprint  domain.Sprint
	groups  []group
	// collapsed holds the collapsed groups by User Story ID, 0 for the
	// group without User Story.
	collapsed map[int64]bool
	cursor    item
	// descriptions holds the descriptions by Task ID.
	descriptions descriptions
}

// group is a User Story with its Tasks in Taskboard order. The group of
// Tasks without User Story has the zero User Story.
type group struct {
	story domain.UserStory
	tasks []domain.Task
}

// item identifies a row that can be selected: a Task, or the User Story
// heading a group.
type item struct {
	task bool
	id   int64
}

// RowKind tells what a Taskboard row shows.
type RowKind int

// The kinds of Taskboard rows.
const (
	// RowUserStory heads the group of a User Story, or with ID 0 the group
	// of Tasks without User Story.
	RowUserStory RowKind = iota
	RowTask
	// RowNoTasks stands under an expanded User Story without Tasks and
	// cannot be selected.
	RowNoTasks
)

// TaskboardRow is one visible row of the Taskboard.
type TaskboardRow struct {
	Kind  RowKind
	ID    int64
	Ref   int
	Title string
	// Expanded and Tasks describe a User Story row; Tasks counts the Tasks
	// in a closed Status as done.
	Expanded bool
	Tasks    domain.TaskProgress
	// Status, Assignees and Mine describe a Task row. Status is the zero
	// Status if it is unknown.
	Status    domain.Status
	Assignees []Assignee
	Mine      bool
	Selected  bool
}

// TaskboardDetail is what the detail view shows of the selected row.
type TaskboardDetail struct {
	// Kind is RowTask or RowUserStory.
	Kind  RowKind
	ID    int64
	Ref   int
	Title string
	// Status is the zero Status if it is unknown.
	Status    domain.Status
	Assignees []Assignee
	// UserStoryRef and UserStoryTitle name the User Story of a Task, 0 and
	// empty if it has none in the Sprint.
	UserStoryRef   int
	UserStoryTitle string
	// Tasks and TaskCounts summarize the Tasks of a User Story: the counts
	// per Status in Taiga's order, those of unknown Statuses last.
	Tasks      domain.TaskProgress
	TaskCounts []StatusCount
	// Description of a Task is loaded separately, see LoadDetail.
	Description Description
}

// StatusCount is the number of Tasks in a Status.
type StatusCount struct {
	// Status is the zero Status for unknown Statuses.
	Status domain.Status
	Count  int
}

// LoadTask is the effect of loading a Task with its description. Its result
// goes back into TaskLoaded or TaskFailed.
type LoadTask struct {
	ID         int64
	Generation int
}

// NewTaskboard returns the Taskboard of sprint in p with the given content,
// seen by the Team Member with ID me. All groups are expanded and the first
// row is selected.
func NewTaskboard(p domain.Project, sprint domain.Sprint, content domain.SprintContent, me int64) Taskboard {
	t := Taskboard{project: p, me: me, sprint: sprint, groups: groupContent(content)}
	if rows := t.items(); len(rows) > 0 {
		t.cursor = rows[0].item
	}
	return t
}

func groupContent(c domain.SprintContent) []group {
	stories := slices.Clone(c.UserStories)
	slices.SortStableFunc(stories, func(a, b domain.UserStory) int { return cmp.Compare(a.SprintOrder, b.SprintOrder) })
	groups := make([]group, len(stories))
	index := map[int64]int{}
	for i, s := range stories {
		groups[i].story = s
		index[s.ID] = i
	}
	var rest group
	for _, task := range c.Tasks {
		if i, ok := index[task.UserStory]; ok && task.UserStory != 0 {
			groups[i].tasks = append(groups[i].tasks, task)
		} else {
			rest.tasks = append(rest.tasks, task)
		}
	}
	if len(rest.tasks) > 0 {
		groups = append(groups, rest)
	}
	for i := range groups {
		slices.SortStableFunc(groups[i].tasks, func(a, b domain.Task) int { return cmp.Compare(a.TaskboardOrder, b.TaskboardOrder) })
	}
	return groups
}

// Reload replaces the content, keeping the collapsed groups and the selected
// row, or selecting the row now at its place if it vanished. Descriptions
// count as outdated: they stay visible until LoadDetail has loaded them
// again.
func (t Taskboard) Reload(c domain.SprintContent) Taskboard {
	old := t.SelectedIndex()
	t.groups = groupContent(c)
	t.descriptions = t.descriptions.reloaded()
	rows := t.items()
	if old < 0 || slices.ContainsFunc(rows, func(r itemRow) bool { return r.item == t.cursor }) {
		return t
	}
	// The row now at the old place, or the nearest one above it. The first
	// row is always selectable.
	t.cursor = item{}
	for j := min(old, len(rows)-1); j >= 0; j-- {
		if rows[j].selectable() {
			t.cursor = rows[j].item
			break
		}
	}
	return t
}

// Sprint returns the Sprint the Taskboard shows.
func (t Taskboard) Sprint() domain.Sprint { return t.sprint }

// Empty reports whether the Sprint has neither User Stories nor Tasks.
func (t Taskboard) Empty() bool { return len(t.groups) == 0 }

// itemRow is a visible row with the item it selects and its group.
type itemRow struct {
	item  item
	kind  RowKind
	group int
	task  int // index into the group's Tasks for RowTask
}

func (r itemRow) selectable() bool { return r.kind != RowNoTasks }

func (t Taskboard) items() []itemRow {
	var rows []itemRow
	for g, gr := range t.groups {
		rows = append(rows, itemRow{item: item{id: gr.story.ID}, kind: RowUserStory, group: g})
		if t.collapsed[gr.story.ID] {
			continue
		}
		if len(gr.tasks) == 0 {
			rows = append(rows, itemRow{kind: RowNoTasks, group: g})
		}
		for i, task := range gr.tasks {
			rows = append(rows, itemRow{item: item{task: true, id: task.ID}, kind: RowTask, group: g, task: i})
		}
	}
	return rows
}

// SelectedIndex is the index of the selected row in Rows, -1 if there are
// no rows.
func (t Taskboard) SelectedIndex() int {
	return slices.IndexFunc(t.items(), func(r itemRow) bool { return r.selectable() && r.item == t.cursor })
}

// TaskSelected reports whether a Task row is selected.
func (t Taskboard) TaskSelected() bool { return t.SelectedIndex() >= 0 && t.cursor.task }

// Rows returns the visible rows.
func (t Taskboard) Rows() []TaskboardRow {
	items := t.items()
	rows := make([]TaskboardRow, len(items))
	for i, it := range items {
		gr := t.groups[it.group]
		switch it.kind {
		case RowUserStory:
			rows[i] = TaskboardRow{
				Kind:     RowUserStory,
				ID:       gr.story.ID,
				Ref:      gr.story.Ref,
				Title:    gr.story.Title,
				Expanded: !t.collapsed[gr.story.ID],
				Tasks:    t.progress(gr.tasks),
			}
		case RowTask:
			task := gr.tasks[it.task]
			rows[i] = TaskboardRow{
				Kind:      RowTask,
				ID:        task.ID,
				Ref:       task.Ref,
				Title:     task.Title,
				Status:    t.taskStatus(task.Status),
				Assignees: t.taskAssignees(task),
				Mine:      task.Assignee != 0 && task.Assignee == t.me,
			}
		case RowNoTasks:
			rows[i] = TaskboardRow{Kind: RowNoTasks}
		}
		rows[i].Selected = it.selectable() && it.item == t.cursor
	}
	return rows
}

func (t Taskboard) progress(tasks []domain.Task) domain.TaskProgress {
	p := domain.TaskProgress{Total: len(tasks)}
	for _, task := range tasks {
		if t.taskStatus(task.Status).Closed {
			p.Done++
		}
	}
	return p
}

func (t Taskboard) taskStatus(id int64) domain.Status {
	return statusByID(t.project.TaskStatuses, id)
}

func statusByID(statuses []domain.Status, id int64) domain.Status {
	if i := slices.IndexFunc(statuses, func(s domain.Status) bool { return s.ID == id }); i >= 0 {
		return statuses[i]
	}
	return domain.Status{}
}

func (t Taskboard) taskAssignees(task domain.Task) []Assignee {
	if task.Assignee == 0 {
		return nil
	}
	return assigneesOf(t.project, t.me, []int64{task.Assignee})
}

// CursorDown selects the next row.
func (t Taskboard) CursorDown() Taskboard { return t.moveCursor(1) }

// CursorUp selects the previous row.
func (t Taskboard) CursorUp() Taskboard { return t.moveCursor(-1) }

func (t Taskboard) moveCursor(d int) Taskboard {
	rows := t.items()
	for j := t.SelectedIndex() + d; j >= 0 && j < len(rows); j += d {
		if rows[j].selectable() {
			t.cursor = rows[j].item
			return t
		}
	}
	return t
}

// Left selects the User Story of the selected Task, or collapses the
// selected User Story.
func (t Taskboard) Left() Taskboard {
	i := t.SelectedIndex()
	if i < 0 {
		return t
	}
	if r := t.items()[i]; r.kind == RowTask {
		t.cursor = item{id: t.groups[r.group].story.ID}
		return t
	}
	return t.setCollapsed(t.cursor.id, true)
}

// Right expands the selected User Story.
func (t Taskboard) Right() Taskboard {
	if t.SelectedIndex() < 0 || t.cursor.task {
		return t
	}
	return t.setCollapsed(t.cursor.id, false)
}

// Toggle collapses or expands the selected User Story.
func (t Taskboard) Toggle() Taskboard {
	if t.SelectedIndex() < 0 || t.cursor.task {
		return t
	}
	return t.setCollapsed(t.cursor.id, !t.collapsed[t.cursor.id])
}

func (t Taskboard) setCollapsed(storyID int64, collapsed bool) Taskboard {
	t.collapsed = maps.Clone(t.collapsed) // never share the map with the receiver
	if t.collapsed == nil {
		t.collapsed = map[int64]bool{}
	}
	if collapsed {
		t.collapsed[storyID] = true
	} else {
		delete(t.collapsed, storyID)
	}
	return t
}

// Detail returns the detail of the selected row, or false if there is none.
func (t Taskboard) Detail() (TaskboardDetail, bool) {
	i := t.SelectedIndex()
	if i < 0 {
		return TaskboardDetail{}, false
	}
	r := t.items()[i]
	gr := t.groups[r.group]
	if r.kind == RowTask {
		task := gr.tasks[r.task]
		return TaskboardDetail{
			Kind:           RowTask,
			ID:             task.ID,
			Ref:            task.Ref,
			Title:          task.Title,
			Status:         t.taskStatus(task.Status),
			Assignees:      t.taskAssignees(task),
			UserStoryRef:   gr.story.Ref,
			UserStoryTitle: gr.story.Title,
			Description:    t.descriptions.get(task.ID),
		}, true
	}
	d := TaskboardDetail{
		Kind:       RowUserStory,
		ID:         gr.story.ID,
		Ref:        gr.story.Ref,
		Title:      gr.story.Title,
		Tasks:      t.progress(gr.tasks),
		TaskCounts: t.taskCounts(gr.tasks),
	}
	if gr.story.ID != 0 {
		d.Status = statusByID(t.project.UserStoryStatuses, gr.story.Status)
		d.Assignees = assigneesOf(t.project, t.me, gr.story.Assignees)
	}
	return d, true
}

func (t Taskboard) taskCounts(tasks []domain.Task) []StatusCount {
	var counts []StatusCount
	unknown := len(tasks)
	for _, s := range t.project.TaskStatuses {
		n := 0
		for _, task := range tasks {
			if task.Status == s.ID {
				n++
			}
		}
		if n > 0 {
			counts = append(counts, StatusCount{Status: s, Count: n})
			unknown -= n
		}
	}
	if unknown > 0 {
		counts = append(counts, StatusCount{Count: unknown})
	}
	return counts
}

// LoadDetail returns the effect of loading the description of the selected
// Task, unless it was loaded or is loading since the last Reload. A failed
// load is tried again. User Story rows need no description.
func (t Taskboard) LoadDetail() (Taskboard, LoadTask, bool) {
	if !t.TaskSelected() {
		return t, LoadTask{}, false
	}
	id := t.cursor.id
	var gen int
	var ok bool
	t.descriptions, gen, ok = t.descriptions.start(id)
	return t, LoadTask{ID: id, Generation: gen}, ok
}

// TaskLoaded takes the description from the result of load.
func (t Taskboard) TaskLoaded(load LoadTask, task domain.Task) Taskboard {
	t.descriptions = t.descriptions.loaded(load.ID, load.Generation, task.Description)
	return t
}

// TaskFailed records that load failed with err.
func (t Taskboard) TaskFailed(load LoadTask, err error) Taskboard {
	t.descriptions = t.descriptions.failed(load.ID, load.Generation, err)
	return t
}
