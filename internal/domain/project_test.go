package domain

import "testing"

var statusProject = Project{
	UserStoryStatuses: []Status{{ID: 1}, {ID: 2, Archived: true}},
	TaskStatuses:      []Status{{ID: 21}},
	Members:           []TeamMember{{ID: 42}, {ID: 43}},
}

func TestProjectKnowsStatuses(t *testing.T) {
	if !statusProject.KnowsUserStoryStatus(1) || !statusProject.KnowsUserStoryStatus(2) {
		t.Error("Status of the project, archived or not, is unknown")
	}
	if statusProject.KnowsUserStoryStatus(21) || statusProject.KnowsUserStoryStatus(0) {
		t.Error("foreign Status is known")
	}
	if !statusProject.KnowsTaskStatus(21) || statusProject.KnowsTaskStatus(1) {
		t.Error("Task Statuses wrong")
	}
}

func TestProjectKnowsMembers(t *testing.T) {
	if !statusProject.KnowsMembers(nil) || !statusProject.KnowsMembers([]int64{42, 43}) {
		t.Error("known members are unknown")
	}
	if statusProject.KnowsMembers([]int64{42, 99}) {
		t.Error("unknown member is known")
	}
}
