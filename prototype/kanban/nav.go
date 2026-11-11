// PROTOTYPE - three ways to switch between Taskboard, Kanban Board, Sprint and project (key N):
//
//	1 tabs in the header; tab switches board, p/s open a full-body picker (no overlay)
//	2 sidebar with projects, boards and sprints; tab moves focus between sidebar and board
//	3 breadcrumb header plus a command palette (":" or ctrl+k) with fuzzy filter
package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var navNames = []string{"1 · tabs + pickers", "2 · sidebar", "3 · command palette"}
var navHint = []string{"tab board · p project · s sprint", "tab sidebar", ": or ctrl+k go to"}

type navItem struct {
	kind     int // 0 heading, 1 board, 2 sprint, 3 project
	idx      int
	label    string
	note     string
	disabled bool
	current  bool
}

func (m model) navFocus() bool {
	return (m.nav == 0 && m.picker != 0) || (m.nav == 1 && m.sbFocus) || (m.nav == 2 && m.palOpen)
}

func sprintNote(i int) string {
	switch {
	case i == currentSprint:
		return "current"
	case sprints[i].Closed:
		return "closed"
	default:
		return "upcoming"
	}
}

func projectNote(p Project) string {
	switch {
	case p.Backlog && p.Kanban:
		return "Taskboard · Kanban"
	case p.Kanban:
		return "Kanban only"
	default:
		return "Taskboard only"
	}
}

func (m model) boardItems() []navItem {
	p := projects[m.proj]
	return []navItem{
		{kind: 1, idx: boardTask, label: "Taskboard", disabled: !p.Backlog, current: m.board == boardTask},
		{kind: 1, idx: boardKanban, label: "Kanban Board", disabled: !p.Kanban, current: m.board == boardKanban},
	}
}

func (m model) sprintItems() []navItem {
	if !projects[m.proj].Backlog || !projects[m.proj].Data {
		return nil
	}
	var out []navItem
	for i := len(sprints) - 1; i >= 0; i-- { // newest first, like Taiga
		out = append(out, navItem{kind: 2, idx: i, label: sprints[i].Name, note: sprintNote(i), current: m.board == boardTask && m.sprint == i})
	}
	return out
}

func (m model) projectItems() []navItem {
	var out []navItem
	for i, p := range projects {
		note := projectNote(p)
		if i == defaultProject {
			note = "★ " + note
		}
		out = append(out, navItem{kind: 3, idx: i, label: p.Name, note: note, current: m.proj == i})
	}
	return out
}

func (m *model) choose(it navItem) {
	switch it.kind {
	case 1:
		if !it.disabled {
			m.board = it.idx
		}
	case 2:
		m.sprint = it.idx
		m.board = boardTask
		m.cIdx = 0
	case 3:
		m.proj = it.idx
		m.sprint = currentSprint
		p := projects[m.proj]
		if m.board == boardKanban && !p.Kanban {
			m.board = boardTask
		}
		if m.board == boardTask && !p.Backlog {
			m.board = boardKanban
		}
		m.cIdx, m.kIdx, m.kRow, m.kCol, m.kTab = 0, 0, 0, 0, 0
	}
	m.sc.top = 0
	m.clamp()
}

// navKey handles keys of the active navigation variant. It returns true if it consumed the key.
func (m *model) navKey(k string) bool {
	switch m.nav {
	case 0:
		if m.picker != 0 {
			items := m.pickerItems()
			switch k {
			case "up", "k":
				m.pIdx = max(0, m.pIdx-1)
			case "down", "j":
				m.pIdx = min(len(items)-1, m.pIdx+1)
			case "enter":
				if m.pIdx < len(items) {
					m.choose(items[m.pIdx])
				}
				m.picker = 0
			case "esc", "q", "p", "s":
				m.picker = 0
			}
			return true
		}
		switch k {
		case "tab", "shift+tab":
			other := 1 - m.board
			if !m.boardItems()[other].disabled {
				m.board = other
				m.sc.top = 0
			}
			return true
		case "p":
			m.picker, m.pIdx = 1, m.proj
			return true
		case "s":
			if len(m.sprintItems()) > 0 {
				m.picker = 2
				m.pIdx = len(sprints) - 1 - m.sprint
			}
			return true
		}
	case 1:
		if k == "tab" || k == "shift+tab" {
			m.sbFocus = !m.sbFocus
			if m.sbFocus {
				m.sbIdx = m.sbCurrent()
			}
			return true
		}
		if !m.sbFocus {
			return false
		}
		items := m.sbItems()
		switch k {
		case "up", "k":
			for i := m.sbIdx - 1; i >= 0; i-- {
				if items[i].kind != 0 {
					m.sbIdx = i
					break
				}
			}
		case "down", "j":
			for i := m.sbIdx + 1; i < len(items); i++ {
				if items[i].kind != 0 {
					m.sbIdx = i
					break
				}
			}
		case "enter", "right", "l":
			it := items[m.sbIdx]
			if !it.disabled {
				m.choose(it)
				m.sbFocus = false
			}
		case "esc":
			m.sbFocus = false
		case "q":
			return false
		}
		return true
	case 2:
		if !m.palOpen {
			if k == ":" || k == "ctrl+k" {
				m.palOpen, m.palQ, m.palIdx = true, "", 0
				return true
			}
			return false
		}
		items := m.palItems()
		switch k {
		case "esc":
			m.palOpen = false
		case "up":
			m.palIdx = max(0, m.palIdx-1)
		case "down", "tab":
			m.palIdx = min(len(items)-1, m.palIdx+1)
		case "enter":
			if m.palIdx < len(items) && !items[m.palIdx].disabled {
				m.choose(items[m.palIdx])
			}
			m.palOpen = false
		case "backspace":
			if m.palQ != "" {
				m.palQ = m.palQ[:len(m.palQ)-1]
			}
			m.palIdx = 0
		case "space":
			m.palQ += " "
			m.palIdx = 0
		default:
			if len([]rune(k)) == 1 {
				m.palQ += k
				m.palIdx = 0
			}
		}
		return true
	}
	return false
}

// ---------- header ----------

func (m model) header() string {
	right := m.filterInfo()
	var left string
	switch m.nav {
	case 0:
		proj := bold.Render(" " + projects[m.proj].Name + " ▾ ")
		var tabs []string
		for _, it := range m.boardItems() {
			lbl := it.label
			if it.idx == boardTask && projects[m.proj].Backlog && projects[m.proj].Data {
				lbl += " · " + sprints[m.sprint].Name + " ▾"
			}
			switch {
			case it.disabled:
				tabs = append(tabs, faint.Strikethrough(true).Render(" "+lbl+" "))
			case it.current:
				tabs = append(tabs, lipgloss.NewStyle().Background(lipgloss.Color("#FFD866")).Foreground(lipgloss.Color("#1E1E1E")).Bold(true).Render(" "+lbl+" "))
			default:
				tabs = append(tabs, lipgloss.NewStyle().Background(lipgloss.Color("#3B3F51")).Render(" "+lbl+" "))
			}
		}
		left = proj + faint.Render("│ ") + strings.Join(tabs, " ")
	default:
		left = " " + m.breadcrumb()
	}
	gap := m.w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return fit(left, m.w)
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m model) breadcrumb() string {
	sep := faint.Render("  ›  ")
	parts := []string{bold.Render(projects[m.proj].Name)}
	if m.board == boardTask {
		parts = append(parts, accent.Render("Taskboard"))
		if projects[m.proj].Backlog && projects[m.proj].Data {
			s := sprints[m.sprint]
			parts = append(parts, bold.Render(s.Name)+faint.Render("  "+s.Dates+"  "+sprintNote(m.sprint)))
		}
	} else {
		parts = append(parts, accent.Render("Kanban Board"))
	}
	return strings.Join(parts, sep)
}

// ---------- 1: pickers ----------

func (m model) pickerItems() []navItem {
	if m.picker == 1 {
		return m.projectItems()
	}
	return m.sprintItems()
}

func (m model) pickerView(W, h int) string {
	title := "Select project"
	foot := "⏎ open · esc cancel · the opened project becomes the default (★) for the next start"
	if m.picker == 2 {
		title = "Select sprint"
		foot = "⏎ open on the Taskboard · esc cancel · the current sprint is chosen automatically at start"
	}
	lines := []string{"", "  " + accent.Render(title), ""}
	for i, it := range m.pickerItems() {
		mark := "  "
		if it.current {
			mark = accent.Render("● ")
		}
		row := fmt.Sprintf("%s%s", mark, fit(it.label, 36)) + faint.Render(it.note)
		if it.kind == 2 {
			row = mark + fit(it.label, 12) + fit(sprints[it.idx].Dates, 24) + faint.Render(it.note)
		}
		if i == m.pIdx {
			row = selStyle.Render(fit(ansiStrip(row), W-6))
		}
		lines = append(lines, "    "+row)
	}
	lines = append(lines, "", "  "+faint.Render(foot))
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines[:h], "\n")
}

// ---------- 2: sidebar ----------

func (m model) sidebarVisible() bool { return m.w >= 100 || m.sbFocus }

func (m model) sbItems() []navItem {
	out := []navItem{{kind: 0, label: "PROJECTS"}}
	out = append(out, m.projectItems()...)
	out = append(out, navItem{kind: 0, label: ""}, navItem{kind: 0, label: "BOARDS"})
	out = append(out, m.boardItems()...)
	if sp := m.sprintItems(); len(sp) > 0 {
		out = append(out, navItem{kind: 0, label: ""}, navItem{kind: 0, label: "SPRINTS"})
		out = append(out, sp...)
	}
	return out
}

func (m model) sbCurrent() int {
	for i, it := range m.sbItems() {
		if it.kind == 1 && it.current {
			return i
		}
	}
	return 1
}

func (m model) sidebar(w, h int) string {
	var lines []string
	for i, it := range m.sbItems() {
		var row string
		switch {
		case it.kind == 0:
			row = faint.Bold(true).Render(" " + it.label)
		default:
			mark := "  "
			if it.current {
				mark = accent.Render("● ")
			}
			lbl := it.label
			note := ""
			if it.kind == 2 {
				note = it.note
				if note == "current" {
					note = "today"
				}
			}
			if it.kind == 3 && it.idx == defaultProject {
				note = "★"
			}
			txt := fit(lbl, w-3-len([]rune(note))-1) + " " + faint.Render(note)
			if it.disabled {
				txt = faint.Strikethrough(true).Render(fit(lbl, w-4))
			}
			row = " " + mark + txt
			if m.sbFocus && i == m.sbIdx {
				row = selStyle.Render(fit(" "+ansiStrip(mark+txt), w))
			}
		}
		lines = append(lines, fit(row, w))
	}
	lines = append(lines, "")
	if m.sbFocus {
		lines = append(lines, faint.Render(fit(" ⏎ open · tab/esc back", w)))
	} else {
		lines = append(lines, faint.Render(fit(" tab: focus sidebar", w)))
	}
	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	return strings.Join(lines[:h], "\n")
}

// ---------- 3: command palette ----------

func (m model) palItems() []navItem {
	var all []navItem
	for _, it := range m.boardItems() {
		it.label = "Go to " + it.label
		all = append(all, it)
	}
	for _, it := range m.sprintItems() {
		it.label = "Sprint: " + it.label
		it.note = sprints[it.idx].Dates + "  " + it.note
		all = append(all, it)
	}
	for _, it := range m.projectItems() {
		it.label = "Project: " + it.label
		all = append(all, it)
	}
	q := strings.ToLower(strings.TrimSpace(m.palQ))
	if q == "" {
		return all
	}
	var out []navItem
	for _, it := range all {
		if fuzzy(strings.ToLower(it.label+" "+it.note), q) {
			out = append(out, it)
		}
	}
	return out
}

// fuzzy matches if all runes of q appear in s in order.
func fuzzy(s, q string) bool {
	r := []rune(s)
	i := 0
	for _, c := range q {
		for i < len(r) && r[i] != c {
			i++
		}
		if i == len(r) {
			return false
		}
		i++
	}
	return true
}

func (m model) palette() string {
	w := min(70, m.w-4)
	inner := w - 4
	lines := []string{accent.Render("› ") + m.palQ + selStyle.Render(" "), faint.Render(strings.Repeat("─", inner))}
	items := m.palItems()
	for i, it := range items {
		mark := "  "
		if it.current {
			mark = accent.Render("● ")
		}
		txt := mark + fit(it.label, inner-31) + faint.Render(fit(it.note, 29))
		if it.disabled {
			txt = mark + faint.Strikethrough(true).Render(fit(it.label, inner-31)) + faint.Render("not enabled")
		}
		if i == m.palIdx {
			txt = selStyle.Render(fit(ansiStrip(txt), inner))
		}
		lines = append(lines, txt)
	}
	if len(items) == 0 {
		lines = append(lines, faint.Render("  no match"))
	}
	lines = append(lines, "", faint.Render("type to filter · ↑↓ select · ⏎ go · esc close"))
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#FFD866")).
		Padding(0, 1).Width(w).Render(strings.Join(lines, "\n"))
}

func ansiStrip(s string) string { return ansi.Strip(s) }
