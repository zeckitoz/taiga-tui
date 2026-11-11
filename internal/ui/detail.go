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

// renderDetail renders the read-only detail view of the selected User
// Story, or of the active Status if its tab is empty, as exactly h lines of
// w cells with one cell of padding on each side. compact saves lines for the
// detail view below the list.
func renderDetail(k board.Kanban, w, h int, compact bool) string {
	inner := w - 2
	var lines []string
	if d, ok := k.Detail(); ok {
		lines = storyDetail(d, inner, compact)
	} else {
		lines = statusDetail(k.ActiveStatus(), inner)
	}
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

	status := faintStyle.Render("Unknown status")
	if d.Status.ID != 0 {
		status = statusStyle(d.Status.Color).Render("● " + plainText(d.Status.Name))
	}
	lines = append(lines, field("Status", status, w)...)

	who := faintStyle.Render("unassigned")
	if len(d.Assignees) > 0 {
		names := make([]string, len(d.Assignees))
		for i, a := range d.Assignees {
			names[i] = plainText(a.Name)
			if a.Mine {
				names[i] = mineStyle.Render(names[i] + " (you)")
			}
		}
		who = strings.Join(names, ", ")
	}
	lines = append(lines, field("Assignees", who, w)...)

	sprint := faintStyle.Render("not in a sprint")
	if d.SprintName != "" {
		sprint = plainText(d.SprintName)
	}
	lines = append(lines, field("Sprint", sprint, w)...)
	lines = append(lines, field("Tasks", progress(d.Tasks, true), w)...)
	lines = append(lines, "")
	return append(lines, descriptionLines(d.Description, w)...)
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
