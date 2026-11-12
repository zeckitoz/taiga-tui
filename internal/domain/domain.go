// Package domain holds the glossary types shared by all other packages. It has
// no dependencies and knows nothing about Taiga's wire format.
package domain

import "slices"

// TeamMember is a person with their own Taiga account.
type TeamMember struct {
	ID       int64
	Username string
	// FullName is the name Taiga displays, falling back to the username.
	FullName string
}

// ProjectSummary is a project as it appears in the project list.
type ProjectSummary struct {
	ID   int64
	Name string
	// Slug identifies the project in Taiga URLs; it is never shown.
	Slug string
	// BacklogEnabled means the project has Sprints and a Taskboard.
	BacklogEnabled bool
	KanbanEnabled  bool
}

// HasBoards reports whether the project has at least one board enabled.
func (p ProjectSummary) HasBoards() bool { return p.BacklogEnabled || p.KanbanEnabled }

// Project is a project with the reference data its boards need.
type Project struct {
	ProjectSummary
	// UserStoryStatuses and TaskStatuses are in Taiga's order.
	UserStoryStatuses []Status
	TaskStatuses      []Status
	Members           []TeamMember
	// Permissions are those of the logged-in Team Member.
	Permissions Permissions
}

// Permissions tell what the logged-in Team Member may do in a project.
type Permissions struct {
	ModifyTask      bool
	ModifyUserStory bool
}

// Member returns the Team Member with the given ID.
func (p Project) Member(id int64) (TeamMember, bool) {
	for _, m := range p.Members {
		if m.ID == id {
			return m, true
		}
	}
	return TeamMember{}, false
}

// KnowsUserStoryStatus reports whether p has a User Story Status with the
// given ID, archived or not.
func (p Project) KnowsUserStoryStatus(id int64) bool {
	return slices.ContainsFunc(p.UserStoryStatuses, func(s Status) bool { return s.ID == id })
}

// KnowsTaskStatus reports whether p has a Task Status with the given ID.
func (p Project) KnowsTaskStatus(id int64) bool {
	return slices.ContainsFunc(p.TaskStatuses, func(s Status) bool { return s.ID == id })
}

// KnowsMembers reports whether every one of the Team Member IDs is a member
// of p.
func (p Project) KnowsMembers(ids []int64) bool {
	return !slices.ContainsFunc(ids, func(id int64) bool {
		_, ok := p.Member(id)
		return !ok
	})
}

// Status is a workflow state of a User Story or Task.
type Status struct {
	ID   int64
	Name string
	// Color is Taiga's color of the Status as "#rrggbb", possibly empty.
	Color    string
	Closed   bool
	Archived bool
	// WIPLimit is the informational work-in-progress limit, 0 if none.
	WIPLimit int
}

// UserStory is a unit of user-facing functionality.
type UserStory struct {
	ID    int64
	Ref   int
	Title string
	// Status is the ID of the User Story's Status.
	Status int64
	// Sprint is the ID of the User Story's Sprint, 0 if it is in none, and
	// SprintName its name.
	Sprint     int64
	SprintName string
	// SprintOrder orders the User Stories of a Sprint on the Taskboard.
	SprintOrder int64
	// Assignees holds Team Member IDs, the main Assignee first.
	Assignees   []int64
	KanbanOrder int64
	Version     int
	Tasks       TaskProgress
	// Description is Taiga's Markdown text. Lists of User Stories leave it
	// empty; only a single User Story loaded on its own carries it.
	Description string
}

// AssignedTo reports whether the Team Member with the given ID is an Assignee.
func (s UserStory) AssignedTo(id int64) bool { return slices.Contains(s.Assignees, id) }

// TaskProgress counts the closed and all Tasks of a User Story.
type TaskProgress struct {
	Done  int
	Total int
}

// Task is a sub-step of a User Story, worked on within a Sprint.
type Task struct {
	ID    int64
	Ref   int
	Title string
	// Status is the ID of the Task's Status.
	Status int64
	// UserStory is the ID of the Task's User Story, 0 if it has none.
	UserStory int64
	// Sprint is the ID of the Task's Sprint, 0 if it is in none.
	Sprint int64
	// Assignee is the ID of the Task's Assignee, 0 if it has none.
	Assignee int64
	// TaskboardOrder orders the Tasks of a User Story on the Taskboard.
	TaskboardOrder int64
	Version        int
	// Description is Taiga's Markdown text. Lists of Tasks leave it empty;
	// only a single Task loaded on its own carries it.
	Description string
}

// SprintContent is what the Taskboard of a Sprint shows.
type SprintContent struct {
	UserStories []UserStory
	Tasks       []Task
}

// TaskPatch is a change to a Task. A zero field stays as it is.
type TaskPatch struct {
	// Status is the new Status, Assignee the new Assignee.
	Status   int64
	Assignee int64
}

// UserStoryPatch is a change to a User Story. A zero or nil field stays as
// it is.
type UserStoryPatch struct {
	Status int64
	// Assignee is the new main Assignee and Assignees all the Assignees
	// then, main one first.
	Assignee  int64
	Assignees []int64
}
