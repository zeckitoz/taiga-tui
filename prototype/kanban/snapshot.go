// PROTOTYPE - snapshot mode for checking without a terminal: go run . -snap 140x40 -keys "tab,down,enter"
package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func snapshot(m model, size, keys string, raw bool) {
	var w, h int
	fmt.Sscanf(size, "%dx%d", &w, &h)
	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: w, Height: h})
	for _, k := range strings.Split(keys, ",") {
		if k == "" {
			continue
		}
		var key tea.Key
		switch k {
		case "tab":
			key = tea.Key{Code: tea.KeyTab}
		case "shift+tab":
			key = tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}
		case "enter":
			key = tea.Key{Code: tea.KeyEnter}
		case "backspace":
			key = tea.Key{Code: tea.KeyBackspace}
		case "ctrl+k":
			key = tea.Key{Code: 'k', Mod: tea.ModCtrl}
		case "esc":
			key = tea.Key{Code: tea.KeyEscape}
		case "space":
			key = tea.Key{Code: tea.KeySpace, Text: " "}
		case "up":
			key = tea.Key{Code: tea.KeyUp}
		case "down":
			key = tea.Key{Code: tea.KeyDown}
		case "left":
			key = tea.Key{Code: tea.KeyLeft}
		case "right":
			key = tea.Key{Code: tea.KeyRight}
		default:
			key = tea.Key{Code: rune(k[0]), Text: k}
		}
		tm, _ = tm.Update(tea.KeyPressMsg(key))
	}
	out := tm.(model).render()
	if !raw {
		out = ansi.Strip(out)
	}
	for _, l := range strings.Split(out, "\n") {
		fmt.Println(l + "|")
	}
}
