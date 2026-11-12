package taiga

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/zeckitoz/taiga-tui/internal/domain"
)

// DefaultTimeout is the time a request may take, including reading the response.
const DefaultTimeout = 15 * time.Second

// Option configures an AuthAPI or a Client.
type Option func(*http.Client)

// WithTimeout replaces DefaultTimeout.
func WithTimeout(d time.Duration) Option {
	return func(c *http.Client) { c.Timeout = d }
}

// WithTransport sends requests through rt instead of http.DefaultTransport,
// e.g. to record them.
func WithTransport(rt http.RoundTripper) Option {
	return func(c *http.Client) { c.Transport = rt }
}

func newHTTPClient(opts []Option) *http.Client {
	c := &http.Client{Timeout: DefaultTimeout}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// TokenSource hands out the access token for each request.
type TokenSource interface {
	// Token returns the current access token, or ErrSessionExpired if there
	// is none.
	Token(ctx context.Context) (string, error)
}

// Client is the authenticated part of the Taiga API.
type Client struct {
	baseURL string
	tokens  TokenSource
	http    *http.Client
}

// NewClient returns the client of the instance at baseURL, which has no
// trailing slash. Every request carries the access token from tokens.
func NewClient(baseURL string, tokens TokenSource, opts ...Option) *Client {
	return &Client{baseURL: baseURL, tokens: tokens, http: newHTTPClient(opts)}
}

type memberDTO struct {
	ID              int64  `json:"id"`
	Username        string `json:"username"`
	FullNameDisplay string `json:"full_name_display"`
}

func (m memberDTO) domain() domain.TeamMember {
	name := m.FullNameDisplay
	if name == "" {
		name = m.Username
	}
	return domain.TeamMember{ID: m.ID, Username: m.Username, FullName: name}
}

// Me returns the logged-in Team Member.
func (c *Client) Me(ctx context.Context) (domain.TeamMember, error) {
	var m memberDTO
	if err := c.get(ctx, "/users/me", nil, &m); err != nil {
		return domain.TeamMember{}, err
	}
	return m.domain(), nil
}

type projectSummaryDTO struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Slug               string `json:"slug"`
	IsBacklogActivated bool   `json:"is_backlog_activated"`
	IsKanbanActivated  bool   `json:"is_kanban_activated"`
}

func (p projectSummaryDTO) domain() domain.ProjectSummary {
	return domain.ProjectSummary{
		ID:             p.ID,
		Name:           p.Name,
		Slug:           p.Slug,
		BacklogEnabled: p.IsBacklogActivated,
		KanbanEnabled:  p.IsKanbanActivated,
	}
}

// Projects returns the projects the Team Member with the given ID belongs to.
func (c *Client) Projects(ctx context.Context, memberID int64) ([]domain.ProjectSummary, error) {
	var dtos []projectSummaryDTO
	q := url.Values{"member": {strconv.FormatInt(memberID, 10)}}
	if err := c.get(ctx, "/projects", q, &dtos); err != nil {
		return nil, err
	}
	out := make([]domain.ProjectSummary, len(dtos))
	for i, d := range dtos {
		out[i] = d.domain()
	}
	return out, nil
}

type statusDTO struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Color      string `json:"color"`
	IsClosed   bool   `json:"is_closed"`
	IsArchived bool   `json:"is_archived"`
	WIPLimit   *int   `json:"wip_limit"`
}

func (s statusDTO) domain() domain.Status {
	st := domain.Status{ID: s.ID, Name: s.Name, Color: s.Color, Closed: s.IsClosed, Archived: s.IsArchived}
	if s.WIPLimit != nil {
		st.WIPLimit = *s.WIPLimit
	}
	return st
}

type projectDTO struct {
	projectSummaryDTO
	// Taiga sends null instead of an empty array for these.
	USStatuses   []statusDTO `json:"us_statuses"`
	TaskStatuses []statusDTO `json:"task_statuses"`
	Members      []memberDTO `json:"members"`
	// MyPermissions are those of the logged-in Team Member.
	MyPermissions []string `json:"my_permissions"`
}

// Project returns the project with the given slug, including its User Story
// and Task Statuses in Taiga's order and its members.
func (c *Client) Project(ctx context.Context, slug string) (domain.Project, error) {
	var d projectDTO
	if err := c.get(ctx, "/projects/by_slug", url.Values{"slug": {slug}}, &d); err != nil {
		return domain.Project{}, err
	}
	p := domain.Project{ProjectSummary: d.domain()}
	for _, s := range d.USStatuses {
		p.UserStoryStatuses = append(p.UserStoryStatuses, s.domain())
	}
	for _, s := range d.TaskStatuses {
		p.TaskStatuses = append(p.TaskStatuses, s.domain())
	}
	for _, m := range d.Members {
		p.Members = append(p.Members, m.domain())
	}
	p.Permissions = domain.Permissions{
		ModifyTask:      slices.Contains(d.MyPermissions, "modify_task"),
		ModifyUserStory: slices.Contains(d.MyPermissions, "modify_us"),
	}
	return p, nil
}

type userStoryDTO struct {
	ID            int64   `json:"id"`
	Ref           int     `json:"ref"`
	Subject       string  `json:"subject"`
	Status        int64   `json:"status"`
	Milestone     *int64  `json:"milestone"`
	MilestoneName *string `json:"milestone_name"`
	SprintOrder   int64   `json:"sprint_order"`
	Description   string  `json:"description"`
	AssignedTo    *int64  `json:"assigned_to"`
	AssignedUsers []int64 `json:"assigned_users"`
	KanbanOrder   int64   `json:"kanban_order"`
	Version       int     `json:"version"`
	Tasks         []struct {
		IsClosed bool `json:"is_closed"`
	} `json:"tasks"`
}

func (u userStoryDTO) domain() domain.UserStory {
	s := domain.UserStory{
		ID:          u.ID,
		Ref:         u.Ref,
		Title:       u.Subject,
		Status:      u.Status,
		SprintOrder: u.SprintOrder,
		KanbanOrder: u.KanbanOrder,
		Version:     u.Version,
		Description: u.Description,
	}
	if u.Milestone != nil {
		s.Sprint = *u.Milestone
	}
	if u.MilestoneName != nil {
		s.SprintName = *u.MilestoneName
	}
	// assigned_users already contains assigned_to, the main Assignee.
	if u.AssignedTo != nil {
		s.Assignees = append(s.Assignees, *u.AssignedTo)
	}
	for _, id := range u.AssignedUsers {
		if u.AssignedTo == nil || id != *u.AssignedTo {
			s.Assignees = append(s.Assignees, id)
		}
	}
	for _, t := range u.Tasks {
		s.Tasks.Total++
		if t.IsClosed {
			s.Tasks.Done++
		}
	}
	return s
}

// Kanban returns the User Stories of the project's Kanban Board, without
// those in archived Statuses, in the order Taiga sends them.
func (c *Client) Kanban(ctx context.Context, projectID int64) ([]domain.UserStory, error) {
	return c.userStories(ctx, url.Values{
		"project":             {strconv.FormatInt(projectID, 10)},
		"status__is_archived": {"false"},
		"include_tasks":       {"1"},
	})
}

func (c *Client) userStories(ctx context.Context, q url.Values) ([]domain.UserStory, error) {
	var dtos []userStoryDTO
	if err := c.get(ctx, "/userstories", q, &dtos); err != nil {
		return nil, err
	}
	out := make([]domain.UserStory, len(dtos))
	for i, d := range dtos {
		out[i] = d.domain()
	}
	return out, nil
}

// UserStory returns the User Story with the given ID, including its
// description, which the lists leave out.
func (c *Client) UserStory(ctx context.Context, id int64) (domain.UserStory, error) {
	var d userStoryDTO
	if err := c.get(ctx, "/userstories/"+strconv.FormatInt(id, 10), nil, &d); err != nil {
		return domain.UserStory{}, err
	}
	return d.domain(), nil
}

// sprintDTO is a Taiga milestone.
type sprintDTO struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	EstimatedStart  string `json:"estimated_start"`
	EstimatedFinish string `json:"estimated_finish"`
	Closed          bool   `json:"closed"`
}

func (s sprintDTO) domain() (domain.Sprint, error) {
	start, err := parseDate(s.EstimatedStart)
	if err != nil {
		return domain.Sprint{}, err
	}
	finish, err := parseDate(s.EstimatedFinish)
	if err != nil {
		return domain.Sprint{}, err
	}
	return domain.Sprint{ID: s.ID, Name: s.Name, Start: start, Finish: finish, Closed: s.Closed}, nil
}

// parseDate parses an ISO date like "2026-11-03".
func parseDate(s string) (domain.Date, error) {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return domain.Date{}, fmt.Errorf("%w: %w", ErrUnexpected, err)
	}
	return domain.DateOf(t), nil
}

// Sprints returns the Sprints of the project, newest first.
func (c *Client) Sprints(ctx context.Context, projectID int64) ([]domain.Sprint, error) {
	var dtos []sprintDTO
	q := url.Values{"project": {strconv.FormatInt(projectID, 10)}}
	if err := c.get(ctx, "/milestones", q, &dtos); err != nil {
		return nil, err
	}
	out := make([]domain.Sprint, len(dtos))
	for i, d := range dtos {
		s, err := d.domain()
		if err != nil {
			return nil, err
		}
		out[i] = s
	}
	return out, nil
}

type taskDTO struct {
	ID             int64  `json:"id"`
	Ref            int    `json:"ref"`
	Subject        string `json:"subject"`
	Status         int64  `json:"status"`
	UserStory      *int64 `json:"user_story"`
	Milestone      *int64 `json:"milestone"`
	AssignedTo     *int64 `json:"assigned_to"`
	TaskboardOrder int64  `json:"taskboard_order"`
	Version        int    `json:"version"`
	Description    string `json:"description"`
}

func (t taskDTO) domain() domain.Task {
	return domain.Task{
		ID:             t.ID,
		Ref:            t.Ref,
		Title:          t.Subject,
		Status:         t.Status,
		UserStory:      deref(t.UserStory),
		Sprint:         deref(t.Milestone),
		Assignee:       deref(t.AssignedTo),
		TaskboardOrder: t.TaskboardOrder,
		Version:        t.Version,
		Description:    t.Description,
	}
}

// deref returns the ID p points to, 0 for null.
func deref(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// Taskboard returns the User Stories and Tasks of the Sprint with the given
// ID in the project, in the order Taiga sends them and without descriptions.
func (c *Client) Taskboard(ctx context.Context, projectID, sprintID int64) (domain.SprintContent, error) {
	q := url.Values{
		"project":   {strconv.FormatInt(projectID, 10)},
		"milestone": {strconv.FormatInt(sprintID, 10)},
	}
	userStories, err := c.userStories(ctx, q)
	if err != nil {
		return domain.SprintContent{}, err
	}
	var dtos []taskDTO
	if err := c.get(ctx, "/tasks", q, &dtos); err != nil {
		return domain.SprintContent{}, err
	}
	content := domain.SprintContent{UserStories: userStories, Tasks: make([]domain.Task, len(dtos))}
	for i, d := range dtos {
		content.Tasks[i] = d.domain()
	}
	return content, nil
}

// Task returns the Task with the given ID, including its description, which
// the lists leave out.
func (c *Client) Task(ctx context.Context, id int64) (domain.Task, error) {
	var d taskDTO
	if err := c.get(ctx, "/tasks/"+strconv.FormatInt(id, 10), nil, &d); err != nil {
		return domain.Task{}, err
	}
	return d.domain(), nil
}

// patchDTO changes a Task or User Story, sending only the fields set.
// Taiga rejects it with a conflict if version is not the current one.
type patchDTO struct {
	Status        int64   `json:"status,omitempty"`
	AssignedTo    int64   `json:"assigned_to,omitempty"`
	AssignedUsers []int64 `json:"assigned_users,omitempty"`
	Version       int     `json:"version"`
}

// PatchTask changes the Task with the given ID, which the Team Member last
// saw in version, and returns it as Taiga saved it.
func (c *Client) PatchTask(ctx context.Context, id int64, version int, p domain.TaskPatch) (domain.Task, error) {
	var d taskDTO
	body := patchDTO{Status: p.Status, AssignedTo: p.Assignee, Version: version}
	if err := c.patch(ctx, "/tasks/"+strconv.FormatInt(id, 10), body, &d); err != nil {
		return domain.Task{}, err
	}
	return d.domain(), nil
}

// PatchUserStory changes the User Story with the given ID, which the Team
// Member last saw in version, and returns it as Taiga saved it.
func (c *Client) PatchUserStory(ctx context.Context, id int64, version int, p domain.UserStoryPatch) (domain.UserStory, error) {
	var d userStoryDTO
	body := patchDTO{Status: p.Status, AssignedTo: p.Assignee, AssignedUsers: p.Assignees, Version: version}
	if err := c.patch(ctx, "/userstories/"+strconv.FormatInt(id, 10), body, &d); err != nil {
		return domain.UserStory{}, err
	}
	return d.domain(), nil
}

// get sends an authenticated GET to the API path and decodes the JSON
// response into dst. Pagination is always disabled; it only affects lists.
func (c *Client) get(ctx context.Context, path string, query url.Values, dst any) error {
	u := c.baseURL + "/api/v1" + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("x-disable-pagination", "1")
	return c.do(req, dst)
}

// patch sends body as JSON in an authenticated PATCH to the API path and
// decodes the JSON response into dst.
func (c *Client) patch(ctx context.Context, path string, body, dst any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.baseURL+"/api/v1"+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, dst)
}

// do sends req with the access token and decodes the JSON response into dst.
func (c *Client) do(req *http.Request, dst any) error {
	token, err := c.tokens.Token(req.Context())
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return transportError(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return transportError(err)
	}
	if err := statusError(resp.StatusCode, data); err != nil {
		return err
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("%w: %w", ErrUnexpected, err)
	}
	return nil
}

// statusError maps a non-2xx response of an authenticated call to a typed error.
func statusError(code int, body []byte) error {
	switch {
	case code >= 200 && code < 300:
		return nil
	case code == http.StatusUnauthorized:
		return ErrSessionExpired
	case code == http.StatusNotFound:
		return ErrNotFound
	case code == http.StatusBadRequest:
		// Taiga reports a version conflict as 400 with a "version" key.
		var fields map[string]json.RawMessage
		if json.Unmarshal(body, &fields) == nil {
			if _, ok := fields["version"]; ok {
				return ErrConflict
			}
		}
	case code >= 500:
		return &ServerError{Code: code}
	}
	return fmt.Errorf("%w: HTTP %d", ErrUnexpected, code)
}
