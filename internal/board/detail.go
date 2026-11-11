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
	Assignees  []Assignee
	SprintName string
	Tasks      domain.TaskProgress
	// Description is loaded separately, see LoadDetail.
	Description Description
}

// Description is the description of a User Story and the state of its load.
type Description struct {
	State DescriptionState
	// Text is the Markdown text; while it is reloaded after a refresh, the
	// earlier text.
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

// descriptions keeps the descriptions loaded for a board by item ID. Its
// methods return a new store and never change the receiver.
type descriptions struct {
	// generation counts the loads of the board; results of description
	// loads from an earlier generation are dropped.
	generation int
	byID       map[int64]description
}

// reloaded marks all descriptions as outdated: they stay visible until they
// are loaded again.
func (d descriptions) reloaded() descriptions {
	d.generation++
	return d
}

// start begins loading the description of id and returns the generation of
// the load, unless it was loaded or is loading since the last reload. A
// failed load is tried again.
func (d descriptions) start(id int64) (descriptions, int, bool) {
	e, known := d.byID[id]
	if known && e.generation == d.generation && (!e.done || e.err == nil) {
		return d, 0, false
	}
	e.generation, e.done, e.err = d.generation, false, nil
	return d.with(id, e), d.generation, true
}

// loaded takes text as the result of the load of id in generation.
func (d descriptions) loaded(id int64, generation int, text string) descriptions {
	return d.ended(id, generation, func(e *description) { e.text, e.loaded = text, true })
}

// failed records that the load of id in generation failed with err.
func (d descriptions) failed(id int64, generation int, err error) descriptions {
	return d.ended(id, generation, func(e *description) { e.err = err })
}

func (d descriptions) ended(id int64, generation int, apply func(*description)) descriptions {
	e, ok := d.byID[id]
	if !ok || generation != d.generation || e.generation != d.generation {
		return d // a result of a load before the last reload
	}
	e.done = true
	apply(&e)
	return d.with(id, e)
}

func (d descriptions) with(id int64, e description) descriptions {
	d.byID = maps.Clone(d.byID) // never share the map with the receiver
	if d.byID == nil {
		d.byID = map[int64]description{}
	}
	d.byID[id] = e
	return d
}

// get returns the description of id and the state of its load.
func (d descriptions) get(id int64) Description {
	e := d.byID[id]
	switch {
	case e.done && e.err != nil:
		return Description{State: DescriptionFailed, Err: e.err}
	case e.loaded:
		return Description{State: DescriptionLoaded, Text: e.text}
	}
	return Description{State: DescriptionLoading}
}

// LoadUserStory is the effect of loading a User Story with its description.
// Its result goes back into UserStoryLoaded or UserStoryFailed.
type LoadUserStory struct {
	ID         int64
	Generation int
}

// LoadDetail returns the effect of loading the description of the selected
// User Story, unless it was loaded or is loading since the last Reload. A
// failed load is tried again.
func (k Kanban) LoadDetail() (Kanban, LoadUserStory, bool) {
	d, ok := k.Detail()
	if !ok {
		return k, LoadUserStory{}, false
	}
	var gen int
	k.descriptions, gen, ok = k.descriptions.start(d.ID)
	return k, LoadUserStory{ID: d.ID, Generation: gen}, ok
}

// UserStoryLoaded takes the description from the result of load.
func (k Kanban) UserStoryLoaded(load LoadUserStory, s domain.UserStory) Kanban {
	k.descriptions = k.descriptions.loaded(load.ID, load.Generation, s.Description)
	return k
}

// UserStoryFailed records that load failed with err.
func (k Kanban) UserStoryFailed(load LoadUserStory, err error) Kanban {
	k.descriptions = k.descriptions.failed(load.ID, load.Generation, err)
	return k
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
		Description: k.descriptions.get(s.ID),
	}
	if j := slices.IndexFunc(k.statuses, func(st domain.Status) bool { return st.ID == s.Status }); j >= 0 {
		d.Status = k.statuses[j]
	}
	d.Assignees = k.assignees(s)
	return d, true
}
