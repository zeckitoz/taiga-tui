// PROTOTYPE - Taskboard as decided in "Prototype: Wie sieht das Taskboard im Terminal aus?" (variant C):
// collapsible list grouped by User Story, Status as coloured label, detail pane right (>= paneMin) or below.
package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// Row is one User Story of the Sprint (nil = Tasks without User Story) with its Tasks per Status.
type Row struct {
	Story *Story
	Cells [][]Task
}

func (r Row) count() int {
	n := 0
	for _, c := range r.Cells {
		n += len(c)
	}
	return n
}

func (r Row) flat() []Task {
	var out []Task
	for _, c := range r.Cells {
		out = append(out, c...)
	}
	return out
}

func (r Row) key() int {
	if r.Story == nil {
		return -1
	}
	return r.Story.Ref
}

func (r Row) title() string {
	if r.Story == nil {
		return "Tasks without User Story"
	}
	return fmt.Sprintf("#%d %s", r.Story.Ref, r.Story.Subject)
}

func (m model) statuses() []Status {
	if m.many {
		return manyStatuses
	}
	return baseStatuses
}

func (m model) statusOf(t Task) string {
	if !m.many {
		if b, ok := collapseToBase[t.Status]; ok {
			return b
		}
	}
	return t.Status
}

func (m model) statusColor(name string) string {
	for _, s := range manyStatuses {
		if s.Name == name {
			return s.Color
		}
	}
	return "#888888"
}

func (m model) rows() []Row {
	sts := m.statuses()
	idx := map[string]int{}
	for i, s := range sts {
		idx[s.Name] = i
	}
	build := func(st *Story, ref int) Row {
		r := Row{Story: st, Cells: make([][]Task, len(sts))}
		for _, t := range tasks {
			if t.Story != ref || (m.mine && t.Assignee != meID) {
				continue
			}
			i := idx[m.statusOf(t)]
			r.Cells[i] = append(r.Cells[i], t)
		}
		return r
	}
	var out []Row
	for i := range stories {
		if stories[i].Sprint != m.sprint {
			continue
		}
		r := build(&stories[i], stories[i].Ref)
		if m.mine && r.count() == 0 {
			continue
		}
		out = append(out, r)
	}
	if m.sprint == orphanSprint {
		if r := build(nil, 0); r.count() > 0 {
			out = append(out, r)
		}
	}
	return out
}

func taskByRef(ref int) *Task {
	for i := range tasks {
		if tasks[i].Ref == ref {
			return &tasks[i]
		}
	}
	return nil
}

type cItem struct {
	row  Row
	task *Task
}

func (m model) cItems(rows []Row) []cItem {
	var out []cItem
	for _, r := range rows {
		out = append(out, cItem{row: r})
		if m.collapsed[r.key()] {
			continue
		}
		for _, t := range r.flat() {
			t := t
			out = append(out, cItem{row: r, task: &t})
		}
	}
	return out
}

func (m *model) cClamp() {
	n := len(m.cItems(m.rows()))
	m.cIdx = max(0, min(m.cIdx, n-1))
}

func (m model) cSel() int {
	items := m.cItems(m.rows())
	if m.cIdx < len(items) && items[m.cIdx].task != nil {
		return items[m.cIdx].task.Ref
	}
	return 0
}

func (m model) cOnStory() bool {
	items := m.cItems(m.rows())
	return m.cIdx < len(items) && items[m.cIdx].task == nil
}

func (m *model) cToggle() {
	rows := m.rows()
	items := m.cItems(rows)
	if m.cIdx >= len(items) {
		return
	}
	key := items[m.cIdx].row.key()
	m.collapsed[key] = !m.collapsed[key]
	for i, it := range m.cItems(rows) {
		if it.task == nil && it.row.key() == key {
			m.cIdx = i
		}
	}
}

func (m *model) cMove(dx, dy int) {
	items := m.cItems(m.rows())
	if len(items) == 0 {
		return
	}
	cur := items[m.cIdx]
	switch {
	case dx < 0 && cur.task != nil:
		for i := m.cIdx; i >= 0; i-- {
			if items[i].task == nil {
				m.cIdx = i
				break
			}
		}
	case dx < 0 && !m.collapsed[cur.row.key()]:
		m.cToggle()
	case dx > 0 && cur.task == nil && m.collapsed[cur.row.key()]:
		m.cToggle()
	}
	m.cIdx += dy
	m.cClamp()
}

func (m model) viewTaskboard(W, h int) string {
	rows := m.rows()
	items := m.cItems(rows)
	listW, listH, paneW := m.split(W, h)
	stW, whoW := 17, 18
	if listW < 70 {
		whoW = 4
	}
	var lines []string
	focus := 0
	for i, it := range items {
		sel := i == m.cIdx && !m.navFocus()
		if i == m.cIdx {
			focus = len(lines)
			if it.task == nil && i > 0 {
				focus++
			}
		}
		if it.task == nil {
			r := it.row
			arrow := "▾"
			if m.collapsed[r.key()] {
				arrow = "▸"
			}
			done := 0
			for _, x := range r.flat() {
				if m.statusOf(x) == "Closed" {
					done++
				}
			}
			bar := faint.Render("no tasks ")
			if n := r.count(); n > 0 {
				bar = progressBar(done, n)
			}
			t := fmt.Sprintf("%s %s", arrow, r.title())
			tw := listW - lipgloss.Width(bar)
			line := accent.Render(fit(t, tw-1)) + " " + bar
			if sel {
				line = selStyle.Bold(true).Render(fit(t, tw-1)) + " " + bar
			}
			if i > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, line)
			continue
		}
		x := *it.task
		st := m.statusOf(x)
		who := assigneeName(x.Assignee)
		if whoW < 8 {
			who = initials(x.Assignee)
		}
		ref := fmt.Sprintf("#%-4d", x.Ref)
		titleW := listW - 4 - len(ref) - 1 - stW - whoW
		if sel {
			lines = append(lines, selStyle.Render(fit("    "+ref+" "+fit(x.Subject, titleW-1)+" "+fit("● "+st, stW)+fit(who, whoW-1), listW)))
			continue
		}
		marker := "    "
		whoR := faint.Render(who)
		if x.Assignee == meID {
			marker = "  " + mineSt.Render("▍") + " "
			whoR = mineSt.Render(who)
		}
		lines = append(lines, marker+faint.Render(ref)+" "+fit(x.Subject, titleW-1)+" "+colorSt(m.statusColor(st)).Render(fit("● "+st, stW))+fit(whoR, whoW-1)+" ")
	}
	if len(items) == 0 {
		lines = append(lines, faint.Render("  No tasks in this sprint for this filter."))
	}
	return m.withPane(lines, focus, W, h, listW, listH, paneW, m.taskPane)
}

func (m model) taskPane(w, h int) string {
	st := lipgloss.NewStyle().Width(w).Height(h).Padding(0, 1)
	items := m.cItems(m.rows())
	if m.cIdx >= len(items) {
		return st.Render("")
	}
	it := items[m.cIdx]
	if it.task == nil {
		r := it.row
		var counts []string
		for i, s := range m.statuses() {
			if n := len(r.Cells[i]); n > 0 {
				counts = append(counts, colorSt(s.Color).Render("● ")+fmt.Sprintf("%-16s %d", s.Name, n))
			}
		}
		return st.Render(lipgloss.JoinVertical(lipgloss.Left,
			append([]string{bold.Width(w - 2).Render(r.title()), ""}, counts...)...))
	}
	t := it.task
	x := m.statusOf(*t)
	us := "no User Story"
	if s := storyByRef(t.Story); s != nil {
		us = fmt.Sprintf("#%d %s", s.Ref, s.Subject)
	}
	who := assigneeName(t.Assignee)
	if t.Assignee == meID {
		who = mineSt.Render(who + " (you)")
	}
	desc := t.Desc
	if desc == "" {
		desc = faint.Render("No description.")
	}
	return st.Render(lipgloss.JoinVertical(lipgloss.Left,
		bold.Width(w-2).Render(fmt.Sprintf("Task #%d  %s", t.Ref, t.Subject)),
		"",
		label("Status")+colorSt(m.statusColor(x)).Render("● "+x),
		label("Assignee")+who,
		label("User Story")+fit(us, w-13),
		"",
		lipgloss.NewStyle().Width(w-2).Render(desc),
	))
}

// ---------- shared by both boards ----------

const paneMin = 110 // from this body width the detail pane sits right of the list
const bottomH = 9   // below paneMin the detail pane sits below the list

// split computes list width/height and pane width for the body.
func (m model) split(W, h int) (listW, listH, paneW int) {
	listW, listH = W, h
	if m.hidePane {
		return
	}
	if W >= paneMin {
		paneW = min(52, W*2/5)
		listW = W - paneW - 1
		return
	}
	listH = max(3, h-bottomH-1)
	return
}

func (m model) withPane(lines []string, focus, W, h, listW, listH, paneW int, pane func(w, h int) string) string {
	list := m.window(lines, focus, listH)
	for i := range list {
		list[i] = fit(list[i], listW)
	}
	switch {
	case m.hidePane:
		return strings.Join(list, "\n")
	case paneW > 0:
		sep := faint.Render(strings.Repeat("│\n", h-1) + "│")
		return lipgloss.JoinHorizontal(lipgloss.Top, strings.Join(list, "\n"), sep, pane(paneW, h))
	default:
		return lipgloss.JoinVertical(lipgloss.Left, strings.Join(list, "\n"),
			faint.Render(strings.Repeat("─", W)), pane(W, h-listH-1))
	}
}

func progressBar(done, n int) string {
	if n == 0 {
		return faint.Render("no tasks ")
	}
	return colorSt("#A8E440").Render(strings.Repeat("▰", done)) + faint.Render(strings.Repeat("▱", n-done)) + faint.Render(fmt.Sprintf(" %d/%d ", done, n))
}
