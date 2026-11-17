# Roadmap dependencies and external feature signals

Research for issue #92 (part of the map #90).
Question: how do the README roadmap items depend on each other and on the current code, where does each one land in the architecture, does it need a structural change first, and which external signals exist for prioritizing them?
This document records facts and dependencies only; it does not prioritize.
Taiga API facts per item (endpoints, taiga-events, Markdown flavor) belong to the sibling research #91 and are only referenced here where they decide a dependency.

## Sources

- The codebase on `main` at `4010f9f` (abbreviated `CODE`): https://github.com/zeckitoz/taiga-tui/tree/4010f9f980785186c6af3d807fb9357d3e7cf796
- `README.md` section "Roadmap", `GLOSSARY.md` and `docs/adr/0001-board-effects-as-data.md` at the same commit.
- All 95 GitHub issues and their comments of zeckitoz/taiga-tui, read on 2026-11-17, in particular the MVP map #1, the module grilling #14, the interaction grilling #11, the MVP spec #18, the roadmap grilling #67 and the v1.0.0 map #64.
- GitHub API for the repository metadata (stars, forks, Discussions) and the pull request authors, read on 2026-11-17.
- The Go module proxy for glamour: https://proxy.golang.org/charm.land/glamour/v2/@v/v2.0.1.mod

## TL;DR

- No roadmap item hard-blocks another; every item can be built alone on today's code.
- Three structural changes are shared by several items and should be built by whichever item comes first:
  - **S1, a scrollable detail pane:** the detail pane is read-only and cuts content off with "…" ([`internal/ui/detail.go`](https://github.com/zeckitoz/taiga-tui/blob/4010f9f980785186c6af3d807fb9357d3e7cf796/internal/ui/detail.go#L28-L47)); rendered Markdown, comments and edit forms all need more room than it has.
  - **S2, a general change to a work item:** a save is hard-wired to "new Status or new Assignee", and the value zero means "unchanged" from `board.Save` down to the JSON body; Unassign, undo and editing all need a change that can carry other fields and an explicit "clear".
  - **S3, more than two kinds of work items and boards:** `board.ItemKind`, `domain.Project` and `config.Board` know exactly User Story/Task and Kanban Board/Taskboard; Epics and Issues need these opened up.
- Undo reverses an explicit MVP decision ("no confirmation and no undo for a Status Change", #11 and #18 story 81), so the maintainer has to revisit that decision before undo is specified.
- External signals: none.
  Every issue, comment and pull request in the repository is by the maintainer, Discussions are disabled, and the repository has 1 star and 0 forks.
  The only signals are internal: the MVP's Out of Scope list (#1, #18), from which the roadmap was drawn (#67), and one non-roadmap item, signing and notarization (#74).

## Dependency graph

Legend: `A ==> B` means B cannot ship without A (hard), `A --> B` means B is easier or better with A (soft), and `S1` to `S3` are the shared structural changes above.

```text
                      S1 scrollable detail pane
                     /            |             \
                    v             v              v
   Rendered Markdown     Comments (read)     Create/Edit forms
          |                       ^                 ^
          '-- soft: preview ------'-----------------'
                                                    |
                      S2 general change to a work item
                     /            |             \
                    v             v              v
             Unassign      Undo Status Change    Edit (fields)
                                  |
                                  '-- soft: undo of an edit reuses the same inverse change

                      S3 more kinds of work items and boards
                                  |
                                  v
                            Epics and Issues

   Live updates      (independent; soft: fewer conflicts for every write item)
   taiga-cli         (independent; soft: reuses every client method the TUI items add)
```

There is no `==>` edge between roadmap items.
Every hard edge runs from a structural change to an item, and each structural change can be built as part of the first item that needs it.

### Answers to the ticket's example questions

- **Does editing need rendered Markdown first?**
  No.
  Taiga stores descriptions as Markdown text (`domain.UserStory.Description`, `domain.Task.Description` in [`internal/domain/domain.go`](https://github.com/zeckitoz/taiga-tui/blob/4010f9f980785186c6af3d807fb9357d3e7cf796/internal/domain/domain.go#L89-L139)), and an editor edits that raw text.
  Rendering is a soft dependency: after an edit the Team Member sees the result in the detail pane, which today shows the raw text through `plainText` and `wrap` ([`descriptionLines`](https://github.com/zeckitoz/taiga-tui/blob/4010f9f980785186c6af3d807fb9357d3e7cf796/internal/ui/detail.go#L94-L106)).
  Both share S1.
- **Does undo need a shared notion of reversible actions that create/edit would reuse?**
  Only partly.
  The undo of a Status Change, an Assign to me or a field edit is the inverse change with the version Taiga returned after the save, which fits S2 directly: once a change can carry any field, its inverse is the same kind of change with the old values.
  The undo of a create is a delete, which is a different effect and a different request, so a create does not reuse the inverse-change mechanism.
  Comments in Taiga live in the history of an item (details in #91), so undoing a comment is again a different request.
- **Would Epics and Issues need new board types or only new detail views?**
  It depends on the item, and the maintainer decides.
  Issues have their own Statuses in Taiga, so a list grouped by Status, like the Kanban Board, is the natural view; that is a new board type.
  Epics relate to User Stories, so the minimum is a detail view (the Epic of a User Story, the User Stories of an Epic); an Epic list is optional.
  Either way S3 comes first, see below.

## Where each item lands

The architecture in short ([ADR 0001](https://github.com/zeckitoz/taiga-tui/blob/4010f9f980785186c6af3d807fb9357d3e7cf796/docs/adr/0001-board-effects-as-data.md), module grilling #14):
`internal/board` is a pure state machine that returns effects as data, `internal/ui` translates effects into Bubble Tea commands that call the `ui.Taiga` interface, `internal/taiga` is the HTTP client and `internal/domain` holds the glossary types.
The ADR's consequence for new work is explicit: "Whoever adds new actions extends `board` with effect and result types instead of firing requests from UI code."

### Rendered Markdown

- **Lands in:** `internal/ui/detail.go` only, in `descriptionLines`, which today prints `plainText(d.Text)` wrapped to the pane width.
  `internal/board` is untouched; it already loads and stores the Markdown text per item with generations ([`internal/board/detail.go`](https://github.com/zeckitoz/taiga-tui/blob/4010f9f980785186c6af3d807fb9357d3e7cf796/internal/board/detail.go)).
- **Library fit:** glamour has a v2 line at `charm.land/glamour/v2` (v2.0.1, 2026-06-12) that requires `charm.land/lipgloss/v2`, the same Charm v2 stack as [`go.mod`](https://github.com/zeckitoz/taiga-tui/blob/4010f9f980785186c6af3d807fb9357d3e7cf796/go.mod), so no second lipgloss major version comes in.
  It adds goldmark, chroma and bluemonday as dependencies ([glamour v2.0.1 go.mod](https://proxy.golang.org/charm.land/glamour/v2/@v/v2.0.1.mod)).
- **Structural change first:** S1.
  `detailPane` renders exactly `h` lines and cuts the rest off with "…"; rendered Markdown with headings, lists and code blocks is longer than the wrapped plain text, so long descriptions become unreadable without scrolling.
- **Further points the design has to settle:**
  - The control-character filter in `plainText` protects the terminal from text stored in Taiga and must still run before or inside rendering.
  - The theme has fixed colors and no light/dark detection ([`internal/ui/theme.go`](https://github.com/zeckitoz/taiga-tui/blob/4010f9f980785186c6af3d807fb9357d3e7cf796/internal/ui/theme.go)), while glamour needs a style choice.
  - On the Taskboard only Tasks load a description; User Story rows deliberately do not ([`Taskboard.LoadDetail`](https://github.com/zeckitoz/taiga-tui/blob/4010f9f980785186c6af3d807fb9357d3e7cf796/internal/board/taskboard.go#L507-L521)).
  - The UI golden tests that contain descriptions change.

### Live updates

- **Lands in:** a new long-running subscription owned by `internal/ui` (connect on project open, reconnect, re-authenticate with the token source), plus a new client next to `internal/taiga` for taiga-events.
  Events enter `board` as new result types, as ADR 0001 requires.
  The landing points already exist: `Kanban.Replace`, `Taskboard.ReplaceTask` and `Taskboard.ReplaceUserStory` take a single changed item ([`internal/board/kanban.go`](https://github.com/zeckitoz/taiga-tui/blob/4010f9f980785186c6af3d807fb9357d3e7cf796/internal/board/kanban.go#L481-L492)), and `Reload` takes a whole list while keeping the cursor.
- **Structural change first:** none in `board`.
  Optimistic saves already overlay the shown value until their result arrives (`saves.status`, `saves.assignees` in [`internal/board/status.go`](https://github.com/zeckitoz/taiga-tui/blob/4010f9f980785186c6af3d807fb9357d3e7cf796/internal/board/status.go#L301-L326)), so a live replacement during a save does not flicker.
  The new part is connection lifecycle in `ui`, which today only knows request/response commands.
- **Relations:** independent of all other items.
  It makes version conflicts rarer for every write item (Status Change today, Unassign, undo and editing later).
  Whether typical Instances run taiga-events at all is a possible hard blocker and is researched in #91.
- **Signal:** the MVP excluded it deliberately ("manual and automatic reloads are enough", #1 and #18 Out of Scope).

### Creating and editing User Stories and Tasks, plus comments

- **Lands in:**
  - `internal/taiga`: new create, edit and comment requests; today the client has only reads and two PATCHes (`PatchTask`, `PatchUserStory` in [`internal/taiga/client.go`](https://github.com/zeckitoz/taiga-tui/blob/4010f9f980785186c6af3d807fb9357d3e7cf796/internal/taiga/client.go#L382-L403)).
  - `internal/board`: new actions and effects (`Create…`, an edit save, `PostComment`) with the existing lock-per-item, conflict and reload rules.
  - `internal/ui`: forms; `bubbles` is already a dependency and its text area fits, but there is no form beyond the login form ([`internal/ui/loginform.go`](https://github.com/zeckitoz/taiga-tui/blob/4010f9f980785186c6af3d807fb9357d3e7cf796/internal/ui/loginform.go)), and the detail pane is explicitly read-only.
- **Structural changes first:** S1 for reading comments and for forms that need more room than the pane, and S2 for editing fields.
  A create is a new kind of effect: there is no item, no version and no ID yet, so it does not fit `Save` and the per-item lock; the board has to place the new item once Taiga returns it.
- **Relations:** soft on Rendered Markdown (preview of an edited description or comment).
  Shares S2 with Unassign and undo.
  Benefits from Live updates (fewer conflicts on longer edits).
- **Signal:** excluded in the MVP (#1, #18 Out of Scope: "Creating or editing User Stories or Tasks (beyond Status Change and Take Over), comments, attachments").
  Attachments were in the same Out of Scope line but did not make it into the roadmap (#67).

### Unassign and undo for a Status Change

- **Lands in:** `internal/board/status.go` next to `assignToMe` and `changeStatus`, plus key handling in `internal/ui/status.go`.
- **Structural change first (Unassign):** S2.
  The value zero means "unchanged" throughout:
  - `domain.TaskPatch` and `domain.UserStoryPatch` say "A zero field stays as it is" ([`internal/domain/domain.go`](https://github.com/zeckitoz/taiga-tui/blob/4010f9f980785186c6af3d807fb9357d3e7cf796/internal/domain/domain.go#L146-L162)).
  - `patchDTO` sends `assigned_to` and `assigned_users` with `omitempty`, so "no Assignee" cannot be sent ([`internal/taiga/client.go`](https://github.com/zeckitoz/taiga-tui/blob/4010f9f980785186c6af3d807fb9357d3e7cf796/internal/taiga/client.go#L373-L380)).
  - `board.Save.Assignee` is 0 for a Status Change, and `saves.assignee` shows the saved Assignee only if it is not 0 ([`internal/board/status.go`](https://github.com/zeckitoz/taiga-tui/blob/4010f9f980785186c6af3d807fb9357d3e7cf796/internal/board/status.go#L319-L326)).
  Unassigning a Task (Assignee becomes none) and removing the last Assignee of a User Story (empty list) both need an explicit "clear".
  Removing oneself from a User Story with several Assignees fits today's model, because `Save.Assignees` already carries the full list.
- **Structural change first (undo):** a record of the last finished change, holding the old value and the version Taiga returned.
  `Save` holds the version as loaded and the new Status, but not the old one, and nothing keeps a save after its result arrives.
  Since `board.Sent` already spans boards and projects and lives in the App for the whole run ([`internal/board/status.go`](https://github.com/zeckitoz/taiga-tui/blob/4010f9f980785186c6af3d807fb9357d3e7cf796/internal/board/status.go#L137-L150)), an undo record that survives leaving the board would sit next to it.
  The undo itself is an inverse `Save` and reuses the existing conflict handling: if someone changed the item in between, Taiga rejects the stale version and the board reloads.
- **Decision to revisit:** the interaction grilling decided "Kein Undo" and "Kein 'Abgeben' im MVP" (#11), and the MVP spec has story 81 "no confirmation and no undo for a Status Change, so that the interaction stays fast and simple" (#18).
  The roadmap grilling kept "Unassign" out of `GLOSSARY.md` until it is built (#67).
- **Relations:** shares S2 with editing; an undo built on S2 extends to edits for free, but not to creates or comments.

### Epics and Issues next to User Stories and Tasks

- **Lands in:** every layer, because the two-kinds assumption is everywhere:
  - `domain`: `Project` has only `UserStoryStatuses`, `TaskStatuses` and `Permissions{ModifyTask, ModifyUserStory}` ([`internal/domain/domain.go`](https://github.com/zeckitoz/taiga-tui/blob/4010f9f980785186c6af3d807fb9357d3e7cf796/internal/domain/domain.go#L28-L44)).
  - `board`: `ItemKind` has `UserStoryItem` and `TaskItem`, and `mayModify`, `statusesOf` and `noPermission` branch on exactly those two ([`internal/board/status.go`](https://github.com/zeckitoz/taiga-tui/blob/4010f9f980785186c6af3d807fb9357d3e7cf796/internal/board/status.go#L16-L23)).
    `Kanban` is typed on `domain.UserStory` throughout, so an Issues board grouped by Status would either duplicate it or need a generic Status-grouped list.
  - `config`: `Board` has the values `kanban` and `taskboard`, persisted in `state.json` as the last board ([`internal/config/state.go`](https://github.com/zeckitoz/taiga-tui/blob/4010f9f980785186c6af3d807fb9357d3e7cf796/internal/config/state.go#L33-L51)).
  - `ui`: `boardOps` is "the one place that tells the boards apart" ([`internal/ui/boards.go`](https://github.com/zeckitoz/taiga-tui/blob/4010f9f980785186c6af3d807fb9357d3e7cf796/internal/ui/boards.go#L59-L61)), so a new board is a new `boardView` plus command menu entries.
- **Structural change first:** S3.
  The module grilling planned a shared core `Items` under `Taskboard` and `Kanban` (#14, point 4), but the code shares only helpers (`saves`, `workItem`, `descriptions`, `Sent`); a third and fourth kind is where such a core would pay off.
- **Glossary:** `GLOSSARY.md` has no entry for Epic or Issue; both need one before design, and "Issue" collides with GitHub issues in this repository's own process.
- **Relations:** independent of the other items.
  Each write item (Status Change, Assign to me, Unassign, editing) multiplies by the number of kinds, so building S3 before the write items avoids repeating the two-kinds switch, and building it after means revisiting them.
- **Signal:** excluded in the MVP ("Taiga issues and epics", #1, #18).

### taiga-cli

- **Lands in:** a separate binary.
  Everything is under `internal/` with "no public Go API" (#14, point 1), so a `cmd/taiga-cli` in this module can import `taiga`, `domain`, `session` (keyring login) and `config`, while a separate repository cannot without first moving packages out of `internal/`.
  `board` and `ui` are not needed; ADR 0001's state machine is TUI-specific.
- **Structural change first:** none for a CLI in this module; promoting packages to a public API only if it lives elsewhere.
- **Relations:** independent; soft on every item that adds client methods (editing, comments, Epics and Issues, Unassign), because each one is also a CLI command.
  The existing-clients research names `KoukeNeko/taiga-cli` (Go, MIT) as the best reference ([`docs/research/existing-clients.md`](https://github.com/zeckitoz/taiga-tui/blob/4010f9f980785186c6af3d807fb9357d3e7cf796/docs/research/existing-clients.md), #7).
- **Signal:** the roadmap grilling labelled it the "furthest idea" (#67).

## External signals

- **Third parties:** none.
  All 95 issues, all their comments and all pull requests were written by the maintainer.
  GitHub Discussions are disabled.
  The repository has 1 star, 0 forks and 0 watchers.
  The README's invitation "Open an issue for what you want most" has not been answered yet.
- **Internal origin of the roadmap:** the six items were drawn from the MVP spec's Out of Scope list (#18) in the roadmap grilling (#67).
  The same grilling dropped swimlanes, remembered collapsed Statuses and offline mode ("maintainer does not plan them"), mouse support, the apt repository (since shipped, #81), notarization and self-update.
- **Not on the roadmap:**
  - Attachments: in the MVP's Out of Scope line next to editing and comments (#18), not carried into the roadmap (#67).
  - Signing and notarizing release binaries: "v1.0.0 ships unsigned ... Not in the Roadmap yet" (#64 Out of scope, #74), with leanings recorded for a later effort (Apple Developer ID, SignPath Foundation).
  - A homebrew-core submission is mentioned as "mostly a PR" once the source-build formula exists (#80); it is distribution, not a feature.
- **Decisions the roadmap would reverse:** "no undo" and "no Abgeben" (#11, #18 story 81), and "no live updates" (#1, #18).
  These were MVP scope decisions, not rejections, but the specs give their reasons ("fast and simple", "reloads are enough").

## Open questions for the maintainer

These are not decided here; they belong to the grilling tickets #93 and #94.

- Whether Issues get their own board and Epics only a detail view, or both get boards.
- Whether undo covers only the last Status Change, or every change including Assign to me, Unassign and edits.
- Whether the shared structural changes S1 to S3 are built as their own preparatory step or inside the first item that needs them.
