// Package version builds the one-line version string shared by --version,
// the debug.log header and crash.log.
package version

import (
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

// Stamp holds the values injected at link time via -ldflags.
type Stamp struct {
	Version string
	Commit  string
	Date    string
}

// Info describes a build.
type Info struct {
	Version   string
	Commit    string
	Date      string
	GoVersion string
	Platform  string
}

// Dev is the version reported when neither ldflags nor the module build
// information name one.
const Dev = "dev"

// Set at link time, for example
// -ldflags "-X github.com/zeckitoz/taiga-tui/internal/version.version=v0.3.0".
var (
	version string
	commit  string
	date    string
)

// Current returns the version information of the running binary.
func Current() Info {
	bi, _ := debug.ReadBuildInfo() // nil when unavailable
	return Resolve(Stamp{Version: version, Commit: commit, Date: date}, bi)
}

// Resolve picks the version information for a build: the ldflags stamp if it
// names a version, else the module build information (module version,
// vcs.revision, vcs.time), else Dev.
func Resolve(stamp Stamp, bi *debug.BuildInfo) Info {
	info := Info{
		Version:   Dev,
		GoVersion: runtime.Version(),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
	}
	switch {
	case stamp.Version != "":
		info.Version = stamp.Version
		info.Commit = shortCommit(stamp.Commit)
		info.Date = shortDate(stamp.Date)
	case bi != nil:
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			info.Version = v
		}
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				info.Commit = shortCommit(s.Value)
			case "vcs.time":
				info.Date = shortDate(s.Value)
			}
		}
	}
	return info
}

// String renders the version line, for example
// "taiga-tui v0.3.0 (commit 3aef22e, built 2026-11-11, go1.27.1, darwin/arm64)".
func (i Info) String() string {
	var details []string
	if i.Commit != "" {
		details = append(details, "commit "+i.Commit)
	}
	if i.Date != "" {
		details = append(details, "built "+i.Date)
	}
	details = append(details, i.GoVersion, i.Platform)
	return "taiga-tui " + i.Version + " (" + strings.Join(details, ", ") + ")"
}

func shortCommit(commit string) string {
	const n = 7
	if len(commit) > n {
		return commit[:n]
	}
	return commit
}

func shortDate(date string) string {
	if t, err := time.Parse(time.RFC3339, date); err == nil {
		return t.UTC().Format(time.DateOnly)
	}
	return date
}
