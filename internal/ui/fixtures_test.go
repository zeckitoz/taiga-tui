package ui

import (
	"time"

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
	TaskStatuses: []domain.Status{
		{ID: 201, Name: "New", Color: "#70728F"},
		{ID: 202, Name: "In progress", Color: "#E47C40"},
		{ID: 203, Name: "Ready for test", Color: "#E4CE40"},
		{ID: 204, Name: "Closed", Color: "#A8E440", Closed: true},
		{ID: 205, Name: "Needs Info", Color: "#5178D3"},
	},
	Members:     []domain.TeamMember{alice, bea, carla},
	Permissions: domain.Permissions{ModifyTask: true, ModifyUserStory: true},
}

// today is the date of the fixed clock, inside Sprint 4.
var today = time.Date(2026, 11, 11, 9, 30, 0, 0, time.Local)

// The Sprints of Web Shop, newest first as Taiga lists them.
var (
	sprint5 = domain.Sprint{ID: 305, Name: "Sprint 5", Start: domain.Date{Year: 2026, Month: 11, Day: 17}, Finish: domain.Date{Year: 2026, Month: 11, Day: 30}}
	sprint4 = domain.Sprint{ID: 304, Name: "Sprint 4", Start: domain.Date{Year: 2026, Month: 11, Day: 3}, Finish: domain.Date{Year: 2026, Month: 11, Day: 16}}
	sprint3 = domain.Sprint{ID: 303, Name: "Sprint 3", Start: domain.Date{Year: 2026, Month: 10, Day: 20}, Finish: domain.Date{Year: 2026, Month: 11, Day: 2}, Closed: true}
)

// webShopTasks are the Tasks of Sprint 4: two of #12, three of #21, none of
// #15 and one without User Story.
var webShopTasks = []domain.Task{
	{
		ID: 801, Ref: 31, Title: "Validate card number", Status: 202, UserStory: 501, Sprint: 304, Assignee: 42, TaskboardOrder: 1,
		Description: "Run the Luhn check before the form is sent.\n\nShow the error next to the field.",
	},
	{ID: 802, Ref: 32, Title: "Redirect to 3-D Secure and back", Status: 204, UserStory: 501, Sprint: 304, Assignee: 43, TaskboardOrder: 2},
	{ID: 803, Ref: 35, Title: "List past orders", Status: 204, UserStory: 504, Sprint: 304, Assignee: 42, TaskboardOrder: 1},
	{ID: 804, Ref: 36, Title: "Order detail page with invoice download and shipment tracking", Status: 204, UserStory: 504, Sprint: 304, Assignee: 44, TaskboardOrder: 2},
	{ID: 805, Ref: 37, Title: "Reorder button", Status: 204, UserStory: 504, Sprint: 304, Assignee: 42, TaskboardOrder: 3},
	{ID: 806, Ref: 40, Title: "Update privacy policy", Status: 201, Sprint: 304, TaskboardOrder: 1},
}

var webShopStories = []domain.UserStory{
	{
		ID: 501, Ref: 12, Title: "Checkout with credit card", Status: 101, KanbanOrder: 20, Sprint: 304, SprintName: "Sprint 4", SprintOrder: 1,
		Assignees: []int64{43, 42}, Tasks: domain.TaskProgress{Done: 1, Total: 2},
		Description: "Customers pay with Visa or Mastercard at the end of the checkout.\r\n\r\n- 3-D Secure is required\n- No PayPal yet\n",
	},
	{ID: 502, Ref: 15, Title: "Product search", Status: 101, KanbanOrder: 10, Sprint: 304, SprintName: "Sprint 4", SprintOrder: 3},
	{
		ID: 503, Ref: 108, Title: "Wishlist for logged-in customers that is shared with friends and family", Status: 101, KanbanOrder: 30,
		Sprint: 305, SprintName: "Sprint 5", Assignees: []int64{44}, Tasks: domain.TaskProgress{Total: 14, Done: 4},
		Description: "A wishlist keeps products for later.\n\nCustomers share it by link.\nFriends see prices but not the customer's address.\nThe list survives logging out.\nIt holds at most 100 products.\nShared lists are read-only.\nOwners can rename a list.\nOwners can delete a list.\nDeleted lists are gone for friends too.",
	},
	{
		ID: 504, Ref: 21, Title: "Order history", Status: 103, KanbanOrder: 10, Sprint: 304, SprintName: "Sprint 4", SprintOrder: 2,
		Assignees: []int64{42}, Tasks: domain.TaskProgress{Done: 3, Total: 3},
	},
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
// boards, mobile app with only the Taskboard and no Sprints, Archive with no
// boards.
func newFake() *taigafake.Fake {
	f := taigafake.New(alice)
	addWebShop(f)
	f.AddProject(mobileApp)
	f.AddProject(archive)
	return f
}

// newFakeWith returns an instance with only the given projects.
func newFakeWith(projects ...domain.Project) *taigafake.Fake {
	f := taigafake.New(alice)
	for _, p := range projects {
		if p.ID == webShop.ID {
			addWebShop(f)
		} else {
			f.AddProject(p)
		}
	}
	return f
}

func addWebShop(f *taigafake.Fake) {
	f.AddProject(webShop, webShopStories...)
	f.SetSprints(webShop.ID, sprint5, sprint4, sprint3)
	f.SetTasks(webShop.ID, webShopTasks...)
}
