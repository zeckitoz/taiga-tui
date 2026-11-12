package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/zeckitoz/taiga-tui/internal/config"
	"github.com/zeckitoz/taiga-tui/internal/domain"
	"github.com/zeckitoz/taiga-tui/internal/taiga"
	"github.com/zeckitoz/taiga-tui/internal/taiga/taigafake"
)

// aliceState is the state of a Team Member whose session resumes.
func aliceState(defaultProject int64) config.State {
	return config.State{
		UserID: 42, Username: "alice", Login: "alice", FullName: "Alice Example",
		DefaultProject: defaultProject, LastBoard: config.BoardKanban,
	}
}

// resumed returns a harness whose stored session is valid, on the given instance.
func resumed(t *testing.T, api *taigafake.Fake, st config.State) *harness {
	t.Helper()
	h := newHarness(t, &fakeSession{resume: true}, st)
	h.taiga = api
	h.app.deps.Taiga = api
	return h
}

func TestPickerListsProjectsAlphabeticallyAndGreysOutProjectsWithoutBoards(t *testing.T) {
	h := resumed(t, newFake(), aliceState(0))

	h.start()

	view := h.plainView()
	if a, m, w := strings.Index(view, "Archive"), strings.Index(view, "mobile app"), strings.Index(view, "Web Shop"); a < 0 || a > m || m > w {
		t.Errorf("not sorted by name ignoring case:\n%s", view)
	}
	if !strings.Contains(view, "Archive  (no boards enabled)") {
		t.Errorf("Archive not greyed out:\n%s", view)
	}
	h.golden("picker")
}

func TestPickerSkipsProjectsWithoutBoardsAndOpensChoiceAsDefault(t *testing.T) {
	h := resumed(t, newFake(), aliceState(0))
	h.start()

	h.key("up") // already on the first selectable project
	h.key("down")
	h.key("up")
	h.key("down")
	h.key("enter")

	if h.app.screen != screenBoard || h.app.project.ID != webShop.ID {
		t.Fatalf("screen %v, project %q", h.app.screen, h.app.project.Name)
	}
	if h.state.saved.DefaultProject != webShop.ID {
		t.Errorf("default project %d not saved", h.state.saved.DefaultProject)
	}
}

func TestPickerFiltersByName(t *testing.T) {
	h := resumed(t, newFake(), aliceState(0))
	h.start()

	h.typeText("SHO")

	view := h.plainView()
	if strings.Contains(view, "mobile app") || !strings.Contains(view, "Web Shop") {
		t.Errorf("filter not applied:\n%s", view)
	}
	h.golden("picker_filtered")

	h.typeText("x")
	if !strings.Contains(h.plainView(), "No project matches “SHOx”") {
		t.Errorf("no empty hint:\n%s", h.plainView())
	}
	h.key("enter")
	if h.app.screen != screenPicker {
		t.Error("enter without a match left the picker")
	}
}

func TestPickerEscClearsFilterThenQuits(t *testing.T) {
	h := resumed(t, newFake(), aliceState(0))
	h.start()
	h.typeText("web")

	h.key("esc")

	if h.quit || h.app.picker.filter.Value() != "" {
		t.Fatalf("first esc: quit %v, filter %q", h.quit, h.app.picker.filter.Value())
	}
	h.key("esc")
	if !h.quit {
		t.Error("second esc did not quit")
	}
}

func TestSingleProjectIsOpenedAutomatically(t *testing.T) {
	h := resumed(t, newFakeWith(webShop), aliceState(0))

	h.start()

	if h.app.screen != screenBoard || h.state.saved.DefaultProject != webShop.ID {
		t.Errorf("screen %v, saved default %d", h.app.screen, h.state.saved.DefaultProject)
	}
}

func TestSingleProjectWithoutBoardsIsNotOpened(t *testing.T) {
	h := resumed(t, newFakeWith(archive), aliceState(0))

	h.start()

	if h.app.screen != screenPicker {
		t.Errorf("screen %v", h.app.screen)
	}
}

func TestNoProjectsShowsEmptyStateWithReloadAndQuit(t *testing.T) {
	api := newFakeWith()
	h := resumed(t, api, aliceState(0))
	h.start()

	h.golden("picker_no_projects")

	api.AddProject(mobileApp)
	h.key("r")
	if api.Calls("Projects") != 2 || h.app.project.ID != mobileApp.ID {
		t.Errorf("r did not reload: %d calls, project %q", api.Calls("Projects"), h.app.project.Name)
	}
}

func TestNoProjectsQuitsWithQ(t *testing.T) {
	h := resumed(t, newFakeWith(), aliceState(0))
	h.start()

	h.key("q")

	if !h.quit {
		t.Error("q did not quit")
	}
}

func TestDefaultProjectOpensOnStart(t *testing.T) {
	h := resumed(t, newFake(), aliceState(webShop.ID))

	h.start()

	if h.app.screen != screenBoard || h.app.project.ID != webShop.ID {
		t.Errorf("screen %v, project %q", h.app.screen, h.app.project.Name)
	}
	if h.state.saves != 0 {
		t.Error("state rewritten although the default project did not change")
	}
}

func TestVanishedDefaultProjectShowsPickerWithHint(t *testing.T) {
	h := resumed(t, newFake(), aliceState(99))

	h.start()

	h.golden("picker_default_gone")
}

func TestDefaultProjectWithoutBoardsShowsPickerWithHint(t *testing.T) {
	h := resumed(t, newFake(), aliceState(archive.ID))

	h.start()

	if view := h.plainView(); h.app.screen != screenPicker || !strings.Contains(view, "Archive has no boards enabled") {
		t.Errorf("view:\n%s", view)
	}
}

func TestPickerScrollsToKeepSelectionVisible(t *testing.T) {
	api := taigafake.New(alice)
	for i := range 30 {
		api.AddProject(domain.Project{ProjectSummary: domain.ProjectSummary{
			ID: int64(100 + i), Name: fmt.Sprintf("Project %02d", i), Slug: fmt.Sprint(i), KanbanEnabled: true,
		}})
	}
	h := resumed(t, api, aliceState(0))
	h.start()

	for range 25 {
		h.key("down")
	}

	if view := h.plainView(); !strings.Contains(view, "Project 25") || strings.Contains(view, "Project 00") {
		t.Errorf("selection not visible:\n%s", view)
	}
}

func TestStartupLoadFailureShowsErrorPageAndRetries(t *testing.T) {
	for _, method := range []string{"Me", "Projects", "Project"} {
		t.Run(method, func(t *testing.T) {
			api := newFake()
			api.FailNext(method, fmt.Errorf("%w: connection refused", taiga.ErrUnreachable))
			h := resumed(t, api, aliceState(webShop.ID))
			h.start()

			if view := h.plainView(); !strings.Contains(view, "Cannot reach taiga.example.org: no connection") {
				t.Fatalf("view:\n%s", view)
			}
			if method == "Me" {
				h.golden("load_failed")
			}
			h.key("r")

			if h.app.screen != screenBoard {
				t.Errorf("retry did not open the board, screen %v", h.app.screen)
			}
		})
	}
}

func TestExpiredSessionDuringLoadShowsLogin(t *testing.T) {
	api := newFake()
	api.FailNext("Projects", taiga.ErrSessionExpired)
	h := resumed(t, api, aliceState(0))

	h.start()

	if h.app.screen != screenLogin {
		t.Errorf("screen %v", h.app.screen)
	}
}

func TestLoginLoadsProjectsOfTheNewTeamMember(t *testing.T) {
	sess := &fakeSession{result: taiga.LoginResult{Member: alice}}
	h := newHarness(t, sess, config.State{UserID: 7, Username: "bob", Login: "bob", DefaultProject: webShop.ID})
	h.start()
	h.key("shift+tab")
	h.key("ctrl+u")
	h.typeText("alice")
	h.key("tab")
	h.typeText("pw")

	h.key("enter")

	// Bob's default project must not be opened for Alice.
	if h.app.screen != screenPicker {
		t.Errorf("screen %v", h.app.screen)
	}
}

func TestProjectBecomesDefaultOnlyAfterItLoaded(t *testing.T) {
	api := newFake()
	api.FailNext("Project", taiga.ErrTimeout)
	h := resumed(t, api, aliceState(0))
	h.start()
	h.key("down") // Web Shop

	h.key("enter")

	if h.state.saves != 0 {
		t.Errorf("default saved although the project failed to load: %+v", h.state.saved)
	}
	if view := h.plainView(); !strings.Contains(view, "Couldn't load Web Shop: timed out") {
		t.Errorf("view:\n%s", view)
	}
	if h.app.screen != screenPicker {
		t.Fatalf("picker not open after a failed switch, screen %v", h.app.screen)
	}
	h.golden("picker_project_failed")
	h.key("enter")
	if h.state.saved.DefaultProject != webShop.ID {
		t.Errorf("default not saved after retry: %+v", h.state.saved)
	}
}
