// PROTOTYPE, throwaway: runs the real UI on invented data. Not part of the product.
package main

import (
	"context"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/zeckitoz/taiga-tui/internal/config"
	"github.com/zeckitoz/taiga-tui/internal/domain"
	"github.com/zeckitoz/taiga-tui/internal/session"
	"github.com/zeckitoz/taiga-tui/internal/taiga"
	"github.com/zeckitoz/taiga-tui/internal/taiga/taigafake"
	"github.com/zeckitoz/taiga-tui/internal/ui"
)

type sess struct{}

func (sess) Resume(string) bool { return true }
func (sess) Authenticate(context.Context, string, string) (taiga.LoginResult, error) {
	return taiga.LoginResult{}, nil
}
func (sess) Save(taiga.LoginResult) (session.Saved, error) { return session.Saved{}, nil }

type store struct{}

func (store) Save(config.State) error { return nil }

func main() {
	f := build()
	d := ui.Deps{
		Host: "taiga.example.org", Session: sess{}, Taiga: f, State: store{},
		InitialState: config.State{UserID: 1, Username: "ada", Login: "ada", FullName: "Ada Lovelace", DefaultProject: 1, LastBoard: config.BoardKanban},
		Version:      "v1.0.0",
	}
	if _, err := tea.NewProgram(ui.New(d)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func build() *taigafake.Fake {
	ada := domain.TeamMember{ID: 1, Username: "ada", FullName: "Ada Lovelace"}
	grace := domain.TeamMember{ID: 2, Username: "grace", FullName: "Grace Hopper"}
	linus := domain.TeamMember{ID: 3, Username: "linus", FullName: "Linus Torvalds"}
	margaret := domain.TeamMember{ID: 4, Username: "margaret", FullName: "Margaret Hamilton"}
	st := func(id int64, n, c string, wip int) domain.Status {
		return domain.Status{ID: id, Name: n, Color: c, WIPLimit: wip}
	}
	p := domain.Project{
		ProjectSummary: domain.ProjectSummary{ID: 1, Name: "Demo Project", Slug: "demo", BacklogEnabled: true, KanbanEnabled: true},
		UserStoryStatuses: []domain.Status{
			st(1, "New", "#70728F", 0), st(2, "Ready", "#E44057", 0), st(3, "In progress", "#E8A317", 3),
			st(4, "Ready for test", "#A5CC7D", 0), {ID: 5, Name: "Done", Color: "#669900", Closed: true},
		},
		TaskStatuses: []domain.Status{st(1, "New", "#70728F", 0), st(2, "In progress", "#E8A317", 0), {ID: 3, Name: "Done", Color: "#669900", Closed: true}},
		Members:      []domain.TeamMember{ada, grace, linus, margaret},
		Permissions:  domain.Permissions{ModifyTask: true, ModifyUserStory: true},
	}
	us := func(id int64, t string, status int64, a ...int64) domain.UserStory {
		return domain.UserStory{ID: id, Ref: int(id), Title: t, Status: status, Assignees: a, KanbanOrder: id, Version: 1}
	}
	s := []domain.UserStory{
		us(101, "Export reports as CSV", 1),
		us(102, "Dark mode for the dashboard", 1, 3),
		us(103, "Rate limiting for the public API", 2, 2),
		us(104, "Onboarding checklist", 2),
		us(105, "Search across all projects", 3, 1, 2),
		us(106, "Two-factor authentication", 3, 4),
		us(107, "Email digest settings", 3, 1),
		us(108, "Fix flaky avatar upload", 4, 2),
		us(109, "Audit log for admin actions", 4, 3),
		us(110, "Keyboard shortcuts overview", 5, 1),
		us(111, "Migrate to the new billing API", 5, 4),
	}
	prog := map[int64][2]int{103: {1, 3}, 105: {2, 4}, 106: {0, 3}, 107: {1, 2}, 108: {2, 2}, 109: {1, 3}, 110: {3, 3}, 111: {4, 4}}
	for i := range s {
		if t, ok := prog[s[i].ID]; ok {
			s[i].Tasks = domain.TaskProgress{Done: t[0], Total: t[1]}
		}
		s[i].Description = "As a team member I want this so that my daily work gets easier.\n\n- Acceptance criteria are agreed with the team\n- Works on every supported platform"
		s[i].Sprint, s[i].SprintName = 1, "Sprint 12"
	}
	f := taigafake.New(ada)
	f.AddProject(p, s...)
	return f
}
