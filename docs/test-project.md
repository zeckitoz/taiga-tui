# Taiga test project and fixtures

The fixtures in `internal/taiga/testdata/` are anonymised responses recorded from a dedicated private test project on https://taiga-fki.ostfalia.de/.
The project's name, its slug and all credentials stay out of the repository and the issue tracker.
The recorder replaces the project with `Test Project` / `test-project` and every Team Member with `member-N` / `Member N`.

## Getting access

1. Log in once to https://taiga-fki.ostfalia.de/ with your own Ostfalia Taiga account.
2. Ask a Team Member who is an admin of the test project to invite you under **Admin → Members**.
3. Accept the invitation from the e-mail Taiga sends you.
   The project then appears in your project list, and its slug is the last part of its URL.

Use your own account for recording.
Never share passwords or tokens, and never write the project's name or slug into a commit, issue or pull request.

## What the project must contain

The tests rely on these cases, so keep them when you change the project:

- Kanban and Backlog are enabled.
- Exactly one Sprint whose date range contains the recording date, holding User Stories A to D.
  The recording takes its Taskboard (`userstories_sprint.json`, `tasks_sprint.json`) and its first Task (`task.json`).
- User Story Statuses from the Scrum template, with a WIP limit of 3 on "In progress", "Done" closed and "Archived" archived.
- User Story A in "In progress" with two Assignees, where `assigned_to` is not the first entry of `assigned_users`, and two Tasks: one closed with an Assignee, one open without.
  It has a description, which `userstory.json` records as `redacted`.
  The web UI makes the first Assignee added `assigned_to`, while `assigned_users` is sorted by ID.
  If `assigned_to` comes out first, remove that Assignee and add them again.
- User Story B in "New" with no Assignee and no Tasks.
- User Story C in "Ready for test" with one Assignee and one open Task.
- User Story D in "Done".
- User Story E in "Archived", which the Kanban Board must leave out.

Two cases cannot come from the live project and are answered inline in `internal/taiga/client_test.go`: a Team Member without a full name, and `tasks: null`.
The error responses (broken JSON, 5xx, version conflict, timeout) are hand-written as well.

## Recording

Run this in your own terminal, so the password is neither echoed nor kept in the shell history:

```zsh
read -r "TAIGA_TUI_LIVE_USER?User: "; read -rs "TAIGA_TUI_LIVE_PASSWORD?Password: "; echo
export TAIGA_TUI_LIVE_USER TAIGA_TUI_LIVE_PASSWORD
TAIGA_TUI_LIVE_PROJECT=<project-slug> go test ./internal/taiga -run TestRecordFixtures -record
```

`TAIGA_TUI_LIVE_URL` overrides the instance.
Pseudonyms are numbered by Team Member ID and projects by project ID, so a new recording keeps them stable and the diff shows only real data changes.

## Reviewing a recording

1. Run `go test ./...`.
   `TestFixturesHoldNoEmailAddressesOrJWTs` must pass, and tests that check recorded values show what changed.
2. Read the fixture diff for anything personal the anonymiser cannot know about, such as names in free text, links to video rooms or other secrets.
   Listing every string value helps:

   ```sh
   cat internal/taiga/testdata/*.json | jq -r '.. | strings' | sort -u
   ```

3. If something slipped through, fix the anonymiser in `internal/taiga/taigarec` with a test and record again, instead of editing the fixtures by hand.
