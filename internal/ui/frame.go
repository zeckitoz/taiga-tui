package ui

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// fit truncates s with "…" or pads it with spaces to exactly w cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	return s + strings.Repeat(" ", w-lipgloss.Width(s))
}

// spread puts left and right on one line of width w, cutting left if both
// don't fit.
func spread(left, right string, w int) string {
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return fit(left, w)
	}
	return left + strings.Repeat(" ", gap) + right
}

// centered places the lines in the middle of a w×h area.
func centered(w, h int, lines ...string) string {
	return lipgloss.Place(w, max(0, h), lipgloss.Center, lipgloss.Center, strings.Join(lines, "\n"))
}

// footer renders the key help line.
func footer(keys string, w int) string {
	return fit(" "+faintStyle.Render(keys), w)
}

// footerKept is the number of entries at the end of a footer that are never
// left out: the menu and the help.
const footerKept = 2

// footerOf renders the key help line from keys, leaving out whole entries
// before the last footerKept ones until the line fits.
func footerOf(keys []string, w int) string {
	keys = slices.Clone(keys)
	line := strings.Join(keys, " · ")
	for len(keys) > footerKept && lipgloss.Width(line)+1 > w {
		keys = slices.Delete(keys, len(keys)-footerKept-1, len(keys)-footerKept)
		line = strings.Join(keys, " · ")
	}
	return footer(line, w)
}

// scrollTop returns the first visible line of a list of n lines in a window
// of h lines, scrolling from top as little as possible to keep sel visible.
func scrollTop(top, sel, n, h int) int {
	if h <= 0 {
		return 0
	}
	if sel >= 0 {
		top = min(top, sel)
		top = max(top, sel-h+1)
	}
	return max(0, min(top, n-h))
}
