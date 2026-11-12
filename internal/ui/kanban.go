package ui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/zeckitoz/taiga-tui/internal/board"
	"github.com/zeckitoz/taiga-tui/internal/domain"
)

// kanbanChromeLines are the lines above the list: tab bar and rule.
const kanbanChromeLines = 2

// Widths of the fixed list columns.
const (
	maxInitialsWidth = 11
	maxProgressBar   = 10
)

// kanbanKeys returns the footer of the Kanban Board for the selected row.
func kanbanKeys(k board.Kanban) []string {
	var keys []string
	if k.SelectedIndex() >= 0 {
		keys = append(keys, "⇧←/→ status", "s pick", "a assign to me")
	}
	return append(keys, "←/→ tab", "↑/↓ move", "enter detail", "r refresh", ": menu", "? help")
}

// Placement of the detail view: right of the list from detailRightFrom
// columns, otherwise below it, never over it.
const (
	detailRightFrom   = 110
	detailMaxWidth    = 52
	detailBelowHeight = 9
	// minListHeight keeps a few rows of the list visible when the detail
	// view is below it.
	minListHeight = 3
)

// boardLayout splits the area of a board's list into the list and the
// detail view.
type boardLayout struct {
	listW, listH     int
	detailW, detailH int
	// right places the detail view right of the list, else below it.
	right bool
}

// layoutBoard lays out the w×h area of a board's list.
func layoutBoard(w, h int, detail bool) boardLayout {
	l := boardLayout{listW: w, listH: h}
	switch {
	case !detail:
	case w >= detailRightFrom:
		l.detailW = min(detailMaxWidth, w*2/5)
		l.detailH = h
		l.listW = w - l.detailW - 1 // the rule between them
		l.right = true
	case h > minListHeight+1:
		l.listH = max(minListHeight, h-detailBelowHeight-1)
		l.detailW = w
		l.detailH = h - l.listH - 1 // the rule between them
	}
	return l
}

// renderKanban renders the Kanban Board into a w×h area, showing the list
// from line top and, if detail is set, the detail view. spin is the frame of
// the spinner on rows that are saving.
func renderKanban(k board.Kanban, top, w, h int, detail bool, spin string) string {
	tabs := k.Tabs()
	head := []string{tabBar(tabs, w), rule(w)}
	if k.Empty() {
		body := centered(w, h-kanbanChromeLines, faintStyle.Render("No user stories on the Kanban Board"), "", faintStyle.Render("r refresh"))
		return strings.Join(append(head, body), "\n")
	}
	l := layoutBoard(w, h-kanbanChromeLines, detail)
	var list string
	if rows := k.Rows(); len(rows) == 0 {
		list = centered(l.listW, l.listH, faintStyle.Render("No user stories in "+plainText(k.ActiveStatus().Name)))
	} else {
		list = storyList(rows, top, l.listW, l.listH, spin)
	}
	return strings.Join(append(head, withDetail(l, w, list, func(inner int, compact bool) []string {
		return kanbanDetailLines(k, inner, compact)
	})), "\n")
}

// withDetail places the detail view next to or below list as laid out by
// l in an area w cells wide. lines renders the detail view's lines for
// width inner.
func withDetail(l boardLayout, w int, list string, lines func(inner int, compact bool) []string) string {
	if l.detailH == 0 {
		return list
	}
	pane := detailPane(lines(l.detailW-2, !l.right), l.detailW, l.detailH)
	if l.right {
		rule := faintStyle.Render(strings.TrimSuffix(strings.Repeat("│\n", l.detailH), "\n"))
		return lipgloss.JoinHorizontal(lipgloss.Top, list, rule, pane)
	}
	return strings.Join([]string{list, rule(w), pane}, "\n")
}

// rule is a horizontal line across w cells.
func rule(w int) string { return faintStyle.Render(strings.Repeat("─", w)) }

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
	label := " ● " + plainText(t.Name) + " " + count + " "
	if short {
		label = " ●" + count + " "
	}
	return tabStyle(t.Color, t.Active).Render(label)
}

// storyList renders the rows of the active tab as aligned columns, with
// spin in front of rows that are saving.
func storyList(rows []board.Row, top, w, h int, spin string) string {
	refW, whoW, progW := 0, 0, 0
	for _, r := range rows {
		refW = max(refW, len(ref(r.Ref)))
		whoW = max(whoW, ansi.StringWidth(initials(r.Assignees, false)))
		progW = max(progW, ansi.StringWidth(progress(r.Tasks, false)))
	}
	whoW = min(whoW, maxInitialsWidth)
	// marker, ref, title and the two right columns, separated by spaces
	titleW := w - 3 - refW - 1 - 2 - whoW - 2 - progW - 1

	lines := make([]string, 0, h)
	for i := top; i < len(rows) && len(lines) < h; i++ {
		r := rows[i]
		marker := savingMarker(r.Saving, spin)
		if r.Mine {
			marker += mineStyle.Render("▍") + " "
		} else {
			marker += "  "
		}
		styled := !r.Selected
		line := marker + fit(ref(r.Ref), refW) + " " + fit(plainText(r.Title), titleW) + "  " +
			fit(initials(r.Assignees, styled), whoW) + "  " + fit(progress(r.Tasks, styled), progW) + " "
		if r.Selected {
			line = cursorStyle.Render(fit(ansi.Strip(line), w))
		}
		lines = append(lines, fit(line, w))
	}
	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	return strings.Join(lines, "\n")
}

func ref(n int) string { return "#" + strconv.Itoa(n) }

// savingMarker is the cell in front of a row: spin while it is saving.
func savingMarker(saving bool, spin string) string {
	if saving {
		return promptStyle.Render(spin)
	}
	return " "
}

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
func progress(tasks domain.TaskProgress, styled bool) string {
	done, total := tasks.Done, tasks.Total
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
