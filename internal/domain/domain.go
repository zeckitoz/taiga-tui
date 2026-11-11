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
	// UserStoryStatuses are in Taiga's order.
	UserStoryStatuses []Status
	Members           []TeamMember
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
	// Assignees holds Team Member IDs, the main Assignee first.
	Assignees   []int64
	KanbanOrder int64
	Version     int
	Tasks       TaskProgress
}

// AssignedTo reports whether the Team Member with the given ID is an Assignee.
func (s UserStory) AssignedTo(id int64) bool { return slices.Contains(s.Assignees, id) }

// TaskProgress counts the closed and all Tasks of a User Story.
type TaskProgress struct {
	Done  int
	Total int
}
