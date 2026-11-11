package config_test

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"testing"

	"github.com/zeckitoz/taiga-tui/internal/config"
	"github.com/zeckitoz/taiga-tui/internal/domain"
)

func TestStateFileMissingYieldsDefaults(t *testing.T) {
	st, warnings := config.NewStateFile(t.TempDir()).Load()

	if !reflect.DeepEqual(st, config.State{}) || len(warnings) != 0 {
		t.Errorf("got %+v, %v", st, warnings)
	}
}

func TestStateFileRoundTrip(t *testing.T) {
	f := config.NewStateFile(t.TempDir())
	want := config.State{
		UserID: 42, Username: "alice", Login: "alice@example.org", FullName: "Alice Example", DefaultProject: 7,
		LastBoard: config.BoardTaskboard, Sprints: map[int64]int64{7: 11, 8: 20},
	}

	if err := f.Save(want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, warnings := f.Load()

	if !reflect.DeepEqual(got, want) || len(warnings) != 0 {
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

	if !reflect.DeepEqual(st, config.State{}) {
		t.Errorf("got %+v, want defaults", st)
	}
	want := []config.Warning{"state.json is corrupt, using defaults"}
	if !slices.Equal(warnings, want) {
		t.Errorf("got %q, want %q", warnings, want)
	}
}

func TestStateLoggedInKeepsStateOfSameUser(t *testing.T) {
	old := config.State{UserID: 42, Username: "alice", Login: "alice", FullName: "Alice", DefaultProject: 9, Sprints: map[int64]int64{9: 3}}
	m := domain.TeamMember{ID: 42, Username: "alice", FullName: "Alice Example"}

	got := old.LoggedIn(m, "alice@example.org")

	want := config.State{UserID: 42, Username: "alice", Login: "alice@example.org", FullName: "Alice Example", DefaultProject: 9, Sprints: map[int64]int64{9: 3}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestStateLoggedInDiscardsStateOfOtherUser(t *testing.T) {
	old := config.State{UserID: 42, Username: "alice", Login: "alice", FullName: "Alice", DefaultProject: 9, LastBoard: config.BoardKanban, Sprints: map[int64]int64{9: 3}}
	m := domain.TeamMember{ID: 7, Username: "bob", FullName: "Bob"}

	got := old.LoggedIn(m, "bob")

	want := config.State{UserID: 7, Username: "bob", Login: "bob", FullName: "Bob"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestStateFileWritesRememberedSprintsByProject(t *testing.T) {
	dir := t.TempDir()

	if err := config.NewStateFile(dir).Save(config.State{Sprints: map[int64]int64{7: 11}}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Sprints map[string]int64 `json:"sprints"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if want := map[string]int64{"7": 11}; !maps.Equal(raw.Sprints, want) {
		t.Errorf("state.json: %s", data)
	}
}

func TestStateWithSprintRemembersSprintWithoutChangingReceiver(t *testing.T) {
	old := config.State{Sprints: map[int64]int64{7: 11}}

	got := old.WithSprint(8, 20).WithSprint(7, 12)

	if want := map[int64]int64{7: 12, 8: 20}; !maps.Equal(got.Sprints, want) {
		t.Errorf("got %v, want %v", got.Sprints, want)
	}
	if want := map[int64]int64{7: 11}; !maps.Equal(old.Sprints, want) {
		t.Errorf("receiver changed to %v", old.Sprints)
	}
}

func TestStateWithoutSprintForgetsSprintWithoutChangingReceiver(t *testing.T) {
	old := config.State{Sprints: map[int64]int64{7: 11, 8: 20}}

	got := old.WithoutSprint(7)

	if want := map[int64]int64{8: 20}; !maps.Equal(got.Sprints, want) {
		t.Errorf("got %v, want %v", got.Sprints, want)
	}
	if len(old.Sprints) != 2 {
		t.Errorf("receiver changed to %v", old.Sprints)
	}
}
