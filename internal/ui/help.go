package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// keyHelp explains one key in the full help.
type keyHelp struct{ key, desc string }

// commonHelp are the keys of both boards, after their own.
var commonHelp = []keyHelp{
	{"r", "refresh"},
	{": ctrl+k", "menu: boards, sprints, projects"},
	{"? esc", "close help"},
	{"q ctrl+c", "quit"},
}

// kanbanHelp are the keys of the Kanban Board.
var kanbanHelp = []keyHelp{
	{"⇧←/→ H/L", "move to previous / next status"},
	{"←/→ h/l", "previous / next tab"},
	{"↑/↓ j/k", "move"},
	{"enter", "show / hide detail"},
}

// taskboardHelp are the keys of the Taskboard.
var taskboardHelp = []keyHelp{
	{"⇧←/→ H/L", "move to previous / next status"},
	{"↑/↓ j/k", "move"},
	{"←/→ h/l", "fold / unfold user story"},
	{"enter", "fold user story, show / hide detail"},
}

// helpColumnGap separates the columns of the full help.
const helpColumnGap = 4

// fullHelp renders keys in as many columns as fit into w cells, filled top
// to bottom, as the lines of the expanded footer.
func fullHelp(keys []keyHelp, w int) []string {
	keyW, descW := 0, 0
	for _, k := range keys {
		keyW = max(keyW, ansi.StringWidth(k.key))
		descW = max(descW, ansi.StringWidth(k.desc))
	}
	colW := keyW + 2 + descW
	cols := max(1, min(len(keys), (w-1+helpColumnGap)/(colW+helpColumnGap)))
	rows := (len(keys) + cols - 1) / cols
	lines := make([]string, rows)
	for i, k := range keys {
		r, c := i%rows, i/rows
		if c > 0 {
			lines[r] += strings.Repeat(" ", helpColumnGap)
		}
		cell := labelStyle.Render(fit(k.key, keyW)) + "  " + faintStyle.Render(k.desc)
		if c < cols-1 {
			cell = fit(cell, colW)
		}
		lines[r] += cell
	}
	for i := range lines {
		lines[i] = fit(" "+lines[i], w)
	}
	return lines
}
