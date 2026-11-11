package ui

import (
	"strconv"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/zeckitoz/taiga-tui/internal/board"
	"github.com/zeckitoz/taiga-tui/internal/domain"
)

// detailLabelWidth is the width of the field labels, e.g. "Assignees ".
const detailLabelWidth = 11

// kanbanDetailLines renders the detail view of the selected User Story, or
// of the active Status if its tab is empty, for width w. compact saves lines
// for the detail view below the list.
func kanbanDetailLines(k board.Kanban, w int, compact bool) []string {
	if d, ok := k.Detail(); ok {
		return storyDetail(d, w, compact)
	}
	return statusDetail(k.ActiveStatus(), w)
}

// detailPane renders lines as the read-only detail view: exactly h lines of
// w cells with one cell of padding on each side, cut off with "…" if they
// don't fit.
func detailPane(lines []string, w, h int) string {
	inner := w - 2
	if len(lines) > h {
		lines = lines[:h]
		for len(lines) > 1 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		lines[len(lines)-1] = withMore(lines[len(lines)-1], inner)
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	for i, l := range lines {
		lines[i] = " " + fit(l, inner) + " "
	}
	return strings.Join(lines, "\n")
}

func storyDetail(d board.Detail, w int, compact bool) []string {
	lines := styleLines(labelStyle, wrap(ref(d.Ref)+" "+plainText(d.Title), w))
	if !compact {
		lines = append(lines, "")
	}

	lines = append(lines, field("Status", statusLabel(d.Status), w)...)

	lines = append(lines, field("Assignees", assigneeNames(d.Assignees), w)...)

	sprint := faintStyle.Render("not in a sprint")
	if d.SprintName != "" {
		sprint = plainText(d.SprintName)
	}
	lines = append(lines, field("Sprint", sprint, w)...)
	lines = append(lines, field("Tasks", progress(d.Tasks, true), w)...)
	lines = append(lines, "")
	return append(lines, descriptionLines(d.Description, w)...)
}

// statusLabel names a Status in its color, or "Unknown status" for the zero
// Status.
func statusLabel(s domain.Status) string {
	if s.ID == 0 {
		return faintStyle.Render("Unknown status")
	}
	return statusStyle(s.Color).Render("● " + plainText(s.Name))
}

// assigneeNames lists the Assignees by name, the logged-in Team Member in
// the accent color, or "unassigned".
func assigneeNames(as []board.Assignee) string {
	if len(as) == 0 {
		return faintStyle.Render("unassigned")
	}
	names := make([]string, len(as))
	for i, a := range as {
		names[i] = plainText(a.Name)
		if a.Mine {
			names[i] = mineStyle.Render(names[i] + " (you)")
		}
	}
	return strings.Join(names, ", ")
}

func descriptionLines(d board.Description, w int) []string {
	switch d.State {
	case board.DescriptionLoading:
		return []string{faintStyle.Render("Loading description…")}
	case board.DescriptionFailed:
		return styleLines(errorStyle, wrap("Couldn't load description: "+reason(d.Err), w))
	}
	text := strings.TrimSpace(plainText(d.Text))
	if text == "" {
		return []string{faintStyle.Render("No description")}
	}
	return wrap(text, w)
}

// statusDetail describes the active Status when its tab has no User Stories.
func statusDetail(s domain.Status, w int) []string {
	lines := []string{statusHeadingStyle(s.Color).Render(fit("● "+plainText(s.Name), w))}
	if s.WIPLimit > 0 {
		lines = append(lines, "")
		lines = append(lines, wrap("WIP limit "+strconv.Itoa(s.WIPLimit)+" (not enforced by Taiga)", w)...)
	}
	return lines
}

// withMore marks the line l of width w as cut off after it.
func withMore(l string, w int) string {
	const more = " …"
	if lipgloss.Width(l)+len([]rune(more)) > w {
		l = ansi.Truncate(l, max(0, w-len([]rune(more))), "")
	}
	return l + faintStyle.Render(more)
}

// field renders "Label      value", wrapping the value under itself.
func field(label, value string, w int) []string {
	vw := max(1, w-detailLabelWidth)
	lines := wrap(value, vw)
	for i, l := range lines {
		prefix := strings.Repeat(" ", detailLabelWidth)
		if i == 0 {
			prefix = faintStyle.Render(fit(label, detailLabelWidth))
		}
		lines[i] = prefix + l
	}
	return lines
}

// wrap breaks s into lines of at most w cells, at spaces where possible.
func wrap(s string, w int) []string {
	return strings.Split(lipgloss.Wrap(s, max(1, w), " "), "\n")
}

func styleLines(st lipgloss.Style, lines []string) []string {
	for i, l := range lines {
		lines[i] = st.Render(l)
	}
	return lines
}

// plainText makes text from Taiga safe to print: it normalizes line ends,
// expands tabs and drops other control characters, which could otherwise
// move the cursor or change colors.
func plainText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\t", "    ")
	return strings.Map(func(r rune) rune {
		if r != '\n' && unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}
