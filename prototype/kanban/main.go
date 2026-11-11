// PROTOTYPE - throwaway code. Answers only:
// "Wie sehen das Kanban Board und der Wechsel zwischen den Boards aus?"
// Two independent dimensions, both switchable at runtime:
//
//	V  Kanban layout:      A grouped list · B columns · C Status tabs       (kanban.go)
//	N  navigation between Taskboard, Kanban Board, Sprint and project:
//	   1 tabs + pickers · 2 sidebar · 3 command palette                     (nav.go)
//
// Start: go run . [-kanban A|B|C] [-nav 1|2|3] [-board taskboard|kanban]
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	boardTask = iota
	boardKanban
)

type scroll struct{ top int }

type model struct {
	w, h     int
	board    int
	proj     int
	sprint   int
	mine     bool
	many     bool
	hidePane bool
	kVar     int // Kanban layout
	nav      int // navigation variant

	// Taskboard
	cIdx      int
	collapsed map[int]bool

	// Kanban Board
	kIdx       int          // A
	kCol, kRow int          // B (kRow -1 = column header), C uses kRow
	kTab       int          // C
	kCollapsed map[int]bool // Status index; local only, Taiga keeps it in browser storage

	// navigation
	picker  int // nav 1: 0 none, 1 project, 2 sprint
	pIdx    int
	sbFocus bool // nav 2
	sbIdx   int
	palOpen bool // nav 3
	palQ    string
	palIdx  int

	sc *scroll
}

func newModel() model {
	return model{
		proj: defaultProject, sprint: currentSprint,
		collapsed: map[int]bool{}, kCollapsed: map[int]bool{4: true}, // Done collapsed by default
		sc: &scroll{},
	}
}

// ---------- styles ----------

var (
	faint    = lipgloss.NewStyle().Faint(true)
	bold     = lipgloss.NewStyle().Bold(true)
	selStyle = lipgloss.NewStyle().Reverse(true)
	mineCol  = lipgloss.Color("#FF5FD7")
	mineSt   = lipgloss.NewStyle().Foreground(mineCol).Bold(true)
	accent   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD866")).Bold(true)
)

func colorSt(c string) lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color(c)) }

func label(s string) string { return faint.Render(fit(s, 11)) }

// fit truncates or pads s to exactly w cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	return s + strings.Repeat(" ", max(0, w-lipgloss.Width(s)))
}

func initials(id int) string {
	if mb := memberByID(id); mb != nil {
		return mb.Initials
	}
	return "--"
}

func assigneeName(id int) string {
	if mb := memberByID(id); mb != nil {
		return mb.Name
	}
	return "unassigned"
}

// window scrolls as little as possible to keep focus visible.
func (m model) window(lines []string, focus, h int) []string {
	if h <= 0 {
		return nil
	}
	top := m.sc.top
	if focus < top {
		top = focus
	}
	if focus >= top+h {
		top = focus - h + 1
	}
	top = max(0, min(top, max(0, len(lines)-h)))
	m.sc.top = top
	out := append([]string{}, lines[top:min(len(lines), top+h)]...)
	for len(out) < h {
		out = append(out, "")
	}
	return out
}

// ---------- update ----------

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.KeyPressMsg:
		k := msg.String()
		if k == "ctrl+c" {
			return m, tea.Quit
		}
		if m.navKey(k) { // the navigation variant consumed the key
			return m, nil
		}
		switch k {
		case "q":
			return m, tea.Quit
		case "V":
			m.kVar = (m.kVar + 1) % 3
			m.sc.top = 0
		case "N":
			m.nav = (m.nav + 1) % 3
			m.picker, m.sbFocus, m.palOpen = 0, false, false
		case "m":
			m.mine = !m.mine
			m.clamp()
		case "S":
			m.many = !m.many
			m.clamp()
		case "enter":
			m.enter()
		case "space", " ":
			m.space()
		default:
			dx, dy := 0, 0
			switch k {
			case "left", "h":
				dx = -1
			case "right", "l":
				dx = 1
			case "up", "k":
				dy = -1
			case "down", "j":
				dy = 1
			}
			if (dx != 0 || dy != 0) && m.boardUsable() {
				if m.board == boardTask {
					m.cMove(dx, dy)
				} else {
					switch m.kVar {
					case 0:
						m.kMoveA(dx, dy)
					case 1:
						m.kMoveB(dx, dy)
					case 2:
						m.kMoveC(dx, dy)
					}
				}
			}
		}
	}
	return m, nil
}

func (m *model) enter() {
	if !m.boardUsable() {
		return
	}
	if m.board == boardTask {
		if m.cOnStory() {
			m.cToggle()
		} else {
			m.hidePane = !m.hidePane
		}
		return
	}
	s, st := m.kSelected()
	if s != nil {
		m.hidePane = !m.hidePane
		return
	}
	m.kCollapsed[st] = !m.kCollapsed[st]
}

func (m *model) space() {
	if !m.boardUsable() {
		return
	}
	if m.board == boardTask {
		m.cToggle()
		return
	}
	_, st := m.kSelected()
	m.kCollapsed[st] = !m.kCollapsed[st]
	if m.kVar == 0 {
		for i, it := range m.kItems() {
			if it.story == nil && it.st == st {
				m.kIdx = i
			}
		}
	}
	if m.kVar == 1 {
		m.kRow = -1
	}
}

func (m *model) clamp() {
	m.cClamp()
	m.kIdx = max(0, min(m.kIdx, len(m.kItems())-1))
	m.kRow = min(m.kRow, len(m.kStories(m.kCol))-1)
}

func (m model) boardUsable() bool {
	p := projects[m.proj]
	if !p.Data {
		return false
	}
	if m.board == boardTask {
		return p.Backlog
	}
	return p.Kanban
}

// ---------- view ----------

func (m model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "Taiga TUI · PROTOTYPE Kanban"
	return v
}

func (m model) render() string {
	if m.w == 0 {
		return ""
	}
	header := m.header()
	footer := m.switcher()
	bodyH := m.h - lipgloss.Height(header) - lipgloss.Height(footer)
	body := m.body(bodyH)
	base := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	if m.nav == 2 && m.palOpen {
		box := m.palette()
		x := max(0, (m.w-lipgloss.Width(box))/2)
		return overlay(base, box, x, 1, m.w)
	}
	return base
}

// body renders navigation chrome (picker, sidebar) around the active board.
func (m model) body(h int) string {
	if m.nav == 0 && m.picker != 0 {
		return m.pickerView(m.w, h)
	}
	if m.nav == 1 && m.sidebarVisible() {
		sbW := 28
		sb := m.sidebar(sbW, h)
		sep := faint.Render(strings.Repeat("│\n", h-1) + "│")
		return lipgloss.JoinHorizontal(lipgloss.Top, sb, sep, m.boardView(m.w-sbW-1, h))
	}
	return m.boardView(m.w, h)
}

func (m model) boardView(W, h int) string {
	p := projects[m.proj]
	msg := ""
	switch {
	case m.board == boardTask && !p.Backlog:
		msg = "The Taskboard is not available: this project has no Scrum/Backlog module.\nUse the Kanban Board instead."
	case m.board == boardKanban && !p.Kanban:
		msg = "The Kanban Board is not enabled for this project.\nUse the Taskboard instead."
	case !p.Data:
		msg = "(prototype: no sample data for this project)"
	}
	if msg != "" {
		return lipgloss.Place(W, h, lipgloss.Center, lipgloss.Center, faint.Render(msg))
	}
	if m.board == boardTask {
		return m.viewTaskboard(W, h)
	}
	switch m.kVar {
	case 0:
		return m.viewKanbanA(W, h)
	case 1:
		return m.viewKanbanB(W, h)
	}
	return m.viewKanbanC(W, h)
}

func (m model) filterInfo() string {
	filter := "all"
	if m.mine {
		filter = mineSt.Render("only mine")
	}
	return faint.Render("filter: ") + filter + faint.Render(" · ") + mineSt.Render(assigneeName(meID)) + " "
}

func (m model) switcher() string {
	pill := lipgloss.NewStyle().Background(lipgloss.Color("#FFD866")).Foreground(lipgloss.Color("#1E1E1E")).Bold(true)
	bar := lipgloss.NewStyle().Background(lipgloss.Color("#3B3F51")).Foreground(lipgloss.Color("#F0F0F0"))
	l := pill.Render(" PROTOTYPE ") + bar.Render(fmt.Sprintf(" V kanban %s · N nav %s ", kanbanNames[m.kVar], navNames[m.nav]))
	r := bar.Render(fmt.Sprintf(" %s · m mine · ⏎ detail · space collapse · q · %d×%d ", navHint[m.nav], m.w, m.h))
	if m.w < 150 {
		l = pill.Render(" PROTO ") + bar.Render(fmt.Sprintf(" V %s · N %s ", kanbanNames[m.kVar][:1], navNames[m.nav][:1]))
		r = bar.Render(fmt.Sprintf(" %s · m mine · q · %d×%d ", navHint[m.nav], m.w, m.h))
	}
	gap := m.w - lipgloss.Width(l) - lipgloss.Width(r)
	if gap < 0 {
		return fit(l+r, m.w)
	}
	return l + bar.Render(strings.Repeat(" ", gap)) + r
}

func main() {
	kv := flag.String("kanban", "A", "Kanban layout A, B or C")
	nv := flag.Int("nav", 1, "navigation variant 1, 2 or 3")
	bd := flag.String("board", "kanban", "start board: taskboard or kanban")
	snap := flag.String("snap", "", "render a snapshot instead of the TUI, e.g. 140x40")
	keys := flag.String("keys", "", "comma separated keys for -snap")
	raw := flag.Bool("raw", false, "snapshot with ANSI colours")
	flag.Parse()
	m := newModel()
	m.kVar = max(0, min(2, int(strings.ToUpper(*kv)[0]-'A')))
	m.nav = max(0, min(2, *nv-1))
	if *bd == "kanban" {
		m.board = boardKanban
	}
	if *snap != "" {
		snapshot(m, *snap, *keys, *raw)
		return
	}
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// overlay splices box into base at cell position (x, y).
func overlay(base, box string, x, y, w int) string {
	bl := strings.Split(base, "\n")
	for i, l := range strings.Split(box, "\n") {
		if y+i >= len(bl) {
			break
		}
		row := fit(bl[y+i], w)
		bw := lipgloss.Width(l)
		bl[y+i] = ansi.Cut(row, 0, x) + l + ansi.Cut(row, x+bw, w)
	}
	return strings.Join(bl, "\n")
}
