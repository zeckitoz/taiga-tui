// PROTOTYP - Variante C: einklappbare Baum-Liste (User Story > Tasks), Status als farbiges Badge.
// Keine Spalten pro Status, daher unabhängig von Statusanzahl und Terminalbreite.
// Ab cPaneMin Spalten Breite steht rechts dauerhaft die Detailansicht des ausgewählten Task.
package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

const cPaneMin = 110

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

func (m *model) cClamp(rows []Row) {
	n := len(m.cItems(rows))
	m.cIdx = max(0, min(m.cIdx, n-1))
}

func (m model) cSel(rows []Row) int {
	items := m.cItems(rows)
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
	// Cursor auf die User-Story-Zeile setzen
	for i, it := range m.cItems(rows) {
		if it.task == nil && it.row.key() == key {
			m.cIdx = i
		}
	}
	m.sel = m.cSel(rows)
}

func (m *model) cMove(dx, dy int) {
	rows := m.rows()
	items := m.cItems(rows)
	if len(items) == 0 {
		return
	}
	cur := items[m.cIdx]
	switch {
	case dx < 0 && cur.task != nil: // auf die User Story springen
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
	m.cClamp(rows)
	m.sel = m.cSel(rows)
}

func (m model) viewC(h int) string {
	rows := m.rows()
	items := m.cItems(rows)
	W := m.w
	pane := W >= cPaneMin
	listW := W
	paneW := 0
	if pane {
		paneW = min(52, W*2/5)
		listW = W - paneW - 1
	}
	stW, whoW := 17, 18
	if listW < 70 {
		whoW = 4 // nur Initialen
	}

	var lines []string
	for i, it := range items {
		sel := i == m.cIdx
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
			bar := ""
			if n := r.count(); n > 0 {
				bar = colorSt("#A8E440").Render(strings.Repeat("▰", done)) + faint.Render(strings.Repeat("▱", n-done)) + faint.Render(fmt.Sprintf(" %d/%d ", done, n))
			} else {
				bar = faint.Render("keine Tasks ")
			}
			t := fmt.Sprintf("%s %s", arrow, r.title())
			tw := listW - lipgloss.Width(bar)
			if sel {
				lines = append(lines, selStyle.Render(fit(t, tw-1))+" "+bar)
			} else {
				lines = append(lines, bold.Render(fit(t, tw-1))+" "+bar)
			}
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
		lines = append(lines, faint.Render("  Keine Tasks für diesen Filter."))
	}
	list := m.window(lines, m.cIdx, h)
	for i := range list {
		list[i] = fit(list[i], listW)
	}
	if !pane {
		return strings.Join(list, "\n")
	}
	sep := faint.Render(strings.Repeat("│\n", h-1) + "│")
	return lipgloss.JoinHorizontal(lipgloss.Top, strings.Join(list, "\n"), sep, m.cPane(paneW, h))
}

func (m model) cPane(w, h int) string {
	st := lipgloss.NewStyle().Width(w).Height(h).Padding(0, 1)
	t := taskByRef(m.sel)
	if t == nil {
		items := m.cItems(m.rows())
		if m.cIdx < len(items) {
			r := items[m.cIdx].row
			var counts []string
			for i, s := range m.statuses() {
				if n := len(r.Cells[i]); n > 0 {
					counts = append(counts, colorSt(s.Color).Render("● ")+fmt.Sprintf("%-16s %d", s.Name, n))
				}
			}
			return st.Render(lipgloss.JoinVertical(lipgloss.Left,
				append([]string{bold.Width(w - 2).Render(r.title()), ""}, append(counts, "", faint.Render("⏎/space ein- und ausklappen"))...)...))
		}
		return st.Render("")
	}
	x := m.statusOf(*t)
	us := "ohne User Story"
	if s := storyByRef(t.Story); s != nil {
		us = fmt.Sprintf("#%d %s", s.Ref, s.Subject)
	}
	who := assigneeName(t.Assignee)
	if t.Assignee == meID {
		who = mineSt.Render(who + " (du)")
	}
	desc := t.Desc
	if desc == "" {
		desc = faint.Render("Keine Beschreibung.")
	}
	label := func(s string) string { return faint.Render(fit(s, 11)) }
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
