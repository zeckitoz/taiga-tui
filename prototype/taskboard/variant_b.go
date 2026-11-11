// PROTOTYP - Variante B: kompakte Matrix als Überblick, darunter die Liste der fokussierten User Story.
// Eine Zeile pro User Story, in den Status-Spalten nur Initialen-Chips je Task.
// Der ganze Sprint passt auf einen Bildschirm; volle Titel stehen unten in der Liste.
package main

import (
	"fmt"
	"strings"
)

func (m *model) bClamp(rows []Row) {
	if len(rows) == 0 {
		m.bRow, m.bK = 0, 0
		return
	}
	m.bRow = max(0, min(m.bRow, len(rows)-1))
	m.bK = max(0, min(m.bK, len(rows[m.bRow].flat())-1))
}

func (m model) bSel(rows []Row) int {
	if len(rows) == 0 {
		return 0
	}
	f := rows[m.bRow].flat()
	if m.bK < len(f) {
		return f[m.bK].Ref
	}
	return 0
}

// statusIndex liefert die Spalte von Task k in der Zeile.
func bCol(r Row, k int) int {
	for ci, c := range r.Cells {
		if k < len(c) {
			return ci
		}
		k -= len(c)
	}
	return 0
}

func (m *model) bMove(dx, dy int) {
	rows := m.rows()
	if len(rows) == 0 {
		return
	}
	if dx != 0 {
		m.bK += dx
	}
	if dy != 0 && m.bRow+dy >= 0 && m.bRow+dy < len(rows) {
		col := bCol(rows[m.bRow], m.bK)
		m.bRow += dy
		// gleiche oder nächstgelegene Status-Spalte in der neuen Zeile
		r := rows[m.bRow]
		best, bestD, k := 0, 1<<30, 0
		for ci, c := range r.Cells {
			if len(c) > 0 {
				d := ci - col
				if d < 0 {
					d = -d
				}
				if d < bestD {
					best, bestD = k, d
				}
			}
			k += len(c)
		}
		m.bK = best
	}
	m.bClamp(rows)
	m.sel = m.bSel(rows)
}

func (m model) viewB(h int) string {
	rows := m.rows()
	sts := m.statuses()
	W := m.w
	titleW := min(40, max(16, W/3))
	cw := max(4, (W-titleW)/len(sts))
	compact := cw < 9 // zu schmal für Chips: nur Zahlen

	var head strings.Builder
	head.WriteString(faint.Render(fit(" User Story", titleW)))
	for _, s := range sts {
		head.WriteString(fit(colorSt(s.Color).Render("●")+" "+s.Name, cw))
	}
	lines := []string{head.String(), faint.Render(strings.Repeat("─", W))}

	for ri, r := range rows {
		rowSel := ri == m.bRow
		t := r.title()
		if rowSel {
			t = bold.Render("› " + t)
		} else {
			t = "  " + t
		}
		var b strings.Builder
		b.WriteString(fit(t, titleW-1) + " ")
		k := 0
		for _, c := range r.Cells {
			var chips []string
			if len(c) == 0 {
				chips = append(chips, faint.Render("·"))
			}
			if compact && len(c) > 0 {
				mine := 0
				for _, x := range c {
					if x.Assignee == meID {
						mine++
					}
				}
				s := fmt.Sprintf("%d", len(c))
				if rowSel && m.bK >= k && m.bK < k+len(c) {
					s = selStyle.Render(s)
				} else if mine > 0 {
					s = mineSt.Render(s)
				}
				chips = append(chips, s)
			} else {
				for j, x := range c {
					chip := initials(x.Assignee)
					switch {
					case rowSel && m.bK == k+j:
						chip = selStyle.Render(chip)
					case x.Assignee == meID:
						chip = mineSt.Render(chip)
					case x.Assignee == 0:
						chip = faint.Render(chip)
					}
					chips = append(chips, chip)
				}
			}
			k += len(c)
			b.WriteString(fit(strings.Join(chips, " "), cw))
		}
		lines = append(lines, b.String())
	}
	if len(rows) == 0 {
		lines = append(lines, faint.Render("  Keine Tasks für diesen Filter."))
	}
	lines = append(lines, "", faint.Render(strings.Repeat("─", W)))

	// untere Liste: alle Tasks der fokussierten User Story mit vollen Titeln
	if len(rows) > 0 {
		r := rows[m.bRow]
		lines = append(lines, bold.Render(" "+r.title()), "")
		stW := 18
		whoW := 18
		for k, x := range r.flat() {
			st := m.statusOf(x)
			who := assigneeName(x.Assignee)
			marker := "  "
			line := fmt.Sprintf("#%-4d ", x.Ref)
			titleW := W - 2 - len(line) - stW - whoW
			if k == m.bK {
				lines = append(lines, selStyle.Render(fit(marker+line+fit(x.Subject, titleW)+fit("● "+st, stW)+who, W)))
				continue
			}
			whoR := faint.Render(who)
			if x.Assignee == meID {
				marker = mineSt.Render("▍ ")
				whoR = mineSt.Render(who)
			}
			lines = append(lines, marker+faint.Render(line)+fit(x.Subject, titleW)+colorSt(m.statusColor(st)).Render(fit("● "+st, stW))+fit(whoR, whoW))
		}
		if r.count() == 0 {
			lines = append(lines, faint.Render("  Diese User Story hat im Sprint noch keine Tasks."))
		}
	}
	// Matrix oben bleibt stehen; gescrollt wird nur, wenn das Terminal zu niedrig ist.
	focus := 2 + m.bRow
	if len(rows) > 0 {
		focus = max(focus, 2+len(rows)+4+m.bK)
	}
	return strings.Join(m.window(lines, min(focus, len(lines)-1), h), "\n")
}
