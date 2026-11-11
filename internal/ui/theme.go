package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// All styles of the app live here.
var (
	accentColor = lipgloss.Color("#FFD866")
	errorColor  = lipgloss.Color("#FF6188")
	warnColor   = lipgloss.Color("#FFD866")
	// The text colors on a tab filled with a light or dark Status color.
	darkTabText  = lipgloss.Color("#1E1E1E")
	lightTabText = lipgloss.Color("#F5F5F5")
	// fallbackStatusColor is used when Taiga has no valid color for a Status.
	fallbackStatusColor = lipgloss.Color("#A0A0A0")
	progressColor       = lipgloss.Color("#A8E440")

	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	faintStyle  = lipgloss.NewStyle().Faint(true)
	labelStyle  = lipgloss.NewStyle().Bold(true)
	errorStyle  = lipgloss.NewStyle().Foreground(errorColor)
	warnStyle   = lipgloss.NewStyle().Foreground(warnColor)
	promptStyle = lipgloss.NewStyle().Foreground(accentColor)
	dialogStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accentColor).
			Padding(1, 3)

	// mineStyle marks what belongs to the logged-in Team Member.
	mineStyle     = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	cursorStyle   = lipgloss.NewStyle().Reverse(true)
	disabledStyle = lipgloss.NewStyle().Faint(true)
	progressStyle = lipgloss.NewStyle().Foreground(progressColor)
	// projectStyle names the project in the header.
	projectStyle = lipgloss.NewStyle().Bold(true)
)

// tabStyle styles a Status tab: the active one filled with the Status
// color, the others written in it.
func tabStyle(hex string, active bool) lipgloss.Style {
	c := statusColor(hex)
	if active {
		return lipgloss.NewStyle().Bold(true).Background(c).Foreground(textOn(c))
	}
	return lipgloss.NewStyle().Foreground(c)
}

// statusColor returns Taiga's color of a Status, which is "#rrggbb" or
// "#rgb", or the fallback for anything else.
func statusColor(hex string) color.Color {
	if !validHex(hex) {
		return fallbackStatusColor
	}
	return lipgloss.Color(hex)
}

func validHex(s string) bool {
	if len(s) != 4 && len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, c := range s[1:] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// textOn returns a text color that stays readable on the background bg.
func textOn(bg color.Color) color.Color {
	r, g, b, _ := bg.RGBA() // 16 bit per channel
	// Perceived brightness after ITU-R BT.601, from 0 to 65535.
	if 299*r+587*g+114*b > 1000*0x8000 {
		return darkTabText
	}
	return lightTabText
}
