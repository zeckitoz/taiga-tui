package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/zeckitoz/taiga-tui/internal/config"
	"github.com/zeckitoz/taiga-tui/internal/domain"
)

func TestStateFileMissingYieldsDefaults(t *testing.T) {
	st, warnings := config.NewStateFile(t.TempDir()).Load()

	if st != (config.State{}) || len(warnings) != 0 {
		t.Errorf("got %+v, %v", st, warnings)
	}
}

func TestStateFileRoundTrip(t *testing.T) {
	f := config.NewStateFile(t.TempDir())
	want := config.State{UserID: 42, Username: "alice", Login: "alice@example.org", FullName: "Alice Example", DefaultProject: 7}

	if err := f.Save(want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, warnings := f.Load()

	if got != want || len(warnings) != 0 {
		t.Errorf("got %+v, %v\nwant %+v", got, warnings, want)
	}
}

func TestStateFileSaveCreatesPrivateDirAndLeavesNoTempFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new", "taiga-tui")
	f := config.NewStateFile(dir)

	if err := f.Save(config.State{UserID: 1}); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := f.Save(config.State{UserID: 2}); err != nil {
		t.Fatalf("second save: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if !slices.Equal(names, []string{"state.json"}) {
		t.Errorf("directory holds %v, want only state.json", names)
	}
	if got, _ := f.Load(); got.UserID != 2 {
		t.Errorf("second save did not replace the file: %+v", got)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Errorf("directory mode %o, want 700", perm)
		}
	}
}

func TestStateFileCorruptFallsBackToDefaultsWithWarning(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"user_id": `), 0o600); err != nil {
		t.Fatal(err)
	}

	st, warnings := config.NewStateFile(dir).Load()

	if st != (config.State{}) {
		t.Errorf("got %+v, want defaults", st)
	}
	want := []config.Warning{"state.json is corrupt, using defaults"}
	if !slices.Equal(warnings, want) {
		t.Errorf("got %q, want %q", warnings, want)
	}
}

func TestStateLoggedInKeepsStateOfSameUser(t *testing.T) {
	old := config.State{UserID: 42, Username: "alice", Login: "alice", FullName: "Alice", DefaultProject: 9}
	m := domain.TeamMember{ID: 42, Username: "alice", FullName: "Alice Example"}

	got := old.LoggedIn(m, "alice@example.org")

	want := config.State{UserID: 42, Username: "alice", Login: "alice@example.org", FullName: "Alice Example", DefaultProject: 9}
	if got != want {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestStateLoggedInDiscardsStateOfOtherUser(t *testing.T) {
	old := config.State{UserID: 42, Username: "alice", Login: "alice", FullName: "Alice", DefaultProject: 9}
	m := domain.TeamMember{ID: 7, Username: "bob", FullName: "Bob"}

	got := old.LoggedIn(m, "bob")

	want := config.State{UserID: 7, Username: "bob", Login: "bob", FullName: "Bob"}
	if got != want {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}
