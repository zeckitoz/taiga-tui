package ui

import "charm.land/lipgloss/v2"

// All styles of the app live here.
var (
	accentColor = lipgloss.Color("#FFD866")
	errorColor  = lipgloss.Color("#FF6188")
	warnColor   = lipgloss.Color("#FFD866")

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
)
