package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zeckitoz/taiga-tui/internal/config"
)

func TestResolveDefaultsWithoutConfigFile(t *testing.T) {
	dir := t.TempDir()
	env := envMap{"TAIGA_TUI_CONFIG_DIR": dir}

	cfg, warnings, err := config.Resolve(nil, env.Get)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	want := config.Config{
		InstanceURL:  "https://taiga-fki.ostfalia.de",
		ConfigDir:    dir,
		CheckUpdates: true,
	}
	if cfg != want {
		t.Errorf("got  %+v\nwant %+v", cfg, want)
	}
}

func TestResolveInstanceURLPrecedence(t *testing.T) {
	const (
		fromFlag = "https://flag.example.org"
		fromEnv  = "https://env.example.org"
		fromFile = "https://file.example.org"
	)
	tests := []struct {
		name string
		flag string
		env  string
		file string
		want string
	}{
		{name: "flag beats env and file", flag: fromFlag, env: fromEnv, file: fromFile, want: fromFlag},
		{name: "env beats file", env: fromEnv, file: fromFile, want: fromEnv},
		{name: "file beats default", file: fromFile, want: fromFile},
		{name: "default", want: "https://taiga-fki.ostfalia.de"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			env := envMap{"TAIGA_TUI_CONFIG_DIR": dir, "TAIGA_TUI_INSTANCE": tt.env}
			if tt.file != "" {
				writeConfig(t, dir, `instance_url = "`+tt.file+`"`)
			}
			var args []string
			if tt.flag != "" {
				args = []string{"--instance", tt.flag}
			}

			cfg, _, err := config.Resolve(args, env.Get)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.InstanceURL != tt.want {
				t.Errorf("got %q, want %q", cfg.InstanceURL, tt.want)
			}
		})
	}
}

func TestResolveConfigDirFlagBeatsEnv(t *testing.T) {
	flagDir, envDir := t.TempDir(), t.TempDir()
	writeConfig(t, flagDir, `instance_url = "https://flag-dir.example.org"`)
	writeConfig(t, envDir, `instance_url = "https://env-dir.example.org"`)
	env := envMap{"TAIGA_TUI_CONFIG_DIR": envDir}

	cfg, _, err := config.Resolve([]string{"--config-dir", flagDir}, env.Get)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ConfigDir != flagDir {
		t.Errorf("config dir: got %q, want %q", cfg.ConfigDir, flagDir)
	}
	if cfg.InstanceURL != "https://flag-dir.example.org" {
		t.Errorf("config.toml must be read from the flag dir, got instance %q", cfg.InstanceURL)
	}
}

func TestResolveConfigDirIsMadeAbsolute(t *testing.T) {
	t.Chdir(t.TempDir())
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	cfg, _, err := config.Resolve([]string{"--config-dir", "rel"}, envMap{}.Get)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := filepath.Join(wd, "rel"); cfg.ConfigDir != want {
		t.Errorf("got %q, want %q", cfg.ConfigDir, want)
	}
}

func TestDefaultConfigDir(t *testing.T) {
	home := "/home/alice"
	tests := []struct {
		name string
		goos string
		env  envMap
		want string
	}{
		{
			name: "XDG_CONFIG_HOME on Linux",
			goos: "linux",
			env:  envMap{"XDG_CONFIG_HOME": "/xdg", "HOME": home},
			want: filepath.Join("/", "xdg", "taiga-tui"),
		},
		{
			name: "XDG_CONFIG_HOME on macOS",
			goos: "darwin",
			env:  envMap{"XDG_CONFIG_HOME": "/xdg", "HOME": home},
			want: filepath.Join("/", "xdg", "taiga-tui"),
		},
		{
			name: "~/.config on macOS, not Library/Application Support",
			goos: "darwin",
			env:  envMap{"HOME": home},
			want: filepath.Join(home, ".config", "taiga-tui"),
		},
		{
			name: "relative XDG_CONFIG_HOME is ignored as the XDG spec demands",
			goos: "linux",
			env:  envMap{"XDG_CONFIG_HOME": "xdg", "HOME": home},
			want: filepath.Join(home, ".config", "taiga-tui"),
		},
		{
			name: "AppData on Windows, ignoring XDG_CONFIG_HOME",
			goos: "windows",
			env:  envMap{"AppData": `C:\Users\alice\AppData\Roaming`, "XDG_CONFIG_HOME": "/xdg"},
			want: filepath.Join(`C:\Users\alice\AppData\Roaming`, "taiga-tui"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.DefaultConfigDir(tt.goos, tt.env.Get)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultConfigDirFailsWithoutHome(t *testing.T) {
	for _, goos := range []string{"linux", "darwin", "windows"} {
		if _, err := config.DefaultConfigDir(goos, envMap{}.Get); err == nil {
			t.Errorf("%s: expected an error without HOME or AppData", goos)
		}
	}
}

func TestResolveUsesDefaultConfigDir(t *testing.T) {
	root := t.TempDir()
	env := envMap{"HOME": root, "XDG_CONFIG_HOME": root, "AppData": root}

	cfg, _, err := config.Resolve(nil, env.Get)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := filepath.Join(root, "taiga-tui"); cfg.ConfigDir != want {
		t.Errorf("got %q, want %q", cfg.ConfigDir, want)
	}
}

func TestResolveReadsCheckUpdates(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "check_updates = false\n")

	cfg, _, err := config.Resolve([]string{"--config-dir", dir}, envMap{}.Get)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CheckUpdates {
		t.Error("check_updates = false was ignored")
	}
}

func TestResolveWarnsAboutUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
instance_url = "https://file.example.org"
check_update = false

[theme]
color = "red"
`)

	cfg, warnings, err := config.Resolve([]string{"--config-dir", dir}, envMap{}.Get)

	if err != nil {
		t.Fatalf("unknown keys must not be an error: %v", err)
	}
	if cfg.InstanceURL != "https://file.example.org" || !cfg.CheckUpdates {
		t.Errorf("known keys must still apply, got %+v", cfg)
	}
	want := []config.Warning{
		`Unknown key "check_update" in config.toml`,
		`Unknown key "theme" in config.toml`,
	}
	if !slices.Equal(warnings, want) {
		t.Errorf("got  %q\nwant %q", warnings, want)
	}
}

func TestResolveRejectsMalformedConfigFile(t *testing.T) {
	tests := map[string]string{
		"syntax error": "instance_url = ",
		"wrong type":   "check_updates = \"no\"",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeConfig(t, dir, content)

			_, _, err := config.Resolve([]string{"--config-dir", dir}, envMap{}.Get)

			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), filepath.Join(dir, config.FileName)) {
				t.Errorf("error should name the file, got %q", err)
			}
		})
	}
}

func TestResolveHelpAndVersionSkipConfigFile(t *testing.T) {
	for _, arg := range []string{"--help", "-h", "--version"} {
		t.Run(arg, func(t *testing.T) {
			dir := t.TempDir()
			writeConfig(t, dir, "this is not toml")

			cfg, _, err := config.Resolve([]string{arg, "--config-dir", dir}, envMap{}.Get)

			if err != nil {
				t.Fatalf("a broken config.toml must not block %s: %v", arg, err)
			}
			if cfg.ConfigDir != dir {
				t.Errorf("config dir: got %q, want %q", cfg.ConfigDir, dir)
			}
			if cfg.ShowHelp != (arg != "--version") || cfg.ShowVersion != (arg == "--version") {
				t.Errorf("%s reported wrongly: %+v", arg, cfg)
			}
		})
	}
}

func TestResolveHelpAndVersionWorkWithoutConfigDir(t *testing.T) {
	for _, arg := range []string{"--help", "--version"} {
		t.Run(arg, func(t *testing.T) {
			cfg, _, err := config.Resolve([]string{arg}, envMap{}.Get)

			if err != nil {
				t.Fatalf("%s must not need HOME or AppData: %v", arg, err)
			}
			if cfg.ConfigDir != "" {
				t.Errorf("config dir should be unknown, got %q", cfg.ConfigDir)
			}
		})
	}
}

func TestResolveRejectsBadArguments(t *testing.T) {
	tests := map[string][]string{
		"unknown flag":        {"--nope"},
		"missing flag value":  {"--instance"},
		"positional argument": {"board"},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			env := envMap{"TAIGA_TUI_CONFIG_DIR": t.TempDir()}
			_, _, err := config.Resolve(args, env.Get)
			if _, ok := errors.AsType[*config.UsageError](err); !ok {
				t.Fatalf("expected a UsageError, got %v", err)
			}
		})
	}
}

func TestResolveNormalizesInstanceURL(t *testing.T) {
	env := envMap{"TAIGA_TUI_CONFIG_DIR": t.TempDir()}

	cfg, _, err := config.Resolve([]string{"--instance", "https://taiga.example.org/"}, env.Get)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.InstanceURL != "https://taiga.example.org" {
		t.Errorf("got %q", cfg.InstanceURL)
	}
}

func TestResolveRejectsInvalidInstanceURL(t *testing.T) {
	tests := map[string]string{
		"no scheme":    "taiga.example.org",
		"wrong scheme": "ftp://taiga.example.org",
		"no host":      "https://",
		"unparsable":   "https://exa mple.org",
	}
	for name, url := range tests {
		t.Run(name, func(t *testing.T) {
			env := envMap{"TAIGA_TUI_CONFIG_DIR": t.TempDir(), "TAIGA_TUI_INSTANCE": url}

			_, _, err := config.Resolve(nil, env.Get)

			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), "TAIGA_TUI_INSTANCE") {
				t.Errorf("error should name the source, got %q", err)
			}
		})
	}
}

type envMap map[string]string

func (e envMap) Get(key string) string { return e[key] }

func writeConfig(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, config.FileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
