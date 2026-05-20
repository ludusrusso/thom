# Agent guide: `thomctl`

You are an LLM agent operating on a repo whose issue tracker is wrapped by
`thomctl`. **Use `thomctl` for every PRD and sub-issue operation.** Do NOT
shell out to `acli` / `gh` directly, and do NOT write ADF JSON by hand —
`thomctl` accepts markdown and handles the translation to the active backend.

## The 30-second mental model

- A **PRD** is a top-level parent issue (Jira Epic / GitHub Issue with the
  `PRD` label). Created from a markdown body. Always gets the `PRD` +
  `ready-for-agent` labels.
- A **sub-issue** is a child of a PRD. Each represents a
  *tracer-bullet vertical slice*. Tag it either `--hitl` (needs a human) or
  `--afk` (you can run it). `--afk` also adds `ready-for-agent`.
- **Labels** are how the `triage` skill drives its state machine. Use
  `thomctl issue label add/remove`. Don't invent new labels.
- The current user is **auto-assigned** on every create. Pass `--assignee ''`
  to leave unassigned, or `--assignee EMAIL` for someone else.

## Before you start: check the config

`thomctl` is driven by `.thomctl.yaml` at the project root (or any ancestor
up to `$HOME`, or `$XDG_CONFIG_HOME/thomctl/config.yaml`). **You are
responsible for making sure this file exists and points at the right
backend/project** — `thomctl` will not invent values for you. Run this
first:

```sh
thomctl config        # prints the resolved config + the file it came from
```

If the command errors with "config not found" (or the values are wrong for
this repo), create `.thomctl.yaml` at the repo root before doing anything
else. Minimal shapes:

```yaml
# Jira backend
backend: jira
prd_label: PRD
ready_label: ready-for-agent
default_labels: [engine]       # optional — applied to every new issue
default_assignee: "@me"        # optional — pass --assignee "" to opt out per call
jira:
  project_key: PROJ            # the Jira project key for this repo
  prd_issue_type: Epic
  subissue_issue_type: Task
  close_status: Done           # target status name; for IT locale use "Completata"
```

```yaml
# GitHub backend
backend: github
prd_label: PRD
ready_label: ready-for-agent
default_labels: [engine]
default_assignee: "@me"
github: {}                     # repo is resolved by `gh` from the cwd
```

If you're unsure which backend / project key the repo uses, **ask the
human** rather than guessing — the project key (Jira) or repo (GitHub) is
not something you can derive from the codebase. Commit `.thomctl.yaml` so
the next agent doesn't repeat this step.

## Cheat sheet

### Create a PRD

```sh
thomctl prd create --title "Improve onboarding" --body-file prd.md
# → OPE-742 (prints the new key on stdout)
```

Body is markdown. All standard CommonMark works: headings, lists, code
blocks, blockquotes, links, hr, bold/italic. `code` marks are
auto-decoupled from `em`/`strong` (Jira ADF rejects the combination).

### Create sub-issues (tracer-bullet slices)

```sh
thomctl issue create --parent OPE-742 --title "Wire schema" --body-file slice1.md --afk
thomctl issue create --parent OPE-742 --title "Pick storage" --body-file slice2.md --hitl
```

**Rule of thumb:**

- `--afk` → the slice is fully specified and you (the agent) can implement
  it without asking the human. Auto-applies `ready-for-agent`.
- `--hitl` → the slice needs a human decision/review first. Does NOT apply
  `ready-for-agent`.

### Issue links (dependencies, duplicates, "see also")

Use these to make dependencies *queryable* — JQL can then ask for "all the
things blocked by OPE-742":

```sh
thomctl issue link add OPE-744 --blocked-by OPE-743   # 744 can't start until 743 finishes
thomctl issue link add OPE-743 --blocks OPE-744       # equivalent, stated from 743's side
thomctl issue link add OPE-745 --relates-to OPE-744   # symmetric "see also"
thomctl issue link add OPE-746 --duplicate-of OPE-742 # 746 is a dupe of canonical 742

thomctl issue link list OPE-744                        # shows both directions
thomctl issue link list OPE-744 --json
thomctl issue link remove 10370                        # ID comes from `link list`
```

For the Pocock `to-issues` flow: after creating all slices for a PRD, run
`thomctl issue link add SLICE_N --blocked-by SLICE_PREV` for each dependency.
You can still narrate the dependency in the body markdown ("Blocked by:
OPE-743") — the link makes it machine-queryable too.

### List

**Completed items are hidden by default.** Every list command (`prd list`,
`issue list`, `prd issues`) drops items whose status is in the tracker's
"Done" category (Jira: `statusCategory = Done`; GitHub: `state = closed`).
Pass `--all` to include them, or pass `--status <name>` to filter exactly
— explicit status overrides `--all`.

```sh
thomctl prd list                            # open PRDs only
thomctl prd list --all                      # include completed ones
thomctl prd list --status BACKLOG --json    # exact status + JSON

thomctl prd issues OPE-742                  # open sub-issues of one PRD
thomctl prd issues OPE-742 --afk            # AFK-ready slices only
thomctl prd issues OPE-742 --all --json     # include completed slices, JSON

thomctl issue list --parent OPE-742         # same data as `prd issues OPE-742`
thomctl issue list --label needs-triage     # project-wide, for triage workflows
thomctl issue list --all --json             # machine output incl. completed
```

`prd issues <KEY>` is the ergonomic alias for `issue list --parent <KEY>`.
Use it when you have a PRD key and want its slices; use `issue list`
(no `--parent`) for project-wide triage queries.

The list output shows a `MODE` column (`HITL` / `AFK` / empty) so you can
eyeball the state at a glance. JSON output includes the full `labels` array.

### View one issue

```sh
thomctl prd view OPE-742           # human-readable, with description as plain text
thomctl issue view OPE-743 --json  # full record, for programmatic consumption
```

JSON shape:

```json
{
  "key": "OPE-743",
  "type": "Task",
  "summary": "...",
  "status": "BACKLOG",
  "labels": ["afk", "engine", "ready-for-agent"],
  "parent_key": "OPE-742",
  "assignee": "Ludovico Russo",
  "description": "..."
}
```

### Comment

```sh
thomctl issue comment OPE-743 --body-file comment.md
# Or inline (good for the triage AI-disclaimer header):
thomctl issue comment OPE-743 --body "> *This was generated by AI during triage.*

Closing because the original report is now obsolete."
```

### Triage: label add/remove

```sh
thomctl issue label add    OPE-743 ready-for-agent
thomctl issue label remove OPE-743 needs-triage
```

The five canonical triage states are labels: `needs-triage`, `needs-info`,
`ready-for-agent`, `ready-for-human`, `wontfix`. Apply exactly one state
label at a time.

### Close

```sh
thomctl issue close OPE-743                            # transitions to default close status
thomctl issue close OPE-743 --comment "Done in PR #42" # with a closing comment
thomctl issue close OPE-743 --status "Annullato"       # override target status (e.g. cancelled vs done)
```

`thomctl` verifies the status actually changed — if the Jira workflow doesn't
permit the transition from the current state, you get an error instead of a
silent no-op.

## What `thomctl` deliberately does NOT do

- Edit an existing issue's body. Re-create or comment instead.
- Delete issues. Use `acli jira workitem delete` if you really need to.
- Manage sprints, attachments, worklog, or transitions other than close.
- Track activity across comments for "needs re-triage" detection.

## Gotchas

- **Jira's JQL index is eventually consistent.** Right after `create`, a
  `list --parent KEY` may return empty for ~3 seconds. Trust the key
  returned by `create`; don't re-list to confirm.
- **`close` uses the configured `close_status` from `.thomctl.yaml`.** For
  Italian-language Jira projects (OPE), that's `"Completata"`. The
  *transition name* (`"Completato"`) is NOT what you pass — pass the
  *target status name*.
- **Default `--assignee` is `@me`.** If you want to assign to someone else,
  pass their email. If you want unassigned, pass `--assignee ''` explicitly.

## Discoverability

- `thomctl --help` — top-level commands
- `thomctl <command> --help` — flags for any subcommand
- `thomctl config` — print the resolved config (project key, default labels, etc.)
- `thomctl llm` — print this guide
