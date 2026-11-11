package ui

import (
	"github.com/zeckitoz/taiga-tui/internal/domain"
	"github.com/zeckitoz/taiga-tui/internal/taiga/taigafake"
)

// Test data of the fake Taiga instance. Alice (42) is logged in.

var (
	bea   = domain.TeamMember{ID: 43, Username: "bkoch", FullName: "Bea Koch"}
	carla = domain.TeamMember{ID: 44, Username: "cmar", FullName: "Carla de la Mar"}
)

var webShop = domain.Project{
	ProjectSummary: domain.ProjectSummary{ID: 7, Name: "Web Shop", Slug: "web-shop", BacklogEnabled: true, KanbanEnabled: true},
	UserStoryStatuses: []domain.Status{
		{ID: 101, Name: "New", Color: "#70728F"},
		{ID: 102, Name: "Ready", Color: "#E4C440"},
		{ID: 103, Name: "In progress", Color: "#E47C40", WIPLimit: 3},
		{ID: 104, Name: "Ready for test", Color: "#5E8C6A"},
		{ID: 105, Name: "Done", Color: "#A8E440", Closed: true},
		{ID: 106, Name: "Archived", Color: "#5C3566", Closed: true, Archived: true},
	},
	Members: []domain.TeamMember{alice, bea, carla},
}

var webShopStories = []domain.UserStory{
	{ID: 501, Ref: 12, Title: "Checkout with credit card", Status: 101, KanbanOrder: 20, Assignees: []int64{43, 42}, Tasks: domain.TaskProgress{Done: 1, Total: 2}},
	{ID: 502, Ref: 15, Title: "Product search", Status: 101, KanbanOrder: 10},
	{ID: 503, Ref: 108, Title: "Wishlist for logged-in customers that is shared with friends and family", Status: 101, KanbanOrder: 30, Assignees: []int64{44}, Tasks: domain.TaskProgress{Total: 14, Done: 4}},
	{ID: 504, Ref: 21, Title: "Order history", Status: 103, KanbanOrder: 10, Assignees: []int64{42}, Tasks: domain.TaskProgress{Done: 3, Total: 3}},
	{ID: 505, Ref: 3, Title: "Login", Status: 105, KanbanOrder: 10, Assignees: []int64{43}},
	{ID: 506, Ref: 1, Title: "Old prototype", Status: 106, KanbanOrder: 10},
}

var mobileApp = domain.Project{
	ProjectSummary: domain.ProjectSummary{ID: 8, Name: "mobile app", Slug: "mobile-app", BacklogEnabled: true},
	Members:        []domain.TeamMember{alice},
}

var archive = domain.Project{
	ProjectSummary: domain.ProjectSummary{ID: 9, Name: "Archive", Slug: "archive"},
}

// newFake returns the instance with three projects: Web Shop with both
// boards, mobile app with only the Taskboard, Archive with no boards.
func newFake() *taigafake.Fake {
	f := taigafake.New(alice)
	f.AddProject(webShop, webShopStories...)
	f.AddProject(mobileApp)
	f.AddProject(archive)
	return f
}

// newFakeWith returns an instance with only the given projects.
func newFakeWith(projects ...domain.Project) *taigafake.Fake {
	f := taigafake.New(alice)
	for _, p := range projects {
		if p.ID == webShop.ID {
			f.AddProject(p, webShopStories...)
		} else {
			f.AddProject(p)
		}
	}
	return f
}
