# Board state returns effects as data

Status Change and Take Over need optimistic changes, a lock per item, a rollback on errors, a reload on conflicts and a reload of the project data on unknown IDs.
This logic lives entirely in `internal/board`, a pure state machine that imports neither Bubble Tea nor HTTP.
An action like `ChangeStatus` returns the new state and an effect as data (e.g. `SaveStatus`, `ReloadBoard`, `ReloadProject`) or a rejection.
The results of the requests (`Saved`, `Conflict`, `NotFound`, `Failed`) are fed back into `board` as input.
`internal/ui` only translates effects into `tea.Cmd`s that call the API client.
This makes the most delicate logic testable without a terminal and without an HTTP fake, and keeps it in one place instead of spreading it across the Taskboard and the Kanban Board.

## Considered Options

- **UI models call the API client directly in `Update` and do the rollback themselves.**
  This is less code, but the rules for locking, rollback and conflicts would be split across two boards and only testable through Bubble Tea tests.
  Rejected.

## Consequences

- Results carry a generation, and `board` discards results of stale loads.
  PATCH requests are not cancelled, because the change takes effect on the server anyway.
- Whoever adds new actions extends `board` with effect and result types instead of firing requests from UI code.
