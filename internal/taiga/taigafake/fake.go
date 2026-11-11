// Package taigafake is an in-memory stand-in for the Taiga client, with
// per-method error and delay injection, for tests of board and ui.
package taigafake

import (
	"context"
	"slices"
	"sync"

	"github.com/zeckitoz/taiga-tui/internal/domain"
	"github.com/zeckitoz/taiga-tui/internal/taiga"
)

// Fake holds the data of one Taiga instance as seen by one Team Member.
// It is safe for concurrent use.
type Fake struct {
	mu       sync.Mutex
	me       domain.TeamMember
	projects []domain.Project
	stories  map[int64][]domain.UserStory // by project ID
	fail     map[string][]error
	holds    map[string]chan struct{}
	calls    map[string]int
}

// New returns a fake instance on which me is logged in.
func New(me domain.TeamMember) *Fake {
	return &Fake{
		me:      me,
		stories: map[int64][]domain.UserStory{},
		fail:    map[string][]error{},
		holds:   map[string]chan struct{}{},
		calls:   map[string]int{},
	}
}

// AddProject adds a project the Team Member belongs to, with the User Stories
// of its Kanban Board, including those in archived Statuses.
func (f *Fake) AddProject(p domain.Project, stories ...domain.UserStory) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.projects = append(f.projects, p)
	f.stories[p.ID] = stories
}

// SetStories replaces the User Stories of the project with the given ID.
func (f *Fake) SetStories(projectID int64, stories ...domain.UserStory) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stories[projectID] = stories
}

// FailNext makes the next call of method (e.g. "Kanban") return err. Several
// calls queue up errors for subsequent calls.
func (f *Fake) FailNext(method string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail[method] = append(f.fail[method], err)
}

// Hold blocks calls of method until the returned release is called or their
// context ends. This injects delays without timers.
func (f *Fake) Hold(method string) (release func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch := make(chan struct{})
	f.holds[method] = ch
	var once sync.Once
	return func() {
		once.Do(func() {
			f.mu.Lock()
			delete(f.holds, method)
			f.mu.Unlock()
			close(ch)
		})
	}
}

// Calls returns how often method was called.
func (f *Fake) Calls(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[method]
}

// enter counts the call, waits for a hold and returns an injected error.
func (f *Fake) enter(ctx context.Context, method string) error {
	f.mu.Lock()
	f.calls[method]++
	hold := f.holds[method]
	f.mu.Unlock()
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if errs := f.fail[method]; len(errs) > 0 {
		f.fail[method] = errs[1:]
		return errs[0]
	}
	return nil
}

// Me implements the Taiga interface.
func (f *Fake) Me(ctx context.Context) (domain.TeamMember, error) {
	if err := f.enter(ctx, "Me"); err != nil {
		return domain.TeamMember{}, err
	}
	return f.me, nil
}

// Projects implements the Taiga interface. It lists every project added,
// assuming memberID is the logged-in Team Member.
func (f *Fake) Projects(ctx context.Context, _ int64) ([]domain.ProjectSummary, error) {
	if err := f.enter(ctx, "Projects"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]domain.ProjectSummary, len(f.projects))
	for i, p := range f.projects {
		out[i] = p.ProjectSummary
	}
	return out, nil
}

// Project implements the Taiga interface.
func (f *Fake) Project(ctx context.Context, slug string) (domain.Project, error) {
	if err := f.enter(ctx, "Project"); err != nil {
		return domain.Project{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.projects {
		if p.Slug == slug {
			return p, nil
		}
	}
	return domain.Project{}, taiga.ErrNotFound
}

// Kanban implements the Taiga interface. Like Taiga, it leaves out User
// Stories in archived Statuses.
func (f *Fake) Kanban(ctx context.Context, projectID int64) ([]domain.UserStory, error) {
	if err := f.enter(ctx, "Kanban"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	i := slices.IndexFunc(f.projects, func(p domain.Project) bool { return p.ID == projectID })
	if i < 0 {
		return nil, taiga.ErrNotFound
	}
	var out []domain.UserStory
	for _, s := range f.stories[projectID] {
		if !archived(f.projects[i], s.Status) {
			out = append(out, s)
		}
	}
	return out, nil
}

func archived(p domain.Project, status int64) bool {
	for _, s := range p.UserStoryStatuses {
		if s.ID == status {
			return s.Archived
		}
	}
	return false
}
