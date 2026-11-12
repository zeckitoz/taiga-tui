package ui

import (
	"strings"
	"testing"

	"github.com/zeckitoz/taiga-tui/internal/domain"
)

// statusLabels lists the labels of the entries the palette offers.
func statusLabels(h *harness) []string {
	var labels []string
	for _, e := range h.app.palette.entries {
		labels = append(labels, e.label)
	}
	return labels
}

func TestSOpensPaletteWithTheStatusesOfAUserStoryAndEscClosesIt(t *testing.T) {
	h := onKanban(t, 100, 24) // #15 in New

	h.key("s")

	want := []string{"Status: New", "Status: Ready", "Status: In progress", "Status: Ready for test", "Status: Done"}
	if !h.app.paletteOpen || strings.Join(statusLabels(h), "|") != strings.Join(want, "|") {
		t.Fatalf("entries %v:\n%s", statusLabels(h), h.plainView())
	}
	if line := lineWith(h, "Status: New"); !strings.Contains(line, "● Status: New") {
		t.Errorf("current Status not marked: %q", line)
	}
	if strings.Contains(h.plainView(), "Go to Kanban Board") {
		t.Error("board entries in Status mode")
	}
	h.key("esc")
	if h.app.paletteOpen || h.taiga.Calls("PatchUserStory") != 0 {
		t.Error("esc did not just close the palette")
	}
}

func TestStatusPickerFiltersAndAppliesTheChangeLikeTheNeighbourKeys(t *testing.T) {
	h := onKanban(t, 100, 24)

	h.key("s")
	h.typeText("tes")
	h.key("enter")

	if h.app.paletteOpen {
		t.Error("palette still open")
	}
	if got := h.app.kanban.ActiveStatus().Name; got != "Ready for test" || selectedRef(h) != 15 {
		t.Errorf("cursor on #%d in %q", selectedRef(h), got)
	}
	if got := storedStory(t, h, 502); got.Status != 104 || got.Version != 1 {
		t.Errorf("stored %+v", got)
	}
}

func TestStatusPickerOnTheCurrentStatusChangesNothing(t *testing.T) {
	h := onKanban(t, 100, 24)

	h.key("s")
	h.key("enter")

	if h.app.paletteOpen || h.taiga.Calls("PatchUserStory") != 0 {
		t.Errorf("open %v, %d requests", h.app.paletteOpen, h.taiga.Calls("PatchUserStory"))
	}
}

func TestStatusPickerOfATaskOffersTaskStatuses(t *testing.T) {
	h := onTaskboard(t, 120, 30)
	h.key("j") // the first Task

	h.key("s")
	h.typeText("closed")
	h.key("enter")

	if h.app.paletteOpen || h.taiga.Calls("PatchTask") != 1 {
		t.Fatalf("open %v, %d requests:\n%s", h.app.paletteOpen, h.taiga.Calls("PatchTask"), h.plainView())
	}
	if strings.Contains(strings.Join(statusLabels(h), "|"), "Status: Done") {
		t.Error("User Story Statuses offered for a Task")
	}
}

func TestStatusPickerOfAUserStoryRowOnTheTaskboardOffersUserStoryStatuses(t *testing.T) {
	h := onTaskboard(t, 120, 30) // a User Story row

	h.key("s")

	if got := statusLabels(h); len(got) != 5 || got[4] != "Status: Done" {
		t.Errorf("entries %v", got)
	}
}

func TestStatusPickerRejectsWithoutPermissionAndWhileSaving(t *testing.T) {
	api := newFake()
	p := webShop
	p.Permissions = domain.Permissions{}
	api.SetProject(p)
	h := resumed(t, api, aliceState(webShop.ID))
	h.start()

	h.key("s")

	if h.app.paletteOpen || !strings.Contains(h.plainView(), "You don't have permission to change user stories") {
		t.Errorf("open %v:\n%s", h.app.paletteOpen, h.plainView())
	}

	k := onKanban(t, 100, 24)
	save := k.pickStatusOnly("ready")
	k.key("s")
	if k.app.paletteOpen || !strings.Contains(k.plainView(), "Still saving…") {
		t.Errorf("open %v:\n%s", k.app.paletteOpen, k.plainView())
	}
	k.runCmd(save)
}

func TestFootersAndFullHelpAnnounceThePicker(t *testing.T) {
	for name, tt := range map[string]struct {
		h    *harness
		want string
	}{
		"kanban":    {onKanban(t, 100, 24), "s status"},
		"taskboard": {onTaskboard(t, 120, 30), "s status"},
	} {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(tt.h.plainView(), tt.want) {
				t.Errorf("no %q in the footer:\n%s", tt.want, tt.h.plainView())
			}
			tt.h.key("?")
			if !strings.Contains(tt.h.plainView(), "pick a status") {
				t.Errorf("full help:\n%s", tt.h.plainView())
			}
		})
	}
}

func TestStatusPalette(t *testing.T) {
	h := onKanban(t, 100, 24)
	h.key("s")
	h.golden("palette_status")
}

func TestStatusPickerDoesNotChangeAnotherRowAfterAReload(t *testing.T) {
	h := onKanban(t, 100, 24) // #15 selected
	h.key("s")
	h.typeText("done")
	var rest []domain.UserStory
	for _, s := range webShopStories {
		if s.Ref != 15 {
			rest = append(rest, s)
		}
	}
	h.taiga.SetStories(webShop.ID, rest...)
	app, reload := h.app.loadKanban()
	h.app = app
	h.runCmd(reload)

	h.key("enter")

	if h.app.paletteOpen || h.taiga.Calls("PatchUserStory") != 0 {
		t.Errorf("open %v, %d requests", h.app.paletteOpen, h.taiga.Calls("PatchUserStory"))
	}
}
