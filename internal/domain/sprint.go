package domain

import (
	"cmp"
	"slices"
	"time"
)

// Date is a calendar day without a time zone.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// DateOf returns the day of t in t's location.
func DateOf(t time.Time) Date {
	y, m, d := t.Date()
	return Date{Year: y, Month: m, Day: d}
}

// Compare returns -1, 0 or +1 as d is before, equal to or after e.
func (d Date) Compare(e Date) int {
	if c := cmp.Compare(d.Year, e.Year); c != 0 {
		return c
	}
	if c := cmp.Compare(d.Month, e.Month); c != 0 {
		return c
	}
	return cmp.Compare(d.Day, e.Day)
}

// Sprint is a time-boxed iteration of a Scrum project.
type Sprint struct {
	ID   int64
	Name string
	// Start and Finish are the first and last day of the Sprint.
	Start, Finish Date
	Closed        bool
}

// Contains reports whether day lies within the Sprint's date range.
func (s Sprint) Contains(day Date) bool {
	return s.Start.Compare(day) <= 0 && day.Compare(s.Finish) <= 0
}

// ResolutionKind tells how the Current Sprint was resolved.
type ResolutionKind int

// The kinds of a Resolution.
const (
	// None means the project has no Sprints.
	None ResolutionKind = iota
	// One means Sprint is the Sprint to open.
	One
	// Choose means the Team Member has to pick one of Choices.
	Choose
)

// Resolution is the result of ResolveCurrentSprint.
type Resolution struct {
	Kind   ResolutionKind
	Sprint Sprint
	// Choices are newest first.
	Choices []Sprint
}

// ResolveCurrentSprint decides which Sprint to open on today. Only the date
// range counts, the closed flag is ignored: a single Sprint containing today
// is opened. Otherwise the Team Member chooses among the Sprints containing
// today, or, if there are none, the open Sprints, or, if there are none
// either, all Sprints. If that list contains the remembered Sprint, it is
// opened without asking.
func ResolveCurrentSprint(sprints []Sprint, today Date, remembered int64) Resolution {
	if len(sprints) == 0 {
		return Resolution{Kind: None}
	}
	choices := filter(sprints, func(s Sprint) bool { return s.Contains(today) })
	if len(choices) == 1 {
		return Resolution{Kind: One, Sprint: choices[0]}
	}
	if len(choices) == 0 {
		choices = filter(sprints, func(s Sprint) bool { return !s.Closed })
	}
	if len(choices) == 0 {
		choices = slices.Clone(sprints)
	}
	if i := slices.IndexFunc(choices, func(s Sprint) bool { return s.ID == remembered }); i >= 0 {
		return Resolution{Kind: One, Sprint: choices[i]}
	}
	slices.SortStableFunc(choices, func(a, b Sprint) int { return b.Start.Compare(a.Start) })
	return Resolution{Kind: Choose, Choices: choices}
}

func filter(sprints []Sprint, keep func(Sprint) bool) []Sprint {
	var out []Sprint
	for _, s := range sprints {
		if keep(s) {
			out = append(out, s)
		}
	}
	return out
}
