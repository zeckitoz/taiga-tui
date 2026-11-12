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
	Taskboard(ctx context.Context, projectID, sprintID int64) (domain.SprintContent, error)
	Task(ctx context.Context, id int64) (domain.Task, error)
	PatchTask(ctx context.Context, id int64, version int, p domain.TaskPatch) (domain.Task, error)
	PatchUserStory(ctx context.Context, id int64, version int, p domain.UserStoryPatch) (domain.UserStory, error)
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
		load    board.LoadKanban
		stories []domain.UserStory
		err     error
	}
	userStoryLoadedMsg struct {
		load  board.LoadUserStory
		story domain.UserStory
		err   error
	}
	sprintsLoadedMsg struct {
		load    board.LoadSprints
		sprints []domain.Sprint
		err     error
	}
	taskboardLoadedMsg struct {
		load    board.LoadTaskboard
		content domain.SprintContent
		err     error
	}
	taskLoadedMsg struct {
		load board.LoadTask
		task domain.Task
		err  error
	}
	// savedMsg is the result of a save on the board on: the Task or
	// User Story as Taiga saved it, or the error.
	savedMsg struct {
		on    config.Board
		save  board.Save
		task  domain.Task
		story domain.UserStory
		err   error
	}
	userStoryReloadedMsg struct {
		load  board.ReloadUserStory
		story domain.UserStory
		err   error
	}
)

// statusMsg is one entry of the status line.
type statusMsg struct {
	text  string
	isErr bool
	// transient entries disappear with the next action or load, hints only
	// with the next action.
	transient bool
	hint      bool
}

// App is the root model.
type App struct {
	deps    Deps
	state   config.State
	screen  screen
	form    loginForm
	spinner spinner
	// saving turns on the rows whose Status Change is being saved.
	saving        spinner
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
	// opened tells how project was opened, which decides its first board.
	opened opening
	// board is the board of project on screenBoard.
	board     config.Board
	kanban    board.Kanban
	kanbanTop int

	// sprints holds the Sprints of project and the Taskboard shown.
	sprints      board.Sprints
	taskboardTop int
	sprintPicker picker
	// sprintPickerBack is the board esc in the Sprint picker returns to,
	// empty if esc ends the app.
	sprintPickerBack config.Board
	// detailOpen shows the detail view, which is open by default. It is UI
	// state only and never persisted.
	detailOpen bool
	// sent tracks the saves waiting for their results on every board.
	sent board.Sent
	// paletteOpen shows the command palette over the board.
	paletteOpen bool
	palette     palette
	// paletteStatus tells that the palette lists the Statuses of the
	// selected row instead of the places to go to.
	paletteStatus bool
	// asked is the question of an Assign to me waiting for its answer.
	asked *board.ConfirmAssignToMe
	// pickedFor is the row the Status palette was opened for.
	pickedFor board.StatusOptions
	// helpOpen expands the footer of the board into the full help.
	helpOpen bool
}

// New returns the App. It starts by looking for a stored session.
func New(d Deps) App {
	if d.Tick == nil {
		d.Tick = tea.Tick
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	a := App{
		deps:       d,
		state:      d.InitialState,
		spinner:    spinner{id: loadingSpinner, tick: d.Tick},
		saving:     spinner{id: savingSpinner, tick: d.Tick},
		detailOpen: true,
	}
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
	m, cmd := a.update(msg)
	next := m.(App)
	saving := next.syncSaving()
	return next, tea.Batch(cmd, saving)
}

// syncSaving runs the spinner of the saving rows while a board saves.
func (a *App) syncSaving() tea.Cmd {
	tb, _ := a.sprints.Taskboard()
	switch busy := a.kanban.Saving() || tb.Saving(); {
	case busy && !a.saving.active:
		return a.saving.start()
	case !busy:
		a.saving.stop()
	}
	return nil
}

func (a App) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinnerTickMsg:
		if msg.id == savingSpinner {
			return a, a.saving.update(msg)
		}
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		a.form.setWidth(msg.Width)
		a.picker.setWidth(msg.Width)
		a.picker.scroll(a.pickerListHeight())
		a.sprintPicker.setWidth(msg.Width)
		a.sprintPicker.scroll(a.pickerListHeight())
		a.palette.setWidth(msg.Width)
		a.palette.scroll(a.paletteListHeight())
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
			if a.opened != openedAtStart {
				what = "Couldn't load " + a.project.Name
			}
			return a.failed(msg.err, what, func(a App) (App, tea.Cmd) { return a.loadProject(slug) })
		}
		return a.projectLoaded(msg.project)
	case kanbanLoadedMsg:
		return a.kanbanLoaded(msg)
	case userStoryLoadedMsg:
		// The board drops results of earlier loads and other projects.
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
		// The board drops results of earlier loads and other Sprints.
		return a.withTaskboard(func(tb board.Taskboard) board.Taskboard {
			if msg.err != nil {
				return tb.TaskFailed(msg.load, msg.err)
			}
			return tb.TaskLoaded(msg.load, msg.task)
		}), nil
	case savedMsg:
		return a.savedResult(msg)
	case userStoryReloadedMsg:
		if msg.err != nil {
			return a, nil // the next refresh corrects the User Story
		}
		// The board drops results for other Sprints.
		return a.withTaskboard(func(tb board.Taskboard) board.Taskboard {
			return tb.UserStoryReloaded(msg.load, msg.story)
		}), nil
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

// clearTransient drops transient status entries and hints when msg is a key
// press.
func (a App) clearTransient(msg tea.Msg) App {
	if _, ok := msg.(tea.KeyPressMsg); !ok {
		return a
	}
	a.status = slices.DeleteFunc(slices.Clone(a.status), func(s statusMsg) bool { return s.transient || s.hint })
	return a
}

func (a App) withoutTransient() App {
	a.status = slices.DeleteFunc(slices.Clone(a.status), func(s statusMsg) bool { return s.transient })
	return a
}

func (a App) showLogin() App {
	a.asked = nil // no question waits across a login
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
			return a.openProject(projects[i], openedAtStart)
		default:
			hint = projects[i].Name + " has no boards enabled"
		}
	}
	if len(projects) == 1 && projects[0].HasBoards() {
		return a.openProject(projects[0], openedAtStart)
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

// opening tells how a project is opened.
type opening int

const (
	// openedAtStart opens the default project on the last used board.
	openedAtStart opening = iota
	// openedFromPicker opens a project chosen in the project picker on the
	// Kanban Board.
	openedFromPicker
	// openedFromPalette opens a project chosen in the command palette on
	// the board shown before.
	openedFromPalette
)

// openProject loads p, which becomes the default project once it loaded.
func (a App) openProject(p domain.ProjectSummary, how opening) (App, tea.Cmd) {
	a.project = domain.Project{ProjectSummary: p}
	a.opened = how
	// Neither board takes results until the project loaded.
	a.kanban, a.sprints = board.Kanban{}, board.Sprints{}
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
	a.kanban = board.UnloadedKanban(p, a.me.ID)
	a.sprints = board.NewSprints(p, a.me.ID)
	if a.state.DefaultProject != p.ID {
		a.state.DefaultProject = p.ID
		a = a.saveState()
	}
	want := a.wantedBoard()
	b := want
	if !boardOps(b).enabled(p.ProjectSummary) {
		b = otherBoard(b)
		// A Team Member who chose a new project in the picker expects no
		// particular board. At start the hint is due only for a board used
		// before.
		if a.opened == openedFromPalette || a.opened == openedAtStart && a.state.LastBoard != "" {
			a.status = append(a.status, statusMsg{
				text: boardOps(want).shortName + " is disabled in " + p.Name + ", showing " + boardOps(b).name,
				hint: true,
			})
		}
	}
	return a.openBoard(b)
}

// wantedBoard returns the board to open in the project if it is enabled:
// the last used board at start, the board shown before for a project
// chosen in the palette, and otherwise the Kanban Board.
func (a App) wantedBoard() config.Board {
	switch {
	case a.opened == openedAtStart && a.state.LastBoard != "":
		return a.state.LastBoard
	case a.opened == openedFromPalette:
		return a.board
	}
	return config.BoardKanban
}

// openBoard shows the board b of the project and remembers it as the last
// used board. A board loaded before stays visible while it refreshes.
func (a App) openBoard(b config.Board) (App, tea.Cmd) {
	// A board left for the Taskboard is where the Sprint picker goes back.
	a.sprintPickerBack = ""
	if a.screen == screenBoard && a.board != b {
		a.sprintPickerBack = a.board
		// Results of saves on the board left no longer count.
		a = boardOps(a.board).dropSaves(a)
	}
	a.board = b
	if a.state.LastBoard != b {
		a.state.LastBoard = b
		a = a.saveState()
	}
	return boardOps(b).open(a)
}

// openTaskboard shows the Taskboard: the one loaded before while it
// refreshes, or else that of the Current Sprint.
func (a App) openTaskboard() (App, tea.Cmd) {
	if tb, ok := a.sprints.Taskboard(); ok {
		a.screen = screenBoard
		a.scrollBoard()
		return a.loadTaskboard(tb.Sprint())
	}
	if a.sprints.Listed() {
		// The palette loaded the Sprints just before.
		return a.showCurrentSprint()
	}
	return a.loadSprints()
}

func (a App) loadSprints() (App, tea.Cmd) {
	var load board.LoadSprints
	a.sprints, load = a.sprints.LoadSprints()
	api := a.deps.Taiga
	cmd := func() tea.Msg {
		sprints, err := api.Sprints(context.Background(), load.ProjectID)
		return sprintsLoadedMsg{load: load, sprints: sprints, err: err}
	}
	if load.Refresh || a.board != config.BoardTaskboard {
		return a, cmd // the board stays visible
	}
	return a.startLoading("Loading "+a.project.Name+"…", cmd)
}

// sprintsLoaded opens the Current Sprint, lets the Team Member choose one,
// or shows that the project has no Sprints.
func (a App) sprintsLoaded(msg sprintsLoadedMsg) (tea.Model, tea.Cmd) {
	if !a.sprints.SprintsCurrent(msg.load) {
		return a, nil
	}
	// Off the Taskboard the palette loaded the Sprints in the background.
	background := a.board != config.BoardTaskboard
	if msg.err != nil {
		if background && !errors.Is(msg.err, taiga.ErrSessionExpired) {
			a = a.withoutTransient()
			a.status = append(a.status, statusMsg{text: "Couldn't load sprints: " + reason(msg.err), isErr: true, transient: true})
			a.scrollBoard()
			return a, nil
		}
		if msg.load.Refresh && !errors.Is(msg.err, taiga.ErrSessionExpired) {
			return a.refreshFailed(msg.err), nil
		}
		return a.failed(msg.err, "Couldn't load Taskboard", App.loadSprints)
	}
	if !background {
		a = a.withoutTransient()
		a.spinner.stop()
	}
	a.sprints, _ = a.sprints.SprintsLoaded(msg.load, msg.sprints)
	if id, ok := a.state.Sprints[a.project.ID]; ok && !a.sprints.Knows(id) {
		a.state = a.state.WithoutSprint(a.project.ID)
		a = a.saveState()
	}
	if a.paletteOpen && !a.paletteStatus {
		a.palette.setEntries(a.paletteEntries())
		a.palette.scroll(a.paletteListHeight())
	}
	if _, ok := a.sprints.Taskboard(); ok || background {
		return a, nil // a refresh of the list keeps the Sprint shown
	}
	return a.showCurrentSprint()
}

// showCurrentSprint opens the Taskboard of the Current Sprint, lets the
// Team Member choose a Sprint, or shows that the project has no Sprints.
func (a App) showCurrentSprint() (App, tea.Cmd) {
	res := domain.ResolveCurrentSprint(a.sprints.List(), a.today(), a.state.Sprints[a.project.ID])
	switch res.Kind {
	case domain.One:
		return a.loadTaskboard(res.Sprint)
	case domain.Choose:
		return a.showSprintPicker(res.Choices, chooseHints[res.Reason]), nil
	}
	a.screen = screenBoard
	return a, nil
}

// chooseHints tell why the Team Member has to choose a Sprint.
var chooseHints = map[domain.ChooseReason]string{
	domain.SeveralCurrent: "Several sprints contain today",
	domain.NoneCurrent:    "No sprint contains today",
	domain.NoneOpen:       "No sprint is open",
}

func (a App) today() domain.Date { return domain.DateOf(a.deps.Now()) }

// showSprintPicker lets the Team Member choose one of sprints.
func (a App) showSprintPicker(sprints []domain.Sprint, hint string) App {
	a.screen = screenSprintPicker
	a.sprintPicker = newPicker(sprintItems(sprints, a.today()), "sprint", hint)
	a.sprintPicker.setWidth(a.width)
	a.sprintPicker.scroll(a.pickerListHeight())
	return a
}

func (a App) updateSprintPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	action, cmd := a.sprintPicker.update(msg, a.pickerListHeight())
	switch action {
	case pickerQuit:
		if back := a.sprintPickerBack; back != "" {
			return a.openBoard(back)
		}
		return a, tea.Quit
	case pickerChoose:
		id, _ := a.sprintPicker.chosen()
		return a.openSprint(id)
	}
	return a, cmd
}

// openSprint shows the Taskboard of the Sprint with the given ID and
// remembers the Sprint and the Taskboard as last used.
func (a App) openSprint(id int64) (App, tea.Cmd) {
	list := a.sprints.List()
	i := slices.IndexFunc(list, func(s domain.Sprint) bool { return s.ID == id })
	if i < 0 {
		return a, nil
	}
	a.board = config.BoardTaskboard
	a.state = a.state.WithSprint(a.project.ID, id)
	a.state.LastBoard = config.BoardTaskboard
	a = a.saveState()
	return a.loadTaskboard(list[i])
}

// loadTaskboard loads the Taskboard of s. Refreshing the Taskboard shown
// keeps it visible.
func (a App) loadTaskboard(s domain.Sprint) (App, tea.Cmd) {
	var load board.LoadTaskboard
	a.sprints, load = a.sprints.Open(s)
	api := a.deps.Taiga
	cmd := func() tea.Msg {
		c, err := api.Taskboard(context.Background(), load.ProjectID, load.Sprint.ID)
		return taskboardLoadedMsg{load: load, content: c, err: err}
	}
	if load.Refresh {
		a.screen = screenBoard
		return a, cmd
	}
	return a.startLoading("Loading "+s.Name+"…", cmd)
}

func (a App) taskboardLoaded(msg taskboardLoadedMsg) (tea.Model, tea.Cmd) {
	if !a.sprints.TaskboardCurrent(msg.load) {
		return a, nil
	}
	if msg.err != nil {
		if msg.load.Refresh && !errors.Is(msg.err, taiga.ErrSessionExpired) {
			return a.refreshFailed(msg.err), nil
		}
		s := msg.load.Sprint
		return a.failed(msg.err, "Couldn't load Taskboard", func(a App) (App, tea.Cmd) { return a.loadTaskboard(s) })
	}
	a = a.withoutTransient()
	a.spinner.stop()
	if !msg.load.Refresh {
		a.taskboardTop = 0
	}
	a.sprints, _ = a.sprints.TaskboardLoaded(msg.load, msg.content)
	a.screen = screenBoard
	a.scrollBoard()
	return a.loadDetail()
}

// withTaskboard applies change to the Taskboard shown, if any.
func (a App) withTaskboard(change func(board.Taskboard) board.Taskboard) App {
	if tb, ok := a.sprints.Taskboard(); ok {
		a.sprints = a.sprints.WithTaskboard(change(tb))
	}
	return a
}

// refreshFailed shows why a refresh failed while the board stays usable.
func (a App) refreshFailed(err error) App {
	a = a.withoutTransient()
	a.status = append(a.status, statusMsg{text: "Couldn't refresh: " + reason(err), isErr: true, transient: true})
	a.scrollBoard() // the status line takes rows from the list
	return a
}

func (a App) loadKanban() (App, tea.Cmd) {
	var load board.LoadKanban
	a.kanban, load = a.kanban.LoadUserStories()
	api := a.deps.Taiga
	cmd := func() tea.Msg {
		stories, err := api.Kanban(context.Background(), load.ProjectID)
		return kanbanLoadedMsg{load: load, stories: stories, err: err}
	}
	if load.Refresh {
		a.screen = screenBoard
		return a, cmd // the board stays visible
	}
	return a.startLoading("Loading "+a.project.Name+"…", cmd)
}

func (a App) kanbanLoaded(msg kanbanLoadedMsg) (tea.Model, tea.Cmd) {
	if !a.kanban.UserStoriesCurrent(msg.load) {
		return a, nil
	}
	if msg.err != nil {
		if msg.load.Refresh && !errors.Is(msg.err, taiga.ErrSessionExpired) {
			return a.refreshFailed(msg.err), nil
		}
		return a.failed(msg.err, "Couldn't load Kanban Board", App.loadKanban)
	}
	a = a.withoutTransient()
	a.spinner.stop()
	if !msg.load.Refresh {
		a.kanbanTop = 0
	}
	a.kanban, _ = a.kanban.UserStoriesLoaded(msg.load, msg.stories)
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
	return a.shownBoard().loadDetail(a)
}

func (a App) loadUserStoryDetail() (App, tea.Cmd) {
	if !a.kanban.Loaded() {
		return a, nil
	}
	k, load, ok := a.kanban.LoadDetail()
	if !ok {
		return a, nil
	}
	a.kanban = k
	api := a.deps.Taiga
	return a, func() tea.Msg {
		s, err := api.UserStory(context.Background(), load.ID)
		return userStoryLoadedMsg{load: load, story: s, err: err}
	}
}

func (a App) loadTaskDetail() (App, tea.Cmd) {
	tb, ok := a.sprints.Taskboard()
	if !ok {
		return a, nil
	}
	tb, load, ok := tb.LoadDetail()
	if !ok {
		return a, nil
	}
	a.sprints = a.sprints.WithTaskboard(tb)
	api := a.deps.Taiga
	return a, func() tea.Msg {
		t, err := api.Task(context.Background(), load.ID)
		return taskLoadedMsg{load: load, task: t, err: err}
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
		return a.openProject(a.projects[indexOfProject(a.projects, id)], openedFromPicker)
	}
	return a, cmd
}

func (a App) updateBoard(msg tea.Msg) (tea.Model, tea.Cmd) {
	if a.paletteOpen {
		return a.updatePalette(msg)
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return a, nil
	}
	if a.asked != nil {
		return a.answerAssignToMe(key)
	}
	switch key.String() {
	case "q":
		return a, tea.Quit
	case ":", "ctrl+k":
		return a.openPalette()
	case "s":
		return a.openStatusPalette(), nil
	case "a":
		return a.assignToMe(board.ConfirmAssignToMe{})
	case "?":
		a.helpOpen = !a.helpOpen
		a.scrollBoard()
		return a, nil
	case "esc":
		if a.helpOpen {
			a.helpOpen = false
			a.scrollBoard()
			return a, nil
		}
	}
	return a.shownBoard().update(a, key)
}

func (a App) updateKanban(key tea.KeyPressMsg) (App, tea.Cmd) {
	switch key.String() {
	case "r":
		if a.kanban.Loaded() {
			return a.loadKanban()
		}
	case "shift+left", "H":
		return a.changeStatus(board.Previous)
	case "shift+right", "L":
		return a.changeStatus(board.Next)
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

func (a App) updateTaskboard(key tea.KeyPressMsg) (App, tea.Cmd) {
	tb, ok := a.sprints.Taskboard()
	if !ok {
		// The project has no Sprints.
		if key.String() == "r" {
			return a.loadSprints()
		}
		return a, nil
	}
	before := tb
	switch key.String() {
	case "r":
		a, sprints := a.loadSprints()
		a, taskboard := a.loadTaskboard(tb.Sprint())
		return a, tea.Batch(sprints, taskboard)
	case "up", "k":
		tb = tb.CursorUp()
	case "down", "j":
		tb = tb.CursorDown()
	case "enter":
		if tb.TaskSelected() {
			a.detailOpen = !a.detailOpen
		} else {
			tb = tb.Toggle()
		}
	}
	if key.String() == "enter" {
		a.detailOpen = detailAfterFold(before, tb, a.detailOpen)
	}
	a.sprints = a.sprints.WithTaskboard(tb)
	a.scrollBoard()
	return a.loadDetail()
}

// detailAfterFold returns whether the detail view is open after the
// Taskboard went from before to after by enter, which
// leaves the cursor on its row. Folding the Tasks of a User Story
// hides the detail view and unfolding them shows it, and nothing else
// changes it.
func detailAfterFold(before, after board.Taskboard, open bool) bool {
	was, ok := before.Expanded()
	now, stillOK := after.Expanded()
	if !ok || !stillOK || was == now {
		return open
	}
	return now
}

// openPalette shows the command palette over the board and loads the
// Sprints for it if the project has a Taskboard and they are not loaded.
func (a App) openPalette() (App, tea.Cmd) {
	a.paletteOpen, a.paletteStatus = true, false
	a.helpOpen = false // the palette has its own footer
	a.palette = newPalette(a.paletteEntries(), "Go to a board, sprint or project")
	a.palette.setWidth(a.width)
	a.palette.scroll(a.paletteListHeight())
	if a.hasTaskboard() && !a.sprints.Listed() && a.board != config.BoardTaskboard {
		return a.loadSprints()
	}
	return a, nil
}

// paletteEntries lists the boards, the Sprints and the projects to go to.
func (a App) paletteEntries() []paletteEntry {
	var es []paletteEntry
	for _, b := range allBoards {
		ops := boardOps(b)
		e := paletteEntry{kind: entryBoard, board: b, label: "Go to " + ops.name, current: b == a.board}
		if !ops.enabled(a.project.ProjectSummary) {
			e.disabled, e.note = true, "(disabled in this project)"
			e.hint = ops.name + " is disabled in " + a.project.Name + ", use the " + boardOps(otherBoard(b)).name
		}
		es = append(es, e)
	}
	if a.hasTaskboard() {
		sprints := domain.NewestFirst(a.sprints.List())
		tb, shown := a.sprints.Taskboard()
		for i, note := range sprintNotes(sprints, a.today()) {
			s := sprints[i]
			es = append(es, paletteEntry{
				kind: entrySprint, id: s.ID, label: "Sprint: " + plainText(s.Name), note: note,
				current: a.board == config.BoardTaskboard && shown && tb.Sprint().ID == s.ID,
			})
		}
	}
	for _, it := range projectItems(a.projects) {
		p := a.projects[indexOfProject(a.projects, it.id)]
		e := paletteEntry{kind: entryProject, id: p.ID, label: "Project: " + p.Name, current: p.ID == a.project.ID}
		if p.ID == a.state.DefaultProject {
			e.label += " ★"
		}
		e.note, e.disabled = it.note, it.disabled
		if !e.disabled {
			e.note = boardNames(p)
		}
		es = append(es, e)
	}
	return es
}

// boardNames lists the boards enabled in p.
func boardNames(p domain.ProjectSummary) string {
	var names []string
	for _, b := range allBoards {
		if ops := boardOps(b); ops.enabled(p) {
			names = append(names, ops.name)
		}
	}
	return strings.Join(names, " · ")
}

// hasTaskboard reports whether the project has the Taskboard enabled.
func (a App) hasTaskboard() bool {
	return boardOps(config.BoardTaskboard).enabled(a.project.ProjectSummary)
}

// updatePalette handles msg while the palette is open.
func (a App) updatePalette(msg tea.Msg) (tea.Model, tea.Cmd) {
	action, cmd := a.palette.update(msg, a.paletteListHeight())
	switch action {
	case paletteClose:
		a.paletteOpen = false
	case paletteChoose:
		a.paletteOpen = false
		e, _ := a.palette.chosen()
		if e.current {
			return a, nil
		}
		switch e.kind {
		case entryStatus:
			// The board may have reloaded under the palette.
			if opts, err := a.shownBoard().statusOptions(a); err == nil && (opts.Kind != a.pickedFor.Kind || opts.Item != a.pickedFor.Item) {
				return a, nil
			}
			return a.pickStatus(e.id)
		case entryBoard:
			return a.openBoard(e.board)
		case entrySprint:
			return a.openSprint(e.id)
		case entryProject:
			return a.openProject(a.projects[indexOfProject(a.projects, e.id)], openedFromPalette)
		}
	}
	return a, cmd
}

// paletteListHeight is the height of the list of the palette.
func (a App) paletteListHeight() int {
	_, listH := paletteBox(a.width, a.bodyHeight())
	return listH
}

// scrollBoard keeps the selected row of the board visible.
func (a *App) scrollBoard() { a.shownBoard().scroll(a) }

func (a *App) scrollTaskboard() {
	tb, _ := a.sprints.Taskboard()
	lineOf, n := taskboardLines(tb.Rows())
	sel := -1
	if i := tb.SelectedIndex(); i >= 0 {
		sel = lineOf[i]
	}
	l := layoutBoard(a.width, a.bodyHeight(), a.detailOpen)
	a.taskboardTop = scrollTop(a.taskboardTop, sel, n, l.listH)
}

func (a *App) scrollKanban() {
	n := len(a.kanban.Rows())
	l := layoutBoard(a.width, a.bodyHeight()-kanbanChromeLines, a.detailOpen)
	a.kanbanTop = scrollTop(a.kanbanTop, a.kanban.SelectedIndex(), n, l.listH)
}

func (a App) pickerListHeight() int { return a.bodyHeight() - pickerChromeLines }

// bodyHeight is the height between header and status line plus footer.
func (a App) bodyHeight() int {
	return a.height - 1 - len(a.statusLines(1)) - len(a.footerLines())
}

// footerLines renders the footer: the key help line, or the full help on
// the board when it is expanded.
func (a App) footerLines() []string {
	switch a.screen {
	case screenPicker:
		return []string{a.pickerFooter()}
	case screenSprintPicker:
		if a.sprintPickerBack != "" {
			return []string{footer(strings.Replace(pickerKeys, "esc quit", "esc back", 1), a.width)}
		}
		return []string{footer(pickerKeys, a.width)}
	case screenBoard:
		switch {
		case a.paletteOpen:
			return []string{footer(paletteKeys, a.width)}
		case a.helpOpen:
			return fullHelp(append(slices.Clone(a.shownBoard().help), commonHelp...), a.width)
		}
		return []string{footerOf(a.boardKeys(), a.width)}
	}
	return []string{""}
}

// View implements tea.Model.
func (a App) View() tea.View {
	if a.width == 0 {
		return tea.View{AltScreen: true}
	}
	var content string
	switch a.screen {
	case screenPicker:
		content = a.framed("Select project", a.pickerBody())
	case screenSprintPicker:
		path := projectStyle.Render(a.project.Name) + pathSep + titleStyle.Render("Select sprint")
		content = a.framed(path, a.sprintPicker.view(a.width, a.bodyHeight()))
	case screenBoard:
		body := a.boardBody()
		if a.paletteOpen {
			boxW, _ := paletteBox(a.width, 0)
			body = overlay(body, a.palette.view(a.width, a.bodyHeight()), (a.width-boxW)/2, paletteTop)
		}
		content = a.framed(a.boardPath(), body)
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
func (a App) framed(path, body string) string {
	header := spread(" "+path, a.me.FullName+" ", a.width)
	lines := []string{header, body}
	lines = append(lines, a.statusLines(1)...)
	lines = append(lines, a.footerLines()...)
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
	return projectStyle.Render(a.project.Name) + pathSep + a.shownBoard().path(a)
}

func (a App) kanbanPath() string { return titleStyle.Render("Kanban Board") }

func (a App) taskboardPath() string {
	tb, ok := a.sprints.Taskboard()
	if !ok {
		return titleStyle.Render("Taskboard")
	}
	path := "Taskboard" + pathSep
	full := path + sprintPath(tb.Sprint(), a.today(), true)
	// Leave out the date range rather than the Team Member's name.
	head := projectStyle.Render(a.project.Name) + pathSep + full
	if lipgloss.Width(head)+lipgloss.Width(a.me.FullName)+3 <= a.width {
		return full
	}
	return path + sprintPath(tb.Sprint(), a.today(), false)
}

func (a App) boardBody() string { return a.shownBoard().body(a, a.bodyHeight()) }

func (a App) kanbanBody(h int) string {
	return renderKanban(a.kanban, a.kanbanTop, a.width, h, a.detailOpen, a.saving.glyph())
}

func (a App) taskboardBody(h int) string {
	tb, ok := a.sprints.Taskboard()
	if !ok {
		return centered(a.width, h,
			faintStyle.Render("No sprints in "+a.project.Name), "",
			faintStyle.Render(noSprintsKeys))
	}
	return renderTaskboard(tb, a.taskboardTop, a.width, h, a.detailOpen, a.saving.glyph())
}

func (a App) boardKeys() []string { return a.shownBoard().keys(a) }

func (a App) kanbanFooter() []string { return kanbanKeys(a.kanban) }

func (a App) taskboardFooter() []string {
	if tb, ok := a.sprints.Taskboard(); ok {
		return taskboardKeys(tb)
	}
	return []string{"r refresh", ": menu", "? help"}
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
