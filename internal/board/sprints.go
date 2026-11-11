package board

import (
	"slices"

	"github.com/zeckitoz/taiga-tui/internal/domain"
)

// Sprints holds the Sprints of one project and the Taskboard of the Sprint
// opened last. It hands out the loads of both as effects and drops results
// of earlier loads and of other projects. The zero Sprints takes no results.
// Its methods return a new Sprints and never change the receiver.
type Sprints struct {
	project domain.Project
	me      int64
	list    []domain.Sprint
	listed  bool
	// sprintsGen and taskboardGen count the loads; only the latest counts.
	sprintsGen, taskboardGen int
	taskboard                Taskboard
	hasTaskboard             bool
}

// LoadSprints is the effect of loading the Sprints of a project. Its result
// goes back into SprintsLoaded.
type LoadSprints struct {
	ProjectID  int64
	Generation int
	// Refresh tells that the Sprints were loaded before.
	Refresh bool
}

// LoadTaskboard is the effect of loading the Taskboard of a Sprint. Its
// result goes back into TaskboardLoaded.
type LoadTaskboard struct {
	ProjectID  int64
	Sprint     domain.Sprint
	Generation int
	// Refresh tells that the Taskboard of the Sprint is shown already and
	// stays visible while it reloads.
	Refresh bool
}

// NewSprints returns the Sprints of p, not loaded yet, seen by the Team
// Member with ID me.
func NewSprints(p domain.Project, me int64) Sprints {
	return Sprints{project: p, me: me}
}

// LoadSprints returns the effect of loading the Sprints.
func (s Sprints) LoadSprints() (Sprints, LoadSprints) {
	s.sprintsGen++
	return s, LoadSprints{ProjectID: s.project.ID, Generation: s.sprintsGen, Refresh: s.listed}
}

// SprintsCurrent reports whether the result of load still counts.
func (s Sprints) SprintsCurrent(load LoadSprints) bool {
	return s.project.ID != 0 && load.ProjectID == s.project.ID && load.Generation == s.sprintsGen
}

// SprintsLoaded takes the Sprints from the result of load, newest first as
// Taiga lists them, or returns false if the result no longer counts.
func (s Sprints) SprintsLoaded(load LoadSprints, list []domain.Sprint) (Sprints, bool) {
	if !s.SprintsCurrent(load) {
		return s, false
	}
	s.list, s.listed = slices.Clone(list), true
	return s, true
}

// Listed reports whether the Sprints are loaded.
func (s Sprints) Listed() bool { return s.listed }

// List returns the Sprints as Taiga lists them.
func (s Sprints) List() []domain.Sprint { return slices.Clone(s.list) }

// Knows reports whether the Sprint with the given ID is among the Sprints.
func (s Sprints) Knows(id int64) bool {
	return slices.ContainsFunc(s.list, func(sp domain.Sprint) bool { return sp.ID == id })
}

// Open returns the effect of loading the Taskboard of sprint. The Taskboard
// shown so far stays until the new one has loaded.
func (s Sprints) Open(sprint domain.Sprint) (Sprints, LoadTaskboard) {
	if s.hasTaskboard && s.taskboard.Sprint().ID != sprint.ID {
		// Results of saves no longer count once another Sprint is chosen.
		s.taskboard = s.taskboard.DropSaves()
	}
	s.taskboardGen++
	return s, LoadTaskboard{
		ProjectID:  s.project.ID,
		Sprint:     sprint,
		Generation: s.taskboardGen,
		Refresh:    s.hasTaskboard && s.taskboard.Sprint().ID == sprint.ID,
	}
}

// TaskboardCurrent reports whether the result of load still counts.
func (s Sprints) TaskboardCurrent(load LoadTaskboard) bool {
	return s.project.ID != 0 && load.ProjectID == s.project.ID && load.Generation == s.taskboardGen
}

// TaskboardLoaded shows the Taskboard from the result of load, or returns
// false if the result no longer counts. A refresh keeps the collapsed User
// Stories and the selection.
func (s Sprints) TaskboardLoaded(load LoadTaskboard, c domain.SprintContent) (Sprints, bool) {
	if !s.TaskboardCurrent(load) {
		return s, false
	}
	if s.hasTaskboard && s.taskboard.Sprint().ID == load.Sprint.ID {
		s.taskboard = s.taskboard.Reload(c)
	} else {
		tb := NewTaskboard(s.project, load.Sprint, c, s.me)
		// Saves of the new Taskboard never match results of the old one.
		tb.saves.generation = s.taskboard.saves.generation
		s.taskboard, s.hasTaskboard = tb, true
	}
	return s, true
}

// Taskboard returns the Taskboard shown, or false if none has loaded.
func (s Sprints) Taskboard() (Taskboard, bool) { return s.taskboard, s.hasTaskboard }

// WithTaskboard replaces the Taskboard shown by tb, which is a Taskboard
// returned by Taskboard and changed since, e.g. by moving the cursor.
func (s Sprints) WithTaskboard(tb Taskboard) Sprints {
	if s.hasTaskboard {
		s.taskboard = tb
	}
	return s
}
