# thomctl

A tracker-agnostic CLI that exposes a stable, semantic surface (PRDs, sub-issues, links, labels, close) over a pluggable **Backend**. Skills speak `thomctl`; `thomctl` speaks the tracker.

## Language

**Backend**:
A pluggable implementation of the tracker-agnostic `backend.Backend` interface. Today: Jira (shells out to `acli`). Next: GitHub (shells out to `gh`).
_Avoid_: Driver, adapter, provider.

**PRD**:
A top-level, parent-style issue that holds the product description. In Jira this is an `Epic` tagged with the `PRD` label; in GitHub it is an Issue tagged with the `PRD` label.
_Avoid_: Epic (Jira-specific), parent issue (ambiguous).

**Sub-issue**:
A tracer-bullet slice of work whose parent is a **PRD**. In Jira this is a `Task` with `parentIssueId` set; in GitHub the parent linkage is TBD.
_Avoid_: Child issue, subtask.

**HITL**:
A **Sub-issue** that needs a human before it can be picked up. Carries the `hitl` label; deliberately does NOT carry `ready-for-agent`.
_Avoid_: Manual, blocked-on-human.

**AFK**:
A **Sub-issue** that an agent can run away-from-keyboard. Carries `afk` + `ready-for-agent`.
Also the name of the unattended **Ralph** run mode (`thomctl ralph <prd> --afk`) — same idea, applied to the loop itself.
_Avoid_: Auto, agent-runnable.

**Ralph**:
The agent loop that drives a **PRD** to a pull request by repeatedly invoking Claude on its open **Sub-issues** until none remain. Exposed as `thomctl ralph <prd>` (loop), `thomctl ralph <prd> --afk` (loop with streaming output for unattended runs), and `thomctl ralph once <prd>` (single Claude invocation). Reads its prompt from `.thom/ralph/prompt.md`. Backend-agnostic for issue tracking; the PR step always shells out to `gh` because the code host is GitHub independent of where issues live.
_Avoid_: Agent, worker, runner.
