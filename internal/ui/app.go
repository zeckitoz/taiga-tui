// Package ui is the Bubble Tea front end: the App router, its screens and
// components.
package ui

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zeckitoz/taiga-tui/internal/board"
	"github.com/zeckitoz/taiga-tui/internal/config"
	"github.com/zeckitoz/taiga-tui/internal/domain"
	"github.com/zeckitoz/taiga-tui/internal/session"
	"github.com/zeckitoz/taiga-tui/internal/taiga"
)

// Session is the part of session.Manager the UI uses.
type Session interface {
	Resume(username string) bool
	Authenticate(ctx context.Context, login, password string) (taiga.LoginResult, error)
	Save(res taiga.LoginResult) (session.Saved, error)
}

// Taiga is the part of the Taiga API the UI uses. taiga.Client implements it
// for real, taigafake.Fake in tests.
type Taiga interface {
	Me(ctx context.Context) (domain.TeamMember, error)
	Projects(ctx context.Context, memberID int64) ([]domain.ProjectSummary, error)
	Project(ctx context.Context, slug string) (domain.Project, error)
	Kanban(ctx context.Context, projectID int64) ([]domain.UserStory, error)
	UserStory(ctx context.Context, id int64) (domain.UserStory, error)
	Sprints(ctx context.Context, projectID int64) ([]domain.Sprint, error)
	Taskboard(ctx context.Context, sprintID int64) (domain.SprintContent, error)
	Task(ctx context.Context, id int64) (domain.Task, error)
}

// StateStore persists state.json.
type StateStore interface {
	Save(config.State) error
}

// Deps wires the App.
type Deps struct {
	// Host names the instance in texts, e.g. "taiga-fki.ostfalia.de".
	Host         string
	Session      Session
	Taiga        Taiga
	State        StateStore
	InitialState config.State
	// Warnings are shown in the status line from the start.
	Warnings []config.Warning
	// Tick replaces tea.Tick in tests.
	Tick TickFunc
	// Now tells the time, whose date in its location decides the Current
	// Sprint. It replaces time.Now in tests.
	Now func() time.Time
}

type screen int

const (
	screenStarting screen = iota
	screenLogin
	screenLoading
	screenFailed
	screenPicker
	screenSprintPicker
	screenBoard
)

// keyringSlowDelay is how long the start may take before "Waiting for system
// keyring…" appears, so that a fast keyring causes no flicker.
const keyringSlowDelay = 250 * time.Millisecond

const waitingForKeyring = "Waiting for system keyring…"

// hintDefaultGone is shown on the project picker when the default project is
// not in the project list any more.
const hintDefaultGone = "Your default project is no longer available"

type (
	resumeDoneMsg  struct{ ok bool }
	keyringSlowMsg struct{}
	authDoneMsg    struct {
		result taiga.LoginResult
		login  string
		err    error
	}
	saveDoneMsg struct {
		result taiga.LoginResult
		login  string
		saved  session.Saved
		err    error
	}
	meLoadedMsg struct {
		me  domain.TeamMember
		err error
	}
	projectsLoadedMsg struct {
		projects []domain.ProjectSummary
		err      error
	}
	projectLoadedMsg struct {
		project domain.Project
		err     error
	}
	kanbanLoadedMsg struct {
		projectID int64
		stories   []domain.UserStory
		err       error
	}
	userStoryLoadedMsg struct {
		projectID int64
		load      board.LoadUserStory
		story     domain.UserStory
		err       error
	}
	sprintsLoadedMsg struct {
		projectID int64
		sprints   []domain.Sprint
		err       error
	}
	taskboardLoadedMsg struct {
		projectID, sprintID int64
		content             domain.SprintContent
		err                 error
	}
	taskLoadedMsg struct {
		projectID, sprintID int64
		load                board.LoadTask
		task                domain.Task
		err                 error
	}
)

// statusMsg is one entry of the status line.
type statusMsg struct {
	text  string
	isErr bool
	// transient entries disappear with the next action or load.
	transient bool
}

// App is the root model.
type App struct {
	deps          Deps
	state         config.State
	screen        screen
	form          loginForm
	spinner       spinner
	keyringIsSlow bool
	status        []statusMsg
	width, height int

	// loading is the text next to the spinner on screenLoading.
	loading string
	// failure and retry describe a failed load on screenFailed.
	failure string
	retry   func(App) (App, tea.Cmd)

	me       domain.TeamMember
	projects []domain.ProjectSummary
	picker   picker
	project  domain.Project
	// picked is set when the Team Member chose project in the picker
	// rather than it being opened at startup.
	picked bool
	// board is the board of project on screenBoard.
	board  config.Board
	kanban board.Kanban
	// hasKanban is set once the Kanban Board of project is loaded.
	hasKanban bool
	kanbanTop int

	// sprints are the Sprints of project once hasSprints is set.
	sprints    []domain.Sprint
	hasSprints bool
	// sprint is the Sprint whose Taskboard is shown or loading.
	sprint    domain.Sprint
	taskboard board.Taskboard
	// hasTaskboard is set once the Taskboard of a Sprint of project is
	// loaded, not necessarily of sprint.
	hasTaskboard bool
	taskboardTop int
	sprintPicker picker
	// sprintPickerBack makes esc in the Sprint picker go back to the
	// Taskboard instead of ending the app.
	sprintPickerBack bool
	// detailOpen shows the detail view, which is open by default. It is UI
	// state only and never persisted.
	detailOpen bool
}

// New returns the App. It starts by looking for a stored session.
func New(d Deps) App {
	if d.Tick == nil {
		d.Tick = tea.Tick
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	a := App{deps: d, state: d.InitialState, spinner: spinner{tick: d.Tick}, detailOpen: true}
	a.me = domain.TeamMember{ID: a.state.UserID, Username: a.state.Username, FullName: a.state.FullName}
	for _, w := range d.Warnings {
		a.status = append(a.status, statusMsg{text: string(w)})
	}
	return a
}

// Init implements tea.Model.
func (a App) Init() tea.Cmd {
	sess, username := a.deps.Session, a.state.Username
	return tea.Batch(
		func() tea.Msg { return resumeDoneMsg{ok: sess.Resume(username)} },
		a.deps.Tick(keyringSlowDelay, func(time.Time) tea.Msg { return keyringSlowMsg{} }),
	)
}

// Update implements tea.Model.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		a.form.setWidth(msg.Width)
		a.picker.setWidth(msg.Width)
		a.picker.scroll(a.pickerListHeight())
		a.sprintPicker.setWidth(msg.Width)
		a.sprintPicker.scroll(a.pickerListHeight())
		a.scrollBoard()
		return a, nil
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
	case keyringSlowMsg:
		if a.screen != screenStarting {
			return a, nil
		}
		a.keyringIsSlow = true
		return a, a.spinner.start()
	case resumeDoneMsg:
		a.spinner.stop()
		if msg.ok {
			return a.loadMe()
		}
		return a.showLogin(), nil
	case authDoneMsg:
		if msg.err != nil {
			a.form.fail(loginErrorText(a.deps.Host, msg.err))
			return a, nil
		}
		sess := a.deps.Session
		return a, tea.Batch(
			a.form.setBusy(waitingForKeyring),
			func() tea.Msg {
				saved, err := sess.Save(msg.result)
				return saveDoneMsg{result: msg.result, login: msg.login, saved: saved, err: err}
			},
		)
	case saveDoneMsg:
		if msg.err != nil {
			a.form.fail("Couldn't save login: " + msg.err.Error())
			return a, nil
		}
		return a.loggedIn(msg).loadMe()
	case meLoadedMsg:
		if msg.err != nil {
			return a.failed(msg.err, a.cannotReach(), App.loadMe)
		}
		a.me = msg.me
		return a.loadProjects()
	case projectsLoadedMsg:
		if msg.err != nil {
			return a.failed(msg.err, a.cannotReach(), App.loadProjects)
		}
		return a.projectsLoaded(msg.projects)
	case projectLoadedMsg:
		if msg.err != nil {
			slug, what := a.project.Slug, a.cannotReach()
			if a.picked {
				what = "Couldn't load " + a.project.Name
			}
			return a.failed(msg.err, what, func(a App) (App, tea.Cmd) { return a.loadProject(slug) })
		}
		return a.projectLoaded(msg.project)
	case kanbanLoadedMsg:
		return a.kanbanLoaded(msg)
	case userStoryLoadedMsg:
		if msg.projectID != a.project.ID {
			return a, nil // a load of a project that is no longer open
		}
		// The board drops results of loads before its last reload.
		if msg.err != nil {
			a.kanban = a.kanban.UserStoryFailed(msg.load, msg.err)
		} else {
			a.kanban = a.kanban.UserStoryLoaded(msg.load, msg.story)
		}
		return a, nil
	case sprintsLoadedMsg:
		return a.sprintsLoaded(msg)
	case taskboardLoadedMsg:
		return a.taskboardLoaded(msg)
	case taskLoadedMsg:
		if msg.projectID != a.project.ID || msg.sprintID != a.taskboard.Sprint().ID {
			return a, nil // a load of a Sprint that is no longer open
		}
		// The board drops results of loads before its last reload.
		if msg.err != nil {
			a.taskboard = a.taskboard.TaskFailed(msg.load, msg.err)
		} else {
			a.taskboard = a.taskboard.TaskLoaded(msg.load, msg.task)
		}
		return a, nil
	}

	switch a.screen {
	case screenStarting, screenLoading:
		if tick, ok := msg.(spinnerTickMsg); ok {
			return a, a.spinner.update(tick)
		}
	case screenLogin:
		return a.updateLogin(msg)
	case screenFailed:
		if key, ok := msg.(tea.KeyPressMsg); ok {
			switch key.String() {
			case "r":
				return a.retry(a)
			case "q":
				return a, tea.Quit
			}
		}
	case screenPicker:
		return a.clearTransient(msg).updatePicker(msg)
	case screenSprintPicker:
		return a.clearTransient(msg).updateSprintPicker(msg)
	case screenBoard:
		return a.clearTransient(msg).updateBoard(msg)
	}
	return a, nil
}

// clearTransient drops transient status entries when msg is a key press.
func (a App) clearTransient(msg tea.Msg) App {
	if _, ok := msg.(tea.KeyPressMsg); !ok {
		return a
	}
	return a.withoutTransient()
}

func (a App) withoutTransient() App {
	a.status = slices.DeleteFunc(slices.Clone(a.status), func(s statusMsg) bool { return s.transient })
	return a
}

func (a App) showLogin() App {
	a.screen = screenLogin
	a.form = newLoginForm(a.deps.Host, a.state.Login, a.deps.Tick)
	a.form.setWidth(a.width)
	return a
}

func (a App) updateLogin(msg tea.Msg) (tea.Model, tea.Cmd) {
	action, cmd := a.form.update(msg)
	switch action {
	case formCancel:
		// The login is the first screen, so canceling it ends the app.
		return a, tea.Quit
	case formSubmit:
		login, password := a.form.values()
		sess := a.deps.Session
		return a, tea.Batch(
			a.form.setBusy("Logging in…"),
			func() tea.Msg {
				res, err := sess.Authenticate(context.Background(), login, password)
				return authDoneMsg{result: res, login: login, err: err}
			},
		)
	}
	return a, cmd
}

func (a App) loggedIn(msg saveDoneMsg) App {
	a.form = loginForm{} // drop the password
	a.state = a.state.LoggedIn(msg.result.Member, msg.login)
	a.me = msg.result.Member
	if msg.saved.File != "" {
		a.status = append(a.status, statusMsg{
			text: "System keyring unavailable, login saved in " + msg.saved.File,
		})
	}
	return a.saveState()
}

// saveState writes state.json and reports a failure in the status line.
func (a App) saveState() App {
	if err := a.deps.State.Save(a.state); err != nil {
		a.status = append(a.status, statusMsg{text: "Couldn't save settings: " + err.Error(), isErr: true})
	}
	return a
}

// startLoading shows text next to a spinner while cmd runs.
func (a App) startLoading(text string, cmd tea.Cmd) (App, tea.Cmd) {
	a.screen = screenLoading
	a.loading = text
	return a, tea.Batch(a.spinner.start(), cmd)
}

// failed handles a failed load: an expired session leads to the login,
// anything else to a full-page error "<what>: <reason>" from which retry
// repeats the load.
func (a App) failed(err error, what string, retry func(App) (App, tea.Cmd)) (tea.Model, tea.Cmd) {
	a.spinner.stop()
	if errors.Is(err, taiga.ErrSessionExpired) {
		return a.showLogin(), nil
	}
	a.screen = screenFailed
	a.failure = what + ": " + reason(err)
	a.retry = retry
	return a, nil
}

// cannotReach introduces failures of the startup loads.
func (a App) cannotReach() string { return "Cannot reach " + a.deps.Host }

func (a App) loadMe() (App, tea.Cmd) {
	api := a.deps.Taiga
	return a.startLoading("Loading projects…", func() tea.Msg {
		me, err := api.Me(context.Background())
		return meLoadedMsg{me: me, err: err}
	})
}

func (a App) loadProjects() (App, tea.Cmd) {
	api, id := a.deps.Taiga, a.me.ID
	return a.startLoading("Loading projects…", func() tea.Msg {
		ps, err := api.Projects(context.Background(), id)
		return projectsLoadedMsg{projects: ps, err: err}
	})
}

// projectsLoaded opens the default project or, if there is none, the only
// project. Otherwise it shows the picker.
func (a App) projectsLoaded(projects []domain.ProjectSummary) (App, tea.Cmd) {
	a.spinner.stop()
	a.projects = projects
	hint := ""
	if id := a.state.DefaultProject; id != 0 {
		i := indexOfProject(projects, id)
		switch {
		case i < 0:
			hint = hintDefaultGone
		case projects[i].HasBoards():
			return a.openProject(projects[i], false)
		default:
			hint = projects[i].Name + " has no boards enabled"
		}
	}
	if len(projects) == 1 && projects[0].HasBoards() {
		return a.openProject(projects[0], false)
	}
	return a.showPicker(hint), nil
}

func (a App) showPicker(hint string) App {
	a.screen = screenPicker
	a.picker = newPicker(projectItems(a.projects), "project", hint)
	a.picker.setWidth(a.width)
	a.picker.scroll(a.pickerListHeight())
	return a
}

func indexOfProject(ps []domain.ProjectSummary, id int64) int {
	for i, p := range ps {
		if p.ID == id {
			return i
		}
	}
	return -1
}

// openProject loads p, which becomes the default project once it loaded.
// picked tells whether the Team Member chose it in the picker.
func (a App) openProject(p domain.ProjectSummary, picked bool) (App, tea.Cmd) {
	a.project = domain.Project{ProjectSummary: p}
	a.picked = picked
	a.hasKanban = false
	a.sprints, a.hasSprints, a.hasTaskboard = nil, false, false
	return a.loadProject(p.Slug)
}

func (a App) loadProject(slug string) (App, tea.Cmd) {
	api, name := a.deps.Taiga, a.project.Name
	return a.startLoading("Loading "+name+"…", func() tea.Msg {
		p, err := api.Project(context.Background(), slug)
		return projectLoadedMsg{project: p, err: err}
	})
}

func (a App) projectLoaded(p domain.Project) (App, tea.Cmd) {
	a.project = p
	if a.state.DefaultProject != p.ID {
		a.state.DefaultProject = p.ID
		a = a.saveState()
	}
	return a.openBoard(a.boardFor(p))
}

// boardFor returns the board to open in p: the last used board at start,
// the Kanban Board for a project chosen in the picker, either only if p has
// it enabled, and otherwise the other board.
func (a App) boardFor(p domain.Project) config.Board {
	b := a.state.LastBoard
	if a.picked || b == "" {
		b = config.BoardKanban
	}
	switch {
	case b == config.BoardKanban && !p.KanbanEnabled:
		return config.BoardTaskboard
	case b == config.BoardTaskboard && !p.BacklogEnabled:
		return config.BoardKanban
	}
	return b
}

// openBoard shows the board b of the project and remembers it as the last
// used board. A board loaded before stays visible while it refreshes.
func (a App) openBoard(b config.Board) (App, tea.Cmd) {
	a.board = b
	if a.state.LastBoard != b {
		a.state.LastBoard = b
		a = a.saveState()
	}
	if b == config.BoardKanban {
		return a.loadKanban()
	}
	if a.hasTaskboard {
		a.screen = screenBoard
		a.scrollBoard()
		return a.loadTaskboard(a.sprint)
	}
	return a.loadSprints()
}

func (a App) loadSprints() (App, tea.Cmd) {
	api, id := a.deps.Taiga, a.project.ID
	load := func() tea.Msg {
		sprints, err := api.Sprints(context.Background(), id)
		return sprintsLoadedMsg{projectID: id, sprints: sprints, err: err}
	}
	if a.hasSprints {
		// A refresh keeps the board visible.
		return a, load
	}
	return a.startLoading("Loading "+a.project.Name+"…", load)
}

// sprintsLoaded opens the Current Sprint, lets the Team Member choose one,
// or shows that the project has no Sprints.
func (a App) sprintsLoaded(msg sprintsLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.projectID != a.project.ID {
		return a, nil // a load of a project that is no longer open
	}
	if msg.err != nil {
		if a.hasSprints && !errors.Is(msg.err, taiga.ErrSessionExpired) {
			return a.refreshFailed(msg.err), nil
		}
		return a.failed(msg.err, "Couldn't load Taskboard", App.loadSprints)
	}
	a = a.withoutTransient()
	a.spinner.stop()
	a.sprints, a.hasSprints = msg.sprints, true
	remembered, ok := a.state.Sprints[a.project.ID]
	if ok && !slices.ContainsFunc(a.sprints, func(s domain.Sprint) bool { return s.ID == remembered }) {
		a.state = a.state.WithoutSprint(a.project.ID)
		a = a.saveState()
	}
	today := a.today()
	res := domain.ResolveCurrentSprint(a.sprints, today, a.state.Sprints[a.project.ID])
	switch res.Kind {
	case domain.One:
		return a.loadTaskboard(res.Sprint)
	case domain.Choose:
		return a.showSprintPicker(res.Choices, chooseHint(a.sprints, today), false), nil
	}
	a.hasTaskboard = false
	a.screen = screenBoard
	return a, nil
}

// chooseHint tells why the Team Member has to choose a Sprint.
func chooseHint(sprints []domain.Sprint, today domain.Date) string {
	switch {
	case slices.ContainsFunc(sprints, func(s domain.Sprint) bool { return s.Contains(today) }):
		return "Several sprints contain today"
	case slices.ContainsFunc(sprints, func(s domain.Sprint) bool { return !s.Closed }):
		return "No sprint contains today"
	}
	return "No sprint is open"
}

func (a App) today() domain.Date { return domain.DateOf(a.deps.Now()) }

// showSprintPicker lets the Team Member choose one of sprints. back makes esc
// return to the Taskboard.
func (a App) showSprintPicker(sprints []domain.Sprint, hint string, back bool) App {
	a.screen = screenSprintPicker
	a.sprintPicker = newPicker(sprintItems(sprints, a.today()), "sprint", hint)
	a.sprintPicker.setWidth(a.width)
	a.sprintPicker.scroll(a.pickerListHeight())
	a.sprintPickerBack = back
	return a
}

func (a App) updateSprintPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	action, cmd := a.sprintPicker.update(msg, a.pickerListHeight())
	switch action {
	case pickerQuit:
		if a.sprintPickerBack {
			a.screen = screenBoard
			return a, nil
		}
		return a, tea.Quit
	case pickerChoose:
		id, _ := a.sprintPicker.chosen()
		i := slices.IndexFunc(a.sprints, func(s domain.Sprint) bool { return s.ID == id })
		if i < 0 {
			return a, nil
		}
		a.state = a.state.WithSprint(a.project.ID, id)
		a = a.saveState()
		return a.loadTaskboard(a.sprints[i])
	}
	return a, cmd
}

// loadTaskboard loads the Taskboard of s. Refreshing the Taskboard shown
// keeps it visible.
func (a App) loadTaskboard(s domain.Sprint) (App, tea.Cmd) {
	refresh := a.hasTaskboard && a.taskboard.Sprint().ID == s.ID
	a.sprint = s
	api, projectID := a.deps.Taiga, a.project.ID
	load := func() tea.Msg {
		c, err := api.Taskboard(context.Background(), s.ID)
		return taskboardLoadedMsg{projectID: projectID, sprintID: s.ID, content: c, err: err}
	}
	if refresh {
		a.screen = screenBoard
		return a, load
	}
	return a.startLoading("Loading "+s.Name+"…", load)
}

func (a App) taskboardLoaded(msg taskboardLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.projectID != a.project.ID || msg.sprintID != a.sprint.ID {
		return a, nil // a load of a Sprint that is no longer open
	}
	refresh := a.hasTaskboard && a.taskboard.Sprint().ID == msg.sprintID
	if msg.err != nil {
		if refresh && !errors.Is(msg.err, taiga.ErrSessionExpired) {
			return a.refreshFailed(msg.err), nil
		}
		s := a.sprint
		return a.failed(msg.err, "Couldn't load Taskboard", func(a App) (App, tea.Cmd) { return a.loadTaskboard(s) })
	}
	a = a.withoutTransient()
	a.spinner.stop()
	if refresh {
		a.taskboard = a.taskboard.Reload(msg.content)
	} else {
		a.taskboard = board.NewTaskboard(a.project, a.sprint, msg.content, a.me.ID)
		a.taskboardTop = 0
		a.hasTaskboard = true
	}
	a.screen = screenBoard
	a.scrollBoard()
	return a.loadDetail()
}

// refreshFailed shows why a refresh failed while the board stays usable.
func (a App) refreshFailed(err error) App {
	a = a.withoutTransient()
	a.status = append(a.status, statusMsg{text: "Couldn't refresh: " + reason(err), isErr: true, transient: true})
	a.scrollBoard() // the status line takes rows from the list
	return a
}

func (a App) loadKanban() (App, tea.Cmd) {
	api, id := a.deps.Taiga, a.project.ID
	load := func() tea.Msg {
		stories, err := api.Kanban(context.Background(), id)
		return kanbanLoadedMsg{projectID: id, stories: stories, err: err}
	}
	if a.hasKanban {
		// A refresh keeps the board visible.
		return a, load
	}
	return a.startLoading("Loading "+a.project.Name+"…", load)
}

func (a App) kanbanLoaded(msg kanbanLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.projectID != a.project.ID {
		return a, nil // a load of a project that is no longer open
	}
	if msg.err != nil {
		if a.hasKanban && !errors.Is(msg.err, taiga.ErrSessionExpired) {
			return a.refreshFailed(msg.err), nil
		}
		return a.failed(msg.err, "Couldn't load Kanban Board", App.loadKanban)
	}
	a = a.withoutTransient()
	a.spinner.stop()
	if a.hasKanban {
		a.kanban = a.kanban.Reload(msg.stories)
	} else {
		a.kanban = board.NewKanban(a.project, msg.stories, a.me.ID)
		a.kanbanTop = 0
		a.hasKanban = true
	}
	a.screen = screenBoard
	a.scrollBoard()
	return a.loadDetail()
}

// loadDetail loads the description of the selected row if the detail view
// shows it and it is not loaded yet.
func (a App) loadDetail() (App, tea.Cmd) {
	if !a.detailOpen || a.screen != screenBoard {
		return a, nil
	}
	if a.board == config.BoardTaskboard {
		return a.loadTaskDetail()
	}
	if !a.hasKanban {
		return a, nil
	}
	k, load, ok := a.kanban.LoadDetail()
	if !ok {
		return a, nil
	}
	a.kanban = k
	api, projectID := a.deps.Taiga, a.project.ID
	return a, func() tea.Msg {
		s, err := api.UserStory(context.Background(), load.ID)
		return userStoryLoadedMsg{projectID: projectID, load: load, story: s, err: err}
	}
}

func (a App) loadTaskDetail() (App, tea.Cmd) {
	if !a.hasTaskboard {
		return a, nil
	}
	tb, load, ok := a.taskboard.LoadDetail()
	if !ok {
		return a, nil
	}
	a.taskboard = tb
	api, projectID, sprintID := a.deps.Taiga, a.project.ID, tb.Sprint().ID
	return a, func() tea.Msg {
		t, err := api.Task(context.Background(), load.ID)
		return taskLoadedMsg{projectID: projectID, sprintID: sprintID, load: load, task: t, err: err}
	}
}

func (a App) updatePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	if len(a.projects) == 0 {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			switch key.String() {
			case "r":
				return a.loadProjects()
			case "q":
				return a, tea.Quit
			}
		}
		return a, nil
	}
	action, cmd := a.picker.update(msg, a.pickerListHeight())
	switch action {
	case pickerQuit:
		return a, tea.Quit
	case pickerChoose:
		id, _ := a.picker.chosen()
		return a.openProject(a.projects[indexOfProject(a.projects, id)], true)
	}
	return a, cmd
}

func (a App) updateBoard(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return a, nil
	}
	switch key.String() {
	case "q":
		return a, tea.Quit
	case "b":
		// A temporary key until the command palette switches boards.
		if other := a.otherBoard(); other != "" {
			return a.openBoard(other)
		}
		return a, nil
	}
	if a.board == config.BoardTaskboard {
		return a.updateTaskboard(key)
	}
	switch key.String() {
	case "r":
		if a.hasKanban {
			return a.loadKanban()
		}
	case "left", "h":
		a.kanban = a.kanban.PrevTab()
	case "right", "l":
		a.kanban = a.kanban.NextTab()
	case "up", "k":
		a.kanban = a.kanban.CursorUp()
	case "down", "j":
		a.kanban = a.kanban.CursorDown()
	case "enter":
		a.detailOpen = !a.detailOpen
	}
	a.scrollBoard()
	return a.loadDetail()
}

// otherBoard returns the board of the project that is not shown, or "" if
// it is disabled.
func (a App) otherBoard() config.Board {
	if a.board == config.BoardKanban && a.project.BacklogEnabled {
		return config.BoardTaskboard
	}
	if a.board == config.BoardTaskboard && a.project.KanbanEnabled {
		return config.BoardKanban
	}
	return ""
}

func (a App) updateTaskboard(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if !a.hasTaskboard {
		// The project has no Sprints. Until the command palette exists,
		// the project picker is the menu.
		switch key.String() {
		case ":":
			return a.showPicker(""), nil
		case "r":
			return a.loadSprints()
		}
		return a, nil
	}
	switch key.String() {
	case ":":
		// A temporary Sprint switcher until the command palette exists.
		a = a.showSprintPicker(newestFirst(a.sprints), "", true)
		a.sprintPicker.selectItem(a.taskboard.Sprint().ID)
		a.sprintPicker.scroll(a.pickerListHeight())
		return a, nil
	case "r":
		return a.loadTaskboard(a.taskboard.Sprint())
	case "up", "k":
		a.taskboard = a.taskboard.CursorUp()
	case "down", "j":
		a.taskboard = a.taskboard.CursorDown()
	case "left", "h":
		a.taskboard = a.taskboard.Left()
	case "right", "l":
		a.taskboard = a.taskboard.Right()
	case "enter":
		if a.taskboard.TaskSelected() {
			a.detailOpen = !a.detailOpen
		} else {
			a.taskboard = a.taskboard.Toggle()
		}
	}
	a.scrollBoard()
	return a.loadDetail()
}

// newestFirst sorts the Sprints by start date, the latest first.
func newestFirst(sprints []domain.Sprint) []domain.Sprint {
	sprints = slices.Clone(sprints)
	slices.SortStableFunc(sprints, func(a, b domain.Sprint) int { return b.Start.Compare(a.Start) })
	return sprints
}

// scrollBoard keeps the selected row of the board visible.
func (a *App) scrollBoard() {
	if a.board == config.BoardTaskboard {
		rows := a.taskboard.Rows()
		lineOf, n := taskboardLines(rows)
		sel := -1
		if i := a.taskboard.SelectedIndex(); i >= 0 {
			sel = lineOf[i]
		}
		l := layoutBoard(a.width, a.bodyHeight(), a.detailOpen)
		a.taskboardTop = scrollTop(a.taskboardTop, sel, n, l.listH)
		return
	}
	n := len(a.kanban.Rows())
	l := layoutBoard(a.width, a.bodyHeight()-kanbanChromeLines, a.detailOpen)
	a.kanbanTop = scrollTop(a.kanbanTop, a.kanban.SelectedIndex(), n, l.listH)
}

func (a App) pickerListHeight() int { return a.bodyHeight() - pickerChromeLines }

// bodyHeight is the height between header and status line plus footer.
func (a App) bodyHeight() int {
	return a.height - 2 - len(a.statusLines(1))
}

// View implements tea.Model.
func (a App) View() tea.View {
	if a.width == 0 {
		return tea.View{AltScreen: true}
	}
	var content string
	switch a.screen {
	case screenPicker:
		content = a.framed("Select project", a.pickerBody(), a.pickerFooter())
	case screenSprintPicker:
		path := projectStyle.Render(a.project.Name) + faintStyle.Render(" › ") + titleStyle.Render("Select sprint")
		keys := pickerKeys
		if a.sprintPickerBack {
			keys = strings.Replace(keys, "esc quit", "esc back", 1)
		}
		content = a.framed(path, a.sprintPicker.view(a.width, a.bodyHeight()), footer(keys, a.width))
	case screenBoard:
		content = a.framed(a.boardPath(), a.boardBody(), footerOf(a.boardKeys(), a.width))
	default:
		content = a.plain()
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// plain renders the screens without header and footer, centered.
func (a App) plain() string {
	var body []string
	switch a.screen {
	case screenStarting:
		if a.keyringIsSlow {
			body = []string{a.spinner.view() + " " + waitingForKeyring}
		}
	case screenLogin:
		body = []string{a.form.view()}
	case screenLoading:
		body = []string{a.spinner.view() + " " + a.loading}
	case screenFailed:
		body = []string{errorStyle.Render(a.failure), "", faintStyle.Render("r retry · q quit")}
	}
	status := a.statusLines(0)
	return strings.Join(append([]string{centered(a.width, a.height-len(status), body...)}, status...), "\n")
}

// framed renders header, body, status line and footer.
func (a App) framed(path, body, foot string) string {
	header := spread(" "+path, a.me.FullName+" ", a.width)
	lines := []string{header, body}
	lines = append(lines, a.statusLines(1)...)
	lines = append(lines, foot)
	return strings.Join(lines, "\n")
}

func (a App) pickerBody() string {
	h := a.bodyHeight()
	if len(a.projects) == 0 {
		return centered(a.width, h,
			faintStyle.Render("You are not a member of any project on "+a.deps.Host), "",
			faintStyle.Render("r reload · q quit"))
	}
	return a.picker.view(a.width, h)
}

func (a App) pickerFooter() string {
	if len(a.projects) == 0 {
		return footer("r reload · q quit", a.width)
	}
	return footer(pickerKeys, a.width)
}

// boardPath is the header of the board screen.
func (a App) boardPath() string {
	sep := faintStyle.Render(" › ")
	path := projectStyle.Render(a.project.Name) + sep
	if a.board == config.BoardKanban {
		return path + titleStyle.Render("Kanban Board")
	}
	if !a.hasTaskboard {
		return path + titleStyle.Render("Taskboard")
	}
	path += "Taskboard" + sep
	full := path + sprintPath(a.taskboard.Sprint(), a.today(), true)
	// Leave out the date range rather than the Team Member's name.
	if lipgloss.Width(full)+lipgloss.Width(a.me.FullName)+3 <= a.width {
		return full
	}
	return path + sprintPath(a.taskboard.Sprint(), a.today(), false)
}

func (a App) boardBody() string {
	h := a.bodyHeight()
	if a.board == config.BoardKanban {
		return renderKanban(a.kanban, a.kanbanTop, a.width, h, a.detailOpen)
	}
	if !a.hasTaskboard {
		return centered(a.width, h,
			faintStyle.Render("No sprints in "+a.project.Name), "",
			faintStyle.Render(noSprintsKeys))
	}
	return renderTaskboard(a.taskboard, a.taskboardTop, a.width, h, a.detailOpen)
}

func (a App) boardKeys() []string {
	switch {
	case a.board == config.BoardKanban:
		return kanbanKeys(a.project.BacklogEnabled)
	case !a.hasTaskboard:
		return []string{": menu", "r refresh", "q quit"}
	}
	return taskboardKeys(a.taskboard, a.project.KanbanEnabled)
}

// statusLines renders the status line, indented by indent cells. Long
// entries wrap rather than being cut, so that a long path stays readable.
func (a App) statusLines(indent int) []string {
	var lines []string
	pad := strings.Repeat(" ", indent)
	for _, s := range a.status {
		style := warnStyle
		if s.isErr {
			style = errorStyle
		}
		for _, l := range strings.Split(lipgloss.Wrap(s.text, max(1, a.width-2*indent), " /"), "\n") {
			lines = append(lines, pad+style.Render(l))
		}
	}
	return lines
}

// loginErrorText is the text the login dialog shows for err. For malformed
// input Taiga's reason tells the Team Member what to fix.
func loginErrorText(host string, err error) string {
	if rerr, ok := errors.AsType[*taiga.RejectedError](err); ok && rerr.InvalidInput() && rerr.Reason != "" {
		return "Login rejected: " + rerr.Reason
	}
	if errors.Is(err, taiga.ErrInvalidCredentials) {
		return "Invalid username or password"
	}
	return "Cannot reach " + host + ": " + reason(err)
}

// reason maps a typed taiga error to the short text shown to the Team Member.
func reason(err error) string {
	if serr, ok := errors.AsType[*taiga.ServerError](err); ok {
		return serr.Error()
	}
	for _, known := range []error{taiga.ErrUnreachable, taiga.ErrTimeout} {
		if errors.Is(err, known) {
			return known.Error()
		}
	}
	return taiga.ErrUnexpected.Error()
}
