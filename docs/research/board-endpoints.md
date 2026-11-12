# Board endpoints and data model (Taskboard and Kanban Board)

Research for ticket #3 (part of the map #1).
Question: which Taiga REST calls build the Taskboard of a Sprint and the Kanban Board, which fields decide ordering and membership, how pagination works, how to filter by Assignee, and how to find the projects of the logged-in Team Member or a project by slug.

## Sources

All claims are backed by these primary sources.
Source permalinks are pinned to the commits that were read on 2026-11-11.

- taiga-back `main` at `8ab6a4e3`: https://github.com/taigaio/taiga-back/tree/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa (abbreviated `[back]` below).
- taiga-front `main` at `da8afa50`: https://github.com/taigaio/taiga-front/tree/da8afa50b2a59c3aaec78854c0ae8145a16e6cd8 (abbreviated `[front]`), i.e. what the official web UI actually calls.
- taiga-doc at `7455fd37`: https://github.com/taigaio/taiga-doc/tree/7455fd37cb164bbf80198c1517fd0ef414e2a7b9, rendered at https://docs.taiga.io/api.html (abbreviated `[doc]`).
- Live, unauthenticated GET requests against public projects on https://taiga-fki.ostfalia.de/api/v1/ (abbreviated `[live]`), no login performed.

The official API docs (`[doc]`) are incomplete for this question: they do not mention swimlanes, the `assigned_users` filter or `page_size`.
The backend source is therefore the authority wherever the two disagree.

## TL;DR: minimal call list

Common bootstrap (once per session / project switch):

1. `GET /api/v1/users/me` -> `id` of the logged-in Team Member.
2. `GET /api/v1/projects?member={me.id}&order_by=user_order` with `x-disable-pagination: 1` -> project selection list.
3. `GET /api/v1/projects/by_slug?slug={slug}` (or `GET /api/v1/projects/{id}`) -> project detail with all Statuses, swimlanes, members and the Sprint list embedded.

Taskboard of the Current Sprint:

4. `GET /api/v1/milestones?project={id}&closed=false&estimated_start__lte={today}&estimated_finish__gte={today}` -> the Current Sprint (pick one if several overlap).
5. `GET /api/v1/milestones/{sprintId}` -> Sprint plus its User Stories embedded (`user_stories`), sort by `sprint_order`.
6. `GET /api/v1/tasks?project={id}&milestone={sprintId}` with `x-disable-pagination: 1` -> all Tasks, group by `user_story` (null = Tasks without a User Story), column by `status`, sort by `taskboard_order`.

Kanban Board:

4. `GET /api/v1/userstories?project={id}&status__is_archived=false` with `x-disable-pagination: 1` -> all non-archived User Stories, column by `status`, row by `swimlane`, sort by `kanban_order`.
5. (Only if the project uses swimlanes and per-swimlane WIP limits matter) `GET /api/v1/swimlanes?project={id}` with `x-disable-pagination: 1`.

"Only mine" filter: `assigned_to={me.id}` on `/tasks`, `assigned_users={me.id}` on `/userstories` (or filter client-side, see below).

That is 3 bootstrap calls plus 3 calls for the Taskboard and 1-2 calls for the Kanban Board.

## Pagination

- Every list endpoint is paginated by default with 30 items per page (`PAGINATE_BY: 30`), the page size can be raised with `?page_size=N` up to 1000 (`MAX_PAGINATE_BY`) and the page selected with `?page=N` or `?page=last`.
  [back] `settings/common.py` L503-505, https://github.com/taigaio/taiga-back/blob/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa/settings/common.py#L503-L505 and `taiga/base/api/pagination.py` L142-247, https://github.com/taigaio/taiga-back/blob/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa/taiga/base/api/pagination.py#L142-L247
- Sending the request header `x-disable-pagination` with any value turns pagination off completely and the full result is returned as a plain JSON array (the backend only checks that the header is present).
  Same source, L151 and L175; documented in [doc] `api/general-notes.adoc` "Pagination" (https://docs.taiga.io/api.html#_pagination).
- Paginated responses carry the headers `x-paginated`, `x-paginated-by`, `x-pagination-count` (total), `x-pagination-current`, and `X-Pagination-Next` / `X-Pagination-Prev` (absolute URLs) when there is a next/previous page.
  Same source, L229-246.
- With `x-lazy-pagination` the count query is skipped and `x-pagination-count` is omitted.
  Same source, L178 and L229.
- The official web UI sends `x-disable-pagination: 1` on every list query unless it explicitly asks for pagination (only the backlog does), so both boards load in one request per resource in the web UI.
  [front] `app/coffee/modules/base/repository.coffee` L139-140, https://github.com/taigaio/taiga-front/blob/da8afa50b2a59c3aaec78854c0ae8145a16e6cd8/app/coffee/modules/base/repository.coffee#L139-L140
- [live] On the Ostfalia instance `X-Pagination-Next` is generated with `http://` instead of `https://` (e.g. `http://taiga-fki.ostfalia.de/api/v1/tasks?project=218&page=2`), because the reverse proxy does not forward the scheme.
  A client that follows that header verbatim would downgrade to plain HTTP.

Recommendation: always send `x-disable-pagination: 1` on the board list calls, like the web UI does.
Board sizes for a university team are small (the largest public project on the instance has 238 Tasks in total, about 490 KB unpaginated, across all Sprints, and a single Sprint is a fraction of that).
If paginating is ever needed, build the next URL from `page` + `page_size` rather than following `X-Pagination-Next`.

## Projects: selection and lookup by slug

- `GET /users/me` returns the logged-in user's full profile including `id`.
  [back] `taiga/users/api.py` L318-324, https://github.com/taigaio/taiga-back/blob/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa/taiga/users/api.py#L318-L324
- `GET /projects?member={userId}` filters to projects the user is a member of (`filter_fields` maps `member` to `members`), `order_by=user_order` sorts by the user's own project order from the membership.
  [back] `taiga/projects/api.py` L77-81, https://github.com/taigaio/taiga-back/blob/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa/taiga/projects/api.py#L77-L81 and `taiga/projects/filters.py` `UserOrderFilterBackend` L100-128.
  This is exactly what the web UI's project list does: [front] `app/coffee/modules/resources/projects.coffee` L27-29 (`listByMember`).
- Without `member`, the list contains every project the requester can see, which includes all public projects of the instance (16 for anonymous requests on Ostfalia [live]), so the `member` filter is required for the project selection.
- The list item (`ProjectSerializer`) already contains `id`, `name`, `slug`, `is_backlog_activated`, `is_kanban_activated`, `i_am_member`, `my_permissions`, `default_us_status`, `default_task_status`, `default_swimlane` and more; `?slight=1` switches to a lighter serializer.
  [back] `taiga/projects/api.py` L155-161 and `taiga/projects/serializers.py` `ProjectSerializer`.
  `is_backlog_activated` (Scrum: Sprints + Taskboard) and `is_kanban_activated` (Kanban Board) tell the TUI which boards to offer per project.
- `GET /projects/by_slug?slug={slug}` returns the same detail payload as `GET /projects/{id}`, and returns 404 if the requester may not see the project.
  [back] `taiga/projects/api.py` L136-153 and L347-350; [doc] `api/projects/get-by-slug.adoc`.
  The slug is the one in the web URL `https://taiga-fki.ostfalia.de/project/{slug}/...`.

### Project detail payload (`ProjectDetailSerializer`)

The detail call returns everything both boards need as reference data, so no separate status or member calls are needed.
[back] `taiga/projects/serializers.py` `ProjectDetailSerializer` and the SQL in `taiga/projects/utils.py` L8-246, https://github.com/taigaio/taiga-back/blob/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa/taiga/projects/utils.py

| Field | Content | Ordering |
| --- | --- | --- |
| `us_statuses[]` | `id, name, slug, order, is_closed, is_archived, color, wip_limit, project_id` | already sorted by `order` (L157-177) |
| `task_statuses[]` | `id, name, slug, order, is_closed, color, project_id` | already sorted by `order` (L226-246) |
| `swimlanes[]` | raw rows `id, name, order, project_id` (no statuses, no per-swimlane WIP) | sorted by `order` (L134-154) |
| `default_swimlane` | swimlane id or null | |
| `members[]` | `id, username, full_name, full_name_display, photo, color, is_active, role, role_name, gravatar_id` | by full name + username (L8-40) |
| `milestones[]` | `id, slug, name, closed` only (no dates) | by `estimated_start` (L43-67) |

These arrays come from `json_agg(...)` and are `null` (not `[]`) when empty, e.g. `swimlanes: null` for a project without swimlanes [live].
The Go structs must treat `null` and missing as empty.

`members` is the lookup table for Assignee display (`assigned_to` on items is just a user id).

## Taskboard of a Sprint

### Finding the Current Sprint

- Sprints are `milestones`; fields: `id, name, slug, estimated_start, estimated_finish` (ISO dates), `closed`, `order`, `project`, `user_stories[]`, `total_points`, `closed_points`.
  [back] `taiga/projects/milestones/serializers.py` L15-46, https://github.com/taigaio/taiga-back/blob/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa/taiga/projects/milestones/serializers.py
- `/milestones` filters: `project`, `project__slug`, `closed`, plus date comparison filters `estimated_start`, `estimated_finish` each with suffixes `__lt`, `__gt`, `__lte`, `__gte`.
  [back] `taiga/projects/milestones/api.py` L45-62 and `taiga/base/filters.py` `BaseCompareFilter`/`BaseDateFilter` L547-596, `EstimatedStartFilter`/`EstimatedFinishFilter` L609-614.
- The list is always ordered by `-estimated_start` (newest first), regardless of `order_by`.
  [back] `taiga/projects/milestones/api.py` L90-95.
- The list response carries `Taiga-Info-Total-Opened-Milestones` and `Taiga-Info-Total-Closed-Milestones` headers.
  Same file, L75-88.

So the Current Sprint (glossary: the Sprint whose date range contains today) is one server-side filtered call:
`GET /milestones?project={id}&estimated_start__lte={today}&estimated_finish__gte={today}`.
Adding `closed=false` drops Sprints that were closed early; whether a closed Sprint still counts as "current" is a product decision, not an API question.
Taiga does not prevent overlapping Sprints, so the result can contain 0, 1 or several Sprints; the newest `estimated_start` comes first.
The project detail's `milestones[]` cannot be used for this because it lacks the dates.

### User Stories of the Sprint (rows)

- `GET /milestones/{id}` embeds the Sprint's User Stories as `user_stories[]` using `UserStoryNestedSerializer`: `id, ref, subject, status, status_extra_info{name,color,is_closed}, assigned_to, assigned_to_extra_info, sprint_order, kanban_order, backlog_order, is_closed, is_blocked, blocked_note, points, total_points, epics, due_date...`.
  [back] `taiga/projects/milestones/serializers.py` L35-36, `taiga/projects/userstories/serializers.py` L192-217.
  The list call `/milestones?...` embeds the same `user_stories[]`, so step 4 and 5 can be merged into one call.
- Row order on the Taskboard is `sprint_order` ascending; the web UI does `_.sortBy(sprint.user_stories, "sprint_order")`.
  [front] `app/coffee/modules/taskboard/main.coffee` L440-442, https://github.com/taigaio/taiga-front/blob/da8afa50b2a59c3aaec78854c0ae8145a16e6cd8/app/coffee/modules/taskboard/main.coffee#L440-L442
- The nested serializer has no `assigned_users` (multiple assignees) and no `swimlane`; if those are needed, use `GET /userstories?project={id}&milestone={sprintId}` instead (same filter set as the Kanban call below).

### Tasks (cards)

- `GET /tasks?project={id}&milestone={sprintId}` returns `TaskListSerializer` items.
  [back] `taiga/projects/tasks/api.py` L47-82 and `taiga/projects/tasks/serializers.py` L25-63, https://github.com/taigaio/taiga-back/blob/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa/taiga/projects/tasks/serializers.py#L25-L63
  The web UI does the same with `order_by=us_order`: [front] `app/coffee/modules/resources/tasks.coffee` L42-47.
- Relevant fields: `id, ref, subject, user_story, user_story_extra_info{id,ref,subject,epics}, status, status_extra_info{name,color,is_closed}, taskboard_order, us_order, assigned_to, assigned_to_extra_info{id,username,full_name_display,photo,...}, milestone, is_closed, is_blocked, is_iocaine, version, tags`.
  Confirmed against a live public Sprint [live].
- Membership: row = `user_story` (a Task with `user_story: null` belongs to the extra "unassigned Tasks" row the web UI renders; [front] `app/coffee/modules/taskboard/taskboard-tasks.coffee` L127-128), column = `status` (matched against `project.task_statuses[].id`).
- Ordering inside a cell: `taskboard_order` ascending, done client-side by the web UI ([front] `taskboard-tasks.coffee` L50 and L82).
  `taskboard_order` is not in the backend's `order_by_fields` (only `us_order` is), so the server cannot sort by it and the TUI must sort locally.
  [back] `taiga/projects/tasks/api.py` L65-73.
  `us_order` is the order of a Task inside its User Story detail, not on the Taskboard.
- Task filters: `project`, `project__slug`, `milestone`, `user_story`, `status`, `status__is_closed`, `assigned_to`, `owner`, `role`, `tags`, `watchers`, `q`, plus `exclude_*` variants.
  [back] `taiga/projects/tasks/api.py` L47-64.
- Column folding on the Taskboard is purely a web-UI preference stored in browser localStorage, not in the API.
  [front] `app/coffee/modules/resources/tasks.coffee` L113-128.

## Kanban Board

### User Stories (cards)

- `GET /userstories?project={id}` returns `UserStoryListSerializer` items.
  [back] `taiga/projects/userstories/api.py` L56-105 and `taiga/projects/userstories/serializers.py` L37-152, https://github.com/taigaio/taiga-back/blob/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa/taiga/projects/userstories/serializers.py#L37-L152
- Relevant fields: `id, ref, subject, status, status_extra_info{name,color,is_closed}, swimlane, kanban_order, sprint_order, backlog_order, milestone, milestone_name, assigned_to, assigned_to_extra_info, assigned_users[], is_closed, is_blocked, blocked_note, total_points, epics, tasks, version`.
- `assigned_users` is the set of all assignees (the M2M `assigned_users` plus `assigned_to`), because a User Story can have several Assignees while a Task has exactly one (`assigned_to`).
  [back] `taiga/projects/userstories/serializers.py` `get_assigned_users`, L82-95.
- Membership: column = `status` (against `project.us_statuses[].id`), row = `swimlane` (against `project.swimlanes[].id`; `null` = "unclassified" lane, which only appears when the project has swimlanes).
  `Taiga-Info-Userstories-Without-Swimlane: true|false` on the list response tells whether such unclassified stories exist.
  [back] `taiga/projects/userstories/api.py` L154-171.
- Ordering inside a cell: `kanban_order` ascending; the web UI sorts client-side with `_.sortBy usList, ['kanban_order']`.
  [front] `app/coffee/modules/kanban/kanban-usertories.coffee` L82, https://github.com/taigaio/taiga-front/blob/da8afa50b2a59c3aaec78854c0ae8145a16e6cd8/app/coffee/modules/kanban/kanban-usertories.coffee#L82
  The server default order is `backlog_order` (model `Meta.ordering = ["project", "backlog_order", "ref"]`, [back] `taiga/projects/userstories/models.py` L152), so either pass `order_by=kanban_order` or sort locally.
  The order key the backend uses for Kanban is `(project, swimlane, status, kanban_order)`; [back] `taiga/projects/userstories/api.py` L148-149.
- `include_tasks=1` embeds a light Task list per story; `include_attachments=1` embeds attachments; the web UI only uses them at high zoom levels.
  [back] `taiga/projects/userstories/api.py` L126-139; [front] `app/coffee/modules/kanban/main.coffee` L448-450.

### Archived and folded Statuses

- `UserStoryStatus.is_archived` marks archived Statuses.
  The web UI's Kanban request is `GET /userstories?project={id}&status__is_archived=false` (archived columns are hidden and their stories not loaded); when the user unfolds an archived column it reloads with `exclude_status={hidden archived ids}` instead.
  [front] `app/coffee/modules/kanban/main.coffee` L437-455, https://github.com/taigaio/taiga-front/blob/da8afa50b2a59c3aaec78854c0ae8145a16e6cd8/app/coffee/modules/kanban/main.coffee#L437-L455 and L593-598.
- Folded (collapsed) columns and folded swimlanes are not stored in Taiga at all; they live in the browser's localStorage per project.
  [front] `app/coffee/modules/resources/kanban.coffee` (`storeStatusColumnModes`, `storeSwimlanesModes`), https://github.com/taigaio/taiga-front/blob/da8afa50b2a59c3aaec78854c0ae8145a16e6cd8/app/coffee/modules/resources/kanban.coffee
  So the TUI has to decide its own default (e.g. archived Statuses folded, as the web UI does) and may keep fold state in its own config.
- Task Statuses have no `is_archived` and no `wip_limit` ([back] `taiga/projects/models.py` L704-730), so the Taskboard has no archived columns and no WIP limits.

### WIP limits

- Project-wide: `us_statuses[].wip_limit` (integer or null).
  [back] `taiga/projects/models.py` L603-628.
- Per swimlane: model `SwimlaneUserStoryStatus(swimlane, status, wip_limit)`, exposed as `GET /swimlanes?project={id}` (each swimlane has `statuses[]` with the Status fields `id, name, slug, order, is_closed, is_archived, color` plus the per-swimlane `wip_limit` and `swimlane_userstory_status_id`) and as `GET /swimlane-userstory-statuses?project={id}` (`id, status, swimlane, wip_limit`).
  [back] `taiga/projects/models.py` L946-975, `taiga/projects/serializers.py` L69-101, `taiga/projects/api.py` L655-698.
- WIP limits are informational: the backend does not reject a Status Change that exceeds them (`wip_limit` is not referenced anywhere in `taiga/projects/userstories/` or `taiga/projects/validators.py`).

### Swimlanes

- `Swimlane(project, name, order)` with optional `project.default_swimlane`.
  [back] `taiga/projects/models.py` L155 and L920-944.
- The project detail already contains `swimlanes[]` sorted by `order`; `GET /swimlanes?project={id}` is only needed for per-swimlane WIP limits.
- None of the 16 public projects on the Ostfalia instance use swimlanes (`swimlanes: null`, `GET /swimlanes?project=218` count 0) [live], so swimlanes can be handled minimally (group by `swimlane`, treat null as a single lane) without hurting the MVP.

## Filtering by Assignee ("only mine")

| Endpoint | Parameter | Semantics | Source |
| --- | --- | --- | --- |
| `/tasks` | `assigned_to={id}[,{id}...]` | `assigned_to IN (...)`; the value `null` matches unassigned Tasks | [back] `taiga/base/filters.py` L387-413 (`BaseRelatedFieldsFilter`), L441-443 (`AssignedToFilter`) |
| `/userstories` | `assigned_users={id}[,...]` | matches stories where the user is `assigned_to` OR in `assigned_users`; `null` matches stories with no Assignee | [back] `taiga/projects/userstories/filters.py` L16-23, L55-75 |
| `/userstories` | `assigned_to={id}` | only the primary `assigned_to`, misses co-assignees | [back] `taiga/base/filters.py` L441-443 |
| both | `exclude_assigned_to`, `exclude_assigned_users` | negation | same |

Values are comma separated; `null`, `true`, `false` are special values ([back] `taiga/base/filters.py` L65-68 and L387-398).
[live] `GET /tasks?milestone=922` returned 5 Tasks, `&assigned_to=null` returned 2.

For the TUI, filtering client-side is simpler: the board data is loaded in full anyway, and toggling "only mine" then needs no extra request.
Use `task.assigned_to == me.id` for Tasks and `me.id in us.assigned_users` for User Stories (so co-assigned stories count as mine).
The server-side parameters above are the fallback if boards ever get large.

## Surprises and pitfalls

1. **Swimlane filter typo.** The User Story swimlane filter's query parameter is spelled `swimnlane`, not `swimlane` ([back] `taiga/projects/userstories/filters.py` L32-35: `param_name = "swimnlane"`).
   `?swimlane=X` is silently ignored and returns all stories; `?swimnlane=X` works [live: `swimlane=999999` -> 30 results, `swimnlane=999999` -> 0].
   Not needed for the MVP (filter client-side), but anyone adding a server-side swimlane filter must know this.
2. **`taskboard_order` cannot be used with `order_by`** on `/tasks`; sort client-side.
3. **`X-Pagination-Next` is `http://` on Ostfalia** [live]; do not follow it, use `x-disable-pagination` instead.
4. **Embedded arrays are `null` when empty** (`swimlanes`, possibly `us_statuses` etc.) because they come from SQL `json_agg`.
5. **Fold state and archived-column visibility are web-UI local state**, not API data; the TUI owns that decision.
6. **The official API docs are behind the code**: no mention of swimlanes, `assigned_users`, `page_size`, `x-lazy-pagination` or the `Taiga-Info-*` headers.
7. **Unauthenticated reads work for public projects** on Ostfalia (16 public projects); useful for integration tests and fixtures without credentials, but the project list must be filtered by `member` once logged in.

## Open points for the map

- Status Change and Assign to me (write path): `PATCH /userstories/{id}` and `PATCH /tasks/{id}` with `version` for optimistic concurrency, plus the bulk order endpoints the web UI uses on drop (`bulk_update_kanban_order`, `bulk_update_taskboard_order`).
  Whether a Status Change from the TUI must also set the card's position (`kanban_order`/`taskboard_order`) or can leave the backend default is not answered here.
- Whether a closed Sprint whose date range contains today still counts as the Current Sprint, and what to show when several Sprints overlap or none matches.
- Realtime updates: the instance exposes `wss://taiga-fki.ostfalia.de/events` (from `conf.json`); the web UI reloads boards on events.
  Whether the TUI uses events or polling is a separate question.
