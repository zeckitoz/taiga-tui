# Existing Taiga clients in Go and Taiga TUIs/CLIs

Research for issue #7 (part of the wayfinder map #1).
Researched 2026-11-11.
All claims are from the repositories' source code, READMEs, git history and GitHub metadata as of that date.

## Question

Which Go libraries for the Taiga API exist (e.g. taigo), how current and complete are they (auth, Sprints/milestones, User Stories, Tasks, PATCH with `version`), and is using one worth it compared to a slim own client?
Are there existing Taiga TUIs or CLIs (any language) whose interaction concept or solutions we can reuse?

## Answer in short

Write our own slim client (`internal/taiga`, `net/http` + `encoding/json`), and use two MIT-licensed Go projects as references rather than as dependencies:

- `theriverman/taigo` for the shape of models and query parameters.
- `KoukeNeko/taiga-cli` for API discovery from `conf.json`, the conflict (stale `version`) classification, token storage in the OS keyring, and cross-platform release packaging.

There is no maintained Taiga TUI in any language.
The only one, `taiga-ncurses`, is an archived 2014 proof of concept.

## Go libraries

### theriverman/taigo (the only real Go library)

Source: https://github.com/theriverman/taigo, docs: https://pkg.go.dev/github.com/theriverman/taigo/v2

- MIT, 14 stars, essentially a single maintainer (124 of ~137 commits by one author).
- Active: v2.0.0 released 2026-03-21, latest tag v2.0.4 on 2026-10-11, last push 2026-10-18.
- About 11.6k lines of non-test Go, depends only on `google/go-querystring` and `golang.org/x/text`.
- `go.mod` declares `go 1.27.2` while the README says "Go 1.25 or newer" (https://github.com/theriverman/taigo/blob/master/go.mod).
  A dependency's `go` directive is a hard minimum for every consumer, so adopting it pins our toolchain floor to whatever the maintainer bumps to.

Coverage relevant to the MVP:

| Need | Taigo | Notes |
| --- | --- | --- |
| Login | `Client.AuthByCredentials` (`type: normal`), `AuthByToken` | `auth.go`, `client.go`. Starts a background goroutine refreshing the token every 12 h unless disabled. |
| Token refresh | `AuthService.RefreshAuthToken` | `auth.go` |
| Projects | `Project.List`, `Get`, `GetBySlug` | `projects.go` |
| Sprints | `Milestone.List/Get/Patch` | `milestones.go` |
| User Stories | `UserStory.List/Get/Patch`, query params incl. `milestone`, `status`, `assigned_to`, `status__is_closed` | `user_stories.go`, `user_stories.models.go` |
| Tasks | `Task.List/Get/Patch`, query params incl. `milestone`, `user_story`, `assigned_to` | `tasks.go`, `tasks.models.go` |
| Statuses | `TaskStatus.List`, `UserStoryStatus.List` | `task_status.go`, `user_story_status.go` |
| `*_extra_info` (assignee, status, user story) | Yes, on detail models | `tasks.models.go` lines 88-121 |
| Pagination | Disabled by default via `x-disable-pagination` | `client.go` line 169 |
| PATCH with `version` | `Task.Patch(id, *TaskPatch)` refuses `Version == 0` | `tasks.go` lines 244-262 |
| Errors | Generic `APIError{StatusCode, Body}`; no typed stale-version error | `requests.go` |
| `context.Context` | Only on the raw `RequestService.*Ctx` methods; typed services (`Task.List`, `Task.Patch`, ...) call `context.Background()` | `requests.go`, `tasks.go` |

Things that would bite a TUI:

- The convenience `Edit(task)` builds the PATCH body by skipping zero values (`if task.AssignedTo != 0`, `tasks.go` lines 177-235), so it cannot clear fields.
  `TaskPatch` uses `*int` with `omitempty`, so it cannot send `"assigned_to": null` either (unassign is not needed for the MVP, but shows the model is lossy).
- No per-request cancellation on typed calls, which matters when the user switches views quickly and in-flight requests should be dropped.
- A stale-version rejection (HTTP 400 with a `version` key) is indistinguishable from any other 400 without parsing `APIError.Body` ourselves.
- The automatic refresh goroutine mutates shared client state behind our back; we would disable it and own refresh anyway.

Verdict: complete and current, but for our ~10 endpoints we would use a thin slice of a large surface, work around three of its design choices, and inherit its toolchain floor.

### Other Go repositories

- `metatexx/taigo`: 0 stars, last push 2020-05-25. Dead. https://github.com/metatexx/taigo
- `takuan-osho/gotaiga`: 0 stars, last update 2018. Dead. https://github.com/takuan-osho/gotaiga
- Go CLIs below keep their client in `internal/` packages, which Go forbids importing from other modules; they are references only.

## Taiga CLIs and TUIs

### KoukeNeko/taiga-cli (Go, the best reference)

Source: https://github.com/KoukeNeko/taiga-cli

- MIT, created 2026-08-31, last push 2026-10-21, ~17.5k lines non-test Go, cobra-based.
- Verified with a Docker E2E against Taiga 6.10.2; earlier Taiga 6 "unverified"; TaigaNext unsupported (https://github.com/KoukeNeko/taiga-cli/blob/main/COMPATIBILITY.md).
- Pure Go binaries for macOS, Linux and Windows (amd64/arm64), `CGO_ENABLED=0`.

Solutions worth copying (with attribution, MIT):

1. **API discovery from the frontend's `conf.json`** (`internal/taiga/discovery.go`, `DiscoverAPI`): the user types any Taiga page URL, the tool reads `conf.json` and probes the `api` URL, including sites under a subpath.
   This fits our case: users know `https://taiga-fki.ostfalia.de/`, not the API path.
2. **Stale-version classification by structure, not wording** (`internal/taiga/client.go`, `classifyAPIError` / `isStaleVersion`): a 400 whose `version` value is a plain string is a conflict, a list is a validation error, because Taiga translates the message text.
3. **Token storage**: OS keyring on macOS/Windows via `github.com/zalando/go-keyring`; on Linux a `0600` file by default because the Secret Service is often locked over SSH (README "Log in", commit c9ab2d7).
4. **Ambiguous writes**: only idempotent GETs are retried; an interrupted write is reported as "may have been committed" instead of resent.
5. **SSO/LDAP accounts**: no generic interactive login; the documented fallback is importing `{"auth_token", "refresh"}` from the web app's Local Storage (COMPATIBILITY.md, "Authentication").

### Confirming the conflict semantics in taiga-back

`taiga/projects/occ/mixins.py` in taiga-back (https://github.com/kaleidos-ventures/taiga-back/blob/main/taiga/projects/occ/mixins.py), `OCCResourceMixin._validate_and_update_version`:

- A missing, negative or future `version` raises `WrongArguments({"version": "The version parameter is not valid"})`, so `version` is mandatory on every PATCH.
- If the sent version is older than the current one, Taiga computes the fields changed since then (`get_modified_fields`) and only rejects (`"The version doesn't match with the current one"`) when they intersect the fields in our PATCH.
  A Status Change therefore succeeds on a stale version as long as nobody else changed `status` in between; an Assign to me only conflicts if `assigned_to` changed.
- On success the version is incremented and returned in the response body, which we must store for the next write.

Note that the "not valid" case is also a plain string under `version`, so KoukeNeko's rule would classify it as a conflict; harmless if we always send the version we last read, but our client should keep the two apart (e.g. by never sending a version we did not receive).

### Other CLIs (concept references only)

- `BasisTI/taiga-cli` (Go, Apache-2.0, created 2026-09-29, 25 commits): agent-oriented CLI with a generic `taiga api` passthrough doing "version-checked writes and transparent pagination". https://github.com/BasisTI/taiga-cli
- `dimaskiddo/taiga-cli` (Go, 7 commits): bulk task creation from a file, credentials in `.env`. Not relevant. https://github.com/dimaskiddo/taiga-cli
- `paulchambaz/taiga-cli` (Rust, GPL-3.0, taskwarrior-style): `taiga demo 3 move in-progress`, `@me` filter, numbers are positions in the last printed list. Good inspiration for status names as fuzzy targets and an "@me" style "only mine" filter; GPL, so ideas only, no code. https://github.com/paulchambaz/taiga-cli
- `nephila/python-taiga` (Python, MIT, 137 stars, active): the most-used Taiga client library overall, useful to cross-check field names. https://github.com/nephila/python-taiga
- Several small Python/TypeScript "taiga-cli" repos (`stvishwa06`, `aliepratama`, `teksimx`, ...) target LLM agents; nothing reusable for a TUI.

### taiga-ncurses (the only Taiga TUI)

Source: https://github.com/kaleidos-ventures/taiga-ncurses

- Official Kaleidos project, Python 3 + urwid, Apache-2.0, archived, last push 2015-02-20.
- README: "a proof of concept ... It isn't finished yet and currently it isn't feature complete."
- Targets the 2014 API; code is not reusable.
- Concept takeaways (`taiga_ncurses/config.py`): top-level single-letter view switches (`P` projects, `B` backlog, `M` milestone/Sprint, `I` issues), per-view key maps with `r` reload and `?` help, a user-overridable keymap and colour palette in a config file, and API calls run on a small thread pool (`executor.py`) so the UI never blocks.

No Go/Bubble Tea or other maintained Taiga TUI was found (GitHub searches "taiga tui", "taiga terminal", "taiga ncurses", "taiga urwid", "taiga bubbletea", "taiga language:go").

## Recommendation

Own slim client, because:

- The MVP needs about ten calls: login, refresh, `users/me`, projects list, milestones list, user stories list, tasks list, statuses (task and user story) and memberships, plus two PATCH variants (status, assigned_to).
  That is a few hundred lines with exactly the semantics we want.
- We want `context.Context` on every call, a typed `ErrConflict` derived from taiga-back's OCC rules, explicit nullable fields, and no background goroutines.
- We avoid inheriting taigo's `go 1.27.2` floor and a single-maintainer dependency.
- Testing is simpler against our own small interface (fake server via `httptest`).

Borrow deliberately:

- Model field names and query parameters from taigo (MIT) and the official API docs.
- `conf.json` discovery, stale-version detection, keyring storage and release packaging ideas from KoukeNeko/taiga-cli (MIT).
- Keymap conventions (`r` reload, `?` help, configurable keys) from taiga-ncurses, and the `@me`/fuzzy-status ideas from paulchambaz/taiga-cli (ideas only, GPL).

## Open points surfaced

- Which Taiga version runs on taiga-fki.ostfalia.de? KoukeNeko only verifies 6.10.2 and declares TaigaNext a different API; worth confirming once via public endpoints.
- Do university accounts log in with `type: normal` (e.g. an LDAP backend behind the normal login) or need a token import fallback like KoukeNeko's?
