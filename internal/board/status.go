package board

import (
	"maps"
	"slices"

	"github.com/zeckitoz/taiga-tui/internal/domain"
)

// Effect is what an action or result asks the App to do, as data: the App
// translates it into a request and feeds the request's result back into the
// board (ADR 0001).
type Effect interface{ isEffect() }

// ItemKind tells a User Story from a Task.
type ItemKind int

// The kinds of work items.
const (
	UserStoryItem ItemKind = iota
	TaskItem
)

// SaveStatus is the effect of saving a Status Change. Its result goes back
// into StatusSaved, TaskSaved or UserStorySaved, or into StatusNotSaved.
type SaveStatus struct {
	Kind ItemKind
	ID   int64
	Ref  int
	// Status is the new Status, Version that of the item as last loaded.
	Status  int64
	Version int
	// Generation tells this save apart from earlier saves of the item, whose
	// results no longer count.
	Generation int
}

// ReloadBoard is the effect of reloading the board shown, after a save
// found the item changed or gone on the server.
type ReloadBoard struct{}

// ReloadUserStory is the effect of reloading the User Story of a Task in a
// Sprint after the Task's Status changed. Its result goes back into
// Taskboard.UserStoryReloaded.
type ReloadUserStory struct {
	ID     int64
	Sprint int64
}

func (SaveStatus) isEffect()      {}
func (ReloadBoard) isEffect()     {}
func (ReloadUserStory) isEffect() {}

// SaveFailure tells why a save did not happen.
type SaveFailure int

// The failures of a save.
const (
	// SaveConflict means someone else changed the item since it was loaded.
	SaveConflict SaveFailure = iota + 1
	// SaveNotFound means the item no longer exists.
	SaveNotFound
	// SaveFailed means the save failed for any other reason.
	SaveFailed
)

// Rejection is why the board refuses an action without asking the server.
// Its text is shown to the Team Member as it is.
type Rejection string

func (r Rejection) Error() string { return string(r) }

// The rejections of a Status Change.
const (
	ErrStillSaving   Rejection = "Still saving…"
	ErrFirstStatus   Rejection = "Already in first status"
	ErrLastStatus    Rejection = "Already in last status"
	ErrUnknownStatus Rejection = "Unknown status"
)

// noPermission is the rejection of a change to an item of kind the Team
// Member may not modify.
func noPermission(kind ItemKind) Rejection {
	if kind == TaskItem {
		return "You don't have permission to change tasks"
	}
	return "You don't have permission to change user stories"
}

// mayModify reports whether the Team Member may change items of kind in p.
func mayModify(p domain.Project, kind ItemKind) bool {
	if kind == TaskItem {
		return p.Permissions.ModifyTask
	}
	return p.Permissions.ModifyUserStory
}

// statusesOf returns the Statuses an item of kind can have in p, in Taiga
// order. User Stories never go into an archived Status, which would hide
// them from the boards.
func statusesOf(p domain.Project, kind ItemKind) []domain.Status {
	if kind == TaskItem {
		return p.TaskStatuses
	}
	return slices.DeleteFunc(slices.Clone(p.UserStoryStatuses), func(s domain.Status) bool { return s.Archived })
}

// adjacent returns the ID of the Status after (dir 1) or before (dir -1)
// the Status current among statuses.
func adjacent(statuses []domain.Status, current int64, dir int) (int64, error) {
	i := slices.IndexFunc(statuses, func(s domain.Status) bool { return s.ID == current })
	switch {
	case i < 0:
		return 0, ErrUnknownStatus
	case i+dir < 0:
		return 0, ErrFirstStatus
	case i+dir >= len(statuses):
		return 0, ErrLastStatus
	}
	return statuses[i+dir].ID, nil
}

// itemKey identifies a work item across both kinds.
type itemKey struct {
	kind ItemKind
	id   int64
}

// saves holds the Status Changes being saved, one per item: the item shows
// its new Status until the result arrives, and further changes to it are
// refused. Its methods return a new store and never change the receiver.
type saves struct {
	// generation counts the saves; it never goes back, so that a result
	// matches only the save that asked for it.
	generation int
	byItem     map[itemKey]SaveStatus
}

// changeStatus starts the Status Change of the item of kind with the given
// ID, ref, Status and version in direction dir, unless the Team Member may
// not change it, it is still saving or its Status has no Status next to it there.
func (s saves) changeStatus(p domain.Project, kind ItemKind, id int64, ref int, status int64, version, dir int) (saves, SaveStatus, error) {
	if !mayModify(p, kind) {
		return s, SaveStatus{}, noPermission(kind)
	}
	if s.saving(kind, id) {
		return s, SaveStatus{}, ErrStillSaving
	}
	to, err := adjacent(statusesOf(p, kind), status, dir)
	if err != nil {
		return s, SaveStatus{}, err
	}
	s.generation++
	save := SaveStatus{Kind: kind, ID: id, Ref: ref, Status: to, Version: version, Generation: s.generation}
	s.byItem = maps.Clone(s.byItem) // never share the map with the receiver
	if s.byItem == nil {
		s.byItem = map[itemKey]SaveStatus{}
	}
	s.byItem[itemKey{kind, id}] = save
	return s, save, nil
}

// saving reports whether the item of kind with the given ID is saving.
func (s saves) saving(kind ItemKind, id int64) bool {
	_, ok := s.byItem[itemKey{kind, id}]
	return ok
}

// any reports whether any item is saving.
func (s saves) any() bool { return len(s.byItem) > 0 }

// status returns the Status the item of kind with the given ID shows: the
// one being saved, or else status.
func (s saves) status(kind ItemKind, id, status int64) int64 {
	if save, ok := s.byItem[itemKey{kind, id}]; ok {
		return save.Status
	}
	return status
}

// end ends save, or returns false if its result no longer counts.
func (s saves) end(save SaveStatus) (saves, bool) {
	key := itemKey{save.Kind, save.ID}
	if cur, ok := s.byItem[key]; !ok || cur != save {
		return s, false
	}
	s.byItem = maps.Clone(s.byItem)
	delete(s.byItem, key)
	return s, true
}

// dropped forgets all saves, whose results then no longer count.
func (s saves) dropped() saves {
	s.byItem = nil
	return s
}

// reloadOn tells whether a failure asks to reload the board.
func reloadOn(f SaveFailure) Effect {
	if f == SaveConflict || f == SaveNotFound {
		return ReloadBoard{}
	}
	return nil
}
