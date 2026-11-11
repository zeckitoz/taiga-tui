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

// pickerChromeLines are the lines above the list: hint, filter and a blank
// line.
const pickerChromeLines = 3

// pickerKeys is the footer of a picker whose esc ends the app.
const pickerKeys = "type to filter · ↑/↓ move · enter open · esc quit"

// pickerItem is one entry of a picker.
type pickerItem struct {
	id   int64
	name string
	// note follows the name, e.g. why the item is disabled.
	note string
	// disabled items are greyed out and cannot be chosen.
	disabled bool
}

// picker is a list of items, filterable by name, of which the Team Member
// chooses one.
type picker struct {
	items    []pickerItem
	filter   textinput.Model
	selected int64 // ID of the selected item, 0 if none
	top      int
	hint     string
	// noun names an item in "No <noun> matches".
	noun string
}

func newPicker(items []pickerItem, noun, hint string) picker {
	p := picker{items: items, filter: newField(), hint: hint, noun: noun}
	p.filter.Prompt = "❯ "
	p.filter.Placeholder = "Filter " + noun + "s"
	p.filter.Focus()
	p.selectFirst()
	return p
}

// projectItems lists the projects alphabetically, ignoring case. Projects
// without boards are disabled.
func projectItems(ps []domain.ProjectSummary) []pickerItem {
	ps = slices.Clone(ps)
	slices.SortStableFunc(ps, func(a, b domain.ProjectSummary) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	items := make([]pickerItem, len(ps))
	for i, p := range ps {
		items[i] = pickerItem{id: p.ID, name: p.Name}
		if !p.HasBoards() {
			items[i].note, items[i].disabled = "(no boards enabled)", true
		}
	}
	return items
}

// visible returns the items matching the filter.
func (p *picker) visible() []pickerItem {
	q := strings.ToLower(strings.TrimSpace(p.filter.Value()))
	var out []pickerItem
	for _, it := range p.items {
		if strings.Contains(strings.ToLower(it.name), q) {
			out = append(out, it)
		}
	}
	return out
}

func (p *picker) selectFirst() {
	p.selected = 0
	for _, it := range p.visible() {
		if !it.disabled {
			p.selected = it.id
			return
		}
	}
}

// selectedIndex returns the visible items and the index of the selected
// one among them, -1 if it is filtered out or there is none.
func (p *picker) selectedIndex() ([]pickerItem, int) {
	vis := p.visible()
	return vis, slices.IndexFunc(vis, func(it pickerItem) bool { return it.id == p.selected })
}

// chosen returns the ID of the selected item.
func (p *picker) chosen() (int64, bool) {
	vis, i := p.selectedIndex()
	if i < 0 {
		return 0, false
	}
	return vis[i].id, true
}

// move selects the next selectable item in direction d, skipping disabled
// ones, and stays put at the ends.
func (p *picker) move(d int) {
	vis, i := p.selectedIndex()
	if i < 0 {
		return
	}
	for j := i + d; j >= 0 && j < len(vis); j += d {
		if !vis[j].disabled {
			p.selected = vis[j].id
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
		lines = append(lines, "   "+faintStyle.Render("No "+p.noun+" matches “"+strings.TrimSpace(p.filter.Value())+"”"))
	}
	for i := p.top; i < len(vis) && i-p.top < listH; i++ {
		it := vis[i]
		note := ""
		if it.note != "" {
			note = "  " + it.note
		}
		switch {
		case it.id == p.selected:
			lines = append(lines, "  "+cursorStyle.Render(fit(" "+it.name+note, w-4)))
		case it.disabled:
			lines = append(lines, "   "+disabledStyle.Render(it.name+note))
		default:
			lines = append(lines, "   "+it.name+faintStyle.Render(note))
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
