package ui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/zeckitoz/taiga-tui/internal/board"
)

// kanbanChromeLines are the lines above the list: tab bar and rule.
const kanbanChromeLines = 2

// Widths of the fixed list columns.
const (
	maxInitialsWidth = 11
	maxProgressBar   = 10
)

const kanbanKeys = "←/→ status · ↑/↓ move · r refresh · q quit"

// renderKanban renders the Kanban Board into a w×h area, showing the list
// from line top.
func renderKanban(k board.Kanban, top, w, h int) string {
	listH := h - kanbanChromeLines
	tabs := k.Tabs()
	head := []string{tabBar(tabs, w), faintStyle.Render(strings.Repeat("─", w))}
	var body string
	rows := k.Rows()
	switch {
	case k.Empty():
		body = centered(w, listH, faintStyle.Render("No user stories on the Kanban Board"), "", faintStyle.Render("r refresh"))
	case len(rows) == 0:
		body = centered(w, listH, faintStyle.Render("No user stories in "+k.ActiveStatus().Name))
	default:
		body = storyList(rows, top, w, listH)
	}
	return strings.Join(append(head, body), "\n")
}

// tabBar renders one tab per Status. If the full labels don't fit, inactive
// tabs shrink to their count.
func tabBar(tabs []board.Tab, w int) string {
	render := func(short bool) string {
		parts := make([]string, len(tabs))
		for i, t := range tabs {
			parts[i] = tabLabel(t, short && !t.Active)
		}
		return strings.Join(parts, faintStyle.Render("│"))
	}
	bar := render(false)
	if lipgloss.Width(bar) > w {
		bar = render(true)
	}
	return fit(bar, w)
}

func tabLabel(t board.Tab, short bool) string {
	count := strconv.Itoa(t.Count)
	if t.WIPLimit > 0 {
		count += "/" + strconv.Itoa(t.WIPLimit)
	}
	label := " ● " + t.Name + " " + count + " "
	if short {
		label = " ●" + count + " "
	}
	return tabStyle(t.Color, t.Active).Render(label)
}

// storyList renders the rows of the active tab as aligned columns.
func storyList(rows []board.Row, top, w, h int) string {
	refW, whoW, progW := 0, 0, 0
	for _, r := range rows {
		refW = max(refW, len(ref(r.Ref)))
		whoW = max(whoW, ansi.StringWidth(initials(r.Assignees, false)))
		progW = max(progW, ansi.StringWidth(progress(r, false)))
	}
	whoW = min(whoW, maxInitialsWidth)
	// marker, ref, title and the two right columns, separated by spaces
	titleW := w - 3 - refW - 1 - 2 - whoW - 2 - progW - 1

	lines := make([]string, 0, h)
	for i := top; i < len(rows) && len(lines) < h; i++ {
		r := rows[i]
		marker := "   "
		if r.Mine {
			marker = " " + mineStyle.Render("▍") + " "
		}
		styled := !r.Selected
		line := marker + fit(ref(r.Ref), refW) + " " + fit(r.Title, titleW) + "  " +
			fit(initials(r.Assignees, styled), whoW) + "  " + fit(progress(r, styled), progW) + " "
		if r.Selected {
			line = cursorStyle.Render(fit(ansi.Strip(line), w))
		}
		lines = append(lines, fit(line, w))
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func ref(n int) string { return "#" + strconv.Itoa(n) }

// initials lists the Assignees' initials, the logged-in Team Member's in the
// accent color when styled.
func initials(as []board.Assignee, styled bool) string {
	if len(as) == 0 {
		if styled {
			return faintStyle.Render("--")
		}
		return "--"
	}
	parts := make([]string, len(as))
	for i, a := range as {
		parts[i] = a.Initials
		if styled && a.Mine {
			parts[i] = mineStyle.Render(a.Initials)
		}
	}
	return strings.Join(parts, " ")
}

// progress renders the task progress as a bar of at most maxProgressBar
// segments followed by done/total, or "no tasks".
func progress(r board.Row, styled bool) string {
	done, total := r.Tasks.Done, r.Tasks.Total
	if total == 0 {
		if styled {
			return faintStyle.Render("no tasks")
		}
		return "no tasks"
	}
	segs, filled := total, done
	if total > maxProgressBar {
		segs = maxProgressBar
		filled = (done*maxProgressBar + total/2) / total
	}
	bar, rest := strings.Repeat("▰", filled), strings.Repeat("▱", segs-filled)
	count := fmt.Sprintf(" %d/%d", done, total)
	if styled {
		return progressStyle.Render(bar) + faintStyle.Render(rest+count)
	}
	return bar + rest + count
}
