package main

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/zeckitoz/taiga-tui/internal/config"
	"github.com/zeckitoz/taiga-tui/internal/crash"
	"github.com/zeckitoz/taiga-tui/internal/ui"
	"github.com/zeckitoz/taiga-tui/internal/version"
)

type result struct {
	code           int
	stdout, stderr string
}

func runWith(t *testing.T, env map[string]string, args ...string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(args, func(k string) string { return env[k] }, &stdout, &stderr)
	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func TestVersionPrintsOneLine(t *testing.T) {
	got := runWith(t, map[string]string{"TAIGA_TUI_CONFIG_DIR": t.TempDir()}, "--version")

	if got.code != 0 || got.stderr != "" {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	if want := version.Current().String() + "\n"; got.stdout != want {
		t.Errorf("got  %q\nwant %q", got.stdout, want)
	}
}

func TestHelpListsFlagsAndConfigDir(t *testing.T) {
	dir := t.TempDir()

	got := runWith(t, map[string]string{"TAIGA_TUI_CONFIG_DIR": dir}, "--help")

	if got.code != 0 || got.stderr != "" {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	want := `taiga-tui - keyboard-driven terminal UI for Taiga Taskboards and Kanban Boards

Usage:
  taiga-tui [flags]          Start the TUI
  taiga-tui logout [flags]   Delete the stored login of the instance

Flags:
  --instance URL     Taiga instance URL (env TAIGA_TUI_INSTANCE,
                     default https://taiga-fki.ostfalia.de)
  --config-dir DIR   Directory for all files of taiga-tui (env TAIGA_TUI_CONFIG_DIR)
  --debug            Write requests and responses, without tokens and
                     password, to debug.log in the config directory
  --version          Print the version and exit
  -h, --help         Show this help and exit

Config directory: ` + dir + "\n"
	if got.stdout != want {
		t.Errorf("got:\n%s\nwant:\n%s", got.stdout, want)
	}
}

func TestHelpWithoutConfigDir(t *testing.T) {
	got := runWith(t, nil, "--help")

	if got.code != 0 || got.stderr != "" {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	want := "Config directory: unknown, set --config-dir or TAIGA_TUI_CONFIG_DIR\n"
	if !strings.HasSuffix(got.stdout, want) {
		t.Errorf("got:\n%s\nwant suffix %q", got.stdout, want)
	}
}

func TestBadArgumentsExitWithUsageError(t *testing.T) {
	got := runWith(t, map[string]string{"TAIGA_TUI_CONFIG_DIR": t.TempDir()}, "--nope")

	if got.code != 2 {
		t.Errorf("exit code %d, want 2", got.code)
	}
	want := "taiga-tui: flag provided but not defined: -nope\nRun 'taiga-tui --help' for usage.\n"
	if got.stderr != want {
		t.Errorf("got  %q\nwant %q", got.stderr, want)
	}
}

func TestBrokenConfigFileExitsWithError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("instance_url ="), 0o600); err != nil {
		t.Fatal(err)
	}

	got := runWith(t, map[string]string{"TAIGA_TUI_CONFIG_DIR": dir})

	if got.code != 1 {
		t.Errorf("exit code %d, want 1", got.code)
	}
	if !strings.HasPrefix(got.stderr, "taiga-tui: parse "+filepath.Join(dir, "config.toml")) {
		t.Errorf("unexpected stderr %q", got.stderr)
	}
}

func TestStartRunsTUIWithWarningsAndState(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "config.toml", "colors = \"red\"\n")
	writeFile(t, dir, "state.json", `{"user_id": 42, "username": "alice", "login": "alice@example.org"}`)
	var got ui.Deps
	stubTUI(t, func(d ui.Deps) error { got = d; return nil })

	res := runWith(t, map[string]string{"TAIGA_TUI_CONFIG_DIR": dir})

	if res.code != 0 || res.stdout != "" || res.stderr != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q", res.code, res.stdout, res.stderr)
	}
	if got.Host != "taiga-fki.ostfalia.de" {
		t.Errorf("host %q", got.Host)
	}
	if got.Session == nil || got.Taiga == nil {
		t.Error("session or Taiga client not wired")
	}
	if want := []config.Warning{`Unknown key "colors" in config.toml`}; !slices.Equal(got.Warnings, want) {
		t.Errorf("warnings %q, want %q", got.Warnings, want)
	}
	if got.InitialState.Login != "alice@example.org" {
		t.Errorf("state not loaded: %+v", got.InitialState)
	}
}

func TestStartPassesCorruptStateWarning(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "state.json", "{")
	var got ui.Deps
	stubTUI(t, func(d ui.Deps) error { got = d; return nil })

	runWith(t, map[string]string{"TAIGA_TUI_CONFIG_DIR": dir})

	if want := []config.Warning{"state.json is corrupt, using defaults"}; !slices.Equal(got.Warnings, want) {
		t.Errorf("warnings %q, want %q", got.Warnings, want)
	}
}

func TestTUIFailureExitsWithError(t *testing.T) {
	stubTUI(t, func(ui.Deps) error { return errors.New("no tty") })

	res := runWith(t, map[string]string{"TAIGA_TUI_CONFIG_DIR": t.TempDir()})

	if res.code != 1 || res.stderr != "taiga-tui: no tty\n" {
		t.Errorf("exit %d, stderr %q", res.code, res.stderr)
	}
}

func TestLogoutDeletesTokensOnlyAndDoesNotStartTUI(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	host := "taiga.example.org"
	writeFile(t, dir, "config.toml", "instance_url = \"https://"+host+"\"\n")
	state := `{"user_id": 42, "username": "alice", "login": "alice"}`
	writeFile(t, dir, "state.json", state)
	for _, kind := range []string{"access", "refresh"} {
		if err := keyring.Set("taiga-tui", host+"/alice/"+kind, "tok"); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, dir, "credentials", `{"`+host+`/alice/refresh": "tok"}`)
	stubTUI(t, func(ui.Deps) error { t.Error("logout started the TUI"); return nil })

	res := runWith(t, map[string]string{"TAIGA_TUI_CONFIG_DIR": dir}, "logout")

	if res.code != 0 || res.stderr != "" {
		t.Fatalf("exit %d, stderr %q", res.code, res.stderr)
	}
	if want := "Logged out of " + host + ".\n"; res.stdout != want {
		t.Errorf("stdout %q, want %q", res.stdout, want)
	}
	for _, kind := range []string{"access", "refresh"} {
		if _, err := keyring.Get("taiga-tui", host+"/alice/"+kind); !errors.Is(err, keyring.ErrNotFound) {
			t.Errorf("%s token still in keyring: %v", kind, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "credentials")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("credentials file still there: %v", err)
	}
	for name, want := range map[string]string{"config.toml": "instance_url = \"https://" + host + "\"\n", "state.json": state} {
		if got, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(got) != want {
			t.Errorf("%s changed: %q, %v", name, got, err)
		}
	}
}

func TestLogoutWithoutStateStillClearsCredentialsFile(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	writeFile(t, dir, "credentials", `{"taiga-fki.ostfalia.de/alice/refresh": "tok"}`)

	res := runWith(t, map[string]string{"TAIGA_TUI_CONFIG_DIR": dir}, "logout")

	if res.code != 0 || res.stdout != "Logged out of taiga-fki.ostfalia.de.\n" {
		t.Errorf("exit %d, stdout %q, stderr %q", res.code, res.stdout, res.stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "credentials")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("credentials file still there: %v", err)
	}
}

func TestLogoutReportsKeyringFailure(t *testing.T) {
	keyring.MockInitWithError(errors.New("secret service not running"))
	dir := t.TempDir()
	writeFile(t, dir, "state.json", `{"user_id": 42, "username": "alice"}`)

	res := runWith(t, map[string]string{"TAIGA_TUI_CONFIG_DIR": dir}, "logout")

	if res.code != 1 || !strings.Contains(res.stderr, "secret service not running") {
		t.Errorf("exit %d, stderr %q", res.code, res.stderr)
	}
}

func stubTUI(t *testing.T, f func(ui.Deps) error) {
	t.Helper()
	orig := runTUI
	runTUI = f
	t.Cleanup(func() { runTUI = orig })
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDebugWritesRedactedTrafficToDebugLog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"id": 42, "username": "alice", "full_name_display": "Alice", "auth_token": "access-secret", "refresh": "refresh-secret"}`)
	}))
	defer srv.Close()
	dir := t.TempDir()
	var deps ui.Deps
	stubTUI(t, func(d ui.Deps) error {
		deps = d
		_, err := d.Session.Authenticate(t.Context(), "alice", "pw-secret")
		return err
	})

	res := runWith(t, map[string]string{"TAIGA_TUI_CONFIG_DIR": dir, "TAIGA_TUI_INSTANCE": srv.URL}, "--debug")
	if res.code != 0 || res.stderr != "" {
		t.Fatalf("exit %d, stderr %q", res.code, res.stderr)
	}

	logPath := filepath.Join(dir, "debug.log")
	if deps.DebugLog != logPath {
		t.Errorf("Deps.DebugLog %q, want %q", deps.DebugLog, logPath)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if first, _, _ := strings.Cut(got, "\n"); first != version.Current().String() {
		t.Errorf("first line %q, want the version line", first)
	}
	if !strings.Contains(got, "POST "+srv.URL+"/api/v1/auth") {
		t.Errorf("request not logged:\n%s", got)
	}
	for _, secret := range []string{"pw-secret", "access-secret", "refresh-secret"} {
		if strings.Contains(got, secret) {
			t.Errorf("debug.log contains %q:\n%s", secret, got)
		}
	}
}

func TestWithoutDebugNothingIsLogged(t *testing.T) {
	dir := t.TempDir()
	var deps ui.Deps
	stubTUI(t, func(d ui.Deps) error { deps = d; return nil })

	runWith(t, map[string]string{"TAIGA_TUI_CONFIG_DIR": dir})

	if deps.DebugLog != "" {
		t.Errorf("Deps.DebugLog %q", deps.DebugLog)
	}
	if _, err := os.Stat(filepath.Join(dir, "debug.log")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("debug.log exists: %v", err)
	}
}

func TestDebugLogThatCannotBeOpenedExitsWithError(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "debug.log"), 0o700); err != nil {
		t.Fatal(err)
	}
	stubTUI(t, func(ui.Deps) error { t.Error("TUI started"); return nil })

	res := runWith(t, map[string]string{"TAIGA_TUI_CONFIG_DIR": dir}, "--debug")

	if res.code != 1 || !strings.Contains(res.stderr, "debug.log") {
		t.Errorf("exit %d, stderr %q", res.code, res.stderr)
	}
}

const fakeJWT = "eyJhbGciOiJIUzI1NiJ9.eyJ1c2VyIjo0Mn0.c2lnbmF0dXJl"

func TestPanicWritesCrashLogAndExitsWithOne(t *testing.T) {
	dir := t.TempDir()
	stubTUI(t, func(ui.Deps) error { panic("kaboom with token " + fakeJWT) })

	res := runWith(t, map[string]string{"TAIGA_TUI_CONFIG_DIR": dir})

	logPath := filepath.Join(dir, "crash.log")
	if res.code != 1 {
		t.Errorf("exit code %d, want 1", res.code)
	}
	if want := "taiga-tui crashed. Details written to " + logPath + "\n"; res.stderr != want {
		t.Errorf("stderr %q, want %q", res.stderr, want)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if first, _, _ := strings.Cut(got, "\n"); first != version.Current().String() {
		t.Errorf("first line %q, want the version line", first)
	}
	for _, want := range []string{"OS: " + runtime.GOOS + "/" + runtime.GOARCH, "kaboom with token", "goroutine ", "TestPanicWritesCrashLogAndExitsWithOne"} {
		if !strings.Contains(got, want) {
			t.Errorf("crash.log lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, fakeJWT) {
		t.Errorf("crash.log contains a token:\n%s", got)
	}
}

func TestCrashReportedByTUIWritesCrashLog(t *testing.T) {
	dir := t.TempDir()
	stubTUI(t, func(ui.Deps) error {
		return &crash.Error{Value: "from a command", Stack: []byte("goroutine 7 [running]:\nmain.work()\n")}
	})

	res := runWith(t, map[string]string{"TAIGA_TUI_CONFIG_DIR": dir})

	if res.code != 1 || !strings.HasPrefix(res.stderr, "taiga-tui crashed. Details written to ") {
		t.Errorf("exit %d, stderr %q", res.code, res.stderr)
	}
	data, err := os.ReadFile(filepath.Join(dir, "crash.log"))
	if err != nil || !strings.Contains(string(data), "from a command") || !strings.Contains(string(data), "main.work()") {
		t.Errorf("crash.log %q, %v", data, err)
	}
}

func TestCrashLogIsOverwritten(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"TAIGA_TUI_CONFIG_DIR": dir}
	stubTUI(t, func(ui.Deps) error { panic("first crash") })
	runWith(t, env)
	stubTUI(t, func(ui.Deps) error { panic("second crash") })

	runWith(t, env)

	data, err := os.ReadFile(filepath.Join(dir, "crash.log"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); strings.Contains(got, "first crash") || !strings.Contains(got, "second crash") {
		t.Errorf("crash.log:\n%s", got)
	}
}

func TestCrashLogThatCannotBeWrittenStillReportsThePanic(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "crash.log"), 0o700); err != nil {
		t.Fatal(err)
	}
	stubTUI(t, func(ui.Deps) error { panic("kaboom") })

	res := runWith(t, map[string]string{"TAIGA_TUI_CONFIG_DIR": dir})

	if res.code != 1 || !strings.Contains(res.stderr, "taiga-tui crashed") || !strings.Contains(res.stderr, "kaboom") {
		t.Errorf("exit %d, stderr %q", res.code, res.stderr)
	}
}
