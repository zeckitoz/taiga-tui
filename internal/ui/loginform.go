package ui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// formAction is what the login form asks its owner to do after a key press.
type formAction int

const (
	formNone formAction = iota
	formSubmit
	formCancel
)

const (
	fieldLogin = iota
	fieldPassword
)

// dialogWidth is the content width of the login dialog on wide terminals.
const dialogWidth = 44

// loginForm is the login dialog: two fields, an error line and a busy state
// with spinner during which input is locked.
type loginForm struct {
	host     string
	login    textinput.Model
	password textinput.Model
	focus    int
	err      string
	// expired is set for the dialog that opens over the board when the
	// session ran out.
	expired bool
	busy    string // non-empty while a request runs
	spinner spinner
	width   int // content width
}

func newLoginForm(host, prefill string, tick TickFunc) loginForm {
	f := loginForm{
		host:     host,
		login:    newField(),
		password: newField(),
		spinner:  spinner{tick: tick},
		width:    dialogWidth,
	}
	f.password.EchoMode = textinput.EchoPassword
	f.password.EchoCharacter = '•'
	f.login.SetValue(prefill)
	if prefill != "" {
		f.focusField(fieldPassword)
	} else {
		f.focusField(fieldLogin)
	}
	return f
}

func newField() textinput.Model {
	in := textinput.New()
	in.Prompt = "  "
	styles := textinput.DefaultDarkStyles()
	styles.Focused.Prompt = promptStyle
	styles.Cursor.Blink = false
	styles.Cursor.Color = accentColor
	in.SetStyles(styles)
	return in
}

func (f *loginForm) focusField(field int) {
	f.focus = field
	f.login.Blur()
	f.password.Blur()
	f.login.Prompt, f.password.Prompt = "  ", "  "
	in := f.field(field)
	in.Prompt = "❯ "
	in.Focus() // static cursor, so no blink command
}

func (f *loginForm) field(field int) *textinput.Model {
	if field == fieldPassword {
		return &f.password
	}
	return &f.login
}

// setWidth fits the dialog into a terminal of the given width.
func (f *loginForm) setWidth(termWidth int) {
	// Border and horizontal padding take 8 cells.
	f.width = max(10, min(dialogWidth, termWidth-8))
	for _, in := range []*textinput.Model{&f.login, &f.password} {
		in.SetWidth(f.width - lipgloss.Width(in.Prompt) - 1)
	}
}

// setBusy locks the form and shows text next to a spinner.
func (f *loginForm) setBusy(text string) tea.Cmd {
	f.err = ""
	wasBusy := f.busy != ""
	f.busy = text
	if wasBusy {
		return nil
	}
	return f.spinner.start()
}

// fail unlocks the form, shows the error and clears the password.
func (f *loginForm) fail(text string) {
	f.busy = ""
	f.spinner.stop()
	f.err = text
	f.password.SetValue("")
	f.focusField(fieldPassword)
}

func (f *loginForm) update(msg tea.Msg) (formAction, tea.Cmd) {
	if tick, ok := msg.(spinnerTickMsg); ok {
		return formNone, f.spinner.update(tick)
	}
	if f.busy != "" {
		return formNone, nil
	}
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			return formCancel, nil
		case "tab", "shift+tab", "up", "down":
			f.focusField(1 - f.focus)
			return formNone, nil
		case "enter":
			switch {
			case strings.TrimSpace(f.login.Value()) == "":
				f.focusField(fieldLogin)
			case f.password.Value() == "":
				f.focusField(fieldPassword)
			default:
				return formSubmit, nil
			}
			return formNone, nil
		}
	}
	in := f.field(f.focus)
	var cmd tea.Cmd
	*in, cmd = in.Update(msg)
	return formNone, cmd
}

func (f *loginForm) values() (login, password string) {
	return strings.TrimSpace(f.login.Value()), f.password.Value()
}

func (f *loginForm) view() string {
	label := func(field int, text string) string {
		if f.focus == field {
			return labelStyle.Render(text)
		}
		return faintStyle.Render(text)
	}
	var status string
	switch {
	case f.busy != "":
		status = f.spinner.view() + " " + f.busy
	case f.err != "":
		status = errorStyle.Render(f.err)
	}
	help := "enter log in · tab switch field · esc quit"
	if f.expired {
		help = "enter log in · tab field · esc keep board"
	}
	if f.busy != "" {
		help = "ctrl+c quit"
	}
	lines := []string{titleStyle.Render("Log in to " + f.host)}
	if f.expired {
		lines = append(lines, faintStyle.Render("Your session has expired"))
	}
	lines = append(lines,
		"",
		label(fieldLogin, "Username or email"),
		f.login.View(),
		"",
		label(fieldPassword, "Password"),
		f.password.View(),
		"",
		status,
		"",
		faintStyle.Render(help),
	)
	content := lipgloss.NewStyle().Width(f.width).Render(strings.Join(lines, "\n"))
	return dialogStyle.Render(content)
}
