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

// Save is the effect of saving a change of an item: a Status Change or a
// Assign to me. Its result goes back into Saved, TaskSaved or UserStorySaved,
// or into NotSaved.
type Save struct {
	Kind ItemKind
	ID   int64
	Ref  int
	// Version is that of the item as last loaded.
	Version int
	// Status is the new Status of a Status Change, 0 for an Assign to me.
	Status int64
	// Assignee is the new main Assignee of an Assign to me, 0 for a Status
	// Change. Assignees are all Assignees of a User Story after a Take
	// Over, the main one first; they are nil for a Task.
	Assignee  int64
	Assignees []int64
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

func (Save) isEffect() {}

// ReloadProject is the effect of reloading the project data, because loaded
// data refers to Statuses or Team Members the project does not know. Its
// result goes back into ProjectReloaded.
type ReloadProject struct {
	// ProjectID and Generation tell which load of which board asked, so that
	// the result of a reload that a newer load made pointless is dropped.
	ProjectID  int64
	Generation int
}

func (ReloadProject) isEffect()   {}
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

// The rejections of a Status Change and an Assign to me.
const (
	ErrStillSaving     Rejection = "Still saving…"
	ErrUnknownStatus   Rejection = "Unknown status"
	ErrAlreadyAssigned Rejection = "Already assigned to you"
)

// ConfirmAssignToMe is what an Assign to me returns instead of an effect when it
// would displace the Assignee of a Task: the Team Member has to confirm
// first. It names the Task, the Assignee to replace and their Name. Passing
// it back to the Assign to me confirms it, as long as the Task still has that
// Assignee.
type ConfirmAssignToMe struct {
	ID       int64
	Assignee int64
	Name     string
}

func (ConfirmAssignToMe) isEffect() {}

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
func (s Sent) Add(save Save) Sent {
	s.generation = max(s.generation, save.Generation)
	s.byItem = maps.Clone(s.byItem) // never share the map with the receiver
	if s.byItem == nil {
		s.byItem = map[itemKey]int{}
	}
	s.byItem[itemKey{save.Kind, save.ID}] = save.Generation
	return s
}

// Answered records that the result of save arrived.
func (s Sent) Answered(save Save) Sent {
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

// workItem is what a Status Change or Assign to me needs to know of a User
// Story or Task.
type workItem struct {
	kind    ItemKind
	id      int64
	ref     int
	status  int64
	version int
	// assignees are the Assignees, the main one first: none or the one of a
	// Task.
	assignees []int64
}

// itemKey identifies a work item across both kinds.
type itemKey struct {
	kind ItemKind
	id   int64
}

// saves holds the changes of a board being saved, one per item: the item
// shows its new Status or Assignees until the result arrives. Its methods
// return a new store and never change the receiver.
type saves struct {
	byItem map[itemKey]Save
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

// changeStatus starts the Status Change of it to the Status with the given
// ID, unless the Team Member may not change it, a save of it is still
// waiting for its result in sent, or it cannot have that Status.
func (s saves) changeStatus(p domain.Project, sent Sent, it workItem, status int64) (saves, Save, error) {
	if err := s.refusal(p, sent, it); err != nil {
		return s, Save{}, err
	}
	if !slices.ContainsFunc(statusesOf(p, it.kind), func(st domain.Status) bool { return st.ID == status }) {
		return s, Save{}, ErrUnknownStatus
	}
	sv, save := s.start(sent, Save{Kind: it.kind, ID: it.id, Ref: it.ref, Version: it.version, Status: status})
	return sv, save, nil
}

// assignToMe starts assigning it to the Team Member with ID me, unless
// the Team Member may not change it, a save of it is still waiting for its
// result in sent, or they are an Assignee already. It returns the effect
// ConfirmAssignToMe instead if the Task has another Assignee and the Team
// Member has not confirmed replacing them.
func (s saves) assignToMe(p domain.Project, me int64, sent Sent, it workItem, confirmed ConfirmAssignToMe) (saves, Effect, error) {
	if err := s.refusal(p, sent, it); err != nil {
		return s, nil, err
	}
	if slices.Contains(it.assignees, me) {
		return s, nil, ErrAlreadyAssigned
	}
	save := Save{Kind: it.kind, ID: it.id, Ref: it.ref, Version: it.version, Assignee: me}
	if it.kind == TaskItem {
		if len(it.assignees) > 0 {
			ask := ConfirmAssignToMe{ID: it.id, Assignee: it.assignees[0], Name: memberName(p, it.assignees[0])}
			if confirmed != ask {
				return s, ask, nil
			}
		}
	} else {
		save.Assignees = append([]int64{me}, it.assignees...)
	}
	sv, save := s.start(sent, save)
	return sv, save, nil
}

// start records save, which gets the next generation after those in sent.
func (s saves) start(sent Sent, save Save) (saves, Save) {
	save.Generation = sent.generation + 1
	s.byItem = maps.Clone(s.byItem) // never share the map with the receiver
	if s.byItem == nil {
		s.byItem = map[itemKey]Save{}
	}
	s.byItem[itemKey{save.Kind, save.ID}] = save
	return s, save
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
	if save, ok := s.byItem[itemKey{kind, id}]; ok && save.Status != 0 {
		return save.Status
	}
	return status
}

// assignees returns the Assignees the User Story with the given ID shows:
// those being saved, or else assignees.
func (s saves) assignees(id int64, assignees []int64) []int64 {
	if save, ok := s.byItem[itemKey{UserStoryItem, id}]; ok && save.Assignees != nil {
		return save.Assignees
	}
	return assignees
}

// assignee returns the Assignee the Task with the given ID shows: the one
// being saved, or else assignee.
func (s saves) assignee(id, assignee int64) int64 {
	if save, ok := s.byItem[itemKey{TaskItem, id}]; ok && save.Assignee != 0 {
		return save.Assignee
	}
	return assignee
}

// end ends save, or returns false if its result no longer counts.
func (s saves) end(save Save) (saves, bool) {
	key := itemKey{save.Kind, save.ID}
	if cur, ok := s.byItem[key]; !ok || cur.Generation != save.Generation {
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
