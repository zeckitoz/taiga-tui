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

// spinnerTickMsg advances the spinner whose run matches.
type spinnerTickMsg struct{ run int }

// spinner shows activity. Each start begins a new run, so ticks of an earlier
// run die out instead of speeding up the animation.
type spinner struct {
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
	if !s.active || msg.run != s.run {
		return nil
	}
	s.frame = (s.frame + 1) % len(spinnerFrames)
	return s.next()
}

func (s *spinner) next() tea.Cmd {
	run := s.run
	return s.tick(spinnerInterval, func(time.Time) tea.Msg { return spinnerTickMsg{run: run} })
}

func (s *spinner) view() string { return promptStyle.Render(spinnerFrames[s.frame]) }
