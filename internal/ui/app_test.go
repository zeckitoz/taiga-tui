package ui

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/zeckitoz/taiga-tui/internal/config"
	"github.com/zeckitoz/taiga-tui/internal/domain"
	"github.com/zeckitoz/taiga-tui/internal/session"
	"github.com/zeckitoz/taiga-tui/internal/taiga"
	"github.com/zeckitoz/taiga-tui/internal/taiga/taigafake"
)

var update = flag.Bool("update", false, "rewrite golden files")

var alice = domain.TeamMember{ID: 42, Username: "alice", FullName: "Alice Example"}

func TestStartWithValidTokenSkipsLogin(t *testing.T) {
	sess := &fakeSession{resume: true}
	h := newHarness(t, sess, config.State{UserID: 42, Username: "alice", Login: "alice", FullName: "Alice Example"})

	h.start()

	if sess.resumedFor != "alice" {
		t.Errorf("resume asked for %q", sess.resumedFor)
	}
	h.golden("picker")
}

func TestStartWithoutTokenShowsLoginWithEmptyFields(t *testing.T) {
	h := newHarness(t, &fakeSession{}, config.State{})

	h.start()

	h.golden("login_empty")
}

func TestStartPrefillsLoginAndFocusesPassword(t *testing.T) {
	h := newHarness(t, &fakeSession{}, config.State{UserID: 42, Username: "alice", Login: "alice@example.org"})

	h.start()
	h.typeText("pw")

	if got := h.app.form.password.Value(); got != "pw" {
		t.Errorf("typing went to the wrong field, password %q", got)
	}
	h.golden("login_prefilled")
}

func TestWaitingForKeyringIsShownWhenStartIsSlow(t *testing.T) {
	h := newHarness(t, &fakeSession{}, config.State{})
	h.send(tea.WindowSizeMsg{Width: 80, Height: 24})

	h.sendOnly(keyringSlowMsg{})

	h.golden("waiting_for_keyring")
}

func TestTabSwitchesFields(t *testing.T) {
	h := newHarness(t, &fakeSession{}, config.State{})
	h.start()

	h.typeText("alice")
	h.key("tab")
	h.typeText("pw")
	h.key("shift+tab")
	h.typeText("!")

	if got := h.app.form.login.Value(); got != "alice!" {
		t.Errorf("login %q", got)
	}
	if got := h.app.form.password.Value(); got != "pw" {
		t.Errorf("password %q", got)
	}
}

func TestEnterWithEmptyFieldFocusesIt(t *testing.T) {
	sess := &fakeSession{}
	h := newHarness(t, sess, config.State{})
	h.start()

	h.key("enter")
	h.typeText("alice")
	h.key("enter")
	h.typeText("pw")

	if sess.authCalls != 0 {
		t.Error("submitted an incomplete form")
	}
	if h.app.form.login.Value() != "alice" || h.app.form.password.Value() != "pw" {
		t.Errorf("got %q / %q", h.app.form.login.Value(), h.app.form.password.Value())
	}
}

func TestSuccessfulLoginSavesTokensAndState(t *testing.T) {
	sess := &fakeSession{result: taiga.LoginResult{Member: alice}}
	h := newHarness(t, sess, config.State{UserID: 7, Username: "bob", Login: "bob", FullName: "Bob"})
	h.start()

	h.key("shift+tab")
	h.key("ctrl+u")
	h.typeText("alice@example.org")
	h.key("tab")
	h.typeText("s3cret")
	h.key("enter")

	if sess.login != "alice@example.org" || sess.password != "s3cret" {
		t.Errorf("authenticated with %q/%q", sess.login, sess.password)
	}
	if sess.savedResult.Member != alice {
		t.Errorf("saved %+v", sess.savedResult)
	}
	want := config.State{UserID: 42, Username: "alice", Login: "alice@example.org", FullName: "Alice Example"}
	if h.state.saved != want {
		t.Errorf("state.json: got %+v\nwant %+v", h.state.saved, want)
	}
	h.golden("picker")
}

func TestLoginWhileRequestRunsShowsSpinnerAndLocksInput(t *testing.T) {
	h := newHarness(t, &fakeSession{}, config.State{Login: "alice"})
	h.start()
	h.typeText("pw")

	h.sendOnly(keyMsg("enter")) // the request is in flight
	h.sendOnly(keyMsg("x"))
	h.sendOnly(keyMsg("esc"))

	if h.quit {
		t.Error("esc quit while the request was running")
	}
	if got := h.app.form.password.Value(); got != "pw" {
		t.Errorf("input not locked, password %q", got)
	}
	h.golden("login_busy")
}

func TestSavingTokensShowsKeyringSpinner(t *testing.T) {
	h := newHarness(t, &fakeSession{}, config.State{Login: "alice"})
	h.start()

	h.sendOnly(authDoneMsg{result: taiga.LoginResult{Member: alice}, login: "alice"})

	h.golden("login_saving")
}

func TestFailedLoginShowsErrorAndClearsPassword(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		want   string
		golden string
	}{
		{name: "invalid", err: taiga.ErrInvalidCredentials, want: "Invalid username or password", golden: "login_invalid"},
		{name: "invalid with reason", err: &taiga.RejectedError{Code: 401, Reason: "No active account found with the given credentials", Err: taiga.ErrInvalidCredentials}, want: "Invalid username or password"},
		{name: "rejected", err: &taiga.RejectedError{Code: 400, Reason: "username: This field may not be blank.", Err: taiga.ErrInvalidCredentials}, want: "Login rejected: username: This field may not be blank.", golden: "login_rejected"},
		{name: "rejected without reason", err: &taiga.RejectedError{Code: 400, Err: taiga.ErrInvalidCredentials}, want: "Invalid username or password"},
		{name: "unreachable", err: fmt.Errorf("%w: dial tcp: connection refused", taiga.ErrUnreachable), want: "Cannot reach taiga.example.org: no connection"},
		{name: "timeout", err: taiga.ErrTimeout, want: "Cannot reach taiga.example.org: timed out"},
		{name: "server", err: &taiga.ServerError{Code: 502}, want: "Cannot reach taiga.example.org: server error (502)"},
		{name: "unexpected", err: taiga.ErrUnexpected, want: "Cannot reach taiga.example.org: unexpected response from server"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, &fakeSession{authErr: tt.err}, config.State{Login: "alice"})
			h.start()
			h.typeText("wrong")

			h.key("enter")

			if got := h.app.form.password.Value(); got != "" {
				t.Errorf("password not cleared: %q", got)
			}
			if got := h.app.form.login.Value(); got != "alice" {
				t.Errorf("login changed: %q", got)
			}
			if view := h.plainView(); !strings.Contains(words(view), tt.want) {
				t.Errorf("view lacks %q:\n%s", tt.want, view)
			}
			if tt.golden != "" {
				h.golden(tt.golden)
			}
		})
	}
}

func TestFailedTokenSaveShowsError(t *testing.T) {
	sess := &fakeSession{result: taiga.LoginResult{Member: alice}, saveErr: errors.New("disk full")}
	h := newHarness(t, sess, config.State{Login: "alice"})
	h.start()
	h.typeText("pw")

	h.key("enter")

	if view := h.plainView(); !strings.Contains(words(view), "Couldn't save login: disk full") {
		t.Errorf("view:\n%s", view)
	}
	if h.state.saves != 0 {
		t.Error("state saved although the tokens were not")
	}
}

func TestKeyringFallbackShowsOneTimeHint(t *testing.T) {
	sess := &fakeSession{
		result:  taiga.LoginResult{Member: alice},
		savedTo: session.Saved{File: "/home/alice/.config/taiga-tui/credentials"},
	}
	h := newHarness(t, sess, config.State{Login: "alice"})
	h.start()
	h.typeText("pw")

	h.key("enter")

	h.golden("logged_in_file_fallback")
}

func TestFailedStateSaveIsShownWithoutQuitting(t *testing.T) {
	sess := &fakeSession{result: taiga.LoginResult{Member: alice}}
	h := newHarness(t, sess, config.State{Login: "alice"})
	h.state.err = errors.New("read-only file system")
	h.start()
	h.typeText("pw")

	h.key("enter")

	if view := h.plainView(); !strings.Contains(view, "Couldn't save settings: read-only file system") || !strings.Contains(view, "Select project") {
		t.Errorf("view:\n%s", view)
	}
}

func TestEscOnLoginQuits(t *testing.T) {
	h := newHarness(t, &fakeSession{}, config.State{})
	h.start()

	h.key("esc")

	if !h.quit {
		t.Error("esc did not quit")
	}
}

func TestQuitKeys(t *testing.T) {
	for _, k := range []string{"q", "ctrl+c"} {
		t.Run(k, func(t *testing.T) {
			h := newHarness(t, &fakeSession{resume: true}, config.State{UserID: 42, Username: "alice", FullName: "Alice", DefaultProject: webShop.ID})
			h.start()

			h.key(k)

			if !h.quit {
				t.Errorf("%s did not quit", k)
			}
		})
	}
	t.Run("ctrl+c on login", func(t *testing.T) {
		h := newHarness(t, &fakeSession{}, config.State{})
		h.start()
		h.typeText("q")

		h.key("ctrl+c")

		if !h.quit || h.app.form.login.Value() != "q" {
			t.Error("q must be typed, ctrl+c must quit")
		}
	})
}

func TestWarningsAppearInStatusLine(t *testing.T) {
	h := newHarness(t, &fakeSession{}, config.State{})
	h.app = New(Deps{
		Host:     "taiga.example.org",
		Session:  h.sess,
		Taiga:    h.taiga,
		State:    h.state,
		Warnings: []config.Warning{`Unknown key "colors" in config.toml`, "state.json is corrupt, using defaults"},
		Tick:     noTick,
	})

	h.start()

	h.golden("login_with_warnings")
}

func TestNarrowTerminalKeepsDialogInside(t *testing.T) {
	h := newHarness(t, &fakeSession{}, config.State{Login: "a-very-long-username@students.example.org"})
	h.send(tea.WindowSizeMsg{Width: 60, Height: 15})
	h.runCmd(h.app.Init())

	for i, line := range strings.Split(h.plainView(), "\n") {
		if w := ansi.StringWidth(line); w > 60 {
			t.Errorf("line %d is %d cells wide: %q", i, w, line)
		}
	}
	h.golden("login_60x15")
}

func TestLoginErrorText(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{taiga.ErrUnreachable, "no connection"},
		{fmt.Errorf("%w: x", taiga.ErrTimeout), "timed out"},
		{&taiga.ServerError{Code: 503}, "server error (503)"},
		{taiga.ErrUnexpected, "unexpected response from server"},
		{context.Canceled, "unexpected response from server"},
	}
	for _, tt := range tests {
		if got := reason(tt.err); got != tt.want {
			t.Errorf("reason(%v) = %q, want %q", tt.err, got, tt.want)
		}
	}
}

// harness drives an App the way Bubble Tea does, but synchronously: every
// command runs right away, and timers never fire.
type harness struct {
	t     *testing.T
	app   App
	sess  *fakeSession
	taiga *taigafake.Fake
	state *fakeState
	quit  bool
}

func newHarness(t *testing.T, sess *fakeSession, st config.State) *harness {
	t.Helper()
	state := &fakeState{}
	api := newFake()
	return &harness{
		t:     t,
		sess:  sess,
		taiga: api,
		state: state,
		app: New(Deps{
			Host:         "taiga.example.org",
			Session:      sess,
			Taiga:        api,
			State:        state,
			InitialState: st,
			Tick:         noTick,
		}),
	}
}

func noTick(time.Duration, func(time.Time) tea.Msg) tea.Cmd { return nil }

func (h *harness) start() {
	h.send(tea.WindowSizeMsg{Width: 80, Height: 24})
	h.runCmd(h.app.Init())
}

// send updates the app and runs the resulting commands to completion.
func (h *harness) send(msg tea.Msg) {
	h.runCmd(h.sendOnly(msg))
}

// sendOnly updates the app and returns the command without running it.
func (h *harness) sendOnly(msg tea.Msg) tea.Cmd {
	m, cmd := h.app.Update(msg)
	h.app = m.(App)
	return cmd
}

func (h *harness) runCmd(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			h.runCmd(c)
		}
	case tea.QuitMsg:
		h.quit = true
	default:
		h.send(msg)
	}
}

func (h *harness) key(k string) { h.send(keyMsg(k)) }

func (h *harness) typeText(s string) {
	for _, r := range s {
		h.send(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func keyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	case "ctrl+u":
		return tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	default:
		r := []rune(k)[0]
		return tea.KeyPressMsg{Code: r, Text: k}
	}
}

// words joins the words of a view with single spaces, dropping box borders,
// so that a check is independent of where the dialog wraps lines.
func words(view string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(view, "│", " ")), " ")
}

func (h *harness) plainView() string {
	return ansi.Strip(h.app.View().Content)
}

// golden compares the plain-text view with testdata/<name>.golden. Run
// go test ./internal/ui -update to rewrite the files. Trailing spaces are
// made visible with a "|" at each line end.
func (h *harness) golden(name string) {
	h.t.Helper()
	h.compareGolden(name+".golden", h.plainView())
}

// ansiGolden is golden with colors, downsampled to the fixed ANSI256
// profile so that the file does not depend on the terminal.
func (h *harness) ansiGolden(name string) {
	h.t.Helper()
	var buf strings.Builder
	w := &colorprofile.Writer{Forward: &buf, Profile: colorprofile.ANSI256}
	if _, err := w.WriteString(h.app.View().Content); err != nil {
		h.t.Fatal(err)
	}
	h.compareGolden(name+".ansi.golden", buf.String())
}

func (h *harness) compareGolden(file, view string) {
	h.t.Helper()
	lines := strings.Split(view, "\n")
	for i, l := range lines {
		lines[i] = l + "|"
	}
	got := strings.Join(lines, "\n") + "\n"
	path := filepath.Join("testdata", file)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			h.t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			h.t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		h.t.Fatalf("%v (run with -update to create it)", err)
	}
	if got != string(want) {
		h.t.Errorf("view differs from %s:\n--- got\n%s--- want\n%s", path, got, want)
	}
}

type fakeSession struct {
	resume     bool
	resumedFor string

	result          taiga.LoginResult
	authErr         error
	authCalls       int
	login, password string

	savedTo     session.Saved
	savedResult taiga.LoginResult
	saveErr     error
}

func (f *fakeSession) Resume(username string) bool {
	f.resumedFor = username
	return f.resume
}

func (f *fakeSession) Authenticate(_ context.Context, login, password string) (taiga.LoginResult, error) {
	f.authCalls++
	f.login, f.password = login, password
	return f.result, f.authErr
}

func (f *fakeSession) Save(res taiga.LoginResult) (session.Saved, error) {
	if f.saveErr != nil {
		return session.Saved{}, f.saveErr
	}
	f.savedResult = res
	return f.savedTo, nil
}

type fakeState struct {
	saved config.State
	saves int
	err   error
}

func (f *fakeState) Save(st config.State) error {
	f.saves++
	if f.err != nil {
		return f.err
	}
	f.saved = st
	return nil
}
