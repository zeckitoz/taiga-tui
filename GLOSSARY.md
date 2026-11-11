# Taiga TUI

A local terminal UI for viewing and moving work items of a Taiga project, as an alternative to the drag-and-drop web UI.

## Language

### Work items

**User Story**:
A unit of user-facing functionality in a Taiga project; the card on the Kanban Board and the row on the Taskboard.
_Avoid_: Story, ticket, card

**Task**:
A sub-step of exactly one User Story, worked on within a Sprint.
_Avoid_: Subtask, todo, card

### Planning

**Sprint**:
A time-boxed iteration of a Scrum project that contains a set of User Stories and their Tasks.
_Avoid_: Milestone (Taiga's API name), iteration

**Current Sprint**:
The Sprint whose date range contains today.
_Avoid_: Active sprint

### Boards

**Taskboard**:
The view of one Sprint showing its Tasks, grouped by User Story and arranged by Status.
_Avoid_: Sprint board, Scrum board

**Kanban Board**:
The view of a project showing its User Stories arranged by Status.
_Avoid_: Board (unqualified)

**Status**:
The workflow state of a User Story or Task; each Status is one column on its board. User Stories and Tasks have separate sets of Statuses.
_Avoid_: Column, Spalte, state, lane

**Status Change**:
Moving a User Story or Task to a different Status.
_Avoid_: Drag, move, transition

### People

**Team Member**:
A person with their own Taiga account who belongs to the project.
_Avoid_: User, Nutzer, account

**Assignee**:
A Team Member responsible for a User Story or Task. A User Story can have several Assignees; a Task has at most one.
_Avoid_: Owner, Zuständiger, responsible

**Take Over**:
Making yourself the Assignee of a User Story or Task.
_Avoid_: Claim, grab, self-assign
