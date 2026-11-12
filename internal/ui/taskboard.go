package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/zeckitoz/taiga-tui/internal/board"
	"github.com/zeckitoz/taiga-tui/internal/domain"
)

// fullNamesFrom is the list width from which Task rows show the Assignee's
// full name instead of the initials.
const fullNamesFrom = 70

// Widths of the Task columns.
const (
	maxStatusWidth = 18
	maxNameWidth   = 18
)

// noUserStory names the group of Tasks without User Story.
const noUserStory = "Tasks without user story"

// taskboardKeys returns the footer of the Taskboard for the selected row.
func taskboardKeys(tb board.Taskboard) []string {
	if tb.Empty() {
		return []string{"r refresh", ": menu", "? help"}
	}
	var keys []string
	if tb.StatusSelected() {
		keys = append(keys, "⇧←/→ status", "s pick", "a assign to me")
	}
	enter := "enter fold"
	if tb.TaskSelected() {
		enter = "enter detail"
	}
	return append(keys, "↑/↓ move", "←/→ fold", enter, "r refresh", ": menu", "? help")
}

// noSprintsKeys is the hint of a project without Sprints.
const noSprintsKeys = ": menu · r refresh"

// sprintPath is the header part naming the Sprint, with its date range if
// withDates, and whether it is current, closed or upcoming on today.
func sprintPath(s domain.Sprint, today domain.Date, withDates bool) string {
	path := titleStyle.Render(plainText(s.Name))
	if withDates {
		path += "  " + faintStyle.Render(dateRange(s))
	}
	if phase := sprintPhase(s, today); phase != "" {
		path += "  " + faintStyle.Render(phase)
	}
	return path
}

// dateRange renders the Sprint's dates like "03.11. - 16.11.2026", with
// both years if they differ.
func dateRange(s domain.Sprint) string {
	day := func(d domain.Date) string { return fmt.Sprintf("%02d.%02d.", d.Day, int(d.Month)) }
	if s.Start.Year == s.Finish.Year {
		return day(s.Start) + " - " + day(s.Finish) + strconv.Itoa(s.Finish.Year)
	}
	return day(s.Start) + strconv.Itoa(s.Start.Year) + " - " + day(s.Finish) + strconv.Itoa(s.Finish.Year)
}

// sprintPhase tells whether the Sprint is current, upcoming or past on
// today and whether it is closed, e.g. "current · closed". A past Sprint is
// "closed" if it is, and "past" only while it is still open.
func sprintPhase(s domain.Sprint, today domain.Date) string {
	var parts []string
	switch {
	case s.Contains(today):
		parts = append(parts, "current")
	case s.Start.Compare(today) > 0:
		parts = append(parts, "upcoming")
	case !s.Closed:
		parts = append(parts, "past")
	}
	if s.Closed {
		parts = append(parts, "closed")
	}
	return strings.Join(parts, " · ")
}

// sprintItems lists the Sprints for the Sprint picker with their date
// ranges and phases in aligned columns.
func sprintItems(sprints []domain.Sprint, today domain.Date) []pickerItem {
	nameW := 0
	for _, s := range sprints {
		nameW = max(nameW, ansi.StringWidth(plainText(s.Name)))
	}
	notes := sprintNotes(sprints, today)
	items := make([]pickerItem, len(sprints))
	for i, s := range sprints {
		items[i] = pickerItem{id: s.ID, name: fit(plainText(s.Name), nameW), note: notes[i]}
	}
	return items
}

// sprintNotes returns the date range and phase of each Sprint, with the
// phases aligned in a column.
func sprintNotes(sprints []domain.Sprint, today domain.Date) []string {
	rangeW := 0
	for _, s := range sprints {
		rangeW = max(rangeW, len(dateRange(s)))
	}
	notes := make([]string, len(sprints))
	for i, s := range sprints {
		note := fit(dateRange(s), rangeW)
		if phase := sprintPhase(s, today); phase != "" {
			note += "  " + phase
		}
		notes[i] = strings.TrimRight(note, " ")
	}
	return notes
}

// taskboardLines returns the line of each row in the rendered list and the
// number of lines: a blank line stands before each User Story but the first.
func taskboardLines(rows []board.TaskboardRow) ([]int, int) {
	lineOf := make([]int, len(rows))
	n := 0
	for i, r := range rows {
		if r.Kind == board.RowUserStory && i > 0 {
			n++
		}
		lineOf[i] = n
		n++
	}
	return lineOf, n
}

// renderTaskboard renders the Taskboard into a w×h area, showing the list
// from line top and, if detail is set, the detail view. spin is the frame of
// the spinner on rows that are saving.
func renderTaskboard(tb board.Taskboard, top, w, h int, detail bool, spin string) string {
	if tb.Empty() {
		return centered(w, h,
			faintStyle.Render("No user stories in "+plainText(tb.Sprint().Name)), "",
			faintStyle.Render(": switch sprint · r refresh"))
	}
	l := layoutBoard(w, h, detail)
	list := taskList(tb.Rows(), top, l.listW, l.listH, spin)
	return withDetail(l, w, list, func(inner int, compact bool) []string {
		d, _ := tb.Detail()
		return taskboardDetail(d, inner, compact)
	})
}

// taskColumns are the widths of the columns of the rows. User Story rows
// share the Status column with the Task rows and show their progress in the
// last column, which is as wide as the Assignees or the progress need.
type taskColumns struct {
	ref, title, status, who int
	fullNames               bool
	// storyTitle is the width of the title of User Story rows, and
	// barless tells that their progress drops the bar to keep the titles.
	storyTitle int
	barless    bool
}

func columnsFor(rows []board.TaskboardRow, w int) taskColumns {
	c := taskColumns{fullNames: w >= fullNamesFrom}
	storyW, barW, countW := 0, 0, 0
	for _, r := range rows {
		switch r.Kind {
		case board.RowTask:
			c.ref = max(c.ref, len(ref(r.Ref)))
			c.status = max(c.status, ansi.StringWidth(ansi.Strip(statusLabel(r.Status))))
			c.who = max(c.who, ansi.StringWidth(taskAssignee(r.Assignees, c.fullNames, false)))
		case board.RowUserStory:
			if r.ID != 0 {
				c.status = max(c.status, ansi.StringWidth(ansi.Strip(statusLabel(r.Status))))
			}
			storyW = max(storyW, ansi.StringWidth(storyTitle(r)))
			if p := storyProgress(r, false, false); p != "" {
				barW = max(barW, ansi.StringWidth(ansi.Strip(p)))
				countW = max(countW, ansi.StringWidth(ansi.Strip(storyProgress(r, true, false))))
			}
		}
	}
	c.status = min(c.status, maxStatusWidth)
	c.who = min(c.who, maxNameWidth)
	// The bar goes first when a User Story title would not fit with it.
	c.barless = w-6-c.status-max(c.who, barW) < storyW
	if c.barless {
		c.who = max(c.who, countW)
	} else {
		c.who = max(c.who, barW)
	}
	// indent and marker, ref, title and the two right columns
	c.title = w - 4 - c.ref - 1 - 2 - c.status - 2 - c.who - 1
	c.storyTitle = w - 1 - 2 - c.status - 2 - c.who - 1
	return c
}

// taskList renders the rows as w×h lines from line top, with spin in front
// of rows that are saving.
func taskList(rows []board.TaskboardRow, top, w, h int, spin string) string {
	cols := columnsFor(rows, w)
	lineOf, n := taskboardLines(rows)
	all := make([]string, n)
	for i, r := range rows {
		var line string
		switch r.Kind {
		case board.RowUserStory:
			line = userStoryRow(r, cols, w, spin)
		case board.RowTask:
			line = taskRow(r, cols, w, spin)
		case board.RowNoTasks:
			line = "    " + faintStyle.Render("No tasks")
		}
		all[lineOf[i]] = line
	}
	lines := make([]string, 0, h)
	for i := top; i < n && len(lines) < h; i++ {
		lines = append(lines, fit(all[i], w))
	}
	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	return strings.Join(lines, "\n")
}

// storyTitle is the collapse arrow, ref and title of a User Story row, or
// the name of the group of Tasks without User Story.
func storyTitle(r board.TaskboardRow) string {
	arrow := "▾"
	if !r.Expanded {
		arrow = "▸"
	}
	if r.ID == 0 {
		return arrow + " " + noUserStory
	}
	return arrow + " " + ref(r.Ref) + " " + plainText(r.Title)
}

// storyProgress renders the Task progress of a User Story row: the bar and
// the count, or just the count if barless. It is empty for an expanded User
// Story without Tasks, which says so in the row below.
func storyProgress(r board.TaskboardRow, barless, styled bool) string {
	if r.Tasks.Total == 0 && r.Expanded {
		return ""
	}
	if barless && r.Tasks.Total > 0 {
		count := fmt.Sprintf("%d/%d", r.Tasks.Done, r.Tasks.Total)
		if styled {
			return faintStyle.Render(count)
		}
		return count
	}
	return progress(r.Tasks, styled)
}

// userStoryRow renders the heading of a group: the collapse arrow, ref and
// title in bold accent color, the Status label in the Status column of the
// Tasks and the Task progress on the right.
func userStoryRow(r board.TaskboardRow, c taskColumns, w int, spin string) string {
	prog := storyProgress(r, c.barless, !r.Selected)
	status := ""
	if r.ID != 0 {
		status = statusLabel(r.Status)
	}
	progW := ansi.StringWidth(ansi.Strip(prog))
	right := fit(status, c.status) + "  " + strings.Repeat(" ", max(0, c.who-progW)) + prog + " "
	marker := savingMarker(r.Saving, spin)
	if r.Selected {
		return cursorStyle.Bold(true).Render(fit(ansi.Strip(marker)+fit(storyTitle(r), c.storyTitle)+"  "+ansi.Strip(right), w))
	}
	return marker + titleStyle.Render(fit(storyTitle(r), c.storyTitle)) + "  " + right
}

// taskRow renders a Task: marker, ref, title, Status label and Assignee.
// Under the cursor the row loses its colors and is shown reversed.
func taskRow(r board.TaskboardRow, c taskColumns, w int, spin string) string {
	marker := " " + savingMarker(r.Saving, spin)
	if r.Mine {
		marker += mineStyle.Render("▍") + " "
	} else {
		marker += "  "
	}
	line := marker + faintStyle.Render(fit(ref(r.Ref), c.ref)) + " " + fit(plainText(r.Title), c.title) + "  " +
		fit(statusLabel(r.Status), c.status) + "  " + fit(taskAssignee(r.Assignees, c.fullNames, true), c.who) + " "
	if r.Selected {
		return cursorStyle.Render(fit(ansi.Strip(line), w))
	}
	return line
}

// taskAssignee names the Assignee of a Task by full name or initials, the
// logged-in Team Member in the accent color when styled.
func taskAssignee(as []board.Assignee, fullNames, styled bool) string {
	if len(as) == 0 {
		if styled {
			return faintStyle.Render("--")
		}
		return "--"
	}
	a := as[0]
	name := a.Initials
	if fullNames {
		name = plainText(a.Name)
	}
	if styled && a.Mine {
		return mineStyle.Render(name)
	}
	return name
}

// taskboardDetail renders the detail view of the selected Task or User
// Story for width w. compact saves lines for the detail view below the list.
func taskboardDetail(d board.TaskboardDetail, w int, compact bool) []string {
	heading := noUserStory
	if d.ID != 0 {
		heading = ref(d.Ref) + " " + plainText(d.Title)
	}
	lines := styleLines(labelStyle, wrap(heading, w))
	if !compact {
		lines = append(lines, "")
	}
	if d.Kind == board.RowTask {
		lines = append(lines, field("Status", statusLabel(d.Status), w)...)
		lines = append(lines, field("Assignee", assigneeNames(d.Assignees), w)...)
		userStory := faintStyle.Render("none")
		if d.UserStoryRef != 0 {
			userStory = ref(d.UserStoryRef) + " " + plainText(d.UserStoryTitle)
		}
		lines = append(lines, field("User Story", userStory, w)...)
		lines = append(lines, "")
		return append(lines, descriptionLines(d.Description, w)...)
	}
	if d.ID != 0 {
		lines = append(lines, field("Status", statusLabel(d.Status), w)...)
		lines = append(lines, field("Assignees", assigneeNames(d.Assignees), w)...)
	}
	lines = append(lines, field("Tasks", progress(d.Tasks, true), w)...)
	if len(d.TaskCounts) == 0 {
		return lines
	}
	lines = append(lines, "")
	return append(lines, taskCounts(d.TaskCounts, w)...)
}

// taskCounts lists the number of Tasks per Status in aligned columns.
func taskCounts(counts []board.StatusCount, w int) []string {
	nameW := 0
	for _, c := range counts {
		nameW = max(nameW, ansi.StringWidth(ansi.Strip(statusLabel(c.Status))))
	}
	nameW = min(nameW, max(1, w-6))
	lines := make([]string, len(counts))
	for i, c := range counts {
		lines[i] = fit(statusLabel(c.Status), nameW) + "  " + fmt.Sprintf("%3d", c.Count)
	}
	return lines
}
