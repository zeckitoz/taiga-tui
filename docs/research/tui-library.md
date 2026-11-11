# Research: Go TUI library for the Taskboard and Kanban Board

Ticket: #5 (part of map #1).
Date of research: 2026-11-11.
All repository metrics were read from the GitHub API on that date.

## Question

Which Go TUI library (Bubble Tea plus Bubbles/Lip Gloss, tview, gocui, others) suits a board with many Status columns and one row per User Story best?
Criteria: horizontal and vertical scrolling, narrow terminals, focus and navigation, overlays/dialogs, Windows Terminal support, testability (e.g. teatest), maintenance state.

## Recommendation

Use **Bubble Tea v2** (`charm.land/bubbletea/v2`) with **Lip Gloss v2** for layout and selected **Bubbles v2** components (viewport, help, key, textinput, list), and write the Taskboard / Kanban Board grid as our own component.
Test the board logic as pure `Update`/`View` functions with plain Go tests, and add a thin layer of end-to-end golden-file tests with `teatest/v2`.

Reasoning in short:

- No candidate ships a widget for "Status columns x User Story rows with several Task cards per cell".
  In every library the board is custom code.
  The question is therefore which library makes custom layout, state and tests cleanest, not which one has the closest widget.
- Bubble Tea's Elm architecture keeps the whole board state (cursor row/column/card, scroll offsets, filter "only mine", open overlay) in one plain Go value.
  Navigation, scrolling and Status Change logic become pure functions that are trivially unit-testable without a terminal.
  This fits the user's priority of quality and long-term maintainability.
- It is by far the most actively maintained option, with a fresh major version (v2.0.0 on 2026-02-24, v2.1.1 on 2026-10-28), Windows in CI, and the largest ecosystem.
- tview is the strongest alternative (retained-mode widgets, built-in focus, `Table`, `Pages`, `Modal`), but its widgets do not cover the board shape either, it is effectively a single-maintainer project that recommends tracking `@master`, and it is still on tcell v2 while tcell itself moved to v3.
- gocui is effectively unmaintained upstream and is ruled out.

## Candidates

### Bubble Tea v2 + Lip Gloss v2 + Bubbles v2 (Charm)

Maintenance (GitHub API, 2026-11-11):

| Repo | Stars | Last push | Latest release | License |
|---|---|---|---|---|
| charmbracelet/bubbletea | 45.7k | 2026-11-09 | v2.1.1 (2026-10-28) | MIT |
| charmbracelet/lipgloss | 12.0k | 2026-11-09 | v2.1.0 (2026-11-09) | MIT |
| charmbracelet/bubbles | 9.0k | 2026-11-08 | v2.2.1 (2026-08-24) | MIT |

- v2 moved to vanity import paths (`charm.land/bubbletea/v2`) and requires Go 1.26 per its `go.mod`.
  Source: https://github.com/charmbracelet/bubbletea/releases/tag/v2.0.0 and https://github.com/charmbracelet/bubbletea/blob/main/go.mod
- New "Cursed Renderer" based on the ncurses algorithm; built-in color downsampling via colorprofile; Lip Gloss v2 is pure and Bubble Tea owns all I/O (fixes v1 I/O races).
  Source: https://github.com/charmbracelet/bubbletea/releases/tag/v2.0.0
- Declarative `tea.View` struct (alt screen, mouse mode, focus reporting are fields, not commands).
  Source: same release notes.
- Key handling: `tea.KeyPressMsg` / `KeyReleaseMsg`, progressive keyboard enhancements where supported, and key repeat info via the Windows Console API on Windows.
  Source: same release notes.

Criteria:

- **Horizontal/vertical scrolling:** Bubbles `viewport` supports vertical and horizontal scrolling (`xOffset`, `SetHorizontalStep`, `HorizontalScrollPercent`, ANSI-aware line cutting with `ansi.Cut`).
  Source: https://github.com/charmbracelet/bubbles/blob/main/viewport/viewport.go
  For a board, the cleaner approach is to scroll by *columns and rows* in our own model (render only the visible Status columns and User Story rows), and keep the cursor always visible.
  That is a few dozen lines of pure code and works the same on every terminal width.
- **Narrow terminals:** `tea.WindowSizeMsg` drives layout; Lip Gloss offers `Width`, `MaxWidth`, `lipgloss.Width()` measurement, `JoinHorizontal` / `JoinVertical` and `Place`.
  Source: https://github.com/charmbracelet/lipgloss (README sections "Width and Height", "Joining Paragraphs", "Measuring Width and Height").
  We decide the policy ourselves (e.g. minimum column width, show N columns, collapse to a single-Status list view below a threshold).
- **Focus and navigation:** no built-in focus manager.
  Focus is explicit state in the model, which for a 2D board (row, column, card index) is what we want anyway; `bubbles/key` and `bubbles/help` give keymaps and a help bar.
- **Overlays/dialogs:** Lip Gloss v2 includes a cell-based compositor (`lipgloss.NewLayer(...).X().Y().Z()`, `Compose`) for layered rendering, which covers the read-only detail view and a Status picker as overlays.
  Source: https://github.com/charmbracelet/lipgloss README (cell-based compositor section) and `examples/canvas`.
  `charm.land/huh/v2` provides forms if needed later (https://github.com/charmbracelet/huh).
- **Windows:** the shared Charm CI workflow builds and tests on `ubuntu-latest`, `macos-latest`, `windows-latest`.
  Source: https://github.com/charmbracelet/meta/blob/main/.github/workflows/build.yml (referenced from bubbletea's `build.yml`).
  Bubble Tea has dedicated `tty_windows.go`, `termios_windows.go`, `signals_windows.go`.
- **Testability:**
  - Model logic: `Update(msg)` is a pure function, so tests send `tea.KeyPressMsg` values and assert on the resulting model and on `View()` strings.
  - End-to-end: `github.com/charmbracelet/x/exp/teatest/v2` provides `NewTestModel`, `WithInitialTermSize`, `Send`, `Type`, `WaitFor`, `FinalModel`, `FinalOutput` and golden-file assertions via `RequireEqualOutput`.
    Source: https://github.com/charmbracelet/x/blob/main/exp/teatest/v2/teatest.go
  - Caveat: teatest lives under `exp/` (no API stability promise; last change 2026-03-11, its `go.mod` still lists `charm.land/bubbletea/v2 v2.0.0-rc.1` as minimum).
    Keep teatest use thin and isolated so an API change is cheap.
  - `tea.WithWindowSize(w, h)` sets the initial size for tests without mocking a terminal (v2 release notes, section "Window Size for Testing").

### tview (on tcell)

| Repo | Stars | Last push | Latest release | License |
|---|---|---|---|---|
| rivo/tview | 14.1k | 2026-08-11 | v0.42.0 (2025-08-27) | MIT |
| gdamore/tcell | 5.2k | 2026-11-09 | v3.6.0 (2026-10-11) | Apache-2.0 |

- Widgets: forms, text views, `Table` (fixed rows/columns via `SetFixed`, `SetOffset`, cell/row selection), tree, list, `Grid`, `Flex`, `Pages`, `Modal`, `Application` wrapper with focus handling.
  Source: https://github.com/rivo/tview (README) and https://github.com/rivo/tview/blob/master/table.go
- README instructs `go get github.com/rivo/tview@master`; releases are rare and most consumers pin pseudo-versions (e.g. jira-cli pins `v0.0.0-20240406141410-79d4cc321256`).
- `go.mod` still requires `github.com/gdamore/tcell/v2 v2.8.1`, while tcell's main line is v3 (v2 remains available as a separate branch).
  Source: https://github.com/rivo/tview/blob/master/go.mod and https://github.com/gdamore/tcell (README).
- Testing: `Application.SetScreen(tcell.Screen)` allows tcell's `SimulationScreen`, but tests drive a retained widget tree with callbacks, which is clunkier than testing a pure model.
- Windows: supported through tcell ("Modern Windows is supported", tcell has a Windows CI workflow).
- Fit for the board: `Table` cells are single text cells, so a cell holding several Task cards per User Story needs a custom `Primitive` anyway; `Flex` does not scroll.
  So the main advantage (ready widgets) mostly evaporates for our core view.
- Notable: k9s, the flagship tview app, runs on its own forks `derailed/tview` and `derailed/tcell`, which hints at the cost of extending tview beyond its widgets.

### gocui

- jroimartin/gocui: last commit 2025-05-01, no releases.
- awesome-gocui/gocui (community fork): last release v1.1.0 on 2022-01-13.
- lazygit, the best-known gocui app, vendors its own fork in-tree (`pkg/gocui` in jesseduffield/lazygit) rather than depending on either.
- Low-level views with manual layout, no widget set, no first-class test harness.
  Ruled out on maintenance grounds alone.

### Others considered briefly

- Raw tcell v3: maximum control, but we would rebuild what Bubble Tea plus Lip Gloss already give (event loop, layout, styling, test harness).
- No other Go TUI framework comes close to the two leaders in maintenance or ecosystem.

## Comparable board-like UIs

| Project | Library | Relevance |
|---|---|---|
| [dlvhdr/gh-dash](https://github.com/dlvhdr/gh-dash) (12.7k stars) | Bubble Tea v2 (`charm.land/bubbletea/v2 v2.0.2`) | Closest analogue: issue/PR dashboard over a REST/GraphQL API with sections, tables, preview pane, keybindings; already on v2. |
| [charmbracelet/kancli](https://github.com/charmbracelet/kancli) | Bubble Tea v1 | Minimal Kanban tutorial by Charm: Status columns built from `bubbles/list`, focus switching between columns. Good starting sketch, outdated API. |
| [ankitpokhrel/jira-cli](https://github.com/ankitpokhrel/jira-cli) (6.1k stars) | tview (pinned pseudo-version) + tcell v2 | Issue-tracker TUI with interactive tables; shows tview's table strengths, but no column board. |
| [derailed/k9s](https://github.com/derailed/k9s) (35k stars) | forked tview + tcell | Large table-centric TUI; demonstrates tview at scale, via forks. |
| [jesseduffield/lazygit](https://github.com/jesseduffield/lazygit) (83.8k stars) | in-tree gocui fork | Multi-panel layout and overlays; shows gocui only survives as a private fork. |

## Implications for the design

- Build one board component used by both Taskboard and Kanban Board: columns = Status, rows = User Story (Taskboard) or a single row / cards stacked (Kanban Board).
- Keep board state (cursor, scroll offsets, filters) in a pure struct with its own unit tests; render with Lip Gloss; overlays (detail view, Status picker) via the Lip Gloss compositor.
- Pin Go 1.26+ (required by Bubble Tea v2).

## Open points surfaced

- Narrow-terminal policy (minimum column width, fallback list view) is a UX decision, not a library question.
- Whether to support mouse input (Bubble Tea supports it; scope decision).
