package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ludusrusso/thom/internal/backend"
	"github.com/ludusrusso/thom/internal/config"
)

func issueCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "issue",
		Short: "Create, list, and read sub-issues of a PRD.",
	}
	c.AddCommand(
		issueCreateCmd(),
		issueEditCmd(),
		issueListCmd(),
		issueViewCmd(),
		issueCommentCmd(),
		issueLabelCmd(),
		issueLinkCmd(),
		issueBacklogCmd(),
		issueTodoCmd(),
		issueStartCmd(),
		issueReviewCmd(),
		issueTransitionCmd(),
		issueCloseCmd(),
	)
	return c
}

func issueCreateCmd() *cobra.Command {
	var (
		parent   string
		title    string
		body     string
		bodyFile string
		labels   []string
		hitl     bool
		afk      bool
		assignee string
		jsonOut  bool
	)
	c := &cobra.Command{
		Use:   "create",
		Short: "Create a sub-issue under a PRD.",
		Long: "Create a sub-issue under a PRD. Use --afk for slices an agent can pick up\n" +
			"away-from-keyboard (auto-adds the ready-for-agent label). Use --hitl for\n" +
			"slices that need a human first (no ready-for-agent — the slice is blocked\n" +
			"on a decision or review).",
		Example: "  cat <<'EOF' | thomctl issue create --parent OPE-123 --title \"Wire schema\" -f - --afk\n" +
			"  ## Slice\n" +
			"  ...\n" +
			"  EOF\n" +
			"  thomctl issue create --parent OPE-123 --title \"Pick storage\" -f slice.md --hitl",
		RunE: func(cmd *cobra.Command, args []string) error {
			if parent == "" {
				return errors.New("--parent is required")
			}
			if title == "" {
				return errors.New("--title is required")
			}
			if hitl && afk {
				return errors.New("--hitl and --afk are mutually exclusive")
			}
			b, err := readBody(body, bodyFile)
			if err != nil {
				return err
			}
			if b == "" {
				return errMissingBody
			}
			be, cfg, err := resolveBackend()
			if err != nil {
				return err
			}
			if !cmd.Flags().Changed("assignee") {
				assignee = cfg.DefaultAssignee
			}
			// HITL slices are explicitly NOT ready-for-agent: they need a human
			// to unblock. AFK slices are. Plain --label flags are appended verbatim.
			out := append([]string{}, labels...)
			switch {
			case hitl:
				out = append(out, "hitl")
			case afk:
				out = append(out, "afk", cfg.ReadyLabel)
			}
			opts := backend.CreateOpts{
				Summary:   title,
				BodyMD:    b,
				Labels:    out,
				ParentKey: parent,
				Assignee:  assignee,
			}
			key, err := be.CreateSubissue(opts)
			if err != nil {
				return err
			}
			if jsonOut {
				return emitJSON(map[string]string{"key": key, "parent": parent})
			}
			fmt.Println(key)
			return nil
		},
	}
	c.Flags().StringVar(&parent, "parent", "", "Parent PRD key (required).")
	c.Flags().StringVar(&title, "title", "", "Sub-issue title (required).")
	c.Flags().StringVar(&body, "body", "", "Body markdown. Mutually exclusive with --body-file.")
	c.Flags().StringVarP(&bodyFile, "body-file", "f", "", "Path to markdown file ('-' for stdin).")
	c.Flags().StringSliceVar(&labels, "label", nil, "Extra labels (repeatable).")
	c.Flags().BoolVar(&hitl, "hitl", false, "Mark as needing human interaction (adds 'hitl' label, no ready-for-agent).")
	c.Flags().BoolVar(&afk, "afk", false, "Mark as agent-runnable (adds 'afk' + ready-for-agent labels).")
	c.Flags().StringVar(&assignee, "assignee", "", "Assignee email or '@me'. Defaults to default_assignee from config. Pass '' explicitly to leave unassigned.")
	c.Flags().BoolVar(&jsonOut, "json", false, "Print JSON to stdout.")
	return c
}

func issueEditCmd() *cobra.Command {
	var (
		title    string
		body     string
		bodyFile string
		jsonOut  bool
	)
	c := &cobra.Command{
		Use:   "edit <KEY>",
		Short: "Update an issue's title and/or description.",
		Long: "Update an existing issue's title (--title) and/or description (--body / -f).\n" +
			"At least one must be given. The description replaces the existing body in full.",
		Example: "  cat <<'EOF' | thomctl issue edit OPE-123 -f -\n" +
			"  ## Updated slice\n" +
			"  ...\n" +
			"  EOF\n" +
			"  thomctl issue edit OPE-123 --title \"Wire schema (revised)\"",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var opts backend.EditOpts
			if cmd.Flags().Changed("title") {
				opts.Summary = &title
			}
			if cmd.Flags().Changed("body") || cmd.Flags().Changed("body-file") {
				b, err := readBody(body, bodyFile)
				if err != nil {
					return err
				}
				if b == "" {
					return errMissingBody
				}
				opts.BodyMD = &b
			}
			if opts.Summary == nil && opts.BodyMD == nil {
				return errors.New("nothing to edit: pass --title and/or --body/--body-file")
			}
			be, _, err := resolveBackend()
			if err != nil {
				return err
			}
			if err := be.Edit(args[0], opts); err != nil {
				return err
			}
			if jsonOut {
				return emitJSON(map[string]string{"key": args[0]})
			}
			fmt.Println(args[0])
			return nil
		},
	}
	c.Flags().StringVar(&title, "title", "", "New title/summary.")
	c.Flags().StringVar(&body, "body", "", "New description markdown. Mutually exclusive with --body-file.")
	c.Flags().StringVarP(&bodyFile, "body-file", "f", "", "Path to markdown file ('-' for stdin).")
	c.Flags().BoolVar(&jsonOut, "json", false, "Print JSON to stdout.")
	return c
}

func issueListCmd() *cobra.Command {
	var (
		parent  string
		labels  []string
		status  string
		limit   int
		hitl    bool
		afk     bool
		all     bool
		jsonOut bool
	)
	c := &cobra.Command{
		Use:   "list",
		Short: "List sub-issues. Pass --parent KEY to scope to one PRD, omit for project-wide (used by triage). Completed issues hidden by default.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if hitl && afk {
				return errors.New("--hitl and --afk are mutually exclusive")
			}
			if hitl {
				labels = append(labels, "hitl")
			}
			if afk {
				labels = append(labels, "afk")
			}
			be, _, err := resolveBackend()
			if err != nil {
				return err
			}
			issues, err := be.ListSubissues(parent, backend.ListOpts{
				Labels:        labels,
				Status:        status,
				Limit:         limit,
				IncludeClosed: all,
			})
			if err != nil {
				return err
			}
			if jsonOut {
				return emitJSON(issues)
			}
			emitIssueTable(issues)
			return nil
		},
	}
	c.Flags().StringVar(&parent, "parent", "", "Parent PRD key. Omit for project-wide listing.")
	c.Flags().StringSliceVar(&labels, "label", nil, "Filter by label (repeatable).")
	c.Flags().StringVar(&status, "status", "", "Filter by status. Overrides --all.")
	c.Flags().IntVar(&limit, "limit", 0, "Max results. 0 = paginate all.")
	c.Flags().BoolVar(&hitl, "hitl", false, "Shortcut for --label hitl.")
	c.Flags().BoolVar(&afk, "afk", false, "Shortcut for --label afk.")
	c.Flags().BoolVar(&all, "all", false, "Include completed issues. Off by default.")
	c.Flags().BoolVar(&jsonOut, "json", false, "Print JSON to stdout.")
	return c
}

func issueViewCmd() *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "view <KEY>",
		Short: "Show an issue's detail.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			be, _, err := resolveBackend()
			if err != nil {
				return err
			}
			issue, err := be.View(args[0])
			if err != nil {
				return err
			}
			if jsonOut {
				return emitJSON(issue)
			}
			emitIssueDetail(issue, be)
			return nil
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "Print JSON to stdout.")
	return c
}

func issueCommentCmd() *cobra.Command {
	var (
		body     string
		bodyFile string
	)
	c := &cobra.Command{
		Use:   "comment <KEY>",
		Short: "Post a markdown comment on an issue.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := readBody(body, bodyFile)
			if err != nil {
				return err
			}
			if b == "" {
				return errMissingBody
			}
			be, _, err := resolveBackend()
			if err != nil {
				return err
			}
			return be.Comment(args[0], b)
		},
	}
	c.Flags().StringVar(&body, "body", "", "Comment markdown.")
	c.Flags().StringVarP(&bodyFile, "body-file", "f", "", "Path to markdown file ('-' for stdin).")
	return c
}

func issueLabelCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "label",
		Short: "Add or remove a label on an issue.",
	}
	add := &cobra.Command{
		Use:   "add <KEY> <LABEL>",
		Short: "Add a label.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			be, _, err := resolveBackend()
			if err != nil {
				return err
			}
			return be.AddLabel(args[0], args[1])
		},
	}
	remove := &cobra.Command{
		Use:   "remove <KEY> <LABEL>",
		Short: "Remove a label.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			be, _, err := resolveBackend()
			if err != nil {
				return err
			}
			return be.RemoveLabel(args[0], args[1])
		},
	}
	c.AddCommand(add, remove)
	return c
}

func issueBacklogCmd() *cobra.Command {
	return statusTransitionCmd(
		"backlog <KEY>",
		"Transition an issue to the backlog status (jira.backlog_status, default \"Backlog\"). Optionally post a comment.",
		"backlog_status",
		func(c config.Config) string { return c.Jira.BacklogStatus },
	)
}

func issueTodoCmd() *cobra.Command {
	return statusTransitionCmd(
		"todo <KEY>",
		"Transition an issue to the to-do status (jira.todo_status, default \"To Do\"). Optionally post a comment.",
		"todo_status",
		func(c config.Config) string { return c.Jira.TodoStatus },
	)
}

func issueStartCmd() *cobra.Command {
	return statusTransitionCmd(
		"start <KEY>",
		"Transition an issue to the in-progress status (jira.start_status, default \"In Progress\"). Optionally post a comment.",
		"start_status",
		func(c config.Config) string { return c.Jira.StartStatus },
	)
}

func issueReviewCmd() *cobra.Command {
	return statusTransitionCmd(
		"review <KEY>",
		"Transition an issue to the review status (jira.review_status, default \"To Review\"). Optionally post a comment.",
		"review_status",
		func(c config.Config) string { return c.Jira.ReviewStatus },
	)
}

// statusTransitionCmd builds a semantic transition subcommand (start/review)
// that moves an issue to a status pulled from config via pick. cfgKey names the
// config field, used in the "not configured" error.
func statusTransitionCmd(use, short, cfgKey string, pick func(config.Config) string) *cobra.Command {
	var (
		comment     string
		commentFile string
	)
	c := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := readBody(comment, commentFile)
			if err != nil {
				return err
			}
			be, cfg, err := resolveBackend()
			if err != nil {
				return err
			}
			status := pick(cfg)
			if status == "" {
				return fmt.Errorf("no status configured (set jira.%s in your config)", cfgKey)
			}
			return be.Transition(args[0], status, body)
		},
	}
	c.Flags().StringVar(&comment, "comment", "", "Comment to post (markdown).")
	c.Flags().StringVarP(&commentFile, "comment-file", "f", "", "Path to markdown file ('-' for stdin).")
	return c
}

func issueTransitionCmd() *cobra.Command {
	var (
		status      string
		comment     string
		commentFile string
	)
	c := &cobra.Command{
		Use:   "transition <KEY>",
		Short: "Transition an issue to an arbitrary status (--status). Optionally post a comment.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if status == "" {
				return errors.New("--status is required")
			}
			body, err := readBody(comment, commentFile)
			if err != nil {
				return err
			}
			be, _, err := resolveBackend()
			if err != nil {
				return err
			}
			return be.Transition(args[0], status, body)
		},
	}
	c.Flags().StringVar(&status, "status", "", "Target status (tracker-native, required).")
	c.Flags().StringVar(&comment, "comment", "", "Comment to post (markdown).")
	c.Flags().StringVarP(&commentFile, "comment-file", "f", "", "Path to markdown file ('-' for stdin).")
	return c
}

func issueCloseCmd() *cobra.Command {
	var (
		status      string
		comment     string
		commentFile string
	)
	c := &cobra.Command{
		Use:   "close <KEY>",
		Short: "Transition an issue to the configured terminal status (e.g. Done / Completato). Optionally post a closing comment.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := readBody(comment, commentFile)
			if err != nil {
				return err
			}
			be, _, err := resolveBackend()
			if err != nil {
				return err
			}
			return be.Close(args[0], status, c)
		},
	}
	c.Flags().StringVar(&status, "status", "", "Target status (defaults to jira.close_status from config).")
	c.Flags().StringVar(&comment, "comment", "", "Closing comment (markdown).")
	c.Flags().StringVarP(&commentFile, "comment-file", "f", "", "Path to markdown file ('-' for stdin).")
	return c
}
