// PROTOTYPE - Kanban Board, three structurally different layouts (key V):
//
//	A  list grouped by Status (like the Taskboard, but grouped by Status instead of User Story)
//	B  real columns per Status, horizontally scrolling, collapsible columns
//	C  one tab per Status, list of the selected Status below
//
// All share the detail pane rule of the Taskboard (right >= paneMin, else below, no overlay).
package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var kanbanNames = []string{"A · grouped by Status", "B · columns", "C · Status tabs"}

// kStories returns the User Stories of one Status, honouring "only mine".
func (m model) kStories(st int) []*Story {
	var out []*Story
	for i := range stories {
		s := &stories[i]
		if s.Status != usStatuses[st].Name || (m.mine && !s.mine()) {
			continue
		}
		out = append(out, s)
	}
	return out
}

func usStatusIdx(name string) int {
	for i, s := range usStatuses {
		if s.Name == name {
			return i
		}
	}
	return 0
}

func assigneeInitials(ids []int) string {
	if len(ids) == 0 {
		return faint.Render("--")
	}
	var parts []string
	for _, id := range ids {
		if id == meID {
			parts = append(parts, mineSt.Render(initials(id)))
		} else {
			parts = append(parts, faint.Render(initials(id)))
		}
	}
	return strings.Join(parts, " ")
}

// storyLine renders one User Story as a list row (A and C).
func storyLine(s *Story, w int, sel bool) string {
	done, total := taskProgress(s.Ref)
	prog := progressBar(done, total)
	if total == 0 {
		prog = faint.Render("no tasks ")
	}
	progW := 14
	whoW := 10
	ref := fmt.Sprintf("#%-4d", s.Ref)
	titleW := w - 4 - len(ref) - 1 - whoW - progW
	if sel {
		return selStyle.Render(fit("    "+ref+" "+fit(s.Subject, titleW-1)+" "+fit(lipgloss.NewStyle().Render(stripInit(s.Assignees)), whoW)+fit(stripProg(done, total), progW), w))
	}
	marker := "    "
	if s.mine() {
		marker = "  " + mineSt.Render("▍") + " "
	}
	return marker + faint.Render(ref) + " " + fit(s.Subject, titleW-1) + " " + fit(assigneeInitials(s.Assignees), whoW) + fit(prog, progW)
}

func stripInit(ids []int) string {
	if len(ids) == 0 {
		return "--"
	}
	var p []string
	for _, id := range ids {
		p = append(p, initials(id))
	}
	return strings.Join(p, " ")
}

func stripProg(done, total int) string {
	if total == 0 {
		return "no tasks"
	}
	return strings.Repeat("▰", done) + strings.Repeat("▱", total-done) + fmt.Sprintf(" %d/%d", done, total)
}

func statusHeader(st int, n int, collapsed bool, w int, sel bool) string {
	s := usStatuses[st]
	arrow := "▾"
	if collapsed {
		arrow = "▸"
	}
	cnt := fmt.Sprintf("%d", n)
	cntR := faint.Render(cnt)
	if s.WIP > 0 {
		cnt = fmt.Sprintf("%d/%d WIP", n, s.WIP)
		cntR = faint.Render(cnt)
		if n > s.WIP {
			cntR = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Bold(true).Render(cnt)
		}
	}
	name := arrow + " ● " + s.Name
	if sel {
		return selStyle.Bold(true).Render(fit(name, w-len(cnt)-2)) + " " + cntR + " "
	}
	return colorSt(s.Color).Bold(true).Render(fit(name, w-len(cnt)-2)) + " " + cntR + " "
}

// ---------- A: grouped list ----------

type kItem struct {
	st    int
	story *Story // nil = Status header
}

func (m model) kItems() []kItem {
	var out []kItem
	for i := range usStatuses {
		out = append(out, kItem{st: i})
		if m.kCollapsed[i] {
			continue
		}
		for _, s := range m.kStories(i) {
			out = append(out, kItem{st: i, story: s})
		}
	}
	return out
}

func (m model) viewKanbanA(W, h int) string {
	items := m.kItems()
	listW, listH, paneW := m.split(W, h)
	var lines []string
	focus := 0
	for i, it := range items {
		sel := i == m.kIdx && !m.navFocus()
		if i == m.kIdx {
			focus = len(lines)
			if it.story == nil && i > 0 {
				focus++
			}
		}
		if it.story == nil {
			if i > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, statusHeader(it.st, len(m.kStories(it.st)), m.kCollapsed[it.st], listW, sel))
			continue
		}
		lines = append(lines, storyLine(it.story, listW, sel))
	}
	return m.withPane(lines, focus, W, h, listW, listH, paneW, m.storyPane)
}

func (m *model) kMoveA(dx, dy int) {
	items := m.kItems()
	cur := items[m.kIdx]
	switch {
	case dx < 0 && cur.story != nil:
		for i := m.kIdx; i >= 0; i-- {
			if items[i].story == nil {
				m.kIdx = i
				break
			}
		}
	case dx < 0 && !m.kCollapsed[cur.st]:
		m.kCollapsed[cur.st] = true
	case dx > 0 && cur.story == nil && m.kCollapsed[cur.st]:
		m.kCollapsed[cur.st] = false
	}
	m.kIdx = max(0, min(m.kIdx+dy, len(m.kItems())-1))
}

// ---------- B: columns ----------

const colCollapsedW = 5
const colMinW = 26
const cardH = 4 // 2 title lines + meta line + gap

func (m model) colWidths(W int) []int {
	n := len(usStatuses)
	open := 0
	for i := 0; i < n; i++ {
		if !m.kCollapsed[i] {
			open++
		}
	}
	free := W - (n-open)*colCollapsedW
	cw := colMinW
	if open > 0 {
		cw = max(colMinW, free/open)
	}
	ws := make([]int, n)
	for i := range ws {
		if m.kCollapsed[i] {
			ws[i] = colCollapsedW
		} else {
			ws[i] = cw
		}
	}
	return ws
}

func (m model) viewKanbanB(W, h int) string {
	_, listH, paneW := m.split(W, h)
	boardW := W
	if paneW > 0 {
		boardW = W - paneW - 1
	}
	ws := m.colWidths(boardW)
	// horizontal scroll: first visible column so that kCol fits
	first := 0
	for {
		used := 0
		for i := first; i <= m.kCol; i++ {
			used += ws[i]
		}
		if used <= boardW || first == m.kCol {
			break
		}
		first++
	}
	var cols []string
	used := 0
	for i := first; i < len(usStatuses); i++ {
		if used+ws[i] > boardW {
			break
		}
		cols = append(cols, m.column(i, ws[i], listH))
		used += ws[i]
	}
	board := lipgloss.JoinHorizontal(lipgloss.Top, cols...)
	lines := strings.Split(board, "\n")
	for i := range lines {
		lines[i] = fit(lines[i], boardW)
	}
	more := ""
	if first > 0 {
		more = "◀ "
	}
	if used < sum(ws[first:]) {
		more += "▶"
	}
	if more != "" && len(lines) > 0 {
		l := lines[0]
		lines[0] = ansi.Truncate(l, boardW-3, "") + faint.Render(fit(more, 3))
	}
	body := strings.Join(lines, "\n")
	switch {
	case m.hidePane:
		return body
	case paneW > 0:
		sep := faint.Render(strings.Repeat("│\n", h-1) + "│")
		return lipgloss.JoinHorizontal(lipgloss.Top, body, sep, m.storyPane(paneW, h))
	default:
		return lipgloss.JoinVertical(lipgloss.Left, body, faint.Render(strings.Repeat("─", W)), m.storyPane(W, h-listH-1))
	}
}

func sum(xs []int) int {
	n := 0
	for _, x := range xs {
		n += x
	}
	return n
}

func (m model) column(st, w, h int) string {
	s := usStatuses[st]
	list := m.kStories(st)
	focused := st == m.kCol && !m.navFocus()
	var lines []string
	if m.kCollapsed[st] {
		head := colorSt(s.Color).Render("▸●")
		if focused {
			head = selStyle.Render("▸●")
		}
		lines = append(lines, " "+head+" ", faint.Render(fmt.Sprintf(" %-3d", len(list))))
		for _, r := range s.Name {
			lines = append(lines, faint.Render("  "+string(r)))
		}
	} else {
		lines = append(lines, statusHeader(st, len(list), false, w-1, focused && m.kRow < 0))
		lines = append(lines, faint.Render(strings.Repeat("─", w-1)))
		// vertical scroll inside the focused column
		perPage := max(1, (h-2)/cardH)
		top := 0
		if focused && m.kRow >= perPage {
			top = m.kRow - perPage + 1
		}
		for i := top; i < len(list) && i < top+perPage; i++ {
			lines = append(lines, m.card(list[i], w-1, focused && i == m.kRow)...)
		}
		if len(list) == 0 {
			lines = append(lines, faint.Render(" empty"))
		}
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	lines = lines[:h]
	for i := range lines {
		lines[i] = fit(lines[i], w-1) + faint.Render("│")
	}
	return strings.Join(lines, "\n")
}

func (m model) card(s *Story, w int, sel bool) []string {
	title := lipgloss.NewStyle().Width(w - 2).Render(s.Subject)
	tl := strings.Split(title, "\n")
	if len(tl) > 2 {
		tl = tl[:2]
		tl[1] = fit(strings.TrimRight(tl[1], " ")+"…", w-2)
	}
	for len(tl) < 2 {
		tl = append(tl, "")
	}
	done, total := taskProgress(s.Ref)
	ref := fmt.Sprintf("#%d", s.Ref)
	marker := " "
	if s.mine() {
		marker = mineSt.Render("▍")
	}
	if sel {
		meta := fit(ref+"  "+stripInit(s.Assignees), w-12) + fit(stripProg(done, total), 11)
		return []string{
			selStyle.Render(fit(" "+tl[0], w)),
			selStyle.Render(fit(" "+tl[1], w)),
			selStyle.Render(fit(" "+meta, w)),
			"",
		}
	}
	prog := faint.Render("no tasks")
	if total > 0 {
		prog = faint.Render(fmt.Sprintf("%d/%d", done, total))
	}
	meta := faint.Render(ref) + "  " + assigneeInitials(s.Assignees)
	return []string{
		marker + fit(tl[0], w-1),
		marker + fit(tl[1], w-1),
		marker + fit(meta, w-10) + fit(prog, 8),
		"",
	}
}

func (m *model) kMoveB(dx, dy int) {
	if dx != 0 {
		m.kCol = max(0, min(len(usStatuses)-1, m.kCol+dx))
		m.kRow = min(m.kRow, len(m.kStories(m.kCol))-1)
		if m.kRow < 0 && len(m.kStories(m.kCol)) > 0 && !m.kCollapsed[m.kCol] {
			m.kRow = 0
		}
	}
	if dy != 0 && !m.kCollapsed[m.kCol] {
		m.kRow = max(-1, min(len(m.kStories(m.kCol))-1, m.kRow+dy))
	}
}

// ---------- C: Status tabs ----------

func (m model) viewKanbanC(W, h int) string {
	// tab bar
	var tabs []string
	for i, s := range usStatuses {
		n := len(m.kStories(i))
		t := fmt.Sprintf(" ● %s %d ", s.Name, n)
		if i == m.kTab {
			tabs = append(tabs, lipgloss.NewStyle().Background(lipgloss.Color(s.Color)).Foreground(lipgloss.Color("#1E1E1E")).Bold(true).Render(t))
		} else {
			tabs = append(tabs, colorSt(s.Color).Render(t))
		}
	}
	bar := strings.Join(tabs, faint.Render("│"))
	if lipgloss.Width(bar) > W { // shorten inactive tabs to their count
		tabs = tabs[:0]
		for i, s := range usStatuses {
			n := len(m.kStories(i))
			if i == m.kTab {
				tabs = append(tabs, lipgloss.NewStyle().Background(lipgloss.Color(s.Color)).Foreground(lipgloss.Color("#1E1E1E")).Bold(true).Render(fmt.Sprintf(" %s %d ", s.Name, n)))
			} else {
				tabs = append(tabs, colorSt(s.Color).Render(fmt.Sprintf(" ●%d ", n)))
			}
		}
		bar = strings.Join(tabs, faint.Render("│"))
	}
	bar = fit(bar, W)
	h -= 2
	listW, listH, paneW := m.split(W, h)
	list := m.kStories(m.kTab)
	var lines []string
	for i, s := range list {
		lines = append(lines, storyLine(s, listW, i == m.kRow && !m.navFocus()))
	}
	if len(list) == 0 {
		lines = append(lines, faint.Render("  No user stories in this status for this filter."))
	}
	return lipgloss.JoinVertical(lipgloss.Left, bar, faint.Render(strings.Repeat("─", W)),
		m.withPane(lines, max(0, m.kRow), W, h, listW, listH, paneW, m.storyPane))
}

func (m *model) kMoveC(dx, dy int) {
	if dx != 0 {
		m.kTab = (m.kTab + dx + len(usStatuses)) % len(usStatuses)
		m.kRow = 0
		m.sc.top = 0
	}
	m.kRow = max(0, min(len(m.kStories(m.kTab))-1, m.kRow+dy))
}

// ---------- selection and detail pane ----------

// kSelected returns the selected User Story (nil on a Status header) and the Status in focus.
func (m model) kSelected() (*Story, int) {
	switch m.kVar {
	case 0:
		items := m.kItems()
		if m.kIdx < len(items) {
			return items[m.kIdx].story, items[m.kIdx].st
		}
	case 1:
		l := m.kStories(m.kCol)
		if m.kRow >= 0 && m.kRow < len(l) && !m.kCollapsed[m.kCol] {
			return l[m.kRow], m.kCol
		}
		return nil, m.kCol
	case 2:
		l := m.kStories(m.kTab)
		if m.kRow < len(l) {
			return l[m.kRow], m.kTab
		}
		return nil, m.kTab
	}
	return nil, 0
}

func (m model) storyPane(w, h int) string {
	st := lipgloss.NewStyle().Width(w).Height(h).Padding(0, 1)
	s, sti := m.kSelected()
	if s == nil {
		ss := usStatuses[sti]
		n := len(m.kStories(sti))
		lines := []string{colorSt(ss.Color).Bold(true).Render("● " + ss.Name), "", fmt.Sprintf("%d user stories", n)}
		if ss.WIP > 0 {
			lines = append(lines, fmt.Sprintf("WIP limit %d (not enforced by Taiga)", ss.WIP))
		}
		lines = append(lines, "", faint.Render("space: collapse / expand status"))
		return st.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
	}
	ss := usStatuses[usStatusIdx(s.Status)]
	var who []string
	for _, id := range s.Assignees {
		n := assigneeName(id)
		if id == meID {
			n = mineSt.Render(n + " (you)")
		}
		who = append(who, n)
	}
	if len(who) == 0 {
		who = []string{faint.Render("unassigned")}
	}
	sp := faint.Render("not in a sprint")
	if s.Sprint >= 0 {
		sp = sprints[s.Sprint].Name
	}
	done, total := taskProgress(s.Ref)
	desc := s.Desc
	if desc == "" {
		desc = faint.Render("No description.")
	}
	lines := []string{
		bold.Width(w - 2).Render(fmt.Sprintf("User Story #%d  %s", s.Ref, s.Subject)),
		"",
		label("Status") + colorSt(ss.Color).Render("● "+ss.Name),
		label("Assignees") + who[0],
	}
	for _, x := range who[1:] {
		lines = append(lines, label("")+x)
	}
	lines = append(lines,
		label("Sprint")+sp,
		label("Tasks")+progressBar(done, total),
		"",
		lipgloss.NewStyle().Width(w-2).Render(desc),
	)
	return st.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}
