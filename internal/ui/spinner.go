package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// TickFunc schedules a message after a delay, like tea.Tick. Tests replace it
// so that no timer ever runs.
type TickFunc func(time.Duration, func(time.Time) tea.Msg) tea.Cmd

var spinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

const spinnerInterval = 100 * time.Millisecond

// spinnerID tells the spinners of the App apart.
type spinnerID int

const (
	// loadingSpinner shows that a screen is loading.
	loadingSpinner spinnerID = iota
	// savingSpinner turns on the rows whose changes are being saved.
	savingSpinner
	// refreshSpinner shows in the header while a board reloads.
	refreshSpinner
)

// spinnerTickMsg advances the spinner with the ID whose run matches.
type spinnerTickMsg struct {
	id  spinnerID
	run int
}

// spinner shows activity. Each start begins a new run, so ticks of an earlier
// run die out instead of speeding up the animation.
type spinner struct {
	id     spinnerID
	tick   TickFunc
	frame  int
	run    int
	active bool
}

func (s *spinner) start() tea.Cmd {
	s.run++
	s.frame = 0
	s.active = true
	return s.next()
}

func (s *spinner) stop() { s.active = false }

func (s *spinner) update(msg spinnerTickMsg) tea.Cmd {
	if !s.active || msg.id != s.id || msg.run != s.run {
		return nil
	}
	s.frame = (s.frame + 1) % len(spinnerFrames)
	return s.next()
}

func (s *spinner) next() tea.Cmd {
	id, run := s.id, s.run
	return s.tick(spinnerInterval, func(time.Time) tea.Msg { return spinnerTickMsg{id: id, run: run} })
}

func (s *spinner) view() string { return promptStyle.Render(s.glyph()) }

// glyph is the current frame without color.
func (s *spinner) glyph() string { return spinnerFrames[s.frame] }
