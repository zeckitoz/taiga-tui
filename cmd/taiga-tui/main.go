// Command taiga-tui is a keyboard-driven terminal UI for Taiga Taskboards and
// Kanban Boards.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/zeckitoz/taiga-tui/internal/config"
	"github.com/zeckitoz/taiga-tui/internal/session"
	"github.com/zeckitoz/taiga-tui/internal/taiga"
	"github.com/zeckitoz/taiga-tui/internal/ui"
	"github.com/zeckitoz/taiga-tui/internal/version"
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

// run executes the command and returns the process exit code: 0 on success,
// 1 on errors and 2 on invalid command line arguments.
func run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	cfg, warnings, err := config.Resolve(args, getenv)
	if _, ok := errors.AsType[*config.UsageError](err); ok {
		fmt.Fprintf(stderr, "taiga-tui: %v\nRun 'taiga-tui --help' for usage.\n", err)
		return 2
	}
	if err != nil {
		fmt.Fprintf(stderr, "taiga-tui: %v\n", err)
		return 1
	}

	switch {
	case cfg.ShowHelp:
		dir := cfg.ConfigDir
		if dir == "" {
			dir = "unknown, set --config-dir or " + config.EnvConfigDir
		}
		fmt.Fprintf(stdout, helpText, config.EnvInstance, config.DefaultInstanceURL, config.EnvConfigDir, dir)
		return 0
	case cfg.ShowVersion:
		fmt.Fprintln(stdout, version.Current())
		return 0
	}

	stateFile := config.NewStateFile(cfg.ConfigDir)
	state, stateWarnings := stateFile.Load()
	warnings = append(warnings, stateWarnings...)
	host := cfg.Host()
	sess := session.NewManager(session.Options{
		Host:    host,
		Auth:    taiga.NewAuthAPI(cfg.InstanceURL),
		Keyring: session.WithTimeout(session.NewKeyringStore(), session.KeyringTimeout),
		File:    session.NewFileStore(cfg.ConfigDir),
		Now:     time.Now,
	})

	if cfg.Logout {
		return logout(sess, host, state, warnings, stdout, stderr)
	}

	err = runTUI(ui.Deps{
		Host:         host,
		Session:      sess,
		Taiga:        taiga.NewClient(cfg.InstanceURL, sess),
		State:        stateFile,
		InitialState: state,
		Warnings:     warnings,
	})
	if err != nil {
		fmt.Fprintf(stderr, "taiga-tui: %v\n", err)
		return 1
	}
	return 0
}

// runTUI runs the terminal UI until the Team Member quits. Tests replace it.
var runTUI = func(d ui.Deps) error {
	_, err := tea.NewProgram(ui.New(d)).Run()
	return err
}

// logout deletes the stored tokens of the instance from the keyring and the
// credentials file. The Team Member's username comes from state.json, and
// config.toml and state.json stay as they are.
func logout(sess *session.Manager, host string, state config.State, warnings []config.Warning, stdout, stderr io.Writer) int {
	for _, w := range warnings {
		fmt.Fprintf(stderr, "Warning: %s\n", w)
	}
	if err := sess.Logout(state.Username); err != nil {
		fmt.Fprintf(stderr, "taiga-tui: logout: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Logged out of %s.\n", host)
	return 0
}

const helpText = `taiga-tui - keyboard-driven terminal UI for Taiga Taskboards and Kanban Boards

Usage:
  taiga-tui [flags]          Start the TUI
  taiga-tui logout [flags]   Delete the stored login of the instance

Flags:
  --instance URL     Taiga instance URL (env %s,
                     default %s)
  --config-dir DIR   Directory for all files of taiga-tui (env %s)
  --version          Print the version and exit
  -h, --help         Show this help and exit

Config directory: %s
`
