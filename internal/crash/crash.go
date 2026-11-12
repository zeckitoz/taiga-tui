// Package crash turns panics of the terminal UI into an error, so that the
// terminal is restored by an orderly exit before the crash is reported.
package crash

import (
	"fmt"
	"regexp"
	"runtime"
	"runtime/debug"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/zeckitoz/taiga-tui/internal/atomicfile"
)

// Error is a panic that ended the program.
type Error struct {
	// Value is what was passed to panic.
	Value any
	// Stack is the stack trace of the goroutine that panicked.
	Stack []byte
}

func (e *Error) Error() string { return fmt.Sprintf("panic: %v", e.Value) }

// newError captures the stack of the goroutine that is panicking. It must be
// called from the deferred function that recovered the panic.
func newError(value any) *Error {
	return &Error{Value: value, Stack: debug.Stack()}
}

// Capture runs f and returns the panic in it as an Error.
func Capture(f func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = newError(r)
		}
	}()
	return f()
}

// Run runs m as a tea.Program until it quits. A panic in the model, in its
// View or in a command, which Bubble Tea runs in goroutines of their own, ends
// the program through tea.Quit and is returned as an *Error.
func Run(m tea.Model, opts ...tea.ProgramOption) error {
	g := &guard{inner: m}
	p := tea.NewProgram(g, opts...)
	// Quit blocks until the event loop takes the message, which it cannot
	// while it is in View.
	g.quit = func() { go p.Quit() }
	_, err := p.Run()
	if c := g.crashed(); c != nil {
		return c
	}
	return err
}

// crashMsg carries a panic of a command to the event loop.
type crashMsg struct{ err *Error }

// guard wraps a model and turns its panics into a recorded crash and a quit.
type guard struct {
	inner tea.Model
	quit  func()

	mu    sync.Mutex
	crash *Error
}

func (g *guard) crashed() *Error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.crash
}

// record keeps the first panic.
func (g *guard) record(err *Error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.crash == nil {
		g.crash = err
	}
}

func (g *guard) Init() (cmd tea.Cmd) {
	defer g.recoverInto(&cmd)
	return wrap(g.inner.Init())
}

func (g *guard) Update(msg tea.Msg) (_ tea.Model, cmd tea.Cmd) {
	if c, ok := msg.(crashMsg); ok {
		g.record(c.err)
		return g, tea.Quit
	}
	if g.crashed() != nil {
		return g, tea.Quit
	}
	defer g.recoverInto(&cmd)
	next, cmd := g.inner.Update(msg)
	g.inner = next
	return g, wrap(cmd)
}

func (g *guard) View() (v tea.View) {
	if g.crashed() != nil {
		return tea.View{AltScreen: true}
	}
	defer func() {
		if r := recover(); r != nil {
			g.record(newError(r))
			g.quit()
			v = tea.View{AltScreen: true}
		}
	}()
	return g.inner.View()
}

// recoverInto records a panic and makes the command quit the program.
func (g *guard) recoverInto(cmd *tea.Cmd) {
	if r := recover(); r != nil {
		g.record(newError(r))
		*cmd = tea.Quit
	}
}

// wrap makes the command report a panic as a crashMsg instead of crashing its
// goroutine. Commands of a batch are wrapped when the batch is run.
func wrap(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				msg = crashMsg{newError(r)}
			}
		}()
		msg = cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			wrapped := make(tea.BatchMsg, len(batch))
			for i, c := range batch {
				wrapped[i] = wrap(c)
			}
			return wrapped
		}
		return msg
	}
}

// jwtPattern matches a JWT: three base64url segments, the first of which
// starts with the encoding of `{"`.
var jwtPattern = regexp.MustCompile(`eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*`)

// WriteLog replaces the file at path with the report of e: the version line,
// the OS, the panic value and the stack trace. Anything that looks like a
// token is removed, as the panic value can be any text.
func WriteLog(path, versionLine string, e *Error) error {
	text := fmt.Sprintf("%s\nOS: %s/%s\n\n%v\n\n%s", versionLine, runtime.GOOS, runtime.GOARCH, e.Value, e.Stack)
	text = jwtPattern.ReplaceAllString(text, "[redacted]")
	return atomicfile.Write(path, []byte(text))
}
