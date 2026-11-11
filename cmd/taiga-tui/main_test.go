package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
  taiga-tui [flags]

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

func TestStartShowsPlaceholderWithWarnings(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("colors = \"red\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := runWith(t, map[string]string{"TAIGA_TUI_CONFIG_DIR": dir})

	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	if want := "Warning: Unknown key \"colors\" in config.toml\n"; got.stderr != want {
		t.Errorf("stderr: got %q, want %q", got.stderr, want)
	}
	if !strings.Contains(got.stdout, "https://taiga-fki.ostfalia.de") {
		t.Errorf("placeholder should name the instance, got %q", got.stdout)
	}
}
