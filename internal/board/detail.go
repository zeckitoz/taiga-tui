package board

import (
	"maps"
	"slices"

	"github.com/zeckitoz/taiga-tui/internal/domain"
)

// Detail is what the detail view shows of the selected User Story.
type Detail struct {
	ID    int64
	Ref   int
	Title string
	// Status is the zero Status if the User Story's Status is unknown.
	Status     domain.Status
	Assignees  []Person
	SprintName string
	Tasks      domain.TaskProgress
	// Description is loaded separately, see LoadDetail.
	Description Description
}

// Description is the description of a User Story and the state of its load.
type Description struct {
	State DescriptionState
	// Text is the Markdown text; while it is reloaded, the earlier text.
	Text string
	// Err is why the load failed.
	Err error
}

// DescriptionState tells whether the description is there.
type DescriptionState int

// The states of a description.
const (
	// DescriptionLoading means no description is there yet.
	DescriptionLoading DescriptionState = iota
	// DescriptionLoaded means Text holds the description.
	DescriptionLoaded
	// DescriptionFailed means the latest load failed with Err.
	DescriptionFailed
)

// description is a description in the store with the state of its latest
// load.
type description struct {
	text string
	// loaded is set once text holds a description, possibly of an earlier
	// generation.
	loaded bool
	// generation is that of the latest load, which done tells has ended,
	// with err if it failed.
	generation int
	done       bool
	err        error
}

// LoadUserStory is the effect of loading a User Story with its description.
// Its result goes back into UserStoryLoaded or UserStoryFailed.
type LoadUserStory struct {
	ID         int64
	Generation int
}

// LoadDetail returns the effect of loading the description of the selected
// User Story, unless it was loaded or is loading since the last Reload.
func (k Kanban) LoadDetail() (Kanban, LoadUserStory, bool) {
	d, ok := k.Detail()
	if !ok {
		return k, LoadUserStory{}, false
	}
	e, known := k.descriptions[d.ID]
	if known && e.generation == k.generation {
		return k, LoadUserStory{}, false
	}
	e.generation, e.done, e.err = k.generation, false, nil
	k.descriptions = maps.Clone(k.descriptions) // never share the map with the receiver
	k.descriptions[d.ID] = e
	return k, LoadUserStory{ID: d.ID, Generation: k.generation}, true
}

// UserStoryLoaded takes the description from the result of load.
func (k Kanban) UserStoryLoaded(load LoadUserStory, s domain.UserStory) Kanban {
	return k.loadEnded(load, func(e *description) { e.text, e.loaded = s.Description, true })
}

// UserStoryFailed records that load failed with err.
func (k Kanban) UserStoryFailed(load LoadUserStory, err error) Kanban {
	return k.loadEnded(load, func(e *description) { e.err = err })
}

func (k Kanban) loadEnded(load LoadUserStory, apply func(*description)) Kanban {
	e, ok := k.descriptions[load.ID]
	if !ok || load.Generation != k.generation || e.generation != k.generation {
		return k // a result of a load before the last Reload
	}
	e.done = true
	apply(&e)
	k.descriptions = maps.Clone(k.descriptions)
	k.descriptions[load.ID] = e
	return k
}

func (k Kanban) description(id int64) Description {
	e := k.descriptions[id]
	switch {
	case e.done && e.err != nil:
		return Description{State: DescriptionFailed, Err: e.err}
	case e.loaded:
		return Description{State: DescriptionLoaded, Text: e.text}
	}
	return Description{State: DescriptionLoading}
}

// Person is a Team Member as shown in the detail view.
type Person struct {
	Name string
	// Mine marks the logged-in Team Member.
	Mine bool
}

// Detail returns the detail of the selected User Story, or false if the
// active tab is empty.
func (k Kanban) Detail() (Detail, bool) {
	i := k.SelectedIndex()
	if i < 0 {
		return Detail{}, false
	}
	s := k.active()[i]
	d := Detail{
		ID:          s.ID,
		Ref:         s.Ref,
		Title:       s.Title,
		SprintName:  s.SprintName,
		Tasks:       s.Tasks,
		Description: k.description(s.ID),
	}
	if j := slices.IndexFunc(k.statuses, func(st domain.Status) bool { return st.ID == s.Status }); j >= 0 {
		d.Status = k.statuses[j]
	}
	for _, id := range s.Assignees {
		p := Person{Name: "Unknown member", Mine: id == k.me}
		if m, ok := k.project.Member(id); ok {
			p.Name = m.FullName
		}
		d.Assignees = append(d.Assignees, p)
	}
	return d, true
}
