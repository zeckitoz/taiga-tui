package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

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
	if want := []string{`Unknown key "colors" in config.toml`}; !slices.Equal(got.Warnings, want) {
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

	if want := []string{"state.json is corrupt, using defaults"}; !slices.Equal(got.Warnings, want) {
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

func TestLogoutWithoutStoredLogin(t *testing.T) {
	keyring.MockInit()

	res := runWith(t, map[string]string{"TAIGA_TUI_CONFIG_DIR": t.TempDir()}, "logout")

	if res.code != 0 || res.stdout != "Not logged in to taiga-fki.ostfalia.de.\n" {
		t.Errorf("exit %d, stdout %q, stderr %q", res.code, res.stdout, res.stderr)
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
