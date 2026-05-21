You are drafting a pull request that ships PRD {{PRD}}.

## PRD title
{{PRD_TITLE}}

## PRD body
{{PRD_BODY}}

## Resolved sub-issues
{{RESOLVED_SUBISSUES}}

## Commits on this branch
{{COMMIT_LOG}}

Write a concise, high-quality PR title and description.

Strict output format:
- Line 1: the PR title (under 70 chars, use conventional-commit prefix like feat/fix/chore when appropriate, no trailing period)
- Line 2: exactly "---"
- Line 3+: the PR body in GitHub-flavored markdown. Include sections:
  - "## Summary" — 1-3 bullets explaining what the PR delivers and why
  - "## Resolved sub-issues" — the list above, verbatim
  - A final line "Closes {{PRD_REF}}"

Output ONLY the title, separator, and body. No preamble, no code fences, no commentary.
