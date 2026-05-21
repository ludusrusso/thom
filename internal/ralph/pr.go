package ralph

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/ludusrusso/thom/internal/backend"
)

// openPullRequest pushes the branch, asks Claude to draft a title and body
// from the PRD + resolved sub-issues + commit log, then shells out to gh.
// baseBranch is the branch ralph was launched from — the PR targets it,
// and the commit log is scoped to it.
// Falls back to a mechanical template if Claude's output doesn't parse.
func openPullRequest(opts Opts, worktreeDir, branch, baseBranch, settingsPath string) error {
	fmt.Println("=== Pushing branch and opening PR ===")
	if err := runIn(worktreeDir, "git", "push", "-u", "origin", branch); err != nil {
		return fmt.Errorf("push %s: %w", branch, err)
	}

	prd, err := opts.Backend.View(opts.PRD)
	if err != nil {
		return fmt.Errorf("fetch PRD: %w", err)
	}
	allIssues, err := opts.Backend.ListSubissues(opts.PRD, backend.ListOpts{IncludeClosed: true})
	if err != nil {
		return fmt.Errorf("list sub-issues: %w", err)
	}
	resolved := formatResolved(allIssues)
	commits := commitLogSince(worktreeDir, baseBranch, branch)

	title, body, ok := draftPR(worktreeDir, settingsPath, draftInputs{
		PRDKey:   opts.PRD,
		PRDRef:   prdRef(opts.PRD),
		PRDTitle: prd.Summary,
		PRDBody:  prd.Description,
		Resolved: resolved,
		Commits:  commits,
	})
	if !ok {
		title = prd.Summary
		body = fallbackBody(prd.Summary, opts.PRD, resolved)
	}

	args := []string{
		"pr", "create",
		"--base", baseBranch,
		"--head", branch,
		"--title", title,
		"--body", body,
	}
	return runIn(worktreeDir, "gh", args...)
}

type draftInputs struct {
	PRDKey   string
	PRDRef   string // "#742" for GitHub
	PRDTitle string
	PRDBody  string
	Resolved string
	Commits  string
}

func draftPR(workdir, settingsPath string, in draftInputs) (title, body string, ok bool) {
	prompt := DefaultPRPrompt()
	repl := strings.NewReplacer(
		"{{PRD}}", in.PRDKey,
		"{{PRD_REF}}", in.PRDRef,
		"{{PRD_TITLE}}", in.PRDTitle,
		"{{PRD_BODY}}", in.PRDBody,
		"{{RESOLVED_SUBISSUES}}", in.Resolved,
		"{{COMMIT_LOG}}", in.Commits,
	)
	rendered := repl.Replace(prompt)

	cmd := exec.Command("claude", "-p", "--model", "sonnet", "--settings", settingsPath, rendered)
	cmd.Dir = workdir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return "", "", false
	}
	return parseDraft(out.String())
}

// parseDraft expects "title\n---\nbody...". Empty title or empty body means
// the draft failed and the caller should fall back.
func parseDraft(s string) (title, body string, ok bool) {
	lines := strings.SplitN(s, "\n", 3)
	if len(lines) < 3 {
		return "", "", false
	}
	t := strings.TrimSpace(lines[0])
	sep := strings.TrimSpace(lines[1])
	b := strings.TrimSpace(lines[2])
	if t == "" || sep != "---" || b == "" {
		return "", "", false
	}
	return t, b, true
}

// prdRef formats a PRD key for the "Closes ..." line of a PR body. GitHub
// keys are numeric; everything else (e.g. Jira "OPE-742") is rendered as-is.
func prdRef(key string) string {
	if isNumeric(key) {
		return "#" + key
	}
	return key
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func formatResolved(issues []backend.Issue) string {
	var b strings.Builder
	for _, i := range issues {
		if !isClosedStatus(i.Status) {
			continue
		}
		fmt.Fprintf(&b, "- %s %s\n", prdRef(i.Key), i.Summary)
	}
	return strings.TrimRight(b.String(), "\n")
}

func commitLogSince(worktreeDir, baseBranch, branch string) string {
	out, err := outputIn(worktreeDir, "git", "log", baseBranch+".."+branch, "--format=- %s")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func fallbackBody(prdTitle, prdKey, resolved string) string {
	return fmt.Sprintf(`## Summary

Implements PRD %s: %s

## Resolved sub-issues

%s

Closes %s
`, prdRef(prdKey), prdTitle, resolved, prdRef(prdKey))
}
