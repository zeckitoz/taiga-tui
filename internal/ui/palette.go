package ui

import (
	"slices"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/zeckitoz/taiga-tui/internal/config"
)

// paletteAction is what the Team Member did in the palette.
type paletteAction int

const (
	paletteNone paletteAction = iota
	paletteChoose
	paletteClose
)

// paletteKeys is the footer while the palette is open.
const paletteKeys = "type to filter · ↑/↓ move · enter go · esc close"

// Size of the palette box: its widest width, its top margin within the
// board area, and the lines inside the border above the list (filter and
// hint).
const (
	paletteMaxWidth    = 76
	paletteTop         = 1
	paletteChromeLines = 2
	// paletteFrame is the border on both sides plus a space of padding.
	paletteFrame = 4
)

// entryKind tells what a palette entry goes to.
type entryKind int

const (
	entryBoard entryKind = iota
	entrySprint
	entryProject
)

// paletteEntry is one choice of the command palette.
type paletteEntry struct {
	kind entryKind
	// board is the board of an entryBoard, id the ID of the Sprint or
	// project of the other kinds.
	board config.Board
	id    int64

	label string
	// note follows the label in an aligned column.
	note string
	// current marks where the Team Member is with "●".
	current bool
	// disabled entries are greyed out and cannot be chosen; hint then tells
	// what to do instead when the filter finds the entry.
	disabled bool
	hint     string
}

// palette is the command palette: a list of entries filtered fuzzily, of
// which the Team Member chooses one. It is an overlay over the board.
type palette struct {
	entries []paletteEntry
	filter  textinput.Model
	// visible holds the indices of the entries matching the filter, best
	// match first.
	visible []int
	// selected is the index of the selected entry, -1 if none.
	selected int
	top      int
}

// newPalette returns the palette with entries and an empty filter.
func newPalette(entries []paletteEntry) palette {
	p := palette{filter: newField(), selected: -1}
	p.filter.Prompt = "❯ "
	p.filter.Placeholder = "Go to a board, sprint or project"
	p.filter.Focus()
	p.setEntries(entries)
	return p
}

// setEntries replaces the entries, keeping the selected entry if it is
// still there.
func (p *palette) setEntries(entries []paletteEntry) {
	old, ok := p.chosen()
	p.entries, p.selected = entries, -1
	if ok {
		p.selected = slices.IndexFunc(entries, func(e paletteEntry) bool { return e.goesTo(old) })
	}
	p.refilter()
}

// goesTo reports whether e and o go to the same place.
func (e paletteEntry) goesTo(o paletteEntry) bool {
	return e.kind == o.kind && e.board == o.board && e.id == o.id
}

// refilter ranks the entries by the filter and selects the first one that
// can be chosen if the selection is gone.
func (p *palette) refilter() {
	p.visible = fuzzyRank(p.filter.Value(), len(p.entries), func(i int) string { return p.entries[i].label })
	if !slices.Contains(p.visible, p.selected) || p.entries[p.selected].disabled {
		p.selectFirst()
	}
}

// selectFirst selects the best match that can be chosen.
func (p *palette) selectFirst() {
	p.selected = -1
	for _, i := range p.visible {
		if !p.entries[i].disabled {
			p.selected = i
			return
		}
	}
}

// chosen returns the selected entry.
func (p *palette) chosen() (paletteEntry, bool) {
	if p.selected < 0 {
		return paletteEntry{}, false
	}
	return p.entries[p.selected], true
}

// hint tells what to do instead of a disabled entry: the first one without
// a filter, else the best match of the filter.
func (p *palette) hint() string {
	if len(p.visible) == 0 {
		return ""
	}
	if strings.TrimSpace(p.filter.Value()) != "" {
		return p.entries[p.visible[0]].hint
	}
	for _, i := range p.visible {
		if h := p.entries[i].hint; h != "" {
			return h
		}
	}
	return ""
}

// move selects the next entry in direction d that can be chosen and stays
// put at the ends.
func (p *palette) move(d int) {
	at := slices.Index(p.visible, p.selected)
	if at < 0 {
		return
	}
	for j := at + d; j >= 0 && j < len(p.visible); j += d {
		if !p.entries[p.visible[j]].disabled {
			p.selected = p.visible[j]
			return
		}
	}
}

// update handles msg, keeping the selection visible in a list of listH
// lines.
func (p *palette) update(msg tea.Msg, listH int) (paletteAction, tea.Cmd) {
	defer p.scroll(listH)
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return paletteNone, nil
	}
	switch key.String() {
	case "up", "ctrl+p":
		p.move(-1)
		return paletteNone, nil
	case "down", "ctrl+n":
		p.move(1)
		return paletteNone, nil
	case "enter":
		if _, ok := p.chosen(); ok {
			return paletteChoose, nil
		}
		return paletteNone, nil
	case "esc":
		return paletteClose, nil
	}
	before := p.filter.Value()
	var cmd tea.Cmd
	p.filter, cmd = p.filter.Update(msg)
	if p.filter.Value() != before {
		p.selected = -1
		p.refilter()
	}
	return paletteNone, cmd
}

// scroll keeps the selection visible in a list of listH lines.
func (p *palette) scroll(listH int) {
	p.top = scrollTop(p.top, slices.Index(p.visible, p.selected), len(p.visible), listH)
}

// paletteBox returns the width of the palette box in a board area w cells
// wide and the height of its list in an area h lines high.
func paletteBox(w, h int) (boxW, listH int) {
	return min(paletteMaxWidth, w-4), h - paletteTop - 2 - paletteChromeLines
}

// setWidth fits the filter into the palette box of a board area w cells
// wide.
func (p *palette) setWidth(w int) {
	boxW, _ := paletteBox(w, 0)
	// The prompt and the cursor take three cells.
	p.filter.SetWidth(max(1, boxW-paletteFrame-3))
}

// view renders the palette box for a board area of w×h, as tall as its
// entries need.
func (p palette) view(w, h int) string {
	boxW, listH := paletteBox(w, h)
	inner := boxW - paletteFrame
	if inner <= 0 || listH <= 0 {
		return ""
	}
	lines := []string{p.filter.View(), warnStyle.Render(p.hint())}
	if len(p.visible) == 0 {
		lines = append(lines, faintStyle.Render("Nothing matches “"+strings.TrimSpace(p.filter.Value())+"”"))
	}
	labelW := 0
	for _, e := range p.entries {
		labelW = max(labelW, ansi.StringWidth(e.label))
	}
	for at := p.top; at < len(p.visible) && at-p.top < listH; at++ {
		i := p.visible[at]
		lines = append(lines, entryLine(p.entries[i], i == p.selected, labelW, inner))
	}
	for i := range lines {
		lines[i] = fit(lines[i], inner)
	}
	return paletteStyle.Width(boxW).Render(strings.Join(lines, "\n"))
}

// entryLine renders an entry with "●" if it is current and its note in a
// column after the labels, which are labelW cells wide.
func entryLine(e paletteEntry, selected bool, labelW, w int) string {
	mark := "  "
	if e.current {
		mark = "● "
	}
	label := fit(e.label, labelW)
	switch {
	case selected:
		return cursorStyle.Render(fit(mark+label+"  "+e.note, w))
	case e.disabled:
		return disabledStyle.Render(mark + label + "  " + e.note)
	case e.current:
		return promptStyle.Render(mark) + label + "  " + faintStyle.Render(e.note)
	default:
		return mark + label + "  " + faintStyle.Render(e.note)
	}
}

// overlay draws box over base with its top left corner at x, y.
func overlay(base, box string, x, y int) string {
	lines := strings.Split(base, "\n")
	for i, b := range strings.Split(box, "\n") {
		if y+i >= len(lines) {
			break
		}
		l := lines[y+i]
		left := fit(ansi.Truncate(l, x, ""), x)
		right := ansi.TruncateLeft(l, x+lipgloss.Width(b), "")
		lines[y+i] = left + b + right
	}
	return strings.Join(lines, "\n")
}

// fuzzyRank returns the indices of the n texts that contain every word of
// query as a subsequence, ignoring case, best match first and otherwise in
// their order.
func fuzzyRank(query string, n int, text func(int) string) []int {
	words := strings.Fields(strings.ToLower(query))
	type match struct{ i, score int }
	var ms []match
	for i := range n {
		t := strings.ToLower(text(i))
		score, ok := 0, true
		for _, w := range words {
			s, found := fuzzyScore(w, t)
			if !found {
				ok = false
				break
			}
			score += s
		}
		if ok {
			ms = append(ms, match{i, score})
		}
	}
	slices.SortStableFunc(ms, func(a, b match) int { return b.score - a.score })
	out := make([]int, len(ms))
	for k, m := range ms {
		out[k] = m.i
	}
	return out
}

// fuzzyScore tells whether the runes of word appear in text in order and
// scores the match: a run of matching runes and a match at the start of a
// word score higher, gaps lower.
func fuzzyScore(word, text string) (int, bool) {
	tr := []rune(text)
	score, prev := 0, -1
	for _, c := range word {
		at := -1
		for j := prev + 1; j < len(tr); j++ {
			if tr[j] == c {
				at = j
				break
			}
		}
		if at < 0 {
			return 0, false
		}
		switch {
		case at == prev+1 && prev >= 0:
			score += 3
		case at == 0 || !unicode.IsLetter(tr[at-1]) && !unicode.IsDigit(tr[at-1]):
			score += 2
		default:
			score -= min(at-prev-1, 3)
		}
		prev = at
	}
	return score, true
}
