// Command taiga-tui is a keyboard-driven terminal UI for Taiga Taskboards and
// Kanban Boards.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/zeckitoz/taiga-tui/internal/config"
	"github.com/zeckitoz/taiga-tui/internal/crash"
	"github.com/zeckitoz/taiga-tui/internal/debuglog"
	"github.com/zeckitoz/taiga-tui/internal/session"
	"github.com/zeckitoz/taiga-tui/internal/taiga"
	"github.com/zeckitoz/taiga-tui/internal/ui"
	"github.com/zeckitoz/taiga-tui/internal/update"
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

	var opts []taiga.Option
	var debugLog string
	if cfg.Debug {
		f, path, err := openDebugLog(cfg.ConfigDir)
		if err != nil {
			fmt.Fprintf(stderr, "taiga-tui: %v\n", err)
			return 1
		}
		defer f.Close() //nolint:errcheck // nothing sensible to do about a failed close
		opts = append(opts, taiga.WithTransport(debuglog.NewTransport(http.DefaultTransport, f)))
		debugLog = path
	}

	stateFile := config.NewStateFile(cfg.ConfigDir)
	state, stateWarnings := stateFile.Load()
	warnings = append(warnings, stateWarnings...)
	host := cfg.Host()
	sess := session.NewManager(session.Options{
		Host:    host,
		Auth:    taiga.NewAuthAPI(cfg.InstanceURL, opts...),
		Keyring: session.WithTimeout(session.NewKeyringStore(), session.KeyringTimeout),
		File:    session.NewFileStore(cfg.ConfigDir),
		Now:     time.Now,
	})

	if cfg.Logout {
		return logout(sess, host, state, warnings, stdout, stderr)
	}

	var checkUpdate func(context.Context) (string, error)
	if cfg.CheckUpdates {
		checkUpdate = (&update.Checker{}).Latest
	}

	err = crash.Capture(func() error {
		return runTUI(ui.Deps{
			Host:         host,
			Session:      sess,
			Taiga:        taiga.NewClient(cfg.InstanceURL, sess, opts...),
			State:        stateFile,
			InitialState: state,
			Warnings:     warnings,
			DebugLog:     debugLog,
			Version:      version.Current().Version,
			CheckUpdate:  checkUpdate,
		})
	})
	if c, ok := errors.AsType[*crash.Error](err); ok {
		return reportCrash(c, cfg.ConfigDir, stderr)
	}
	if err != nil {
		fmt.Fprintf(stderr, "taiga-tui: %v\n", err)
		return 1
	}
	return 0
}

// reportCrash writes crash.log to dir and tells the Team Member where it is.
// If the file cannot be written, the report goes to stderr instead.
func reportCrash(c *crash.Error, dir string, stderr io.Writer) int {
	path := filepath.Join(dir, crashLogName)
	if err := crash.WriteLog(path, version.Current().String(), c); err != nil {
		fmt.Fprintf(stderr, "taiga-tui crashed. Could not write %s: %v\n%v\n\n%s", path, err, c.Value, c.Stack)
		return 1
	}
	fmt.Fprintf(stderr, "taiga-tui crashed. Details written to %s\n", path)
	return 1
}

const crashLogName = "crash.log"

// openDebugLog creates debug.log in dir, replacing the log of an earlier run,
// and writes the version line as its header.
func openDebugLog(dir string) (*os.File, string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, "", fmt.Errorf("debug log: %w", err)
	}
	path := filepath.Join(dir, debugLogName)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, "", fmt.Errorf("debug log: %w", err)
	}
	if _, err := fmt.Fprintln(f, version.Current()); err != nil {
		_ = f.Close()
		return nil, "", fmt.Errorf("debug log: %w", err)
	}
	return f, path, nil
}

const debugLogName = "debug.log"

// runTUI runs the terminal UI until the Team Member quits. Tests replace it.
var runTUI = func(d ui.Deps) error {
	return crash.Run(ui.New(d))
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
  --debug            Write requests and responses, without tokens and
                     password, to debug.log in the config directory
  --version          Print the version and exit
  -h, --help         Show this help and exit

Config directory: %s
`
