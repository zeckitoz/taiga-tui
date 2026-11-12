# Status Change and Assign to me via the Taiga API

Research for issue #4 (part of the MVP map #1).

Question: How does a client change the Status and the Assignee of a Task or User Story (PATCH with `version`), what does a version conflict look like, which order fields must be maintained, which bulk endpoints does the web UI use for drag-and-drop, and which permissions does a Team Member need?

## Sources

All claims are traced to these primary sources.
Line links point at fixed commits so they stay valid.

- Official API docs: https://docs.taiga.io/api.html (section "1.2. OCC - Optimistic concurrency control", endpoint summaries for User Stories and Tasks).
- taiga-back at commit `8ab6a4e3` (2026-11-03): https://github.com/taigaio/taiga-back/tree/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa
- taiga-front at commit `da8afa50` (2026-10-27): https://github.com/taigaio/taiga-front/tree/da8afa50b2a59c3aaec78854c0ae8145a16e6cd8

Abbreviations below: `BACK` = `https://github.com/taigaio/taiga-back/blob/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa`, `FRONT` = `https://github.com/taigaio/taiga-front/blob/da8afa50b2a59c3aaec78854c0ae8145a16e6cd8`.

The Ostfalia instance serves a front build tagged `v-1778767629306` (a build timestamp from May 2026), so it runs a current Taiga 6.x line and the code below applies.
This was checked with an unauthenticated GET of the start page only.

## Summary

- Status Change and Assign to me are plain `PATCH /api/v1/tasks/{id}` and `PATCH /api/v1/userstories/{id}` requests that send only the changed fields plus the `version` read last.
- The server maintains `kanban_order`, `taskboard_order`, `sprint_order` and `backlog_order` on its own when the Status changes; a client that does not care about the position inside a column can ignore them.
- A version conflict is HTTP `400` with body `{"version": "The version doesn't match with the current one"}`, but only if the conflicting earlier edits touched one of the fields this request sends; edits to other fields merge silently.
- The web UI's Kanban drag-and-drop uses `POST /api/v1/userstories/bulk_update_kanban_order` (no `version`, no OCC at all); the Taskboard drag-and-drop uses a normal `PATCH` on the Task plus an optional `set-orders` header.
- A Team Member needs `modify_task` (Tasks) or `modify_us` (User Stories); all default roles except "Stakeholder" have both.
- Assign to me on a Task sets `assigned_to`; on a User Story the web UI adds the user to `assigned_users` and sets `assigned_to` to the user, keeping other assignees.

Recommendation for the TUI: use `PATCH` with `version` for both Status Change and Assign to me, sending only `status` (or the assignee fields) plus `version`; do not use the bulk endpoints in the MVP; on a `400` with a `version` key, reload the item and tell the user.

## Optimistic concurrency control (OCC)

`TaskViewSet` and `UserStoryViewSet` both include `OCCResourceMixin` (`BACK/taiga/projects/tasks/api.py#L40`, `BACK/taiga/projects/userstories/api.py#L49`).
The mixin lives in `BACK/taiga/projects/occ/mixins.py#L16-L76`.

Behaviour on every update (`PUT` or `PATCH`) of an existing object:

1. `version` is read from the request body.
   If it is not an integer the response is `400 {"version": "The version must be an integer"}`.
2. If `version` is missing, negative, or greater than the current version, the response is `400 {"version": "The version parameter is not valid"}`.
   So `version` is effectively mandatory for updates.
3. If `version` equals the current version, the update is applied.
4. If `version` is older, the server takes the last `current - sent` history entries of the object and collects the field names they changed (`get_modified_fields`, `BACK/taiga/projects/history/services.py#L347-L364`).
   If any of those fields is also in the request body (ignoring `version`), the response is `400 {"version": "The version doesn't match with the current one"}`.
   Otherwise the update is applied anyway.
5. On success, `version` is incremented with an `F('version') + 1` expression and reloaded, so the response body carries the new `version`.

The docs describe the same semantics: "They are updating different attributes on the element. In this situation the API is smart enough for accepting both requests" (https://docs.taiga.io/api.html, section 1.2).

Error shape: `WrongArguments` is a `BadRequest`, which is a `BaseException` with `status_code = 400` (`BACK/taiga/base/exceptions.py#L126-L161`).
Because the detail is a dict, `format_exception` returns it unchanged (`BACK/taiga/base/exceptions.py#L218-L229`).
There is no `409 Conflict` and no `_error_type` for this case.

Consequence for the client: a conflict is detected by `status == 400` and the body having a `version` key.
Other validation errors are also `400` but are keyed by field name (for example `assigned_to`).

### Gaps in OCC

`bulk_update_kanban_order` changes `status`, `swimlane` and `kanban_order` with a queryset `UPDATE` that does not touch `version` (`BACK/taiga/projects/userstories/services.py#L350-L362`).
It writes a history snapshot asynchronously (`BACK/taiga/projects/userstories/services.py#L388-L399`), but the version stays the same.
So a web user dragging a card on the Kanban Board does not invalidate the TUI's `version`, and a later TUI `PATCH` of `status` with that same `version` passes step 3 and silently overwrites the drag.
Same in the other direction: the bulk endpoint never checks a version.
This is last-writer-wins; the TUI cannot detect it except by re-reading or by the events websocket.

## Status Change of a Task

Request (only changed fields plus `version`):

```http
PATCH /api/v1/tasks/1234
Authorization: Bearer <token>
Content-Type: application/json

{"status": 57, "version": 3}
```

Success: `200 OK` with the full Task detail serializer, including `"status": 57`, `"version": 4`, recalculated `"is_closed"` and `"finished_date"`.
Response header `Taiga-Info-Order-Updated` contains a JSON map `{"<task_id>": <order>, ...}` of every Task whose `taskboard_order` or `us_order` was rewritten (`BACK/taiga/projects/tasks/api.py#L153-L173`).

Server side effects:

- The status must belong to the Task's project, else `400 "You don't have permissions to set this status to this task."` (`BACK/taiga/projects/tasks/api.py#L98-L108`).
- `finished_date` is set or cleared from `status.is_closed` (`BACK/taiga/projects/tasks/signals.py#L94-L100`).
- The parent User Story and the Sprint are closed or reopened automatically when all their Tasks are closed (`BACK/taiga/projects/tasks/signals.py#L27-L80`).
  The TUI should therefore reload the User Story after a Task Status Change.
- Order: the viewset computes a key `project-user_story-status-taskboard_order` before and after the save (`BACK/taiga/projects/tasks/api.py#L119-L134`).
  If it differs (it does when `status` changes), it calls `update_tasks_order_in_bulk` for the Tasks in the new User Story and Status cell, keeping the Task's sent or existing `taskboard_order` and shifting the others to make room (`BACK/taiga/projects/tasks/api.py#L136-L151`, `BACK/taiga/projects/tasks/services.py#L75-L107`).
  So the client does not have to send `taskboard_order`.
  If it does not, the Task keeps its old numeric `taskboard_order` in the new column, which places it at an arbitrary but stable position.

How the web Taskboard does it (`FRONT/app/coffee/modules/taskboard/main.coffee#L626-L660`, `FRONT/app/coffee/modules/taskboard/taskboard-tasks.coffee#L76-L113`):

- Locally computes the new `taskboard_order` from the drop index (previous card's order + 1, or 0 at the top).
- Saves the Task with `PATCH`, body = modified attributes plus `version` (`FRONT/app/coffee/modules/base/model.coffee#L48-L54`), i.e. `{"status": .., "user_story": .., "taskboard_order": .., "version": ..}`.
- Sends header `set-orders: {"<task_id>": <order>, ...}` only when several preceding Tasks share the same order, so the server pins them (`BACK/taiga/projects/tasks/api.py#L140-L143`).
- Reads `taiga-info-order-updated` from the response and applies the orders locally.

Bulk endpoints for Tasks also exist but the web UI does not use them for a Status Change:

- `POST /api/v1/tasks/bulk_update_taskboard_order` with `{"project_id", "bulk_tasks": [{"task_id", "order"}], "status_id"?, "us_id"?, "milestone_id"?}` only rewrites `taskboard_order`; it never changes `status` (`BACK/taiga/projects/tasks/api.py#L287-L324`, validator `BACK/taiga/projects/tasks/validators.py#L86-L91`).
- `POST /api/v1/tasks/bulk_update_us_order` the same for `us_order`.

## Status Change of a User Story

Request:

```http
PATCH /api/v1/userstories/567
Content-Type: application/json

{"status": 12, "version": 8}
```

Success: `200 OK` with the full User Story detail, new `version`, recalculated `is_closed`, and a `Taiga-Info-Order-Updated` header for `backlog_order`/`sprint_order` changes (`BACK/taiga/projects/userstories/api.py#L239-L259`).

Order: before validation the viewset records keys for backlog, sprint and kanban order (`BACK/taiga/projects/userstories/api.py#L145-L184`).
If the kanban key `project-swimlane-status-kanban_order` changes, i.e. on any Status or swimlane change, `pre_save` sets `kanban_order = NEW_KANBAN_ORDER()`, which puts the User Story at the end of the new column (`BACK/taiga/projects/userstories/api.py#L201-L208`).
`sprint_order` and `backlog_order` are only touched when the Sprint or backlog position changes.
So a Status Change via `PATCH` needs no order fields.
Any `kanban_order` the client sends together with a Status Change is overwritten.

The status (and swimlane, Sprint) must belong to the same project, else `403 "You don't have permissions to set this status to this user story."` (`BACK/taiga/projects/userstories/api.py#L186-L199`).

How the web Kanban Board does it (`FRONT/app/coffee/modules/kanban/main.coffee#L615-L650`, `FRONT/app/coffee/modules/resources/userstories.coffee#L112-L130`):

```http
POST /api/v1/userstories/bulk_update_kanban_order
Content-Type: application/json

{
  "project_id": 42,
  "status_id": 12,
  "swimlane_id": 3,
  "after_userstory_id": 560,
  "bulk_userstories": [567]
}
```

- `status_id` is required; `swimlane_id`, `after_userstory_id`, `before_userstory_id` are optional, and `after`/`before` are mutually exclusive (`BACK/taiga/projects/userstories/validators.py#L169-L175`).
- Without `swimlane_id` the server treats the target cell as "no swimlane" (`swimlane__isnull=True`) and also sets `swimlane = NULL` on the moved stories (`BACK/taiga/projects/userstories/services.py#L304-L362`).
  On a project with swimlanes, omitting it would move cards out of their swimlane.
- Without `after`/`before` the cards go to the top of the column.
- It renumbers the cell, sets `status` and `swimlane` on the moved stories, and returns `200` with a list `[{"id", "swimlane", "status", "kanban_order"}, ...]` for every renumbered story (`BACK/taiga/projects/userstories/services.py#L284-L385`).
- `is_closed` recalculation and the history snapshot run afterwards (Celery or inline).
- No `version` is sent or checked, and `version` is not incremented (see "Gaps in OCC").
- Fails with `403 "Archived project"` on archived projects and with a "Blocked element" error on blocked ones (`BACK/taiga/projects/userstories/api.py#L466-L481`).

`POST /api/v1/userstories/bulk_update_sprint_order` and `bulk_update_backlog_order` exist for the backlog and Sprint lists; they are not needed for the MVP.

## Assign to me (Assignee)

Assign to me means making yourself the Assignee.
The current user's id comes from `GET /api/v1/users/me`.

### Task

Tasks have a single `assigned_to`.

```http
PATCH /api/v1/tasks/1234
{"assigned_to": 99, "version": 4}
```

The web UI's "assign to me" does exactly this (`FRONT/app/modules/components/ticket-assigned/assigned-to.directive.coffee#L23-L26`, `#L75`).
The server validates that the user is a project member, else `400 {"assigned_to": ["The user must be a project member."]}` (`BACK/taiga/projects/mixins/validators.py#L16-L31`).

### User Story

User Stories have both `assigned_to` (a single main Assignee, kept for compatibility) and `assigned_users` (a list) (`BACK/taiga/projects/userstories/models.py#L111-L121`).
The web UI's "assign to me" on a User Story appends the current user to `assigned_users` and sets `assigned_to` to the current user (`FRONT/app/modules/components/ticket-assigned/assigned-users.directive.coffee#L22-L31`, `#L83-L86`):

```http
PATCH /api/v1/userstories/567
{"assigned_users": [17, 99], "assigned_to": 99, "version": 9}
```

Since `assigned_users` is a whole list, the client must send the full new list, based on the list it read with the same `version`.
OCC protects this: if someone changed the assignees in between, the request fails with the `version` error instead of dropping their change.
The Kanban Board filters by `assigned_users`, and `filters_data` keeps `assigned_to` only for retro-compatibility (`BACK/taiga/projects/userstories/api.py#L350-L363`), so the "only mine" filter for User Stories should check `assigned_users`, not only `assigned_to`.

Design decision for the TUI: Assign to me on a User Story can either add the user (web UI behaviour) or replace all Assignees.
Matching the web UI (add, and make main Assignee) is the least surprising choice.

## Permissions

Both viewsets use `CommentAndOrUpdatePerm` for `update` and `partial_update` (`BACK/taiga/projects/tasks/permissions.py#L14-L27`, `BACK/taiga/projects/userstories/permissions.py#L14-L28`):

- Task `PATCH`: project permission `modify_task` (or `comment_task` if the body only has `comment`).
- User Story `PATCH`: `modify_us` (or `comment_us` for comment only).
- `bulk_update_kanban_order`, `bulk_update_taskboard_order`: `modify_us` respectively `modify_task`.
- Read: `view_tasks`, `view_us`.
- Project admins and superusers always pass (`enough_perms`).

`CommentAndOrUpdatePerm` ignores `version` when checking whether the body is comment-only (`BACK/taiga/permissions/permissions.py#L49-L71`).

In the default Scrum and Kanban project templates every role except "Stakeholder" has `modify_task`, `modify_us`, `view_tasks` and `view_us`; "Stakeholder" only has view and comment (`BACK/taiga/projects/fixtures/initial_project_templates.json`).
Roles are per project and editable by the project admin, so the TUI should read the user's actual permissions from the project detail (`GET /api/v1/projects/{id}` returns `my_permissions`) and disable Status Change and Assign to me when `modify_task`/`modify_us` is missing, instead of relying on the `403`.

A missing permission yields `403` with `{"_error_message": "You do not have permission to perform this action.", "_error_type": "taiga.base.exceptions.PermissionDenied"}` (`BACK/taiga/base/exceptions.py#L81-L84`, `#L218-L229`).

## Recommendation for the TUI

1. Status Change: `PATCH /tasks/{id}` or `/userstories/{id}` with `{"status": <id>, "version": <v>}`.
   Do not send order fields.
   The server puts User Stories at the end of the new column and keeps the Task's `taskboard_order`.
   Sort columns client-side by `kanban_order` and `taskboard_order` as returned.
2. Assign to me: Task `{"assigned_to": me, "version": v}`; User Story `{"assigned_users": old + [me], "assigned_to": me, "version": v}`.
3. Replace the cached item with the response body (it carries the new `version`).
   After a Task Status Change, also reload its User Story (its `is_closed` may have flipped).
4. Conflict handling: on `400` whose body has a `version` key, reload the item, show a short message ("changed by someone else"), and let the user retry.
   No automatic retry, since the conflict means the same field was changed.
5. Do not use `bulk_update_kanban_order` in the MVP.
   It bypasses OCC and needs correct swimlane handling.
   It becomes relevant only if the TUI ever supports reordering inside a column.
6. Gate actions on `my_permissions` from the project detail.

## Open points

- The `PATCH` path has no protection against concurrent web Kanban drags (bulk endpoint does not bump `version`).
  Only re-reading or listening to Taiga's events websocket would show such changes; this may deserve its own question about live refresh.
- Projects with swimlanes: the MVP Kanban Board shows Status columns only; whether swimlanes must be displayed or at least preserved is open.
  A `PATCH` with only `status` preserves the swimlane.
