package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestTooSmallTerminalShowsMessage(t *testing.T) {
	h := onTaskboard(t, 80, 24)

	h.send(tea.WindowSizeMsg{Width: 59, Height: 15})

	h.golden("too_small_59x15")
}

func TestTooSmallTerminalReportsTheSize(t *testing.T) {
	h := onTaskboard(t, 80, 24)

	h.send(tea.WindowSizeMsg{Width: 60, Height: 14})

	if got := words(h.plainView()); got != "Terminal too small (need 60×15, have 60×14)" {
		t.Errorf("got %q", got)
	}
}

func TestMinimumSizeIsNotTooSmall(t *testing.T) {
	h := onTaskboard(t, 60, 15)

	if got := words(h.plainView()); len(got) == 0 || got[:8] == "Terminal" {
		t.Errorf("60×15 shows %q", got)
	}
}

func TestTooSmallTerminalOnlyQuitKeysWork(t *testing.T) {
	for _, k := range []string{"q", "ctrl+c"} {
		t.Run(k, func(t *testing.T) {
			h := onTaskboard(t, 59, 15)

			h.key(k)

			if !h.quit {
				t.Errorf("%s did not quit", k)
			}
		})
	}
	for _, k := range []string{"j", "m", "r", "esc", "enter", ":", "?", "ctrl+k"} {
		t.Run("ignores "+k, func(t *testing.T) {
			h := onTaskboard(t, 59, 15)
			before := h.plainView()

			h.key(k)

			if h.quit || h.plainView() != before {
				t.Errorf("%s had an effect: quit %v\n%s", k, h.quit, h.plainView())
			}
		})
	}
}

func TestTooSmallTerminalKeepsStateUntilTheWindowGrows(t *testing.T) {
	h := onTaskboard(t, 80, 24)
	h.key("j")
	h.key("?")
	want, ref := h.plainView(), selectedTaskboardRef(h)

	h.send(tea.WindowSizeMsg{Width: 40, Height: 10})
	for _, k := range []string{"j", "j", "?", "esc"} {
		h.key(k)
	}
	h.send(tea.WindowSizeMsg{Width: 80, Height: 24})

	if got := selectedTaskboardRef(h); got != ref {
		t.Errorf("selection moved from #%d to #%d", ref, got)
	}
	if got := h.plainView(); got != want {
		t.Errorf("view changed:\n--- got\n%s--- want\n%s", got, want)
	}
}

func TestTooSmallTerminalOnTheLoginFormDoesNotTypeIntoIt(t *testing.T) {
	h := newHarness(t, &fakeSession{}, aliceState(webShop.ID))
	h.send(tea.WindowSizeMsg{Width: 59, Height: 15})
	h.runCmd(h.app.Init())

	h.typeText("xyz")
	h.send(tea.WindowSizeMsg{Width: 80, Height: 24})

	if got := h.plainView(); strings.Contains(got, "xyz") {
		t.Errorf("typed text reached the form:\n%s", got)
	}
}

func TestTooSmallTerminalIgnoresPaste(t *testing.T) {
	h := newHarness(t, &fakeSession{}, aliceState(webShop.ID))
	h.send(tea.WindowSizeMsg{Width: 59, Height: 15})
	h.runCmd(h.app.Init())

	h.send(tea.PasteMsg{Content: "pasted-text"})
	h.send(tea.WindowSizeMsg{Width: 80, Height: 24})

	if got := h.plainView(); strings.Contains(got, "pasted-text") {
		t.Errorf("pasted text reached the form:\n%s", got)
	}
}

func TestDebugLogPathIsShownInTheStatusLine(t *testing.T) {
	h := newHarness(t, &fakeSession{}, aliceState(webShop.ID))
	h.app.deps.DebugLog = "/home/alice/.config/taiga-tui/debug.log"
	h.app = New(h.app.deps)
	h.start()

	if got := words(h.plainView()); !strings.Contains(got, "Debug log: /home/alice/.config/taiga-tui/debug.log") {
		t.Errorf("status line lacks the path:\n%s", h.plainView())
	}
}
