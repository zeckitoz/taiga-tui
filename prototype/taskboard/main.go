// PROTOTYP - Wegwerfcode. Beantwortet nur: "Wie sieht das Taskboard des Current Sprint im Terminal aus?"
// Drei strukturell verschiedene Varianten, umschaltbar mit tab / shift+tab oder 1-3.
// Start: go run . [-variant A|B|C]
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

var variantNames = []string{
	"A · Raster: Swimlanes × Status (wie Web-UI)",
	"B · Matrix-Überblick + Liste der User Story",
	"C · Baum-Liste + Detailspalte",
}

// Zeile des Boards: eine User Story (nil = Tasks ohne User Story) mit Tasks pro Status.
type Row struct {
	Story *Story
	Cells [][]Task
}

type scroll struct{ top, colOff int }

type model struct {
	variant   int
	w, h      int
	mine      bool
	many      bool
	detail    bool
	sel       int // Ref des ausgewählten Task, 0 = keiner
	aRow      int
	aCol      int
	aK        int
	bRow      int
	bK        int
	cIdx      int
	collapsed map[int]bool // Story-Ref (-1 = ohne User Story)
	hidePane  bool         // Variante C: Detailspalte ausgeblendet (d)
	usStyle   int          // Variante C: Darstellung der User-Story-Zeilen (entschieden: 1)
	sc        *scroll
}

func (m model) statuses() []Status {
	if m.many {
		return manyStatuses
	}
	return baseStatuses
}

func (m model) statusOf(t Task) string {
	if !m.many {
		if b, ok := collapseToBase[t.Status]; ok {
			return b
		}
	}
	return t.Status
}

func (m model) statusColor(name string) string {
	for _, s := range manyStatuses {
		if s.Name == name {
			return s.Color
		}
	}
	return "#888888"
}

func (m model) rows() []Row {
	sts := m.statuses()
	idx := map[string]int{}
	for i, s := range sts {
		idx[s.Name] = i
	}
	build := func(st *Story, ref int) Row {
		r := Row{Story: st, Cells: make([][]Task, len(sts))}
		for _, t := range tasks {
			if t.Story != ref || (m.mine && t.Assignee != meID) {
				continue
			}
			i := idx[m.statusOf(t)]
			r.Cells[i] = append(r.Cells[i], t)
		}
		return r
	}
	var out []Row
	for i := range stories {
		r := build(&stories[i], stories[i].Ref)
		if m.mine && r.count() == 0 {
			continue
		}
		out = append(out, r)
	}
	if r := build(nil, 0); r.count() > 0 {
		out = append(out, r)
	}
	return out
}

func (r Row) count() int {
	n := 0
	for _, c := range r.Cells {
		n += len(c)
	}
	return n
}

func (r Row) flat() []Task {
	var out []Task
	for _, c := range r.Cells {
		out = append(out, c...)
	}
	return out
}

func (r Row) key() int {
	if r.Story == nil {
		return -1
	}
	return r.Story.Ref
}

func (r Row) title() string {
	if r.Story == nil {
		return "Tasks ohne User Story"
	}
	return fmt.Sprintf("#%d %s", r.Story.Ref, r.Story.Subject)
}

func taskByRef(ref int) *Task {
	for i := range tasks {
		if tasks[i].Ref == ref {
			return &tasks[i]
		}
	}
	return nil
}

// ---------- Stile ----------

var (
	faint    = lipgloss.NewStyle().Faint(true)
	bold     = lipgloss.NewStyle().Bold(true)
	selStyle = lipgloss.NewStyle().Reverse(true)
	mineCol  = lipgloss.Color("#FF5FD7")
	mineSt   = lipgloss.NewStyle().Foreground(mineCol).Bold(true)
	bandSt   = lipgloss.NewStyle().Background(lipgloss.Color("#2A2D3A")).Foreground(lipgloss.Color("#E6E6E6"))
)

func colorSt(c string) lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color(c)) }

// fit kürzt oder füllt s auf genau w Zellen.
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
	return "nicht zugewiesen"
}

// window scrollt so wenig wie möglich, damit focus sichtbar bleibt.
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
	end := min(len(lines), top+h)
	out := append([]string{}, lines[top:end]...)
	for len(out) < h {
		out = append(out, "")
	}
	return out
}

// ---------- Update ----------

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.KeyPressMsg:
		k := msg.String()
		if m.detail {
			if k == "esc" || k == "enter" || k == "q" {
				m.detail = false
			}
			return m, nil
		}
		switch k {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.setVariant((m.variant + 1) % 3)
		case "shift+tab":
			m.setVariant((m.variant + 2) % 3)
		case "1", "2", "3":
			m.setVariant(int(k[0] - '1'))
		case "m":
			m.mine = !m.mine
			m.sync()
		case "S":
			m.many = !m.many
			m.sync()
		case "enter":
			if m.variant == 2 {
				if m.cOnStory() {
					m.cToggle()
				} else if m.sel != 0 {
					m.hidePane = !m.hidePane // Detailansicht ein-/ausblenden, kein Overlay
				}
			} else if m.sel != 0 {
				m.detail = true
			}
		case " ":
			if m.variant == 2 {
				m.cToggle()
			}
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
			if dx != 0 || dy != 0 {
				switch m.variant {
				case 0:
					m.aMove(dx, dy)
				case 1:
					m.bMove(dx, dy)
				case 2:
					m.cMove(dx, dy)
				}
			}
		}
	}
	return m, nil
}

func (m *model) setVariant(v int) {
	m.variant = v
	m.sc.top = 0
	m.sync()
}

// sync setzt den Cursor der aktiven Variante auf den ausgewählten Task.
func (m *model) sync() {
	rows := m.rows()
	if t := taskByRef(m.sel); t != nil && !(m.mine && t.Assignee != meID) {
		for ri, r := range rows {
			for ci, c := range r.Cells {
				for k, x := range c {
					if x.Ref == m.sel {
						m.aRow, m.aCol, m.aK = ri, ci, k
					}
				}
			}
			for k, x := range r.flat() {
				if x.Ref == m.sel {
					m.bRow, m.bK = ri, k
				}
			}
		}
		for i, it := range m.cItems(rows) {
			if it.task != nil && it.task.Ref == m.sel {
				m.cIdx = i
			}
		}
	} else {
		m.aRow, m.aCol, m.aK, m.bRow, m.bK, m.cIdx = 0, 0, 0, 0, 0, 0
	}
	m.aClamp(rows)
	m.bClamp(rows)
	m.cClamp(rows)
	switch m.variant {
	case 0:
		m.sel = m.aSel(rows)
	case 1:
		m.sel = m.bSel(rows)
	case 2:
		m.sel = m.cSel(rows)
	}
}

// ---------- View ----------

func (m model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "Taiga TUI · PROTOTYP Taskboard"
	return v
}

func (m model) render() string {
	if m.w == 0 {
		return ""
	}
	header := m.header()
	footer := m.switcher()
	bodyH := m.h - lipgloss.Height(header) - lipgloss.Height(footer)
	var body string
	switch m.variant {
	case 0:
		body = m.viewA(bodyH)
	case 1:
		body = m.viewB(bodyH)
	case 2:
		body = m.viewC(bodyH)
	}
	base := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	if m.detail {
		box := m.detailBox()
		x := max(0, (m.w-lipgloss.Width(box))/2)
		y := max(0, (m.h-lipgloss.Height(box))/2)
		return lipgloss.NewCompositor(
			lipgloss.NewLayer(base),
			lipgloss.NewLayer(box).X(x).Y(y).Z(1),
		).Render()
	}
	return base
}

func (m model) header() string {
	left := bold.Render(" "+projectName) + faint.Render("  ›  ") + bold.Render(sprintName) + faint.Render("  "+sprintDates)
	filter := "alle Tasks"
	if m.mine {
		filter = mineSt.Render("nur meine")
	}
	right := faint.Render("Filter: ") + filter + faint.Render("  ·  angemeldet als ") + mineSt.Render(assigneeName(meID)) + " "
	if m.w-lipgloss.Width(left)-lipgloss.Width(right) < 1 {
		right = faint.Render("Filter: ") + filter + " "
	}
	gap := m.w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return fit(left, m.w-lipgloss.Width(right)-1) + " " + right
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m model) switcher() string {
	pill := lipgloss.NewStyle().Background(lipgloss.Color("#FFD866")).Foreground(lipgloss.Color("#1E1E1E")).Bold(true)
	bar := lipgloss.NewStyle().Background(lipgloss.Color("#3B3F51")).Foreground(lipgloss.Color("#F0F0F0"))
	many := "aus"
	if m.many {
		many = "an"
	}
	name := variantNames[m.variant]
	if m.variant == 2 {
		name += "  ·  ⏎ auf Task: Detail ein/aus, auf User Story: einklappen"
	}
	l := pill.Render(" PROTOTYP ") + bar.Render(" ◀ shift+tab  "+name+"  tab ▶ ")
	r := bar.Render(fmt.Sprintf(" %d×%d · m nur meine · S 8 Status: %s · ⏎ Detail · q ", m.w, m.h, many))
	if m.w-lipgloss.Width(l)-lipgloss.Width(r) < 0 {
		r = bar.Render(fmt.Sprintf(" %d×%d · m meine · S Status · ⏎ · q ", m.w, m.h))
	}
	if m.w < 120 {
		l = pill.Render(" PROTOTYP ") + bar.Render(" ⇥ "+variantNames[m.variant][:1]+" ")
		r = bar.Render(fmt.Sprintf(" %d×%d · m meine · S Status · ⏎ · q ", m.w, m.h))
	}
	gap := m.w - lipgloss.Width(l) - lipgloss.Width(r)
	if gap < 0 {
		return fit(l+r, m.w)
	}
	return l + bar.Render(strings.Repeat(" ", gap)) + r
}

func (m model) detailBox() string {
	t := taskByRef(m.sel)
	if t == nil {
		return ""
	}
	w := min(72, m.w-4)
	us := "ohne User Story"
	if s := storyByRef(t.Story); s != nil {
		us = fmt.Sprintf("#%d %s", s.Ref, s.Subject)
	}
	st := m.statusOf(*t)
	who := assigneeName(t.Assignee)
	if t.Assignee == meID {
		who = mineSt.Render(who + " (du)")
	} else if t.Assignee == 0 {
		who = faint.Render(who)
	}
	label := func(s string) string { return faint.Render(fit(s, 12)) }
	desc := t.Desc
	if desc == "" {
		desc = faint.Render("Keine Beschreibung.")
	}
	inner := w - 4
	content := lipgloss.JoinVertical(lipgloss.Left,
		bold.Render(lipgloss.NewStyle().Width(inner).Render(fmt.Sprintf("Task #%d  %s", t.Ref, t.Subject))),
		"",
		label("Status")+colorSt(m.statusColor(st)).Render("● "+st),
		label("Assignee")+who,
		label("User Story")+fit(us, inner-12),
		label("Sprint")+sprintName,
		"",
		lipgloss.NewStyle().Width(inner).Render(desc),
		"",
		faint.Render("nur lesend · esc schließen"),
	)
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#FFD866")).Padding(0, 1).Width(w).Render(content)
}

func main() {
	v := flag.String("variant", "A", "A, B oder C")
	snap := flag.String("snap", "", "Snapshot statt TUI, z. B. 140x40")
	keys := flag.String("keys", "", "Tasten für -snap, kommagetrennt")
	raw := flag.Bool("raw", false, "Snapshot mit ANSI-Farben")
	flag.Parse()
	m := model{collapsed: map[int]bool{}, sc: &scroll{}, usStyle: 1}
	m.variant = max(0, min(2, int(strings.ToUpper(*v)[0]-'A')))
	m.sync()
	if *snap != "" {
		snapshot(m, *snap, *keys, *raw)
		return
	}
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
