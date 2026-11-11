package version_test

import (
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/zeckitoz/taiga-tui/internal/version"
)

const platform = runtime.GOOS + "/" + runtime.GOARCH

func TestResolvePrefersLdflags(t *testing.T) {
	stamp := version.Stamp{
		Version: "v0.3.0",
		Commit:  "3aef22e9b1c04d5e8f7a6b5c4d3e2f1a0b9c8d7e",
		Date:    "2026-11-11T09:30:00Z",
	}
	bi := buildInfo("v9.9.9", "ffffffffffffffffffffffffffffffffffffffff", "2020-01-01T00:00:00Z")

	got := version.Resolve(stamp, bi).String()

	want := "taiga-tui v0.3.0 (commit 3aef22e, built 2026-11-11, " + runtime.Version() + ", " + platform + ")"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestResolveFallsBackToBuildInfo(t *testing.T) {
	tests := []struct {
		name string
		bi   *debug.BuildInfo
		want string
	}{
		{
			name: "go install of a tagged version has no VCS settings",
			bi:   buildInfo("v0.3.0", "", ""),
			want: "taiga-tui v0.3.0 (" + runtime.Version() + ", " + platform + ")",
		},
		{
			name: "go build in a checkout stamps a pseudo-version and VCS settings",
			bi: buildInfo(
				"v0.2.1-0.20261111093000-3aef22e9b1c0",
				"3aef22e9b1c04d5e8f7a6b5c4d3e2f1a0b9c8d7e",
				"2026-11-11T09:30:00Z",
			),
			want: "taiga-tui v0.2.1-0.20261111093000-3aef22e9b1c0 (commit 3aef22e, built 2026-11-11, " + runtime.Version() + ", " + platform + ")",
		},
		{
			name: "go build without VCS reports (devel)",
			bi:   buildInfo("(devel)", "3aef22e9b1c04d5e8f7a6b5c4d3e2f1a0b9c8d7e", "2026-11-11T23:30:00+02:00"),
			want: "taiga-tui dev (commit 3aef22e, built 2026-11-11, " + runtime.Version() + ", " + platform + ")",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := version.Resolve(version.Stamp{}, tt.bi).String()
			if got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestResolveFallsBackToDev(t *testing.T) {
	got := version.Resolve(version.Stamp{}, nil).String()

	want := "taiga-tui dev (" + runtime.Version() + ", " + platform + ")"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCurrentProducesAVersionLine(t *testing.T) {
	got := version.Current().String()
	if !strings.HasPrefix(got, "taiga-tui ") || !strings.HasSuffix(got, platform+")") {
		t.Errorf("unexpected version line %q", got)
	}
}

func buildInfo(mainVersion, revision, vcsTime string) *debug.BuildInfo {
	bi := &debug.BuildInfo{Main: debug.Module{Path: "github.com/zeckitoz/taiga-tui", Version: mainVersion}}
	if revision != "" {
		bi.Settings = append(bi.Settings, debug.BuildSetting{Key: "vcs.revision", Value: revision})
	}
	if vcsTime != "" {
		bi.Settings = append(bi.Settings, debug.BuildSetting{Key: "vcs.time", Value: vcsTime})
	}
	return bi
}
