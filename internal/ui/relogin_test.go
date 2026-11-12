package ui

import (
	"strings"
	"testing"

	"github.com/zeckitoz/taiga-tui/internal/taiga"
)

const sessionRenewed = "Session renewed, board reloaded - please repeat your action"

// expireOnRefresh makes the next reload of the Kanban Board find the session
// expired and presses r.
func expireOnRefresh(h *harness) {
	h.taiga.FailNext("Kanban", taiga.ErrSessionExpired)
	h.key("r")
}

// logInAgain answers the modal login with the password pw.
func logInAgain(h *harness, pw string) {
	h.typeText(pw)
	h.key("enter")
}

func TestExpiredSessionOnTheBoardOpensAModalLoginOverIt(t *testing.T) {
	h := onKanban(t, 100, 24)
	h.key("m")
	h.key("j")
	ref := selectedRef(h)

	expireOnRefresh(h)

	if h.app.screen != screenBoard || !h.app.relogin {
		t.Fatalf("screen %v, modal %v", h.app.screen, h.app.relogin)
	}
	view := h.plainView()
	for _, want := range []string{"Log in to taiga.example.org", "Your session has expired", "Web Shop", "mine  Alice Example", "esc keep board"} {
		if !strings.Contains(view, want) {
			t.Errorf("%q missing:\n%s", want, view)
		}
	}
	if selectedRef(h) != ref || !h.app.mine {
		t.Errorf("selection #%d and filter %v were not kept", selectedRef(h), h.app.mine)
	}
	if h.app.form.login.Value() != "alice" || h.app.form.focus != fieldPassword {
		t.Error("login is not prefilled with the cursor in the password field")
	}
	h.golden("relogin_modal")
}

func TestLoginInTheModalReloadsTheBoardAndAsksToRepeatTheAction(t *testing.T) {
	h := onKanban(t, 100, 24)
	h.key("m")
	h.key("j")
	ref := selectedRef(h)
	expireOnRefresh(h)
	h.sess.result = taiga.LoginResult{Member: alice}
	loads := h.taiga.Calls("Kanban")

	logInAgain(h, "s3cret")

	if h.app.relogin || h.app.screen != screenBoard {
		t.Fatalf("modal %v, screen %v:\n%s", h.app.relogin, h.app.screen, h.plainView())
	}
	if h.sess.login != "alice" || h.sess.password != "s3cret" {
		t.Errorf("authenticated with %q/%q", h.sess.login, h.sess.password)
	}
	if got := h.taiga.Calls("Kanban"); got != loads+1 {
		t.Errorf("board loaded %d times after login, want 1", got-loads)
	}
	if !strings.Contains(words(h.plainView()), sessionRenewed) {
		t.Errorf("status line missing:\n%s", h.plainView())
	}
	if selectedRef(h) != ref || !h.app.mine {
		t.Errorf("selection #%d and filter %v were not kept", selectedRef(h), h.app.mine)
	}
	h.key("j")
	if strings.Contains(h.plainView(), sessionRenewed) {
		t.Error("the hint stays after the next action")
	}
}

func TestSessionExpiredDuringSaveDropsTheActionAfterLogin(t *testing.T) {
	h := onKanban(t, 100, 24)
	h.taiga.FailNext("PatchUserStory", taiga.ErrSessionExpired)

	pickStatus(h, "ready")

	if !h.app.relogin || h.app.screen != screenBoard {
		t.Fatalf("modal %v, screen %v", h.app.relogin, h.app.screen)
	}
	if h.app.kanban.Saving() || h.app.saving.active {
		t.Error("still saving behind the dialog")
	}
	h.sess.result = taiga.LoginResult{Member: alice}
	logInAgain(h, "pw")

	if h.app.kanban.ActiveStatus().Name != "New" || h.taiga.Calls("PatchUserStory") != 1 {
		t.Errorf("the interrupted action was not dropped: status %q, %d patches", h.app.kanban.ActiveStatus().Name, h.taiga.Calls("PatchUserStory"))
	}
	if !strings.Contains(words(h.plainView()), sessionRenewed) {
		t.Errorf("status line missing:\n%s", h.plainView())
	}
}

func TestSessionExpiredOnTheTaskboardReloadsTheTaskboard(t *testing.T) {
	h := onTaskboard(t, 120, 40)
	h.key("j")
	ref := selectedTaskboardRef(h)
	h.taiga.FailNext("PatchTask", taiga.ErrSessionExpired)

	pickStatus(h, "ready for test")

	if !h.app.relogin {
		t.Fatalf("no modal:\n%s", h.plainView())
	}
	h.golden("relogin_modal_taskboard")
	h.sess.result = taiga.LoginResult{Member: alice}
	loads := h.taiga.Calls("Taskboard")
	logInAgain(h, "pw")

	if h.taiga.Calls("Taskboard") != loads+1 || selectedTaskboardRef(h) != ref {
		t.Errorf("%d reloads, cursor on #%d", h.taiga.Calls("Taskboard")-loads, selectedTaskboardRef(h))
	}
	if !strings.Contains(words(h.plainView()), sessionRenewed) {
		t.Errorf("status line missing:\n%s", h.plainView())
	}
}

func TestEscInTheModalKeepsTheBoardAndAnyFurtherActionReopensIt(t *testing.T) {
	h := onKanban(t, 100, 24)
	expireOnRefresh(h)

	h.key("esc")

	if h.app.relogin || h.app.screen != screenBoard || h.quit {
		t.Fatalf("modal %v, screen %v, quit %v", h.app.relogin, h.app.screen, h.quit)
	}
	if view := h.plainView(); strings.Contains(view, "Log in to") || !strings.Contains(view, "Checkout with credit card") {
		t.Errorf("board not visible:\n%s", view)
	}
	h.key("j") // reading the board needs no session
	if h.app.relogin {
		t.Error("moving the selection reopened the dialog")
	}

	h.taiga.FailNext("Kanban", taiga.ErrSessionExpired)
	h.key("r")

	if !h.app.relogin {
		t.Errorf("the dialog stayed closed:\n%s", h.plainView())
	}
}

func TestEscInTheModalAfterAFailedLoginKeepsTheBoard(t *testing.T) {
	h := onKanban(t, 100, 24)
	expireOnRefresh(h)
	h.sess.authErr = taiga.ErrInvalidCredentials
	logInAgain(h, "wrong")

	if !h.app.relogin || !strings.Contains(h.plainView(), "Invalid username or password") {
		t.Fatalf("modal %v:\n%s", h.app.relogin, h.plainView())
	}
	h.key("esc")

	if h.app.relogin || h.quit {
		t.Errorf("modal %v, quit %v", h.app.relogin, h.quit)
	}
}

func TestSessionExpiredWhileTheModalIsOpenKeepsItOpen(t *testing.T) {
	h := onKanban(t, 100, 24)
	expireOnRefresh(h)
	h.typeText("abc")

	k, load := h.app.kanban.LoadUserStories()
	h.app.kanban = k
	h.send(kanbanLoadedMsg{load: load, err: taiga.ErrSessionExpired})

	if !h.app.relogin || h.app.form.password.Value() != "abc" {
		t.Errorf("modal %v, password %q: the dialog was reset", h.app.relogin, h.app.form.password.Value())
	}
}

func TestModalLoginAsAnotherTeamMemberStartsOver(t *testing.T) {
	h := onKanban(t, 100, 24)
	expireOnRefresh(h)
	h.sess.result = taiga.LoginResult{Member: bea}
	meLoads := h.taiga.Calls("Me")

	logInAgain(h, "pw")

	if h.app.relogin || h.taiga.Calls("Me") != meLoads+1 || h.state.saved.UserID != bea.ID {
		t.Errorf("modal %v, %d loads of the Team Member, saved %+v", h.app.relogin, h.taiga.Calls("Me")-meLoads, h.state.saved)
	}
	if strings.Contains(words(h.plainView()), sessionRenewed) {
		t.Error("the board of the previous Team Member was only reloaded")
	}
}

func TestSessionExpiredWhileTheMenuLoadsInTheBackgroundOpensTheModal(t *testing.T) {
	h := onKanban(t, 100, 24)
	h.taiga.FailNext("Sprints", taiga.ErrSessionExpired)

	h.key(":") // loads the Sprints for the menu

	if h.app.screen != screenBoard || !h.app.relogin || h.app.paletteOpen {
		t.Errorf("screen %v, modal %v, palette %v", h.app.screen, h.app.relogin, h.app.paletteOpen)
	}
}
