# Taiga API facts per roadmap item

Research for issue [#91](https://github.com/zeckitoz/taiga-tui/issues/91) (part of map [#90](https://github.com/zeckitoz/taiga-tui/issues/90)).
Researched on 2026-11-17.

Question: for each roadmap item, what do the Taiga REST API and taiga-events offer, and which hard blockers exist on some Instances?

## Sources

All claims are traced to these primary sources.
Line links point at fixed commits so they stay valid.

- Official API docs: https://docs.taiga.io/api.html (sections 18 User Stories, 23 Tasks, 36 History, Resolver, Epics, Issues).
- taiga-back at commit `8ab6a4e3` (2026-11-03): https://github.com/taigaio/taiga-back/tree/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa
- taiga-events at commit `e411856d` (2026-04-20): https://github.com/taigaio/taiga-events/tree/e411856df4653dccdd727994b0d957db28d15770
- taiga-front at commit `da8afa50` (2026-10-27): https://github.com/taigaio/taiga-front/tree/da8afa50b2a59c3aaec78854c0ae8145a16e6cd8
- taiga-docker at commit `e12c106b` (2025-10-13), the official self-hosting setup: https://github.com/taigaio/taiga-docker/tree/e12c106b265ab8c51ee7e62b744ffddd0f0d9771
- Live checks on 2026-11-17 against taiga.io SaaS (`tree.taiga.io`) and the team Instance (`taiga-fki.ostfalia.de`): unauthenticated `GET /conf.json` and an unauthenticated websocket `ping`.
  No credentials were sent.

Abbreviations: `BACK` = `https://github.com/taigaio/taiga-back/blob/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa`, `EVENTS` = `https://github.com/taigaio/taiga-events/blob/e411856df4653dccdd727994b0d957db28d15770`, `FRONT` = `https://github.com/taigaio/taiga-front/blob/da8afa50b2a59c3aaec78854c0ae8145a16e6cd8`, `DOCKER` = `https://github.com/taigaio/taiga-docker/blob/e12c106b265ab8c51ee7e62b744ffddd0f0d9771`.

Related earlier notes: [status-change.md](status-change.md) (OCC, Status Change, Assign to me), [board-endpoints.md](board-endpoints.md), [auth.md](auth.md).

## Summary

- **Live updates:** taiga-events is a plain JSON-over-websocket server at `wss://<instance>/events`.
  A client sends `auth` with its JWT access token, then `subscribe` to routing keys such as `changes.project.<id>.userstories` and `changes.project.<id>.tasks`.
  Messages only carry `{type, matches, pk}`, so the client must re-fetch the changed item.
  It is part of the official taiga-docker setup and is live on both taiga.io SaaS and the team Instance (both answered `pong`).
  It is optional for self-hosters, so a client must degrade gracefully (polling) when `/events` is missing.
- **Create and edit:** `POST /api/v1/userstories` and `POST /api/v1/tasks` need only `project` and `subject`; `status` defaults to the project's default Status.
  Edits are `PATCH` with `version` (see status-change.md for OCC).
  Comments are posted by `PATCH`ing the item with `{"comment": "...", "version": n}`, read via `GET /api/v1/history/{userstory|task}/{id}?type=comment`, and edited or deleted via `POST /api/v1/history/.../{id}/edit_comment|delete_comment?id=<entry>`.
  A comment-only `PATCH` never hits a version conflict but does bump `version`.
- **Undo and unassign:** there is no server-side undo or revert endpoint.
  Undo of a Status Change means the client sends the inverse `PATCH` with the `version` from the first response; OCC then rejects the undo if someone else changed the Status in between.
  Unassign on a Task sets `assigned_to: null`; on a User Story the client sends the full `assigned_users` list without the Team Member and fixes up `assigned_to`, exactly as the web UI does.
- **Epics and Issues:** full REST resources (`/api/v1/epics`, `/api/v1/issues`), each with its own Status set (`/epic-statuses`, `/issue-statuses`).
  Epics relate to User Stories many-to-many via `/api/v1/epics/{id}/related_userstories`; every User Story payload carries an `epics` list.
  Issues have no board in the web UI (a filtered list only), plus types, priorities and severities.
  Both modules are per-project feature flags (`is_epics_activated` defaults to off, `is_issues_activated` to on).
  Epic changes emit no taiga-events messages.
- **Rendered Markdown:** descriptions and comments are stored as Markdown text and rendered server-side by Python-Markdown with Taiga extensions (`@mentions`, `#ref` links, `[[wiki]]` links, emoji, task lists, strikethrough, tables via `extra`, and hard line breaks via `nl2br`).
  Every payload ships the rendered HTML alongside (`description_html`, `comment_html`), and `POST /api/v1/wiki/render` renders arbitrary text for a preview.
  Attachment links carry a `#_taiga-refresh=<type>:<id>` fragment because self-hosted media URLs are signed and expire (default 360 s in taiga-docker).

Hard blockers: none found for the team Instance or taiga.io SaaS.
The only Instance-dependent risks are a self-hosted Instance without taiga-events (live updates fall back to polling) and projects with the Epics or Issues module switched off.

## 1. Live updates (taiga-events)

### Architecture

- taiga-back publishes events to a RabbitMQ topic exchange named `events` (`BACK/taiga/events/events.py#L23-L40`, `EVENTS/src/rabbitmq/index.js`).
  The upstream default backend is PostgreSQL `NOTIFY` (`BACK/settings/common.py#L216-L219`), but the Docker image configures RabbitMQ (`BACK/docker/config.py#L88-L98`).
- taiga-events is a Node.js `ws` server that relays RabbitMQ messages to subscribed websocket clients (`EVENTS/src/index.js`).
- In taiga-docker it is a standard service with its own RabbitMQ (`DOCKER/docker-compose.yml#L111-L132`) and the gateway proxies `location /events` to it with 7-day timeouts (`DOCKER/taiga-gateway/taiga.conf#L65-L74`).
- The web UI learns the URL from `conf.json` key `eventsUrl`.
  taiga.io SaaS: `"eventsUrl": "wss://api.taiga.io/events"`.
  Team Instance: `"eventsUrl": "wss://taiga-fki.ostfalia.de/events"`.
  Both accepted a websocket upgrade (HTTP 101) and answered `{"cmd":"pong"}` to `{"cmd":"ping"}` on 2026-11-17.

### Protocol

All frames are JSON text (`EVENTS/src/ws/client.js#L59-L88`).

| Client sends | Effect |
| --- | --- |
| `{"cmd": "ping"}` | Server replies `{"cmd": "pong"}`; works without auth (`#L135`). |
| `{"cmd": "auth", "data": {"token": "<JWT>", "sessionId": "<random id>"}}` | Stores the auth if the JWT verifies (`#L95-L99`). No reply on success or failure. |
| `{"cmd": "subscribe", "routing_key": "...", "options": {"selfNotification": false}}` | Binds a queue to that routing key; silently ignored before a successful `auth`. |
| `{"cmd": "unsubscribe", "routing_key": "..."}` | Removes the binding. |

- The token is the same JWT access token the REST API uses (`auth_token`), as the web UI sends `@auth.getToken()` (`FRONT/app/coffee/modules/events.coffee#L239-L243`).
  taiga-events verifies it with the backend's shared `SECRET` and `ignoreExpiration: 'true'` (`EVENTS/src/crypto/index.js#L14-L20`), so an expired access token still authenticates an open socket.
- There is no per-project permission check on `subscribe`: any authenticated user can bind any `changes.project.*` key (`EVENTS/src/ws/client.js#L74-L83`).
  The payload only contains ids, and re-fetching goes through the normal REST permission checks.
- The web UI pings every `eventsHeartbeatIntervalTime` (30 s on SaaS, 60 s default) and reconnects after 5 to 10 s (`FRONT/app/coffee/modules/events.coffee#L26`, `#L124-L141`, `#L281-L282`).

### Messages and routing keys

- Model saves and deletes emit `{"session_id": ..., "data": {"type": "create|change|delete", "matches": "<app.model>", "pk": <id>}}` on `changes.project.<projectId>.<app>` (`BACK/taiga/events/events.py#L43-L77`, `BACK/taiga/events/signal_handlers.py#L16-L37`).
  taiga-events adds `routing_key` before forwarding (`EVENTS/src/rabbitmq/subscription.js#L37-L50`).
- Bulk endpoints (for example `bulk_update_kanban_order`) emit one message whose `pk` is a list of ids (`BACK/taiga/events/events.py`, `emit_event_for_ids`; callers in `BACK/taiga/projects/userstories/services.py`).
- Watched types: `userstories.userstory`, `tasks.task`, `issues.issue`, `milestones.milestone`, `wiki.wiki_page`, plus `projects.userstorystatus`, `projects.swimlane`, `projects.swimlaneuserstorystatus` (`BACK/taiga/projects/apps.py#L111-L118` and each app's `apps.py`).
  Epics are not watched, and comment edits or deletes through the history endpoints emit nothing.
  A new comment does emit a `change`, because it is a `PATCH` that saves the item.
- Keys the web UI subscribes to: Kanban Board `changes.project.<id>.userstories` and `.projects`; Taskboard `.tasks`, `.issues`, `.userstories` (`FRONT/app/coffee/modules/kanban/main.coffee#L263-L268`, `FRONT/app/coffee/modules/taskboard/main.coffee#L381-L389`).
- Echo suppression: the server drops a message whose `session_id` equals the subscriber's `sessionId` unless `selfNotification` is set (`EVENTS/src/rabbitmq/subscription.js#L39`).
  REST requests carry the session id in the `X-Session-Id` header (`BACK/taiga/events/middleware.py`, `FRONT/app/coffee/app.coffee#L593`).
  The TUI should generate one random session id per run and send it both in `auth` and as `X-Session-Id` on every REST call, so its own changes do not echo back.
- Per-user `live_notifications.<userId>` and `web_notifications.<userId>` keys exist for notification toasts (`BACK/taiga/events/events.py#L80-L176`).

### Instance risks

- taiga-events is a separate service.
  Self-hosters on older or hand-rolled setups may not run it, or may not proxy `/events`.
  The client should derive the URL from `<instance>/conf.json` (`eventsUrl`), treat a failed connect as "no live updates", and keep a manual or periodic refresh.
- Unverified without credentials: that the `auth` command succeeds with a TUI-issued token on SaaS and the team Instance.
  Since the server gives no auth acknowledgement, a client can only detect failure by never receiving events; a HITL check with a second session making a change is the way to confirm.

## 2. Creating and editing User Stories and Tasks, and comments

### Create

- `POST /api/v1/userstories`: required `project` and `subject` (docs section 18.2).
  Everything else is optional; `status` defaults to `project.default_us_status` (`BACK/taiga/projects/userstories/models.py#L154-L159`), and `owner` is set to the caller.
  `ref`, `kanban_order`, `owner` and dates are read-only (`BACK/taiga/projects/userstories/validators.py#L48-L57`).
- `POST /api/v1/tasks`: required `project` and `subject` (docs section 23.2).
  `user_story` is nullable in the model (`BACK/taiga/projects/tasks/models.py#L24-L31`), so the API allows a Task without a User Story, unlike the GLOSSARY definition; the TUI should always send `user_story`.
  `milestone` is overwritten with the User Story's Sprint (`BACK/taiga/projects/tasks/api.py#L125-L127`), and Status, User Story and Sprint must belong to the same project (`#L98-L111`).
  `status` defaults to `project.default_task_status` (`BACK/taiga/projects/tasks/models.py#L105-L111`).
- Bulk creation exists (`bulk_create` with newline-separated subjects) for both, as used by the web UI's quick-add.
- Permissions: `add_us` and `add_task` (`BACK/taiga/projects/tasks/permissions.py#L18`).

### Edit

- `PATCH /api/v1/userstories/{id}` or `/tasks/{id}` with only the changed fields plus `version` (docs section 18.5).
  OCC details, error shape (`400 {"version": ...}`) and field-level merge are in [status-change.md](status-change.md#optimistic-concurrency-control-occ).
- Editing a description therefore conflicts only if someone else edited the description since the client's `version`; parallel Status Changes merge silently.

### Comments

- **Post:** `PATCH` the item with `{"comment": "<markdown>", "version": n}` (`BACK/taiga/projects/history/mixins.py#L51-L53`).
  The comment is stored on the history entry of that save, rendered to `comment_html`, and mentioned Team Members are notified.
  Permission: a body whose only key besides `version` is `comment` needs only `comment_us` or `comment_task`, otherwise `modify_*` (`BACK/taiga/permissions/permissions.py#L49-L73`).
  OCC never rejects it, because `comment` is never a field in a history diff, but the save still increments `version` (`BACK/taiga/projects/occ/mixins.py#L41-L65`).
  The client must therefore adopt the `version` from the response.
- **Read:** `GET /api/v1/history/userstory/{id}?type=comment` (or `history/task/{id}`), newest first; `type=activity` returns only field changes; adding `page=` paginates (`BACK/taiga/projects/history/api.py#L182-L202`, docs section 36.1).
  Each entry has `id` (UUID), `user`, `created_at`, `comment`, `comment_html`, `delete_comment_date`, `edit_comment_date` (`BACK/taiga/projects/history/serializers.py#L18-L40`).
- **Edit, delete, undelete:** `POST /api/v1/history/{type}/{objId}/edit_comment?id=<entryId>` with `{"comment": ...}`, `delete_comment?id=`, `undelete_comment?id=` (`BACK/taiga/projects/history/api.py#L80-L176`).
  Allowed for the comment's author or a project admin (`BACK/taiga/projects/history/permissions.py#L42-L56`).
  Deleted comments stay in the list with `delete_comment_date` set, so the client must hide or grey them.

## 3. Unassign and undo of a Status Change

- There is no undo, revert or rollback endpoint in taiga-back.
  The history entries record `values_diff`, but nothing replays them.
- **Undo of a Status Change** is the inverse `PATCH {"status": <old>, "version": <version from the first PATCH response>}`.
  If another Team Member changed the Status in between, OCC rejects the undo with the `version` error, which is the safe behaviour.
  Side effects to expect:
  - A User Story's `kanban_order` is reset to the end of the target column on every Status Change (`BACK/taiga/projects/userstories/api.py#L201-L207`), so undo puts the card at the bottom of its original column, not at its old position.
  - Closing all Tasks of a User Story sets the User Story's `is_closed` and `finish_date`, and reopening a Task clears them again (`BACK/taiga/projects/tasks/signals.py#L38-L52`, `BACK/taiga/projects/userstories/services.py#L473-L488`).
    Undo reverses `is_closed` but writes a fresh `finish_date` timestamp.
  - Both the change and the undo appear in the item's activity history and trigger watcher notifications.
- **Unassign on a Task:** `PATCH {"assigned_to": null, "version": n}`.
- **Unassign on a User Story:** send the full new `assigned_users` list without the Team Member, and set `assigned_to` to `null` if the list is empty, or to the first remaining Assignee if the old `assigned_to` was removed.
  This mirrors the web UI (`FRONT/app/modules/components/ticket-assigned/assigned-users.directive.coffee#L22-L31`, `#L88-L97`); the backend does not keep the two fields in sync by itself.

## 4. Epics and Issues

### Epics

- `GET|POST|PATCH|DELETE /api/v1/epics` with OCC like User Stories (`BACK/taiga/projects/epics/models.py#L23`, `BACK/taiga/routers.py#L184-L187`).
- Own Statuses: `/api/v1/epic-statuses`, default `project.default_epic_status` (`BACK/taiga/projects/models.py#L126`).
- Relation to User Stories is many-to-many through `RelatedUserStory` with an `order` (`BACK/taiga/projects/epics/models.py#L74`, `#L105-L115`).
  Managed via `GET|POST|PATCH|DELETE /api/v1/epics/{epicId}/related_userstories[/{usId}]` and `bulk_create` (docs "Epics related user stories").
  An Epic may hold User Stories from other projects, which is why the `epics` entries on a User Story include a `project` object.
- Every User Story list and detail payload carries `epics: [{id, ref, subject, color, project}]` (`BACK/taiga/projects/userstories/utils.py#L92-L120`), and `GET /api/v1/userstories?epic=<id>` filters by Epic.
- Module flag `is_epics_activated` defaults to `false` (`BACK/taiga/projects/models.py#L198`).
- No taiga-events messages for Epics (see section 1).

### Issues

- `GET|POST|PATCH|DELETE /api/v1/issues` with OCC, plus `history/issue` comments (`BACK/taiga/routers.py#L208`, `#L240`).
- Own Statuses (`/api/v1/issue-statuses`), and also `/issue-types`, `/priorities`, `/severities` (`BACK/taiga/projects/issues/models.py`).
- Single `assigned_to` like a Task; optional Sprint (`milestone`), so Issues can show up on a Taskboard (the web UI subscribes to `.issues` there).
- No board in the web UI: Issues are a filtered list (`FRONT/app/coffee/modules/issues/list.coffee`).
  A TUI board of Issues by Status would be a new view, not a copy of a web UI one.
- Promotion to a User Story: `POST /api/v1/issues/{id}/promote_to_user_story` (needs `add_us`, `BACK/taiga/projects/issues/permissions.py#L33`).
- Module flag `is_issues_activated` defaults to `true` (`BACK/taiga/projects/models.py#L206`).

## 5. Rendered Markdown

- Storage: `description`, `blocked_note` and comments are raw Markdown text; taiga-front 6 uses a WYSIWYG editor that converts to and from Markdown before saving (`FRONT/app/modules/components/wysiwyg/wysiwyg.directive.coffee`).
- Rendering: Python-Markdown with these extensions, then `bleach` sanitising (`BACK/taiga/mdrender/service.py#L63-L79`, `#L129-L131`):
  `pymdownx.tasklist`, autolink, automail, semi-sane lists, strikethrough (`~~x~~`), `[[wiki-slug|title]]` links, `:emoji:`, `@mentions`, `#ref` references, target-blank links, attachment refresh, `markdown.extensions.extra` (tables, fenced code, footnotes, definition lists, abbreviations), `codehilite`, `sane_lists`, `toc`, and `nl2br` (every newline is a hard break).
  This is Python-Markdown, not CommonMark or GFM; `nl2br` is the most visible difference.
- `@username` (`\B(@)([\w.-]+)\b`) becomes a link only when the user is a project member (`BACK/taiga/mdrender/extensions/mentions.py#L48-L80`).
- `#123` (`(?<=^|(?<=[^a-zA-Z0-9-\[]))#(\d+)`) resolves to whichever Epic, User Story, Task, Issue or Sprint has that `ref` in the project, with CSS class `reference epic|user-story|task|issue` (`BACK/taiga/mdrender/extensions/references.py#L46-L75`).
  `GET /api/v1/resolver?project=<slug>&ref=<n>` resolves a ref to its type and id (docs "Resolver").
- Attachments: links to media get a `#_taiga-refresh=<type>:<attachmentId>` fragment (`BACK/taiga/projects/attachments/services.py#L24-L63`, `BACK/taiga/mdrender/extensions/refresh_attachment.py`).
  On self-hosted taiga-docker, media is served through `taiga-protected` with signed URLs that expire after `ATTACHMENTS_MAX_AGE=360` seconds (`DOCKER/.env#L33`, `DOCKER/docker-compose.yml#L136-L139`).
  The web UI re-fetches the attachment to get a fresh URL (`FRONT/app/modules/components/wysiwyg/wysiwyg.service.coffee#L98-L136`); a TUI showing attachment links should do the same.
- Pre-rendered HTML ships with every payload: `description_html` and `blocked_note_html` on User Stories and Tasks (`BACK/taiga/projects/userstories/serializers.py#L147-L160`, `BACK/taiga/projects/tasks/serializers.py#L68-L80`) and `comment_html` on history entries.
- Preview of unsaved text: `POST /api/v1/wiki/render` with `{"content": ..., "project_id": ...}` returns `{"data": "<html>"}` (`BACK/taiga/projects/wiki/api.py#L60-L76`).
- Implication for the TUI: either render the raw Markdown locally (a Go Markdown renderer plus Taiga-specific `@mention` and `#ref` handling and `nl2br`) or convert `*_html` to terminal text.
  The HTML route gives exactly what the web UI shows, including resolved refs and mentions.

## Open points

- HITL check: authenticate a websocket on the team Instance with a TUI token and observe a `changes.project.<id>.tasks` message after a change from the web UI.
- Whether self-hosted Instances other than the team Instance matter enough to design an explicit "no live updates" state.
