package crash_test

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/zeckitoz/taiga-tui/internal/crash"
)

type hello struct{}

func (hello) Init() tea.Cmd                         { return func() tea.Msg { return "go" } }
func (m hello) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, tea.Quit }
func (hello) View() tea.View                        { return tea.NewView("hello") }

// faulty panics in the place named by where.
type faulty struct{ where string }

func (m faulty) Init() tea.Cmd {
	switch m.where {
	case "init":
		panic("boom in init")
	case "init-cmd":
		return func() tea.Msg { panic("boom in init cmd") }
	case "batch":
		return tea.Batch(func() tea.Msg { return nil }, func() tea.Msg { panic("boom in batch") })
	case "update":
		return func() tea.Msg { return "go" }
	}
	return nil
}

func (m faulty) Update(tea.Msg) (tea.Model, tea.Cmd) {
	if m.where == "update" {
		panic("boom in update")
	}
	return m, nil
}

func (m faulty) View() tea.View {
	if m.where == "view" {
		panic("boom in view")
	}
	return tea.NewView("ok")
}

func run(t *testing.T, m tea.Model) error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		done <- crash.Run(m, tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard), tea.WithWindowSize(80, 24))
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("program did not stop after the panic")
		return nil
	}
}

func TestRunReturnsNilWhenProgramEndsNormally(t *testing.T) {
	if err := run(t, hello{}); err != nil {
		t.Fatal(err)
	}
}

func TestPanicsAreCaughtEverywhere(t *testing.T) {
	for _, where := range []string{"init", "init-cmd", "batch", "update", "view"} {
		t.Run(where, func(t *testing.T) {
			err := run(t, faulty{where: where})

			c, ok := errors.AsType[*crash.Error](err)
			if !ok {
				t.Fatalf("got %v, want *crash.Error", err)
			}
			if want := "boom in " + strings.ReplaceAll(where, "-", " "); c.Value != want {
				t.Errorf("value %v, want %q", c.Value, want)
			}
			if !strings.Contains(string(c.Stack), "crash_test.go") {
				t.Errorf("stack does not reach the panic:\n%s", c.Stack)
			}
		})
	}
}
