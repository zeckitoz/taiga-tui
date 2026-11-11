// PROTOTYP - Variante A: Raster wie in der Web-UI.
// Jede User Story ist ein Band über die volle Breite, darunter eine Zeile Zellen pro Status.
// Passen nicht alle Status in die Breite, scrollt das Raster horizontal (Mindestspaltenbreite).
package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

const aMinCol = 26

func (m *model) aClamp(rows []Row) {
	if len(rows) == 0 {
		m.aRow, m.aCol, m.aK = 0, 0, 0
		return
	}
	m.aRow = max(0, min(m.aRow, len(rows)-1))
	m.aCol = max(0, min(m.aCol, len(m.statuses())-1))
	m.aK = max(0, min(m.aK, len(rows[m.aRow].Cells[m.aCol])-1))
}

func (m model) aSel(rows []Row) int {
	if len(rows) == 0 {
		return 0
	}
	c := rows[m.aRow].Cells[m.aCol]
	if m.aK < len(c) {
		return c[m.aK].Ref
	}
	return 0
}

func (m *model) aMove(dx, dy int) {
	rows := m.rows()
	if len(rows) == 0 {
		return
	}
	if dx != 0 {
		m.aCol += dx
	}
	if dy < 0 {
		if m.aK > 0 {
			m.aK--
		} else if m.aRow > 0 {
			m.aRow--
			m.aK = len(rows[m.aRow].Cells[max(0, min(m.aCol, len(m.statuses())-1))]) - 1
		}
	}
	if dy > 0 {
		cell := rows[m.aRow].Cells[m.aCol]
		if m.aK < len(cell)-1 {
			m.aK++
		} else if m.aRow < len(rows)-1 {
			m.aRow++
			m.aK = 0
		}
	}
	m.aClamp(rows)
	m.sel = m.aSel(rows)
}

func (m model) viewA(h int) string {
	rows := m.rows()
	sts := m.statuses()
	W := m.w
	visible := max(1, min(len(sts), W/aMinCol))
	// horizontales Fenster nachziehen
	if m.aCol < m.sc.colOff {
		m.sc.colOff = m.aCol
	}
	if m.aCol >= m.sc.colOff+visible {
		m.sc.colOff = m.aCol - visible + 1
	}
	m.sc.colOff = max(0, min(m.sc.colOff, len(sts)-visible))
	off := m.sc.colOff
	colW := W / visible

	// Kopfzeile mit Status
	var head []string
	for i := off; i < off+visible; i++ {
		s := sts[i]
		n := 0
		for _, r := range rows {
			n += len(r.Cells[i])
		}
		label := colorSt(s.Color).Render("● ") + bold.Render(s.Name) + faint.Render(fmt.Sprintf(" %d", n))
		if i == off && off > 0 {
			label = mineSt.Render(fmt.Sprintf("‹%d ", off)) + label
		}
		if i == off+visible-1 && off+visible < len(sts) {
			more := mineSt.Render(fmt.Sprintf(" %d›", len(sts)-off-visible))
			head = append(head, fit(label, colW-lipgloss.Width(more))+more)
			continue
		}
		head = append(head, fit(label, colW))
	}
	headLine := strings.Join(head, "")
	rule := faint.Render(strings.Repeat("─", W))

	var lines []string
	focus := 0
	if len(rows) == 0 {
		lines = append(lines, faint.Render("  Keine Tasks für diesen Filter."))
	}
	for ri, r := range rows {
		done := len(r.Cells[len(sts)-2]) // "Closed" ist vorletzter Status in beiden Sets
		band := fmt.Sprintf(" ▾ %s", r.title())
		meta := fmt.Sprintf("%d/%d erledigt ", done, r.count())
		if r.count() == 0 {
			meta = "keine Tasks "
		}
		bw := W - lipgloss.Width(meta)
		lines = append(lines, bandSt.Render(fit(band, bw-1)+" "+meta))
		height := 1
		for i := off; i < off+visible; i++ {
			height = max(height, len(r.Cells[i]))
		}
		for k := 0; k < height; k++ {
			var cells []string
			for i := off; i < off+visible; i++ {
				isSel := ri == m.aRow && i == m.aCol && (k == m.aK || (len(r.Cells[i]) == 0 && k == 0))
				if isSel {
					focus = len(lines)
				}
				cells = append(cells, m.aCard(r.Cells[i], k, colW, isSel))
			}
			lines = append(lines, strings.Join(cells, ""))
		}
		lines = append(lines, "")
	}
	body := m.window(lines, focus, h-2)
	return lipgloss.JoinVertical(lipgloss.Left, append([]string{headLine, rule}, body...)...)
}

// aCard rendert Task k einer Zelle als einzeilige Karte: Marker, Ref, Titel, Initialen.
func (m model) aCard(cell []Task, k, w int, sel bool) string {
	inner := w - 1
	if k >= len(cell) {
		if sel {
			return selStyle.Render(fit("  ·", inner)) + " "
		}
		if k == 0 && len(cell) == 0 {
			return faint.Render(fit("  ·", inner)) + " "
		}
		return strings.Repeat(" ", w)
	}
	t := cell[k]
	own := t.Assignee == meID
	marker := " "
	if own {
		marker = "▍"
	}
	ini := initials(t.Assignee)
	ref := fmt.Sprintf("#%d ", t.Ref)
	titleW := inner - 1 - len(ref) - 3
	if sel {
		return selStyle.Render(marker+ref+fit(t.Subject, titleW)+" "+ini) + " "
	}
	iniR := faint.Render(ini)
	if own {
		marker = mineSt.Render(marker)
		iniR = mineSt.Render(ini)
	}
	return marker + faint.Render(ref) + fit(t.Subject, titleW) + " " + iniR + " "
}
