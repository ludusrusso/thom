package ralph

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// preflight refuses to start Ralph if the working tree is dirty or HEAD is
// detached. The branch ralph started on is recorded as the PR base, so any
// branch is acceptable — pre-flight just guards against silent state loss.
// The user can override with --force.
func preflight(repoRoot string) error {
	if dirty, err := isDirty(repoRoot); err != nil {
		return err
	} else if dirty {
		return errors.New("working tree is dirty; commit or stash first (or pass --force)")
	}
	branch, err := currentBranch(repoRoot)
	if err != nil {
		return err
	}
	if branch == "HEAD" {
		return errors.New("HEAD is detached; check out a branch first (or pass --force)")
	}
	return nil
}

// setupWorktree creates (or reuses) the ralph/<prd> branch and worktree at
// .worktrees/ralph/<prd>. Returns the branch name and the absolute worktree
// directory.
func setupWorktree(repoRoot, prd string) (branch, dir string, err error) {
	branch = "ralph/" + prd
	dir = filepath.Join(repoRoot, ".worktrees", "ralph", prd)

	if !branchExists(repoRoot, branch) {
		if err := runIn(repoRoot, "git", "branch", branch); err != nil {
			return "", "", fmt.Errorf("create branch %s: %w", branch, err)
		}
	}
	if !pathExists(dir) {
		if err := runIn(repoRoot, "git", "worktree", "add", dir, branch); err != nil {
			return "", "", fmt.Errorf("create worktree %s: %w", dir, err)
		}
	}
	return branch, dir, nil
}

func isDirty(repoRoot string) (bool, error) {
	out, err := outputIn(repoRoot, "git", "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

func currentBranch(repoRoot string) (string, error) {
	out, err := outputIn(repoRoot, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func branchExists(repoRoot, branch string) bool {
	return runIn(repoRoot, "git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch) == nil
}

func gitRepoRoot() (string, error) {
	out, err := outputIn("", "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", errors.New("not inside a git repository")
	}
	return strings.TrimSpace(out), nil
}

func gitWorktreeRemove(dir string) error {
	return runIn("", "git", "worktree", "remove", "--force", dir)
}

func pathExists(p string) bool {
	return fileExists(p)
}
