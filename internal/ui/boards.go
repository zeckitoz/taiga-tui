package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/zeckitoz/taiga-tui/internal/board"
	"github.com/zeckitoz/taiga-tui/internal/config"
	"github.com/zeckitoz/taiga-tui/internal/domain"
)

// pathSep separates the parts of the header path.
var pathSep = faintStyle.Render(" › ")

// boardView is what the App does differently on each board.
type boardView struct {
	// name and shortName name the board in texts, e.g. "Kanban Board" and
	// "Kanban".
	name, shortName string
	enabled         func(domain.ProjectSummary) bool
	// open shows the board of the project, loading it if needed.
	open       func(App) (App, tea.Cmd)
	update     func(App, tea.KeyPressMsg) (App, tea.Cmd)
	loadDetail func(App) (App, tea.Cmd)
	// reload reloads the board while it stays visible.
	reload func(App) (App, tea.Cmd)
	// savedResult feeds the result of a save back, and dropSaves forgets
	// the saves when the Team Member leaves the board.
	savedResult func(App, savedMsg) (App, board.Effect, bool)
	dropSaves   func(App) App
	// statusOptions lists the Statuses the selected row can be changed to,
	// pickStatus moves it to one of them.
	statusOptions func(App) (board.StatusOptions, error)
	pickStatus    func(App, int64) (App, board.Effect, error)
	// assignToMe makes the Team Member the Assignee of the selected row.
	assignToMe func(App, board.ConfirmAssignToMe) (App, board.Effect, error)
	// scroll keeps the selected row visible.
	scroll func(*App)
	// path is the header after the project name.
	path func(App) string
	body func(App, int) string
	keys func(App) []string
	// help lists the keys of the board for the full help.
	help []keyHelp
}

// boardOps returns what the App does on board b. It is the one place that
// tells the boards apart.
func boardOps(b config.Board) boardView {
	if b == config.BoardTaskboard {
		return boardView{
			name:          "Taskboard",
			shortName:     "Taskboard",
			enabled:       func(p domain.ProjectSummary) bool { return p.BacklogEnabled },
			open:          App.openTaskboard,
			update:        App.updateTaskboard,
			loadDetail:    App.loadTaskDetail,
			reload:        App.reloadTaskboard,
			statusOptions: App.statusOptionsTaskboard,
			pickStatus:    App.pickTaskboardStatus,
			assignToMe:    App.assignToMeTaskboard,
			savedResult:   App.taskboardSaved,
			dropSaves: func(a App) App {
				return a.withTaskboard(board.Taskboard.DropSaves)
			},
			scroll: (*App).scrollTaskboard,
			path:   App.taskboardPath,
			body:   App.taskboardBody,
			keys:   App.taskboardFooter,
			help:   taskboardHelp,
		}
	}
	return boardView{
		name:          "Kanban Board",
		shortName:     "Kanban",
		enabled:       func(p domain.ProjectSummary) bool { return p.KanbanEnabled },
		open:          App.loadKanban,
		update:        App.updateKanban,
		loadDetail:    App.loadUserStoryDetail,
		reload:        App.loadKanban,
		statusOptions: App.statusOptionsKanban,
		pickStatus:    App.pickKanbanStatus,
		assignToMe:    App.assignToMeKanban,
		savedResult:   App.kanbanSaved,
		dropSaves: func(a App) App {
			a.kanban = a.kanban.DropSaves()
			return a
		},
		scroll: (*App).scrollKanban,
		path:   App.kanbanPath,
		body:   App.kanbanBody,
		keys:   App.kanbanFooter,
		help:   kanbanHelp,
	}
}

// allBoards are the boards in the order the palette offers them.
var allBoards = []config.Board{config.BoardTaskboard, config.BoardKanban}

// otherBoard returns the board that is not b.
func otherBoard(b config.Board) config.Board {
	if b == config.BoardTaskboard {
		return config.BoardKanban
	}
	return config.BoardTaskboard
}

// shownBoard returns what the App does on the board it shows.
func (a App) shownBoard() boardView { return boardOps(a.board) }
