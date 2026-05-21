// Package ralph runs the Ralph loop: a worktreed branch where Claude is
// invoked repeatedly to chew through a PRD's open sub-issues, followed by a
// pull request drafted from the resolved work.
//
// Public entry points: Run (the full loop), RunOnce (one Claude invocation),
// Clean (garbage-collect orphan ralph/* worktrees). The loop's sub-issue
// predicate goes through the configured Backend, so it works with any tracker
// (Jira or GitHub); the PR step always shells out to `gh`, on the assumption
// that the code host is GitHub regardless of where issues live.
package ralph

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ludusrusso/thom/internal/backend"
	"github.com/ludusrusso/thom/internal/config"
)

// Opts groups the inputs every Ralph entry point needs.
type Opts struct {
	PRD     string          // PRD key (e.g. "742" for GitHub, "OPE-123" for Jira)
	Backend backend.Backend // resolved backend
	Cfg     config.Config   // for source path → repo root
	Force   bool            // skip pre-flight checks
	AFK     bool            // streaming output mode for unattended runs
}

// Run executes the full Ralph loop: pre-flight, worktree, loop, PR.
func Run(opts Opts) error {
	if err := requirePRD(opts.PRD); err != nil {
		return err
	}
	repoRoot, err := gitRepoRoot()
	if err != nil {
		return err
	}
	if !opts.Force {
		if err := preflight(repoRoot); err != nil {
			return err
		}
	}
	baseBranch, err := currentBranch(repoRoot)
	if err != nil {
		return err
	}
	branch, worktreeDir, err := setupWorktree(repoRoot, opts.PRD)
	if err != nil {
		return err
	}
	fmt.Printf("=== Ralph: working in %s on branch %s (base: %s) ===\n", worktreeDir, branch, baseBranch)

	prompt, settingsPath, err := loadPromptAndSettings(repoRoot, worktreeDir)
	if err != nil {
		return err
	}
	if err := loop(opts, worktreeDir, prompt, settingsPath); err != nil {
		return err
	}
	return openPullRequest(opts, worktreeDir, branch, baseBranch, settingsPath)
}

// RunOnce makes one Claude invocation against the loop prompt in the current
// directory. No worktree, no PR.
func RunOnce(opts Opts) error {
	if err := requirePRD(opts.PRD); err != nil {
		return err
	}
	repoRoot, err := gitRepoRoot()
	if err != nil {
		return err
	}
	prompt, settingsPath, err := loadPromptAndSettings(repoRoot, repoRoot)
	if err != nil {
		return err
	}
	rendered := renderPrompt(prompt, opts.PRD)
	return invokeClaude(repoRoot, settingsPath, rendered, opts.AFK)
}

// Clean removes all ralph/<n> worktrees. Their branches are left in place so
// the user can revisit the work; `git branch -D ralph/<n>` is one command.
func Clean() error {
	repoRoot, err := gitRepoRoot()
	if err != nil {
		return err
	}
	worktreesDir := filepath.Join(repoRoot, ".worktrees", "ralph")
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("no ralph worktrees to clean")
			return nil
		}
		return err
	}
	var removed int
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(worktreesDir, e.Name())
		if err := gitWorktreeRemove(dir); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to remove %s: %v\n", dir, err)
			continue
		}
		fmt.Printf("removed %s\n", dir)
		removed++
	}
	fmt.Printf("cleaned %d worktree(s)\n", removed)
	return nil
}

func requirePRD(prd string) error {
	if prd == "" {
		return errors.New("PRD key is required")
	}
	return nil
}
