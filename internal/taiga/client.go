package taiga

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	USStatuses []statusDTO `json:"us_statuses"`
	Members    []memberDTO `json:"members"`
}

// Project returns the project with the given slug, including its User Story
// Statuses in Taiga's order and its members.
func (c *Client) Project(ctx context.Context, slug string) (domain.Project, error) {
	var d projectDTO
	if err := c.get(ctx, "/projects/by_slug", url.Values{"slug": {slug}}, &d); err != nil {
		return domain.Project{}, err
	}
	p := domain.Project{ProjectSummary: d.domain()}
	for _, s := range d.USStatuses {
		p.UserStoryStatuses = append(p.UserStoryStatuses, s.domain())
	}
	for _, m := range d.Members {
		p.Members = append(p.Members, m.domain())
	}
	return p, nil
}

type userStoryDTO struct {
	ID            int64   `json:"id"`
	Ref           int     `json:"ref"`
	Subject       string  `json:"subject"`
	Status        int64   `json:"status"`
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
		KanbanOrder: u.KanbanOrder,
		Version:     u.Version,
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
	var dtos []userStoryDTO
	q := url.Values{
		"project":             {strconv.FormatInt(projectID, 10)},
		"status__is_archived": {"false"},
		"include_tasks":       {"1"},
	}
	if err := c.get(ctx, "/userstories", q, &dtos); err != nil {
		return nil, err
	}
	out := make([]domain.UserStory, len(dtos))
	for i, d := range dtos {
		out[i] = d.domain()
	}
	return out, nil
}

// get sends an authenticated GET to the API path and decodes the JSON
// response into dst. Pagination is always disabled; it only affects lists.
func (c *Client) get(ctx context.Context, path string, query url.Values, dst any) error {
	token, err := c.tokens.Token(ctx)
	if err != nil {
		return err
	}
	u := c.baseURL + "/api/v1" + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-disable-pagination", "1")

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
