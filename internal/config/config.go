// Package config resolves the user configuration from flags, environment
// variables and config.toml, with precedence flag > environment > file > default.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// DefaultInstanceURL is the Taiga instance used when nothing else is configured.
const DefaultInstanceURL = "https://taiga-fki.ostfalia.de"

// Environment variables read by Resolve.
const (
	EnvInstance  = "TAIGA_TUI_INSTANCE"
	EnvConfigDir = "TAIGA_TUI_CONFIG_DIR"
)

// FileName is the name of the user-owned config file inside the config directory.
const FileName = "config.toml"

const appName = "taiga-tui"

// Config is the resolved configuration.
type Config struct {
	// InstanceURL is the base URL of the Taiga instance, without trailing slash.
	InstanceURL string
	// ConfigDir is the absolute directory holding all files of the app.
	ConfigDir string
	// CheckUpdates enables the daily update check.
	CheckUpdates bool
	// ShowHelp is set by --help or -h.
	ShowHelp bool
	// ShowVersion is set by --version.
	ShowVersion bool
}

// Warning is a non-fatal configuration problem meant for the status line.
type Warning string

// Resolve resolves the configuration from the command line arguments (without
// the program name) and the environment, read through getenv.
//
// A missing config.toml is fine, and unknown keys in it become warnings.
// With --help or --version, config.toml is not read, so a broken file or
// environment never hides the help or the version. Only ConfigDir is resolved
// then, and it is empty if it cannot be determined.
func Resolve(args []string, getenv func(string) string) (Config, []Warning, error) {
	fl, err := parseFlags(args)
	if err != nil {
		return Config{}, nil, err
	}
	dir, err := configDir(fl.configDir, getenv)
	cfg := Config{
		InstanceURL:  DefaultInstanceURL,
		ConfigDir:    dir,
		CheckUpdates: true,
		ShowHelp:     fl.help,
		ShowVersion:  fl.version,
	}
	if cfg.ShowHelp || cfg.ShowVersion {
		return cfg, nil, nil
	}
	if err != nil {
		return Config{}, nil, err
	}

	f, warnings, err := readFile(filepath.Join(dir, FileName))
	if err != nil {
		return Config{}, nil, err
	}
	if f.CheckUpdates != nil {
		cfg.CheckUpdates = *f.CheckUpdates
	}
	instance, source := DefaultInstanceURL, "default"
	switch {
	case fl.instance != "":
		instance, source = fl.instance, "--instance"
	case getenv(EnvInstance) != "":
		instance, source = getenv(EnvInstance), EnvInstance
	case f.InstanceURL != nil:
		instance, source = *f.InstanceURL, "instance_url in "+FileName
	}
	cfg.InstanceURL, err = normalizeInstanceURL(instance)
	if err != nil {
		return Config{}, nil, fmt.Errorf("invalid instance URL from %s: %w", source, err)
	}
	return cfg, warnings, nil
}

// UsageError reports invalid command line arguments.
type UsageError struct{ err error }

func (e *UsageError) Error() string { return e.err.Error() }

func (e *UsageError) Unwrap() error { return e.err }

type flags struct {
	instance  string
	configDir string
	help      bool
	version   bool
}

func parseFlags(args []string) (flags, error) {
	var fl flags
	fs := flag.NewFlagSet(appName, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&fl.instance, "instance", "", "")
	fs.StringVar(&fl.configDir, "config-dir", "", "")
	fs.BoolVar(&fl.help, "help", false, "")
	fs.BoolVar(&fl.help, "h", false, "")
	fs.BoolVar(&fl.version, "version", false, "")
	if err := fs.Parse(args); err != nil {
		return flags{}, &UsageError{err: err}
	}
	if fs.NArg() > 0 {
		return flags{}, &UsageError{err: fmt.Errorf("unexpected argument %q", fs.Arg(0))}
	}
	return fl, nil
}

// configDir resolves the config directory: flag, then environment variable,
// then the platform default. The result is absolute.
func configDir(fromFlag string, getenv func(string) string) (string, error) {
	dir := fromFlag
	if dir == "" {
		dir = getenv(EnvConfigDir)
	}
	if dir == "" {
		return DefaultConfigDir(runtime.GOOS, getenv)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve config directory %q: %w", dir, err)
	}
	return abs, nil
}

// DefaultConfigDir returns the config directory used when neither --config-dir
// nor TAIGA_TUI_CONFIG_DIR is set: %AppData%\taiga-tui on Windows, and
// $XDG_CONFIG_HOME/taiga-tui, else ~/.config/taiga-tui, on every Unix including
// macOS.
func DefaultConfigDir(goos string, getenv func(string) string) (string, error) {
	if goos == "windows" {
		appData := getenv("AppData")
		if appData == "" {
			return "", errors.New("cannot determine the config directory: %AppData% is not set, use --config-dir")
		}
		return filepath.Join(appData, appName), nil
	}
	// The XDG Base Directory spec says relative paths are invalid and must be ignored.
	if xdg := getenv("XDG_CONFIG_HOME"); path.IsAbs(xdg) {
		return filepath.Join(xdg, appName), nil
	}
	home := getenv("HOME")
	if home == "" {
		return "", errors.New("cannot determine the config directory: $HOME is not set, use --config-dir")
	}
	return filepath.Join(home, ".config", appName), nil
}

func normalizeInstanceURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("%q must start with https:// or http://", raw)
	}
	if u.Host == "" {
		return "", fmt.Errorf("%q has no host", raw)
	}
	return strings.TrimRight(u.String(), "/"), nil
}

// file mirrors config.toml. Pointers tell a missing key from a zero value.
type file struct {
	InstanceURL  *string `toml:"instance_url"`
	CheckUpdates *bool   `toml:"check_updates"`
}

// readFile decodes config.toml. A missing file yields an empty result.
func readFile(name string) (file, []Warning, error) {
	var f file
	data, err := os.ReadFile(name)
	if errors.Is(err, fs.ErrNotExist) {
		return f, nil, nil
	}
	if err != nil {
		return f, nil, fmt.Errorf("read %s: %w", name, err)
	}
	md, err := toml.Decode(string(data), &f)
	if err != nil {
		return f, nil, fmt.Errorf("parse %s: %w", name, err)
	}
	return f, unknownKeys(md.Undecoded()), nil
}

// unknownKeys turns undecoded keys into warnings. Keys inside an unknown table
// are not reported separately.
func unknownKeys(keys []toml.Key) []Warning {
	var warnings []Warning
	var reported []toml.Key
	for _, key := range keys {
		if slices.ContainsFunc(reported, func(parent toml.Key) bool { return isPrefix(parent, key) }) {
			continue
		}
		reported = append(reported, key)
		warnings = append(warnings, Warning(fmt.Sprintf("Unknown key %q in %s", key.String(), FileName)))
	}
	return warnings
}

func isPrefix(prefix, key toml.Key) bool {
	return len(prefix) <= len(key) && slices.Equal(prefix, key[:len(prefix)])
}
