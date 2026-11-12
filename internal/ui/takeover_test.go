package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/zeckitoz/taiga-tui/internal/domain"
	"github.com/zeckitoz/taiga-tui/internal/taiga"
)

// storedTask returns the Task as the fake instance holds it.
func storedTask(t *testing.T, h *harness, id int64) domain.Task {
	t.Helper()
	task, err := h.taiga.Task(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func TestAOnTheKanbanBoardAddsMeToTheAssigneesOfAUserStory(t *testing.T) {
	h := onKanban(t, 100, 24) // #15, nobody

	h.key("a")

	if got := storedStory(t, h, 502); !slices.Equal(got.Assignees, []int64{42}) || got.Version != 1 {
		t.Errorf("stored %+v", got)
	}
	if line := lineWith(h, "Product search"); !strings.Contains(line, "AE") {
		t.Errorf("row not updated: %q", line)
	}
}

func TestAOnTheKanbanBoardKeepsTheOtherAssignees(t *testing.T) {
	h := onKanban(t, 100, 24)
	h.key("j")
	h.key("j") // #108, Carla

	h.key("a")

	if got := storedStory(t, h, 503); !slices.Equal(got.Assignees, []int64{42, 44}) {
		t.Errorf("stored %+v", got)
	}
}

func TestAWhenAlreadyAssigneeSendsNothing(t *testing.T) {
	h := onKanban(t, 100, 24)
	h.key("j") // #12, Alice is an Assignee

	h.key("a")

	if h.taiga.Calls("PatchUserStory") != 0 || !strings.Contains(h.plainView(), "Already assigned to you") {
		t.Errorf("%d requests:\n%s", h.taiga.Calls("PatchUserStory"), h.plainView())
	}
	h.key("j")
	if strings.Contains(h.plainView(), "Already assigned to you") {
		t.Error("hint stays after the next action")
	}
}

func TestAOfAnUnassignedTaskTakesOverWithoutAsking(t *testing.T) {
	h := onTaskboard(t, 120, 30)
	for range 9 {
		h.key("j") // #40, nobody
	}

	h.key("a")

	if got := storedTask(t, h, 806); got.Assignee != 42 || got.Version != 1 {
		t.Errorf("stored %+v", got)
	}
}

func TestAOfATaskOfSomeoneElseAsksAndYReplaces(t *testing.T) {
	h := onTaskboard(t, 120, 30)
	h.key("j")
	h.key("j") // #32, Bea Koch

	h.key("a")

	if !strings.Contains(h.plainView(), "Replace Bea Koch as assignee? y/n") || h.taiga.Calls("PatchTask") != 0 {
		t.Fatalf("%d requests:\n%s", h.taiga.Calls("PatchTask"), h.plainView())
	}
	h.key("y")
	if got := storedTask(t, h, 802); got.Assignee != 42 {
		t.Errorf("stored %+v", got)
	}
	if strings.Contains(h.plainView(), "Replace") {
		t.Errorf("question stays:\n%s", h.plainView())
	}
}

func TestAnyOtherKeyThanYKeepsTheAssigneeOfATask(t *testing.T) {
	for _, key := range []string{"n", "esc", "j", "enter"} {
		t.Run(key, func(t *testing.T) {
			h := onTaskboard(t, 120, 30)
			h.key("j")
			h.key("j")
			h.key("a")

			h.key(key)

			if h.taiga.Calls("PatchTask") != 0 || storedTask(t, h, 802).Assignee != 43 {
				t.Error("replaced the Assignee")
			}
			if selectedTaskboardRef(h) != 32 || strings.Contains(h.plainView(), "Replace") {
				t.Errorf("answer not consumed, or question stays:\n%s", h.plainView())
			}
		})
	}
}

func TestAOfAUserStoryOnTheTaskboardKeepsOthersAndNeverAsks(t *testing.T) {
	h := onTaskboard(t, 120, 30)
	for range 7 {
		h.key("j") // #15, nobody
	}

	h.key("a")

	if got := storedStory(t, h, 502); !slices.Equal(got.Assignees, []int64{42}) {
		t.Errorf("stored %+v", got)
	}
}

func TestAOnTheGroupWithoutUserStoryDoesNothing(t *testing.T) {
	h := onTaskboard(t, 120, 30)
	for range 8 {
		h.key("j")
	}

	h.key("a")

	if h.taiga.Calls("PatchTask")+h.taiga.Calls("PatchUserStory") != 0 || len(h.app.status) != 0 {
		t.Errorf("status %+v", h.app.status)
	}
}

func TestFailedTakeOverIsUndoneLikeAStatusChange(t *testing.T) {
	h := onKanban(t, 100, 24)
	h.taiga.FailNext("PatchUserStory", taiga.ErrConflict)

	h.key("a")

	if !strings.Contains(h.plainView(), "#15 was changed by someone else") || strings.Contains(lineWith(h, "Product search"), "AE") {
		t.Errorf("view:\n%s", h.plainView())
	}
}

func TestTakeOverIsRejectedWithoutPermissionAndWhileSaving(t *testing.T) {
	k := onKanban(t, 100, 24)
	save := k.sendOnly(keyMsg("shift+right"))
	k.key("a")
	if !strings.Contains(k.plainView(), "Still saving…") {
		t.Errorf("view:\n%s", k.plainView())
	}
	k.runCmd(save)

	api := newFake()
	p := webShop
	p.Permissions = domain.Permissions{}
	api.SetProject(p)
	h := resumed(t, api, aliceState(webShop.ID))
	h.start()
	h.key("a")
	if !strings.Contains(h.plainView(), "You don't have permission to change user stories") || api.Calls("PatchUserStory") != 0 {
		t.Errorf("view:\n%s", h.plainView())
	}
}

func TestFootersAndFullHelpAnnounceTakeOver(t *testing.T) {
	for _, h := range []*harness{onKanban(t, 100, 24), onTaskboard(t, 120, 30)} {
		if !strings.Contains(h.plainView(), "a assign to me") {
			t.Errorf("no \"a assign to me\" in the footer:\n%s", h.plainView())
		}
		h.key("?")
		if !strings.Contains(h.plainView(), "assign to me") {
			t.Errorf("full help:\n%s", h.plainView())
		}
	}
}

func TestAssignToMeQuestion(t *testing.T) {
	h := onTaskboard(t, 100, 24)
	h.key("j")
	h.key("j")
	h.key("a")
	h.golden("taskboard_takeover_question")
}
