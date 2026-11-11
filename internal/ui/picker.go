package ui

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/zeckitoz/taiga-tui/internal/domain"
)

type pickerAction int

const (
	pickerNone pickerAction = iota
	pickerChoose
	pickerQuit
)

// pickerChromeLines are the lines above the project list: hint, filter and a
// blank line.
const pickerChromeLines = 3

const pickerKeys = "type to filter · ↑/↓ move · enter open · esc quit"

// picker is the project picker: projects sorted by name and filterable by
// name. Projects without boards are greyed out and cannot be chosen.
type picker struct {
	projects []domain.ProjectSummary
	filter   textinput.Model
	selected int64 // ID of the selected project, 0 if none
	top      int
	hint     string
}

func newPicker(projects []domain.ProjectSummary, hint string) picker {
	p := picker{projects: sortedProjects(projects), filter: newField(), hint: hint}
	p.filter.Prompt = "❯ "
	p.filter.Placeholder = "Filter projects"
	p.filter.Focus()
	p.selectFirst()
	return p
}

// sortedProjects sorts alphabetically, ignoring case.
func sortedProjects(ps []domain.ProjectSummary) []domain.ProjectSummary {
	ps = slices.Clone(ps)
	slices.SortStableFunc(ps, func(a, b domain.ProjectSummary) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return ps
}

// visible returns the projects matching the filter.
func (p *picker) visible() []domain.ProjectSummary {
	q := strings.ToLower(strings.TrimSpace(p.filter.Value()))
	var out []domain.ProjectSummary
	for _, pr := range p.projects {
		if strings.Contains(strings.ToLower(pr.Name), q) {
			out = append(out, pr)
		}
	}
	return out
}

func (p *picker) selectFirst() {
	p.selected = 0
	for _, pr := range p.visible() {
		if pr.HasBoards() {
			p.selected = pr.ID
			return
		}
	}
}

// selectedIndex returns the visible projects and the index of the selected
// one among them, -1 if it is filtered out or there is none.
func (p *picker) selectedIndex() ([]domain.ProjectSummary, int) {
	vis := p.visible()
	return vis, slices.IndexFunc(vis, func(pr domain.ProjectSummary) bool { return pr.ID == p.selected })
}

// chosen returns the selected project.
func (p *picker) chosen() (domain.ProjectSummary, bool) {
	vis, i := p.selectedIndex()
	if i < 0 {
		return domain.ProjectSummary{}, false
	}
	return vis[i], true
}

// move selects the next selectable project in direction d, skipping greyed
// out ones, and stays put at the ends.
func (p *picker) move(d int) {
	vis, i := p.selectedIndex()
	if i < 0 {
		return
	}
	for j := i + d; j >= 0 && j < len(vis); j += d {
		if vis[j].HasBoards() {
			p.selected = vis[j].ID
			return
		}
	}
}

func (p *picker) update(msg tea.Msg, listH int) (pickerAction, tea.Cmd) {
	defer p.scroll(listH)
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return pickerNone, nil
	}
	switch key.String() {
	case "up", "ctrl+p":
		p.move(-1)
		return pickerNone, nil
	case "down", "ctrl+n":
		p.move(1)
		return pickerNone, nil
	case "enter":
		if _, ok := p.chosen(); ok {
			return pickerChoose, nil
		}
		return pickerNone, nil
	case "esc":
		if p.filter.Value() == "" {
			return pickerQuit, nil
		}
		p.filter.SetValue("")
		p.selectFirst()
		return pickerNone, nil
	}
	before := p.filter.Value()
	var cmd tea.Cmd
	p.filter, cmd = p.filter.Update(msg)
	if p.filter.Value() != before {
		if _, ok := p.chosen(); !ok {
			p.selectFirst()
		}
	}
	return pickerNone, cmd
}

func (p *picker) scroll(listH int) {
	vis, sel := p.selectedIndex()
	p.top = scrollTop(p.top, sel, len(vis), listH)
}

func (p *picker) setWidth(w int) {
	p.filter.SetWidth(max(1, w-6))
}

func (p picker) view(w, h int) string {
	if h <= 0 {
		return ""
	}
	hint := ""
	if p.hint != "" {
		hint = " " + warnStyle.Render(p.hint)
	}
	lines := []string{fit(hint, w), " " + p.filter.View(), ""}
	vis := p.visible()
	listH := h - pickerChromeLines
	if len(vis) == 0 {
		lines = append(lines, "   "+faintStyle.Render("No project matches “"+strings.TrimSpace(p.filter.Value())+"”"))
	}
	for i := p.top; i < len(vis) && i-p.top < listH; i++ {
		pr := vis[i]
		switch {
		case pr.ID == p.selected:
			lines = append(lines, "  "+cursorStyle.Render(fit(" "+pr.Name, w-4)))
		case !pr.HasBoards():
			lines = append(lines, "   "+disabledStyle.Render(pr.Name+"  (no boards enabled)"))
		default:
			lines = append(lines, "   "+pr.Name)
		}
	}
	for i := range lines {
		lines[i] = fit(lines[i], w)
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines[:h], "\n")
}
