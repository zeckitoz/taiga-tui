// Command taiga-tui is a keyboard-driven terminal UI for Taiga Taskboards and
// Kanban Boards.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/zeckitoz/taiga-tui/internal/config"
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

	// The status line takes over these warnings once the UI exists.
	for _, w := range warnings {
		fmt.Fprintf(stderr, "Warning: %s\n", w)
	}
	// Placeholder until the UI arrives.
	fmt.Fprintf(stdout, "taiga-tui is not usable yet.\nInstance:         %s\nConfig directory: %s\n", cfg.InstanceURL, cfg.ConfigDir)
	return 0
}

const helpText = `taiga-tui - keyboard-driven terminal UI for Taiga Taskboards and Kanban Boards

Usage:
  taiga-tui [flags]

Flags:
  --instance URL     Taiga instance URL (env %s,
                     default %s)
  --config-dir DIR   Directory for all files of taiga-tui (env %s)
  --version          Print the version and exit
  -h, --help         Show this help and exit

Config directory: %s
`
