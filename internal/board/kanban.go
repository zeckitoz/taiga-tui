// Package board holds the state of the Taskboard and the Kanban Board as a
// pure state machine. It imports neither Bubble Tea nor HTTP: the ui package
// feeds it data and key actions and only renders the rows it exposes.
package board

import (
	"cmp"
	"maps"
	"slices"
	"strings"
	"unicode"

	"github.com/zeckitoz/taiga-tui/internal/domain"
)

// Kanban is the Kanban Board of one project: one tab per active User Story
// Status with the User Stories of the selected Status. Its methods return a
// new Kanban and never change the receiver.
type Kanban struct {
	project  domain.Project
	me       int64
	statuses []domain.Status // active (non-archived) Statuses in Taiga order
	// stories are the User Stories as last loaded or saved; byStatus groups
	// them by the Status they show, which is the one being saved if any.
	stories  []domain.UserStory
	byStatus map[int64][]domain.UserStory
	saves    saves
	tab      int
	// cursor remembers the selected User Story ID per Status ID.
	cursor map[int64]int64
	// descriptions holds the descriptions by User Story ID.
	descriptions descriptions
	// loaded is set once the User Stories are loaded; loadGen counts the
	// loads of the User Stories, of which only the latest counts.
	loaded  bool
	loadGen int
	// mine shows only the User Stories of the Team Member, see ToggleMine.
	mine bool
	// refreshing tells that a reload of the shown board is on its way.
	refreshing bool
}

// LoadKanban is the effect of loading the User Stories of a Kanban Board.
// Its result goes back into UserStoriesLoaded.
type LoadKanban struct {
	ProjectID  int64
	Generation int
	// Refresh tells that the board is shown and stays visible while it
	// reloads.
	Refresh bool
}

// Tab is one Status tab.
type Tab struct {
	Name  string
	Color string
	Count int
	// WIPLimit is informational, 0 if none.
	WIPLimit int
	Active   bool
}

// Row is one visible User Story of the active tab.
type Row struct {
	ID        int64
	Ref       int
	Title     string
	Assignees []Assignee
	// Mine marks User Stories the logged-in Team Member is assigned to.
	Mine     bool
	Tasks    domain.TaskProgress
	Selected bool
	// Saving tells that a Status Change of the User Story is being saved.
	Saving bool
}

// Assignee is an Assignee as shown in a row or the detail view.
type Assignee struct {
	Name     string
	Initials string
	// Mine marks the logged-in Team Member.
	Mine bool
}

// NewKanban returns the Kanban Board of p with the given User Stories, seen
// by the Team Member with ID me. The first tab is active.
func NewKanban(p domain.Project, stories []domain.UserStory, me int64) Kanban {
	k := UnloadedKanban(p, me)
	k.loaded = true
	return k.withStories(stories)
}

// UnloadedKanban returns the Kanban Board of p, seen by the Team Member with
// ID me, before its User Stories have loaded. The zero Kanban takes no
// results.
func UnloadedKanban(p domain.Project, me int64) Kanban {
	k := Kanban{project: p, me: me, cursor: map[int64]int64{}}
	for _, s := range p.UserStoryStatuses {
		if !s.Archived {
			k.statuses = append(k.statuses, s)
		}
	}
	return k
}

// Loaded reports whether the User Stories are loaded.
func (k Kanban) Loaded() bool { return k.loaded }

// LoadUserStories returns the effect of loading the User Stories.
func (k Kanban) LoadUserStories() (Kanban, LoadKanban) {
	k.loadGen++
	k.refreshing = k.loaded
	return k, LoadKanban{ProjectID: k.project.ID, Generation: k.loadGen, Refresh: k.loaded}
}

// UserStoriesCurrent reports whether the result of load still counts.
func (k Kanban) UserStoriesCurrent(load LoadKanban) bool {
	return k.project.ID != 0 && load.ProjectID == k.project.ID && load.Generation == k.loadGen
}

// UserStoriesLoaded takes the User Stories from the result of load, like
// Reload once they were loaded before, or returns false if the result no
// longer counts.
func (k Kanban) UserStoriesLoaded(load LoadKanban, stories []domain.UserStory) (Kanban, bool) {
	if !k.UserStoriesCurrent(load) {
		return k, false
	}
	k.refreshing = false
	if k.loaded {
		return k.Reload(stories), true
	}
	k.loaded = true
	return k.withStories(stories), true
}

// UserStoriesFailed ends the load that failed, or returns false if the
// failure no longer counts.
func (k Kanban) UserStoriesFailed(load LoadKanban) (Kanban, bool) {
	if !k.UserStoriesCurrent(load) {
		return k, false
	}
	k.refreshing = false
	return k, true
}

// Refreshing reports whether the shown board is being reloaded while it
// stays visible.
func (k Kanban) Refreshing() bool { return k.refreshing }

// CancelLoads drops the load on its way: its result no longer counts.
func (k Kanban) CancelLoads() Kanban {
	k.loadGen++
	k.refreshing = false
	return k
}

// ToggleMine switches between showing every User Story and only those
// assigned to the Team Member. The tabs stay, with their counts filtered. It
// is never persisted.
func (k Kanban) ToggleMine() Kanban { return k.WithMine(!k.mine) }

// WithMine shows only the User Stories of the Team Member if mine is set,
// like ToggleMine.
func (k Kanban) WithMine(mine bool) Kanban {
	k.mine = mine
	return k.regrouped()
}

// Mine reports whether only the User Stories of the Team Member are shown.
func (k Kanban) Mine() bool { return k.mine }

// Reload replaces the User Stories, keeping the active tab and the selected
// User Story, which the board follows into another tab if it moved there, or
// else selecting its neighbor. Descriptions count as outdated: they stay
// visible until LoadDetail has loaded them again.
func (k Kanban) Reload(stories []domain.UserStory) Kanban {
	k.descriptions = k.descriptions.reloaded()
	sel, ok := k.selected()
	k = k.withStories(stories)
	if !ok || slices.ContainsFunc(k.active(), func(s domain.UserStory) bool { return s.ID == sel.ID }) {
		return k
	}
	for id, list := range k.byStatus {
		if slices.ContainsFunc(list, func(s domain.UserStory) bool { return s.ID == sel.ID }) {
			return k.follow(sel.ID, id)
		}
	}
	return k
}

func (k Kanban) withStories(stories []domain.UserStory) Kanban {
	k.stories = stories
	return k.regrouped()
}

// regrouped groups the User Stories by the Status they show. In a Status
// whose selected User Story left it, the next User Story is selected, or else
// the previous one.
func (k Kanban) regrouped() Kanban {
	old := k.byStatus
	k.cursor = maps.Clone(k.cursor) // never share the map with the receiver
	k.byStatus = map[int64][]domain.UserStory{}
	for _, s := range k.stories {
		s.Status = k.saves.status(UserStoryItem, s.ID, s.Status)
		s.Assignees = k.saves.assignees(s.ID, s.Assignees)
		if k.mine && !s.AssignedTo(k.me) {
			continue
		}
		k.byStatus[s.Status] = append(k.byStatus[s.Status], s)
	}
	for _, list := range k.byStatus {
		slices.SortStableFunc(list, func(a, b domain.UserStory) int { return cmp.Compare(a.KanbanOrder, b.KanbanOrder) })
	}
	for id, sel := range k.cursor {
		if containsID(k.byStatus[id], sel) {
			continue
		}
		delete(k.cursor, id)
		if next, ok := neighbor(old[id], sel, k.byStatus[id]); ok {
			k.cursor[id] = next
		}
	}
	return k
}

func containsID(list []domain.UserStory, id int64) bool {
	return slices.ContainsFunc(list, func(s domain.UserStory) bool { return s.ID == id })
}

// neighbor returns the ID of the User Story nearest to the one with ID
// gone in old that is in list, looking forward first.
func neighbor(old []domain.UserStory, gone int64, list []domain.UserStory) (int64, bool) {
	at := slices.IndexFunc(old, func(s domain.UserStory) bool { return s.ID == gone })
	if at < 0 {
		return 0, false
	}
	for j := at + 1; j < len(old); j++ {
		if containsID(list, old[j].ID) {
			return old[j].ID, true
		}
	}
	for j := at - 1; j >= 0; j-- {
		if containsID(list, old[j].ID) {
			return old[j].ID, true
		}
	}
	return 0, false
}

// Tabs returns the Status tabs in Taiga order.
func (k Kanban) Tabs() []Tab {
	tabs := make([]Tab, len(k.statuses))
	for i, s := range k.statuses {
		tabs[i] = Tab{
			Name:     s.Name,
			Color:    s.Color,
			Count:    len(k.byStatus[s.ID]),
			WIPLimit: s.WIPLimit,
			Active:   i == k.tab,
		}
	}
	return tabs
}

// ActiveStatus returns the Status of the active tab, or the zero Status if
// the project has no active Statuses.
func (k Kanban) ActiveStatus() domain.Status {
	if len(k.statuses) == 0 {
		return domain.Status{}
	}
	return k.statuses[k.tab]
}

// Empty reports whether the board has no User Stories at all.
func (k Kanban) Empty() bool {
	for _, s := range k.statuses {
		if len(k.byStatus[s.ID]) > 0 {
			return false
		}
	}
	return true
}

// Rows returns the User Stories of the active tab in Kanban order.
func (k Kanban) Rows() []Row {
	list := k.active()
	sel := k.SelectedIndex()
	rows := make([]Row, len(list))
	for i, s := range list {
		rows[i] = Row{
			ID:        s.ID,
			Ref:       s.Ref,
			Title:     s.Title,
			Assignees: k.assignees(s),
			Mine:      s.AssignedTo(k.me),
			Tasks:     s.Tasks,
			Selected:  i == sel,
			Saving:    k.saves.saving(UserStoryItem, s.ID),
		}
	}
	return rows
}

// NextTab activates the tab to the right, wrapping around.
func (k Kanban) NextTab() Kanban { return k.moveTab(1) }

// PrevTab activates the tab to the left, wrapping around.
func (k Kanban) PrevTab() Kanban { return k.moveTab(-1) }

func (k Kanban) moveTab(d int) Kanban {
	if n := len(k.statuses); n > 0 {
		k.tab = (k.tab + d + n) % n
	}
	return k
}

// CursorDown selects the next User Story of the active tab.
func (k Kanban) CursorDown() Kanban { return k.moveCursor(1) }

// CursorUp selects the previous User Story of the active tab.
func (k Kanban) CursorUp() Kanban { return k.moveCursor(-1) }

func (k Kanban) moveCursor(d int) Kanban {
	list := k.active()
	if len(list) == 0 {
		return k
	}
	i := max(0, min(len(list)-1, k.SelectedIndex()+d))
	return k.withCursor(k.statuses[k.tab].ID, list[i].ID)
}

// withCursor selects the User Story with the given ID in the Status with
// the given ID.
func (k Kanban) withCursor(statusID, storyID int64) Kanban {
	k.cursor = maps.Clone(k.cursor) // never share the map with the receiver
	if k.cursor == nil {
		k.cursor = map[int64]int64{}
	}
	k.cursor[statusID] = storyID
	return k
}

// follow activates the tab of the Status with the given ID and selects the
// User Story with the given ID there.
func (k Kanban) follow(storyID, statusID int64) Kanban {
	i := slices.IndexFunc(k.statuses, func(s domain.Status) bool { return s.ID == statusID })
	if i < 0 {
		return k
	}
	k.tab = i
	return k.withCursor(statusID, storyID)
}

// selected returns the selected User Story, or false if the active tab is
// empty.
func (k Kanban) selected() (domain.UserStory, bool) {
	i := k.SelectedIndex()
	if i < 0 {
		return domain.UserStory{}, false
	}
	return k.active()[i], true
}

// PickStatus moves the selected User Story to the Status with the given
// ID right away, with the cursor following it into its new tab, and
// returns the effect of saving the change, which the App records in sent
// once it sends it. It returns a Rejection instead if the change cannot be
// made, and neither if no User Story is selected.
func (k Kanban) PickStatus(id int64, sent Sent) (Kanban, Effect, error) {
	it, ok := k.selectedItem()
	if !ok {
		return k, nil, nil
	}
	sv, save, err := k.saves.changeStatus(k.project, sent, it, id)
	if err != nil {
		return k, nil, err
	}
	k.saves = sv
	return k.regrouped().follow(it.id, save.Status), save, nil
}

// AssignToMe makes the Team Member the Assignee of the selected User Story
// right away, keeping the other Assignees, and returns the effect of saving
// the change, which the App records in sent once it sends it. It returns a
// Rejection instead if the change cannot be made, and neither if no User
// Story is selected.
func (k Kanban) AssignToMe(sent Sent) (Kanban, Effect, error) {
	it, ok := k.selectedItem()
	if !ok {
		return k, nil, nil
	}
	sv, eff, err := k.saves.assignToMe(k.project, k.me, sent, it, ConfirmAssignToMe{})
	if err != nil {
		return k, nil, err
	}
	k.saves = sv
	return k.regrouped(), eff, nil
}

// StatusOptions returns the Statuses the selected User Story can be moved
// to, or a Rejection if it cannot be changed right now. It returns no
// Statuses if no User Story is selected.
func (k Kanban) StatusOptions(sent Sent) (StatusOptions, error) {
	it, ok := k.selectedItem()
	if !ok {
		return StatusOptions{}, nil
	}
	return k.saves.options(k.project, sent, it)
}

// selectedItem returns the selected User Story as a work item.
func (k Kanban) selectedItem() (workItem, bool) {
	s, ok := k.selected()
	if !ok || !k.loaded {
		return workItem{}, false
	}
	return workItem{kind: UserStoryItem, id: s.ID, ref: s.Ref, status: s.Status, version: s.Version, assignees: s.Assignees}, true
}

// Saved takes the User Story as Taiga saved it from the result of
// save, or returns false if the result no longer counts.
func (k Kanban) Saved(save Save, saved domain.UserStory) (Kanban, bool) {
	var ok bool
	if k.saves, ok = k.saves.end(save); !ok {
		return k, false
	}
	return k.Replace(saved), true
}

// Replace takes the User Story as Taiga saved it if the board shows it,
// e.g. from a save whose result arrived after the Team Member came back to
// the board, so that the next change sends its current version.
func (k Kanban) Replace(saved domain.UserStory) Kanban {
	if i := slices.IndexFunc(k.stories, func(s domain.UserStory) bool { return s.ID == saved.ID }); i >= 0 {
		k.stories = slices.Clone(k.stories) // never share the list with the receiver
		k.stories[i] = saved
	}
	return k.regrouped()
}

// NotSaved undoes the change of save, with the cursor following the
// User Story back if it is selected, and returns the effect of reloading
// the board if the failure asks for it. It returns false if the result no
// longer counts.
func (k Kanban) NotSaved(save Save, f SaveFailure) (Kanban, Effect, bool) {
	var ok bool
	if k.saves, ok = k.saves.end(save); !ok {
		return k, nil, false
	}
	sel, ok := k.selected()
	k = k.regrouped()
	if ok && sel.ID == save.ID {
		if i := slices.IndexFunc(k.stories, func(s domain.UserStory) bool { return s.ID == save.ID }); i >= 0 {
			k = k.follow(save.ID, k.stories[i].Status)
		}
	}
	return k, reloadOn(f), true
}

// DropSaves forgets the Status Changes being saved, e.g. when the Team
// Member leaves the board. Their results no longer count.
func (k Kanban) DropSaves() Kanban {
	k.saves = k.saves.dropped()
	return k.regrouped()
}

// Saving reports whether any Status Change is being saved.
func (k Kanban) Saving() bool { return k.saves.any() }

func (k Kanban) active() []domain.UserStory {
	if len(k.statuses) == 0 {
		return nil
	}
	return k.byStatus[k.statuses[k.tab].ID]
}

// SelectedIndex is the index of the selected row in Rows, -1 if the active
// tab is empty. Without an earlier selection it is the first row.
func (k Kanban) SelectedIndex() int {
	list := k.active()
	if len(list) == 0 {
		return -1
	}
	sel := k.cursor[k.statuses[k.tab].ID]
	return max(0, slices.IndexFunc(list, func(s domain.UserStory) bool { return s.ID == sel }))
}

func (k Kanban) assignees(s domain.UserStory) []Assignee {
	return assigneesOf(k.project, k.me, s.Assignees)
}

// assigneesOf returns the Assignees with the given IDs as members of p seen
// by the Team Member with ID me.
func assigneesOf(p domain.Project, me int64, ids []int64) []Assignee {
	if len(ids) == 0 {
		return nil
	}
	out := make([]Assignee, len(ids))
	for i, id := range ids {
		out[i] = Assignee{Name: "Unknown member", Initials: "??", Mine: id == me}
		if m, ok := p.Member(id); ok {
			out[i].Name, out[i].Initials = m.FullName, Initials(m.FullName)
		}
	}
	return out
}

// memberName returns the name of the Team Member with the given ID in p.
func memberName(p domain.Project, id int64) string {
	return assigneesOf(p, 0, []int64{id})[0].Name
}

// Initials returns two uppercase letters for a name: the first letters of
// its first and last word, or the first two letters of a single word.
func Initials(name string) string {
	words := strings.FieldsFunc(name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	var rs []rune
	switch len(words) {
	case 0:
		return "??"
	case 1:
		rs = []rune(words[0])
		if len(rs) > 2 {
			rs = rs[:2]
		}
	default:
		rs = []rune{[]rune(words[0])[0], []rune(words[len(words)-1])[0]}
	}
	return strings.ToUpper(string(rs))
}
