package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zeckitoz/taiga-tui/internal/config"
)

// fakeUpdates is the faked update check effect.
type fakeUpdates struct {
	found string
	err   error
	calls int
}

func (f *fakeUpdates) check(context.Context) (string, error) {
	f.calls++
	return f.found, f.err
}

const updateHint = "v0.4.0 available - see github.com/zeckitoz/taiga-tui/releases"

func withUpdates(t *testing.T, st config.State, version string, f *fakeUpdates) *harness {
	t.Helper()
	h := resumed(t, newFake(), st)
	h.app.deps.Version = version
	if f != nil {
		h.app.deps.CheckUpdate = f.check
	}
	h.app = New(h.app.deps)
	return h
}

func TestUpdateHintShownWhenNewerReleaseFound(t *testing.T) {
	f := &fakeUpdates{found: "v0.4.0"}
	h := withUpdates(t, aliceState(0), "v0.3.0", f)

	h.start()

	if f.calls != 1 {
		t.Errorf("%d checks, want 1", f.calls)
	}
	if view := h.plainView(); !strings.Contains(view, updateHint) {
		t.Errorf("hint missing:\n%s", view)
	}
	if h.state.saved.UpdateVersion != "v0.4.0" || !h.state.saved.UpdateCheckedAt.Equal(today) {
		t.Errorf("check not remembered: %+v", h.state.saved)
	}
	if h.state.saved.UserID != 42 {
		t.Errorf("rest of the state lost: %+v", h.state.saved)
	}
}

func TestUpdateCheckIsSilentWhenNothingNewer(t *testing.T) {
	f := &fakeUpdates{found: "v0.3.0"}
	h := withUpdates(t, aliceState(0), "v0.3.0", f)

	h.start()

	if view := h.plainView(); strings.Contains(view, "available") {
		t.Errorf("unexpected hint:\n%s", view)
	}
	if h.state.saved.UpdateVersion != "v0.3.0" {
		t.Errorf("found version not remembered: %+v", h.state.saved)
	}
}

func TestUpdateCheckErrorIsSilentButCountsAsCheck(t *testing.T) {
	f := &fakeUpdates{err: errors.New("no network")}
	st := aliceState(0)
	st.UpdateVersion = "v0.2.0"
	h := withUpdates(t, st, "v0.3.0", f)

	h.start()

	view := h.plainView()
	if strings.Contains(view, "no network") || strings.Contains(view, "available") || strings.Contains(view, "Couldn't") {
		t.Errorf("error leaked:\n%s", view)
	}
	if !h.state.saved.UpdateCheckedAt.Equal(today) || h.state.saved.UpdateVersion != "v0.2.0" {
		t.Errorf("want check time updated and version kept: %+v", h.state.saved)
	}
}

func TestUpdateCheckRunsAtMostOncePerDay(t *testing.T) {
	f := &fakeUpdates{found: "v0.5.0"}
	st := aliceState(0)
	st.UpdateCheckedAt = today.Add(-23 * time.Hour)
	st.UpdateVersion = "v0.4.0"
	h := withUpdates(t, st, "v0.3.0", f)

	h.start()

	if f.calls != 0 {
		t.Errorf("%d checks, want none", f.calls)
	}
	if view := h.plainView(); !strings.Contains(view, updateHint) {
		t.Errorf("hint from the stored version missing:\n%s", view)
	}
}

func TestUpdateHintGoneOnceUpdated(t *testing.T) {
	st := aliceState(0)
	st.UpdateCheckedAt = today.Add(-time.Hour)
	st.UpdateVersion = "v0.4.0"
	h := withUpdates(t, st, "v0.4.0", &fakeUpdates{})

	h.start()

	if view := h.plainView(); strings.Contains(view, "available") {
		t.Errorf("unexpected hint:\n%s", view)
	}
}

func TestUpdateCheckIgnoredForDevBuilds(t *testing.T) {
	f := &fakeUpdates{found: "v0.4.0"}
	h := withUpdates(t, aliceState(0), "dev", f)

	h.start()

	if view := h.plainView(); strings.Contains(view, "available") {
		t.Errorf("dev build got a hint:\n%s", view)
	}
}

func TestUpdateCheckOffMakesNoCallAndShowsNothing(t *testing.T) {
	st := aliceState(0)
	st.UpdateCheckedAt = today.Add(-time.Hour)
	st.UpdateVersion = "v0.4.0"
	h := withUpdates(t, st, "v0.3.0", nil)

	h.start()

	if view := h.plainView(); strings.Contains(view, "available") {
		t.Errorf("unexpected hint:\n%s", view)
	}
}

func TestUpdateCheckDoesNotRunInsideInit(t *testing.T) {
	f := &fakeUpdates{found: "v0.4.0"}
	h := withUpdates(t, aliceState(0), "v0.3.0", f)

	cmd := h.app.Init()

	// The check is a command of its own, which Bubble Tea runs off the
	// update loop, so building the start commands never waits for GitHub.
	if cmd == nil || f.calls != 0 {
		t.Errorf("Init ran %d checks", f.calls)
	}
}

func TestUpdateHintShownOnLoginScreenToo(t *testing.T) {
	f := &fakeUpdates{found: "v0.4.0"}
	h := newHarness(t, &fakeSession{}, config.State{})
	h.app.deps.Version, h.app.deps.CheckUpdate = "v0.3.0", f.check
	h.app = New(h.app.deps)

	h.start()

	if view := h.plainView(); !strings.Contains(view, updateHint) {
		t.Errorf("hint missing:\n%s", view)
	}
}
