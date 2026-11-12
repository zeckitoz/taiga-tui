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

// Direction tells whether a Status Change goes to the previous or the next
// Status.
type Direction int

// The directions of a Status Change in Taiga order.
const (
	Previous Direction = -1
	Next     Direction = 1
)

// adjacent returns the ID of the Status next to the Status current among
// statuses in direction d.
func adjacent(statuses []domain.Status, current int64, d Direction) (int64, error) {
	i := slices.IndexFunc(statuses, func(s domain.Status) bool { return s.ID == current })
	j := i + int(d)
	switch {
	case i < 0:
		return 0, ErrUnknownStatus
	case j < 0:
		return 0, ErrFirstStatus
	case j >= len(statuses):
		return 0, ErrLastStatus
	}
	return statuses[j].ID, nil
}

// Sent tracks the saves sent to Taiga whose results have not arrived yet,
// on every board and in every project. The App keeps one for the whole run
// and records each save it sends and each result it receives. It locks an
// item until the result of its save arrives, even after the Team Member
// left the board, so that their own requests never conflict with each
// other, and it numbers the saves so that a result matches only its own
// save. Its methods return a new Sent and never change the receiver.
type Sent struct {
	// generation is that of the latest save sent; it never goes back.
	generation int
	byItem     map[itemKey]int
}

// Add records that save was sent.
func (s Sent) Add(save SaveStatus) Sent {
	s.generation = max(s.generation, save.Generation)
	s.byItem = maps.Clone(s.byItem) // never share the map with the receiver
	if s.byItem == nil {
		s.byItem = map[itemKey]int{}
	}
	s.byItem[itemKey{save.Kind, save.ID}] = save.Generation
	return s
}

// Answered records that the result of save arrived.
func (s Sent) Answered(save SaveStatus) Sent {
	key := itemKey{save.Kind, save.ID}
	if s.byItem[key] != save.Generation {
		return s
	}
	s.byItem = maps.Clone(s.byItem)
	delete(s.byItem, key)
	return s
}

// saving reports whether a save of the item of kind with the given ID is
// waiting for its result.
func (s Sent) saving(kind ItemKind, id int64) bool {
	_, ok := s.byItem[itemKey{kind, id}]
	return ok
}

// workItem is what a Status Change needs to know of a User Story or Task.
type workItem struct {
	kind    ItemKind
	id      int64
	ref     int
	status  int64
	version int
}

// itemKey identifies a work item across both kinds.
type itemKey struct {
	kind ItemKind
	id   int64
}

// saves holds the Status Changes of a board being saved, one per item: the
// item shows its new Status until the result arrives. Its methods return a
// new store and never change the receiver.
type saves struct {
	byItem map[itemKey]SaveStatus
}

// StatusOptions are the Statuses the selected row can be changed to.
type StatusOptions struct {
	Kind ItemKind
	// Item is the ID of the row's item, so that a caller can tell whether
	// the same row is still selected. Statuses are in Taiga order. Current
	// is the ID of the Status the row has.
	Item     int64
	Statuses []domain.Status
	Current  int64
}

// options returns the Statuses it can be changed to, or the reason why the
// Team Member cannot change it right now.
func (s saves) options(p domain.Project, sent Sent, it workItem) (StatusOptions, error) {
	if err := s.refusal(p, sent, it); err != nil {
		return StatusOptions{}, err
	}
	return StatusOptions{Kind: it.kind, Item: it.id, Statuses: statusesOf(p, it.kind), Current: it.status}, nil
}

// refusal is the Rejection of any change to it, or nil if there is none:
// the Team Member may not change it, or a save of it is still waiting for
// its result in sent.
func (s saves) refusal(p domain.Project, sent Sent, it workItem) error {
	if !mayModify(p, it.kind) {
		return noPermission(it.kind)
	}
	if sent.saving(it.kind, it.id) || s.saving(it.kind, it.id) {
		return ErrStillSaving
	}
	return nil
}

// target picks the Status a change of an item goes to among its statuses.
type target func(statuses []domain.Status, current int64) (int64, error)

// toNeighbour goes to the Status next to the current one in direction d.
func toNeighbour(d Direction) target {
	return func(statuses []domain.Status, current int64) (int64, error) {
		return adjacent(statuses, current, d)
	}
}

// toStatus goes to the Status with the given ID, which must be one the item
// can have.
func toStatus(id int64) target {
	return func(statuses []domain.Status, _ int64) (int64, error) {
		if !slices.ContainsFunc(statuses, func(s domain.Status) bool { return s.ID == id }) {
			return 0, ErrUnknownStatus
		}
		return id, nil
	}
}

// changeStatus starts the Status Change of it to the Status that to picks,
// unless the Team Member may not change it, a save of it is still waiting
// for its result in sent, or to refuses.
func (s saves) changeStatus(p domain.Project, sent Sent, it workItem, to target) (saves, SaveStatus, error) {
	if err := s.refusal(p, sent, it); err != nil {
		return s, SaveStatus{}, err
	}
	status, err := to(statusesOf(p, it.kind), it.status)
	if err != nil {
		return s, SaveStatus{}, err
	}
	save := SaveStatus{Kind: it.kind, ID: it.id, Ref: it.ref, Status: status, Version: it.version, Generation: sent.generation + 1}
	s.byItem = maps.Clone(s.byItem) // never share the map with the receiver
	if s.byItem == nil {
		s.byItem = map[itemKey]SaveStatus{}
	}
	s.byItem[itemKey{it.kind, it.id}] = save
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
