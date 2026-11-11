// Package ui is the Bubble Tea front end: the App router, its screens and
// components.
package ui

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/zeckitoz/taiga-tui/internal/config"
	"github.com/zeckitoz/taiga-tui/internal/session"
	"github.com/zeckitoz/taiga-tui/internal/taiga"
)

// Session is the part of session.Manager the UI uses.
type Session interface {
	Resume(username string) bool
	Authenticate(ctx context.Context, login, password string) (taiga.LoginResult, error)
	Save(res taiga.LoginResult) (session.Saved, error)
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
	State        StateStore
	InitialState config.State
	// Warnings are shown in the status line from the start.
	Warnings []string
	// Tick replaces tea.Tick in tests.
	Tick TickFunc
}

type screen int

const (
	screenStarting screen = iota
	screenLogin
	screenLoggedIn
)

// keyringSlowDelay is how long the start may take before "Waiting for system
// keyring…" appears, so that a fast keyring causes no flicker.
const keyringSlowDelay = 250 * time.Millisecond

const waitingForKeyring = "Waiting for system keyring…"

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
)

// statusMsg is one entry of the status line.
type statusMsg struct {
	text  string
	isErr bool
}

// App is the root model.
type App struct {
	deps          Deps
	state         config.State
	screen        screen
	form          loginForm
	startSpinner  spinner
	keyringIsSlow bool
	status        []statusMsg
	width, height int
}

// New returns the App. It starts by looking for a stored session.
func New(d Deps) App {
	if d.Tick == nil {
		d.Tick = tea.Tick
	}
	a := App{deps: d, state: d.InitialState, startSpinner: spinner{tick: d.Tick}}
	for _, w := range d.Warnings {
		a.status = append(a.status, statusMsg{text: w})
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
		return a, a.startSpinner.start()
	case resumeDoneMsg:
		a.startSpinner.stop()
		if msg.ok {
			a.screen = screenLoggedIn
			return a, nil
		}
		a.screen = screenLogin
		a.form = newLoginForm(a.deps.Host, a.state.Login, a.deps.Tick)
		a.form.setWidth(a.width)
		return a, nil
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
		return a.loggedIn(msg), nil
	}

	switch a.screen {
	case screenStarting:
		if tick, ok := msg.(spinnerTickMsg); ok {
			return a, a.startSpinner.update(tick)
		}
	case screenLogin:
		return a.updateLogin(msg)
	case screenLoggedIn:
		if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "q" {
			return a, tea.Quit
		}
	}
	return a, nil
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
	a.screen = screenLoggedIn
	a.form = loginForm{} // drop the password
	a.state = a.state.LoggedIn(msg.result.Member, msg.login)
	if msg.saved.UsedFile {
		a.status = append(a.status, statusMsg{
			text: "System keyring unavailable, login saved in " + msg.saved.File,
		})
	}
	if err := a.deps.State.Save(a.state); err != nil {
		a.status = append(a.status, statusMsg{text: "Couldn't save settings: " + err.Error(), isErr: true})
	}
	return a
}

// View implements tea.Model.
func (a App) View() tea.View {
	if a.width == 0 {
		return tea.View{AltScreen: true}
	}
	var body string
	switch a.screen {
	case screenStarting:
		if a.keyringIsSlow {
			body = a.startSpinner.view() + " " + waitingForKeyring
		}
	case screenLogin:
		body = a.form.view()
	case screenLoggedIn:
		body = "Logged in as " + titleStyle.Render(a.state.FullName) + "\n\n" + faintStyle.Render("q quit")
	}
	status := a.statusLine()
	height := a.height - lipgloss.Height(status)
	if status == "" {
		height = a.height
	}
	content := lipgloss.Place(a.width, max(0, height), lipgloss.Center, lipgloss.Center, body)
	if status != "" {
		content += "\n" + status
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (a App) statusLine() string {
	lines := make([]string, 0, len(a.status))
	for _, s := range a.status {
		text := ansi.Truncate(s.text, a.width, "…")
		if s.isErr {
			lines = append(lines, errorStyle.Render(text))
		} else {
			lines = append(lines, warnStyle.Render(text))
		}
	}
	return strings.Join(lines, "\n")
}

// loginErrorText is the text the login dialog shows for err.
func loginErrorText(host string, err error) string {
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
