# thomctl — RedCarbon issue-tracker CLI

`thomctl` is a small Go binary that wraps an issue tracker (today: **Jira via
`acli`**) behind a tracker-agnostic surface designed for **agent skills**.

The Matt Pocock skill set (`write-a-prd`, `to-prd`, `to-issues`,
`prd-to-issues`, `triage`) was written for GitHub Issues — it shells out to
`gh issue create / view / list / comment / label`. RedCarbon tracks work in
Jira. Instead of teaching every skill how to write ADF JSON, where to put
`parentIssueId`, what Italian terminal-status name to transition to, and
which sentinel acli accepts where, `thomctl` exposes a stable, semantic surface:

```
thomctl prd    create | list | view
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
   a GitHub, GitLab, or local-markdown backend is one file under
   `internal/backend/<name>` plus a case in `internal/cli/root.go`. The
   CLI surface — and therefore the skill instructions — does not change.

## Install

```sh
make install   # → ~/.local/bin/thomctl
# or
go install github.com/ludusrusso/thom/cmd/thomctl@latest
```

Requires Go 1.25+ and a working `acli` (Atlassian CLI) with `acli auth login`
already done.

## Configure

`thomctl` reads `.thomctl.yaml` from the closest ancestor of the current directory
(stopping at `$HOME`), or `$XDG_CONFIG_HOME/thomctl/config.yaml`. Override the
file path with `$THOMCTL_CONFIG`. Env vars beat file values:

| Env                          | Purpose                                              |
| ---------------------------- | ---------------------------------------------------- |
| `THOMCTL_BACKEND`               | `jira` (only supported today)                        |
| `THOMCTL_JIRA_PROJECT_KEY`      | e.g. `OPE`                                           |
| `THOMCTL_JIRA_PRD_TYPE`         | default `Epic`                                       |
| `THOMCTL_JIRA_SUBISSUE_TYPE`    | default `Task`                                       |
| `THOMCTL_JIRA_PRD_LABEL`        | default `PRD`                                        |
| `THOMCTL_JIRA_READY_LABEL`      | default `ready-for-agent`                            |
| `THOMCTL_JIRA_DEFAULT_LABELS`   | comma-separated, applied to every new issue          |
| `THOMCTL_JIRA_DEFAULT_ASSIGNEE` | default `@me` (auto-assign current user on create)   |
| `THOMCTL_JIRA_CLOSE_STATUS`     | target status for `close`, default `Done`            |

`thomctl config` prints the resolved values and the file they came from.

Example `.thomctl.yaml` for the RedCarbon OPE project:

```yaml
backend: jira

jira:
  project_key: OPE
  prd_issue_type: Epic
  subissue_issue_type: Task
  prd_label: PRD
  ready_label: ready-for-agent
  default_labels:
    - engine
  default_assignee: "@me"      # auto-assign current user on create
  close_status: Completata     # target status name (NOT transition name)
```

## Command tour

### PRDs

```sh
# Create a PRD from a markdown file. Auto-applies PRD + ready-for-agent + engine labels,
# auto-assigns the current user.
thomctl prd create --title "RC.report" --body-file prd.md
# → OPE-742

thomctl prd list                                    # all PRDs in the project
thomctl prd list --label engine --status BACKLOG    # filtered
thomctl prd list --json                             # machine-readable

thomctl prd view OPE-742          # human-readable, description rendered to plain text
thomctl prd view OPE-742 --json
```

### Sub-issues (tracer-bullet slices)

```sh
# Two flavours: --afk auto-applies ready-for-agent; --hitl does NOT.
thomctl issue create --parent OPE-742 --title "Wire schema" --body-file slice1.md --afk
thomctl issue create --parent OPE-742 --title "Pick storage" --body-file slice2.md --hitl

# Listing is project-wide by default; --parent scopes to one PRD.
thomctl issue list --parent OPE-742          # all slices of one PRD
thomctl issue list --parent OPE-742 --afk    # AFK-ready ones
thomctl issue list --parent OPE-742 --hitl   # HITL ones
thomctl issue list --label needs-triage      # project-wide, used by triage
thomctl issue view OPE-743 --json
```

The list table includes a `MODE` column showing `HITL` / `AFK` / empty so
you can scan slice readiness at a glance.

### Issue links

Make dependencies queryable in Jira instead of just narrating them in the
body:

```sh
thomctl issue link add OPE-744 --blocked-by OPE-743   # 744 can't start until 743 finishes
thomctl issue link add OPE-743 --blocks OPE-744       # equivalent, stated from 743's side
thomctl issue link add OPE-745 --relates-to OPE-744   # symmetric "see also"
thomctl issue link add OPE-746 --duplicate-of OPE-742

thomctl issue link list OPE-744          # shows both directions, with LINK-ID
thomctl issue link remove 10370          # remove a single link by its Jira ID
```

For uncommon link types: `--type Cloners --to OPE-740`.

### Triage

```sh
thomctl issue comment OPE-743 --body-file note.md
thomctl issue comment OPE-743 --body "> *This was generated by AI during triage.*

Closing because the original report is now obsolete."

thomctl issue label add    OPE-743 ready-for-agent
thomctl issue label remove OPE-743 needs-triage

thomctl issue close OPE-743                            # default close_status from config
thomctl issue close OPE-743 --comment "Done in PR #42" # with a closing comment
thomctl issue close OPE-743 --status "Annullato"       # override (e.g. cancelled vs done)
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
cmd/thomctl/             # main() — thin entry point
cmd/dump-adf/         # debug: markdown file → ADF JSON, for inspection
internal/cli/         # cobra command tree (root, prd, issue, link, config, llm)
internal/cli/llm_guide.md   # embedded agent-facing usage doc
internal/config/      # .thomctl.yaml loader + env var overrides
internal/adf/         # markdown → ADF (goldmark-based)
internal/adf/adf_test.go    # regression tests for ADF edge cases
internal/backend/
  ├── backend.go      # Backend interface (tracker-agnostic)
  └── jira/           # JiraBackend — shells out to acli
docs/agents/
  └── issue-tracker.md      # Pocock-skill recipe to drop into consumer repos
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
- **Italian Jira workflow.** The OPE project's terminal status is named
  `Completata` (target status), and the *transition* to get there is named
  `Completato`. `acli` wants the **target** name — pass `"Completata"` to
  `--status` or set `close_status = "Completata"` in `.thomctl.yaml`.
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
- [x] Markdown → ADF (CommonMark via goldmark, with `code` mark filtering)
- [x] Project-level `.thomctl.yaml` + env var overrides
- [x] HITL / AFK first-class flags
- [x] Auto-assign current user on create (`default_assignee = "@me"`)
- [x] Embedded agent guide (`thomctl llm`)
- [ ] GitHub backend
- [ ] Local-markdown backend
- [ ] Edit existing issues
- [ ] Attachments
- [ ] Reporter-activity detection for re-triage
