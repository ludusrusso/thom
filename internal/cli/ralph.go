package cli

import (
	"github.com/spf13/cobra"

	"github.com/ludusrusso/thom/internal/ralph"
)

func ralphCmd() *cobra.Command {
	var (
		afk   bool
		force bool
	)
	c := &cobra.Command{
		Use:   "ralph <PRD>",
		Short: "Run the Ralph loop on a PRD: chew through open sub-issues, then open a PR.",
		Long: "Pre-flights the worktree (clean tree + on default branch), creates `ralph/<PRD>` " +
			"branch + worktree at `.worktrees/ralph/<PRD>`, loops Claude until no open sub-issues " +
			"remain (or two iterations make no progress), then drafts and opens a PR with `gh`.\n\n" +
			"Reads its prompt from `.thom/ralph/prompt.md` (scaffolded on first run). " +
			"Backend-agnostic for issue tracking (Jira or GitHub); the PR step always shells out to `gh`.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := buildRalphOpts(args[0])
			if err != nil {
				return err
			}
			opts.AFK = afk
			opts.Force = force
			return ralph.Run(opts)
		},
	}
	c.Flags().BoolVar(&afk, "afk", false, "Stream Claude output for unattended runs (no TTY).")
	c.Flags().BoolVar(&force, "force", false, "Skip pre-flight checks (dirty tree, branch).")

	c.AddCommand(ralphOnceCmd(), ralphCleanCmd())
	return c
}

func ralphOnceCmd() *cobra.Command {
	var afk bool
	c := &cobra.Command{
		Use:   "once <PRD>",
		Short: "Run one Claude iteration against the loop prompt in the current directory.",
		Long:  "For debugging the prompt or doing one slice by hand — no worktree, no PR.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := buildRalphOpts(args[0])
			if err != nil {
				return err
			}
			opts.AFK = afk
			return ralph.RunOnce(opts)
		},
	}
	c.Flags().BoolVar(&afk, "afk", false, "Stream Claude output (no TTY).")
	return c
}

func ralphCleanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clean",
		Short: "Remove all .worktrees/ralph/* worktrees (branches are kept).",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ralph.Clean()
		},
	}
}

func buildRalphOpts(prd string) (ralph.Opts, error) {
	be, cfg, err := resolveBackend()
	if err != nil {
		return ralph.Opts{}, err
	}
	return ralph.Opts{PRD: prd, Backend: be, Cfg: cfg}, nil
}
