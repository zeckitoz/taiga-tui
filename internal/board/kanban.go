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
	byStatus map[int64][]domain.UserStory
	tab      int
	// cursor remembers the selected User Story ID per Status ID.
	cursor map[int64]int64
	// generation counts the loads of the User Stories; results of
	// description loads from an earlier generation are dropped.
	generation int
	// descriptions holds the descriptions by User Story ID.
	descriptions map[int64]description
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
}

// Assignee is an Assignee as shown in a row.
type Assignee struct {
	Initials string
	Mine     bool
}

// NewKanban returns the Kanban Board of p with the given User Stories, seen
// by the Team Member with ID me. The first tab is active.
func NewKanban(p domain.Project, stories []domain.UserStory, me int64) Kanban {
	k := Kanban{project: p, me: me, cursor: map[int64]int64{}, descriptions: map[int64]description{}}
	for _, s := range p.UserStoryStatuses {
		if !s.Archived {
			k.statuses = append(k.statuses, s)
		}
	}
	return k.withStories(stories)
}

// Reload replaces the User Stories, keeping the active tab and the selected
// User Story if it is still in that tab. Descriptions count as outdated:
// they stay visible until LoadDetail has loaded them again.
func (k Kanban) Reload(stories []domain.UserStory) Kanban {
	k.cursor = maps.Clone(k.cursor) // never share the map with the receiver
	k.generation++
	return k.withStories(stories)
}

func (k Kanban) withStories(stories []domain.UserStory) Kanban {
	k.byStatus = map[int64][]domain.UserStory{}
	for _, s := range stories {
		k.byStatus[s.Status] = append(k.byStatus[s.Status], s)
	}
	for id, list := range k.byStatus {
		slices.SortStableFunc(list, func(a, b domain.UserStory) int { return cmp.Compare(a.KanbanOrder, b.KanbanOrder) })
		if sel, ok := k.cursor[id]; ok && !slices.ContainsFunc(list, func(s domain.UserStory) bool { return s.ID == sel }) {
			delete(k.cursor, id)
		}
	}
	return k
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
	k.cursor = maps.Clone(k.cursor) // never share the map with the receiver
	k.cursor[k.statuses[k.tab].ID] = list[i].ID
	return k
}

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
	if len(s.Assignees) == 0 {
		return nil
	}
	out := make([]Assignee, len(s.Assignees))
	for i, id := range s.Assignees {
		out[i] = Assignee{Initials: "??", Mine: id == k.me}
		if m, ok := k.project.Member(id); ok {
			out[i].Initials = Initials(m.FullName)
		}
	}
	return out
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
