# GitHub parent-linkage via native sub-issues

The GitHub backend models the **PRD → sub-issue** relationship using GitHub's native sub-issues feature (the GraphQL `parent` / `subIssues` connection on `Issue`), not a tasklist in the PRD body, not a `prd:<n>` label convention, and not a `Parent: #N` body reference. This is the only option that gives us a real, queryable parent link symmetric with Jira's `parentIssueId` — listing a PRD's children is one GraphQL call from the parent ID, and the child's own `parent` is a first-class field. The trade-off is that the parent linkage and child listing need `gh api graphql` rather than the plain `gh issue ...` flags, and the feature is newer than tasklists; we accept that cost to keep the `Backend` interface honest and to avoid building (and later maintaining) a brittle markdown-parsing or label-mining alternative.

## Considered options

- **Native sub-issues (chosen).** Real parent edge, queryable both directions, survives edits.
- **Tasklist in PRD body** (`- [ ] #743`). No child-side parent field; listing requires parsing PRD markdown; fragile across manual edits.
- **Label convention** (`prd:742` on every sub-issue). Queryable via `gh issue list --label prd:742`, but auto-created labels pollute the repo's label palette and the link can be broken by anyone editing labels.
- **Body reference** (`Parent: #742` in sub-issue body). Cheapest to write, effectively un-queryable: project-wide listing needs full-text search.
