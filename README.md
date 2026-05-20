# thomctl — a tracker-agnostic issue CLI for autonomous agents

`thomctl` is a small Go binary that wraps an issue tracker (Jira via `acli`
or GitHub via `gh`) behind a tracker-agnostic surface designed for **agent
skills** — in particular, ralph-style autonomous coding loops.

The Matt Pocock skill set (`write-a-prd`, `to-prd`, `to-issues`,
`prd-to-issues`, `triage`) was written for GitHub Issues; it shells out to
`gh issue create / view / list / comment / label`. Drop the same skills onto
a Jira-tracked project and they fall over: there's ADF JSON to write, JQL
to quote, `parentIssueId` instead of `--parent`, locale-specific terminal
status names, and a handful of acli flags that silently no-op. `thomctl`
exposes one stable, semantic surface and makes the tracker-specific calls
on the agent's behalf:

```
thomctl prd    create | list | view | issues
thomctl issue  create | list | view | comment | label | link | close
thomctl config                               # show resolved config
thomctl llm                                  # print the agent-facing guide
```

Markdown goes in; the right ADF, JQL, and Jira workflow calls come out.

## Why a wrapper?

1. **Skills are markdown-native.** PRDs and slices are written in markdown
   in the agent's context. Jira's REST API speaks ADF JSON, with rules
   (e.g. `code` mark excludes `em` / `strong`) that produce silent
   `INVALID_INPUT` errors when violated. `thomctl` does that translation once,
   correctly.

2. **Agents shouldn't probe the API.** Without `thomctl`, agents spend turns
   discovering that `acli --from-json` ignores `--assignee`, that the
   transition name (`Completato`) is not the target status name
   (`Completata`), that creating an Epic uses `parentIssueId` not
   `--parent`, that `@me` resolves only on certain flags. `thomctl` makes one
   choice per operation and sticks to it.

3. **Tracker swap.** The Backend interface is the extension point. Adding
   a GitLab or local-markdown backend is one file under
   `internal/backend/<name>` plus a case in `internal/cli/root.go`. The
   CLI surface — and therefore the skill instructions — does not change.

## Designed for ralphs

A **ralph** is an autonomous coding loop: an LLM that runs the same small
prompt over and over against a queue of slices, picking the next ready-for-agent
item, doing the work, opening a PR, closing the issue, repeating. The pattern
only pays off when the ralph spends its turns on the *task*, not on
re-discovering how its tools work.

`thomctl` is the tracker-shaped surface a ralph needs:

- **One verb per operation.** `thomctl issue create --parent KEY --afk` —
  the ralph doesn't pick between `acli workitem create` and
  `gh issue create`, doesn't choose `parentIssueId` vs `--parent`, doesn't
  know what label means "ready for me." It just creates the slice.
- **Markdown in, tracker-native out.** PRDs and slices come out of the
  LLM as markdown. `thomctl` translates to ADF (Jira) or pass-through
  (GitHub). No agent-side ADF JSON, no `code`-mark gotchas.
- **No probing.** Every decision an agent would otherwise waste turns
  discovering (which transition name closes an issue, whether `@me`
  resolves on this flag, whether `--assignee` is silently ignored under
  `--from-json`) is encoded once in the backend. The ralph reads
  `thomctl llm` once and is done.
- **Stable across trackers.** Migrating a project from Jira to GitHub
  doesn't change the ralph's prompt. Flip `backend: jira` to
  `backend: github` in `.thomctl.yaml` and the same skills keep working.

## Install

```sh
make install   # → ~/.local/bin/thomctl
# or
go install github.com/ludusrusso/thom/cmd/thomctl@latest
```

Requires Go 1.25+ and a working `acli` (Atlassian CLI) with `acli auth login`
already done.

## Configure

`thomctl` reads `.thomctl.yaml` from the closest ancestor of the current
directory (stopping at `$HOME`), or `$XDG_CONFIG_HOME/thomctl/config.yaml`.
Override the file path with `$THOMCTL_CONFIG`. **All other settings live in
yaml** — there are no value-override env vars; the config file is the single
source of truth.

`thomctl config` prints the resolved values and the file they came from.

### Shared knobs (any backend)

These are skill conventions, not tracker-specific. They live at the top
level of `.thomctl.yaml`:

| Field             | Default           | Purpose                                            |
| ----------------- | ----------------- | -------------------------------------------------- |
| `backend`         | `jira`            | `jira` or `github`                                 |
| `prd_label`       | `PRD`             | Label that marks an issue as a PRD                 |
| `ready_label`     | `ready-for-agent` | Label applied to AFK slices                        |
| `default_labels`  | (none)            | Labels applied to every new issue                  |
| `default_assignee`| `@me`             | Auto-assign on create (pass `--assignee ""` to opt out per call) |

### Jira backend

```yaml
backend: jira

prd_label: PRD
ready_label: ready-for-agent
default_labels:
  - engine
default_assignee: "@me"

jira:
  project_key: PROJ
  prd_issue_type: Epic
  subissue_issue_type: Task
  close_status: Done           # target status name (NOT transition name)
```

For Italian-localized Jira workflows, set `close_status` to the locale's
target status name (e.g. `Completata`); see *Known quirks* below.

### GitHub backend

```yaml
backend: github

prd_label: PRD
ready_label: ready-for-agent
default_labels:
  - engine
default_assignee: "@me"

github: {}  # no fields today — the repo is resolved by `gh` from the cwd
```

The GitHub backend shells out to `gh`, which already knows the repo from
the current directory's git remote (or `gh repo set-default`). Run
`thomctl` from inside a checkout, or configure `gh` accordingly.

## Command tour

### PRDs

```sh
# Create a PRD from a markdown file. Auto-applies PRD + ready-for-agent + engine labels,
# auto-assigns the current user.
thomctl prd create --title "RC.report" --body-file prd.md
# → PROJ-742

thomctl prd list                                    # open PRDs only (completed hidden by default)
thomctl prd list --all                              # incl. completed
thomctl prd list --label engine --status BACKLOG    # filtered (explicit --status overrides --all)
thomctl prd list --json                             # machine-readable

thomctl prd view PROJ-742          # human-readable, description rendered to plain text
thomctl prd view PROJ-742 --json

thomctl prd issues PROJ-742        # list sub-issues of a PRD (open only by default)
thomctl prd issues PROJ-742 --afk  # AFK-ready slices only
thomctl prd issues PROJ-742 --all  # include completed slices
```

### Sub-issues (tracer-bullet slices)

```sh
# Two flavours: --afk auto-applies ready-for-agent; --hitl does NOT.
thomctl issue create --parent PROJ-742 --title "Wire schema" --body-file slice1.md --afk
thomctl issue create --parent PROJ-742 --title "Pick storage" --body-file slice2.md --hitl

# Listing is project-wide by default; --parent scopes to one PRD. Completed
# slices are hidden by default — pass --all to include them.
thomctl issue list --parent PROJ-742          # open slices of one PRD
thomctl issue list --parent PROJ-742 --all    # incl. completed
thomctl issue list --parent PROJ-742 --afk    # AFK-ready ones
thomctl issue list --parent PROJ-742 --hitl   # HITL ones
thomctl issue list --label needs-triage       # project-wide, used by triage
thomctl issue view PROJ-743 --json
```

The list table includes a `MODE` column showing `HITL` / `AFK` / empty so
you can scan slice readiness at a glance.

### Issue links

Make dependencies queryable in Jira instead of just narrating them in the
body:

```sh
thomctl issue link add PROJ-744 --blocked-by PROJ-743   # 744 can't start until 743 finishes
thomctl issue link add PROJ-743 --blocks PROJ-744       # equivalent, stated from 743's side
thomctl issue link add PROJ-745 --relates-to PROJ-744   # symmetric "see also"
thomctl issue link add PROJ-746 --duplicate-of PROJ-742

thomctl issue link list PROJ-744          # shows both directions, with LINK-ID
thomctl issue link remove 10370          # remove a single link by its Jira ID
```

For uncommon link types: `--type Cloners --to PROJ-740`.

### Triage

```sh
thomctl issue comment PROJ-743 --body-file note.md
thomctl issue comment PROJ-743 --body "> *This was generated by AI during triage.*

Closing because the original report is now obsolete."

thomctl issue label add    PROJ-743 ready-for-agent
thomctl issue label remove PROJ-743 needs-triage

thomctl issue close PROJ-743                            # default close_status from config
thomctl issue close PROJ-743 --comment "Done in PR #42" # with a closing comment
thomctl issue close PROJ-743 --status "Annullato"       # override (e.g. cancelled vs done)
```

`close` verifies the status actually changed — if the Jira workflow doesn't
permit the transition, you get an error rather than a silent no-op.

### Self-documenting

```sh
thomctl --help              # top-level
thomctl issue --help        # any subcommand
thomctl config              # resolved config + source file
thomctl llm                 # embedded agent-facing guide (markdown)
```

`thomctl llm` prints `internal/cli/llm_guide.md` — focused, copy-pasteable,
terse. Run it at the start of an agent session if the agent hasn't already
read `docs/agents/issue-tracker.md`.

## For agent skills

The Pocock skills are prompt-driven: they read a per-repo recipe at
`docs/agents/issue-tracker.md` to learn how to talk to the tracker. Two ways
to wire that up:

1. **Pointer.** Have the consuming repo's `docs/agents/issue-tracker.md`
   contain a one-line "run `thomctl llm` and follow that". Skills can then
   fetch the latest agent guide directly from the binary.
2. **Copy.** Copy `docs/agents/issue-tracker.md` from this repo into the
   consuming repo. It's a static snapshot of how the skills should call
   `thomctl`.

For a fresh repo, run `setup-matt-pocock-skills` first to scaffold the
expected directory structure, then drop in the `thomctl` recipe.

## Layout

```
cmd/thomctl/                  # main() — thin entry point
cmd/dump-adf/                 # debug: markdown file → ADF JSON, for inspection
internal/cli/                 # cobra command tree (root, prd, issue, link, config, llm)
internal/cli/llm_guide.md     # embedded agent-facing usage doc
internal/config/              # .thomctl.yaml loader (yaml-only)
internal/adf/                 # markdown → ADF (goldmark-based)
internal/adf/adf_test.go      # regression tests for ADF edge cases
internal/backend/
  ├── backend.go              # Backend interface (tracker-agnostic)
  ├── jira/                   # JiraBackend — shells out to acli
  └── github/                 # GithubBackend — shells out to gh (+ gh api graphql/REST)
docs/adr/                     # architectural decision records
docs/agents/
  └── issue-tracker.md        # Pocock-skill recipe to drop into consumer repos
CONTEXT.md                    # glossary of domain terms (PRD, Sub-issue, HITL, AFK)
```

## Known quirks

- **Jira search lag.** `thomctl issue list --parent KEY` right after
  `thomctl issue create --parent KEY ...` may return empty for ~3 seconds — the
  JQL index is eventually consistent. Trust the key returned by `create`;
  don't re-list to confirm.
- **`code` mark exclusivity.** ADF rejects `code` combined with `em` /
  `strong` / `strike`. `thomctl` strips the conflicting marks when emitting a
  code-marked text node so markdown like ``_an `thomctl` tool_`` becomes "an"
  (em) + "thomctl" (code) + "tool" (em), not the invalid em+code combo.
- **Italian-localized Jira workflows.** On Italian projects the terminal
  status is typically named `Completata` (target status), and the
  *transition* to get there is named `Completato`. `acli` wants the
  **target** name — pass `"Completata"` to `--status` or set
  `close_status = "Completata"` in `.thomctl.yaml`. The same pattern
  applies to any locale where transition and status names differ.
- **`@me` only resolves on certain acli flags.** `acli workitem create
  --from-json` silently ignores `--assignee` and rejects `@me` inside the
  JSON payload with "User not found". `thomctl` works around this by issuing
  a follow-up `acli workitem assign --assignee @me` after create. End
  result is the same; just be aware the binary does two API calls per
  create when `default_assignee` is set.
- **`acli link list` drops direction.** When the queried key is on the
  *outward* side of a link, `acli`'s dedicated `link list` returns
  `outwardIssueKey: null`. `thomctl` reads `issuelinks` from `workitem view`
  instead, which exposes both `inwardIssue` and `outwardIssue` cleanly.

## What thomctl deliberately doesn't do

- Edit an existing issue's body (re-create or comment instead).
- Delete issues (use `acli jira workitem delete` if you really need to).
- Sprints, attachments, worklog, transitions other than close.
- Track comment timestamps for "needs re-triage" detection — out of scope.

## Status

- [x] Jira backend: PRD + sub-issue lifecycle, links, triage labels, close
- [x] GitHub backend: same surface via `gh` + native sub-issues + dependencies
- [x] Markdown → ADF (CommonMark via goldmark, with `code` mark filtering)
- [x] Project-level `.thomctl.yaml` (yaml-only; no value-override env vars)
- [x] HITL / AFK first-class flags
- [x] Auto-assign current user on create (`default_assignee = "@me"`)
- [x] Embedded agent guide (`thomctl llm`)
- [ ] Local-markdown backend
- [ ] Edit existing issues
- [ ] Attachments
- [ ] Reporter-activity detection for re-triage
