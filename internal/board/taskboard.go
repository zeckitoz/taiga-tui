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
	// content is the content as last loaded or saved; groups show it with
	// the Statuses being saved.
	content domain.SprintContent
	saves   saves
	groups  []group
	// collapsed holds the collapsed groups by User Story ID, 0 for the
	// group without User Story.
	collapsed map[int64]bool
	cursor    item
	// mine shows only the work of the Team Member, see ToggleMine.
	mine bool
	// descriptions holds the descriptions by Task ID.
	descriptions descriptions
}

// hasUnknownIDs reports whether the content refers to Statuses or Team
// Members the project does not know.
func (t Taskboard) hasUnknownIDs() bool {
	p := t.project
	return slices.ContainsFunc(t.content.UserStories, func(s domain.UserStory) bool {
		return !p.KnowsUserStoryStatus(s.Status) || !p.KnowsMembers(s.Assignees)
	}) || slices.ContainsFunc(t.content.Tasks, func(task domain.Task) bool {
		return !p.KnowsTaskStatus(task.Status) || task.Assignee != 0 && !p.KnowsMembers([]int64{task.Assignee})
	})
}

// withProject takes the reloaded project.
func (t Taskboard) withProject(p domain.Project) Taskboard {
	t.project = p
	return t.regrouped()
}

// group is a User Story with its Tasks in Taskboard order. The group of
// Tasks without User Story has the zero User Story. tasks are the Tasks the
// group shows, which are fewer than allTasks while only the Team Member's work
// is shown.
type group struct {
	userStory domain.UserStory
	tasks     []domain.Task
	allTasks  []domain.Task
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
	// Status is the Status of a Task or User Story row, with the Status
	// being saved applied. It is the zero Status if it is unknown, and for
	// the group of Tasks without User Story. Assignees and Mine describe a
	// Task row.
	Status    domain.Status
	Assignees []Assignee
	Mine      bool
	Selected  bool
	// Saving tells that a Status Change of the row's item is being saved.
	Saving bool
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
	Sprint     int64
	Generation int
}

// NewTaskboard returns the Taskboard of sprint in p with the given content,
// seen by the Team Member with ID me. All groups are expanded and the first
// row is selected.
func NewTaskboard(p domain.Project, sprint domain.Sprint, content domain.SprintContent, me int64) Taskboard {
	t := Taskboard{project: p, me: me, sprint: sprint, content: content}.regrouped()
	if rows := t.items(); len(rows) > 0 {
		t.cursor = rows[0].item
	}
	return t
}

// regrouped groups the content with the Statuses being saved. A selected row
// that is no longer shown gives way to the next shown row, or else to the
// previous one.
func (t Taskboard) regrouped() Taskboard {
	old := t.items()
	c := domain.SprintContent{
		UserStories: slices.Clone(t.content.UserStories),
		Tasks:       slices.Clone(t.content.Tasks),
	}
	for i, s := range c.UserStories {
		c.UserStories[i].Status = t.saves.status(UserStoryItem, s.ID, s.Status)
		c.UserStories[i].Assignees = t.saves.assignees(s.ID, s.Assignees)
	}
	for i, task := range c.Tasks {
		c.Tasks[i].Status = t.saves.status(TaskItem, task.ID, task.Status)
		c.Tasks[i].Assignee = t.saves.assignee(task.ID, task.Assignee)
	}
	t.groups = t.filtered(groupContent(c))
	return t.settled(old)
}

// filtered drops what only-mine hides: other people's Tasks, and the User
// Stories left without Tasks of the Team Member unless they are assigned to
// them.
func (t Taskboard) filtered(groups []group) []group {
	for i := range groups {
		groups[i].allTasks = groups[i].tasks
	}
	if !t.mine {
		return groups
	}
	var out []group
	for _, g := range groups {
		g.tasks = slices.DeleteFunc(slices.Clone(g.tasks), func(task domain.Task) bool { return task.Assignee != t.me })
		if len(g.tasks) > 0 || g.userStory.ID != 0 && g.userStory.AssignedTo(t.me) {
			out = append(out, g)
		}
	}
	return out
}

// settled moves the cursor off a row of old that is no longer shown.
func (t Taskboard) settled(old []itemRow) Taskboard {
	selectable := func(rows []itemRow, r itemRow) bool {
		return r.selectable() && slices.ContainsFunc(rows, func(n itemRow) bool { return n.selectable() && n.item == r.item })
	}
	at := slices.IndexFunc(old, func(r itemRow) bool { return r.selectable() && r.item == t.cursor })
	rows := t.items()
	if at < 0 || selectable(rows, old[at]) {
		return t
	}
	for j := at + 1; j < len(old); j++ {
		if selectable(rows, old[j]) {
			t.cursor = old[j].item
			return t
		}
	}
	for j := at - 1; j >= 0; j-- {
		if selectable(rows, old[j]) {
			t.cursor = old[j].item
			return t
		}
	}
	t.cursor = item{}
	return t
}

// ToggleMine switches between showing everything and only the work of the
// Team Member: their Tasks, with the User Stories that have one or are
// assigned to them. It is never persisted.
func (t Taskboard) ToggleMine() Taskboard { return t.WithMine(!t.mine) }

// WithMine shows only the work of the Team Member if mine is set, like
// ToggleMine.
func (t Taskboard) WithMine(mine bool) Taskboard {
	t.mine = mine
	return t.regrouped()
}

// Mine reports whether only the work of the Team Member is shown.
func (t Taskboard) Mine() bool { return t.mine }

func groupContent(c domain.SprintContent) []group {
	userStories := slices.Clone(c.UserStories)
	slices.SortStableFunc(userStories, func(a, b domain.UserStory) int { return cmp.Compare(a.SprintOrder, b.SprintOrder) })
	groups := make([]group, len(userStories))
	index := map[int64]int{}
	for i, s := range userStories {
		groups[i].userStory = s
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
	t.content = c
	t = t.regrouped()
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

// itemKind tells whether the item is a Task or a User Story.
func (it item) itemKind() ItemKind {
	if it.task {
		return TaskItem
	}
	return UserStoryItem
}

func (t Taskboard) items() []itemRow {
	var rows []itemRow
	for g, gr := range t.groups {
		rows = append(rows, itemRow{item: item{id: gr.userStory.ID}, kind: RowUserStory, group: g})
		if t.collapsed[gr.userStory.ID] {
			continue
		}
		if len(gr.allTasks) == 0 {
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
				ID:       gr.userStory.ID,
				Ref:      gr.userStory.Ref,
				Title:    gr.userStory.Title,
				Expanded: !t.collapsed[gr.userStory.ID],
				Tasks:    t.progress(gr.allTasks),
			}
			if gr.userStory.ID != 0 {
				rows[i].Status = statusByID(t.project.UserStoryStatuses, gr.userStory.Status)
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
		rows[i].Saving = it.kind != RowNoTasks && t.saves.saving(it.item.itemKind(), it.item.id)
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

// Expanded reports whether the Tasks of the selected User Story are shown.
// It returns false for ok if no User Story row is selected.
func (t Taskboard) Expanded() (expanded, ok bool) {
	if t.SelectedIndex() < 0 || t.cursor.task {
		return false, false
	}
	return !t.collapsed[t.cursor.id], true
}

// Toggle collapses or expands the selected User Story.
func (t Taskboard) Toggle() Taskboard {
	if t.SelectedIndex() < 0 || t.cursor.task {
		return t
	}
	return t.setCollapsed(t.cursor.id, !t.collapsed[t.cursor.id])
}

func (t Taskboard) setCollapsed(userStoryID int64, collapsed bool) Taskboard {
	t.collapsed = maps.Clone(t.collapsed) // never share the map with the receiver
	if t.collapsed == nil {
		t.collapsed = map[int64]bool{}
	}
	if collapsed {
		t.collapsed[userStoryID] = true
	} else {
		delete(t.collapsed, userStoryID)
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
			UserStoryRef:   gr.userStory.Ref,
			UserStoryTitle: gr.userStory.Title,
			Description:    t.descriptions.get(task.ID),
		}, true
	}
	d := TaskboardDetail{
		Kind:       RowUserStory,
		ID:         gr.userStory.ID,
		Ref:        gr.userStory.Ref,
		Title:      gr.userStory.Title,
		Tasks:      t.progress(gr.allTasks),
		TaskCounts: t.taskCounts(gr.allTasks),
	}
	if gr.userStory.ID != 0 {
		d.Status = statusByID(t.project.UserStoryStatuses, gr.userStory.Status)
		d.Assignees = assigneesOf(t.project, t.me, gr.userStory.Assignees)
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
	return t, LoadTask{ID: id, Sprint: t.sprint.ID, Generation: gen}, ok
}

// TaskLoaded takes the description from the result of load, unless it was
// loaded for another Sprint.
func (t Taskboard) TaskLoaded(load LoadTask, task domain.Task) Taskboard {
	if load.Sprint != t.sprint.ID {
		return t
	}
	t.descriptions = t.descriptions.loaded(load.ID, load.Generation, task.Description)
	return t
}

// TaskFailed records that load failed with err.
func (t Taskboard) TaskFailed(load LoadTask, err error) Taskboard {
	if load.Sprint != t.sprint.ID {
		return t
	}
	t.descriptions = t.descriptions.failed(load.ID, load.Generation, err)
	return t
}

// selectedItem returns the selected Task or User Story, or false if no row
// or the group of Tasks without User Story is selected.
func (t Taskboard) selectedItem() (workItem, bool) {
	i := t.SelectedIndex()
	if i < 0 {
		return workItem{}, false
	}
	r := t.items()[i]
	gr := t.groups[r.group]
	switch {
	case r.kind == RowTask:
		task := gr.tasks[r.task]
		w := workItem{kind: TaskItem, id: task.ID, ref: task.Ref, status: task.Status, version: task.Version}
		if task.Assignee != 0 {
			w.assignees = []int64{task.Assignee}
		}
		return w, true
	case gr.userStory.ID != 0:
		us := gr.userStory
		return workItem{kind: UserStoryItem, id: us.ID, ref: us.Ref, status: us.Status, version: us.Version, assignees: us.Assignees}, true
	}
	return workItem{}, false // the group of Tasks without User Story
}

// StatusSelected reports whether the selected row has a Status to change.
func (t Taskboard) StatusSelected() bool {
	_, ok := t.selectedItem()
	return ok
}

// PickStatus moves the selected Task or User Story to the Status of its
// kind with the given ID right away, keeping the row in place, and returns
// the effect of saving the change, which the App records in sent once it
// sends it. It returns a Rejection instead if the change cannot be made,
// and neither if no item is selected.
func (t Taskboard) PickStatus(id int64, sent Sent) (Taskboard, Effect, error) {
	it, ok := t.selectedItem()
	if !ok {
		return t, nil, nil
	}
	sv, save, err := t.saves.changeStatus(t.project, sent, it, id)
	if err != nil {
		return t, nil, err
	}
	t.saves = sv
	return t.regrouped(), save, nil
}

// AssignToMe makes the Team Member the Assignee of the selected Task or User
// Story right away, keeping the other Assignees of a User Story, and
// returns the effect of saving the change, which the App records in sent
// once it sends it. If that replaces the Assignee of a Task, it returns the
// effect ConfirmAssignToMe instead, to be answered by calling it again with
// it as confirmed; confirmed is the zero value otherwise. It returns a
// Rejection if the change cannot be made, and no effect if no item is
// selected.
func (t Taskboard) AssignToMe(sent Sent, confirmed ConfirmAssignToMe) (Taskboard, Effect, error) {
	it, ok := t.selectedItem()
	if !ok {
		return t, nil, nil
	}
	sv, eff, err := t.saves.assignToMe(t.project, t.me, sent, it, confirmed)
	if err != nil {
		return t, nil, err
	}
	if _, saving := eff.(Save); !saving {
		return t, eff, nil
	}
	t.saves = sv
	return t.regrouped(), eff, nil
}

// StatusOptions returns the Statuses the selected row can be changed to, or
// a Rejection if it cannot be changed right now. It returns no Statuses if
// the selected row has no Status.
func (t Taskboard) StatusOptions(sent Sent) (StatusOptions, error) {
	it, ok := t.selectedItem()
	if !ok {
		return StatusOptions{}, nil
	}
	return t.saves.options(t.project, sent, it)
}

// TaskSaved takes the Task as Taiga saved it from the result of save and,
// after a Status Change, returns the effect of reloading its User Story,
// whose closed state and progress may have changed with it. It returns
// false if the result no longer counts.
func (t Taskboard) TaskSaved(save Save, saved domain.Task) (Taskboard, Effect, bool) {
	var ok bool
	if t.saves, ok = t.saves.end(save); !ok {
		return t, nil, false
	}
	t = t.ReplaceTask(saved)
	if save.Status == 0 || saved.UserStory == 0 || !slices.ContainsFunc(t.content.UserStories, func(s domain.UserStory) bool { return s.ID == saved.UserStory }) {
		return t, nil, true
	}
	return t, ReloadUserStory{ID: saved.UserStory, Sprint: t.sprint.ID}, true
}

// UserStorySaved takes the User Story as Taiga saved it from the result of
// save, or returns false if the result no longer counts.
func (t Taskboard) UserStorySaved(save Save, saved domain.UserStory) (Taskboard, bool) {
	var ok bool
	if t.saves, ok = t.saves.end(save); !ok {
		return t, false
	}
	return t.ReplaceUserStory(saved), true
}

// ReplaceTask takes the Task as Taiga saved it if the Taskboard shows it,
// e.g. from a save whose result arrived after the Team Member came back to
// the Taskboard, so that the next change sends its current version.
func (t Taskboard) ReplaceTask(saved domain.Task) Taskboard {
	if i := slices.IndexFunc(t.content.Tasks, func(task domain.Task) bool { return task.ID == saved.ID }); i >= 0 {
		t.content.Tasks = slices.Clone(t.content.Tasks) // never share the list with the receiver
		t.content.Tasks[i] = saved
	}
	return t.regrouped()
}

// ReplaceUserStory takes the User Story as Taiga saved it if the Taskboard
// shows it, like ReplaceTask.
func (t Taskboard) ReplaceUserStory(saved domain.UserStory) Taskboard {
	return t.withUserStory(saved).regrouped()
}

// NotSaved undoes the change of save and returns the effect of
// reloading the board if the failure asks for it. It returns false if the
// result no longer counts.
func (t Taskboard) NotSaved(save Save, f SaveFailure) (Taskboard, Effect, bool) {
	var ok bool
	if t.saves, ok = t.saves.end(save); !ok {
		return t, nil, false
	}
	return t.regrouped(), reloadOn(f), true
}

// UserStoryReloaded takes the User Story from the result of load, unless it
// was loaded for another Sprint.
func (t Taskboard) UserStoryReloaded(load ReloadUserStory, s domain.UserStory) Taskboard {
	if load.Sprint != t.sprint.ID {
		return t
	}
	return t.withUserStory(s).regrouped()
}

// withUserStory replaces the User Story with the ID of s in the content.
func (t Taskboard) withUserStory(s domain.UserStory) Taskboard {
	if i := slices.IndexFunc(t.content.UserStories, func(us domain.UserStory) bool { return us.ID == s.ID }); i >= 0 {
		t.content.UserStories = slices.Clone(t.content.UserStories) // never share the list with the receiver
		t.content.UserStories[i] = s
	}
	return t
}

// DropSaves forgets the Status Changes being saved, e.g. when the Team
// Member leaves the board. Their results no longer count.
func (t Taskboard) DropSaves() Taskboard {
	t.saves = t.saves.dropped()
	return t.regrouped()
}

// Saving reports whether any Status Change is being saved.
func (t Taskboard) Saving() bool { return t.saves.any() }
