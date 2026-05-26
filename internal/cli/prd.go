package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ludusrusso/thom/internal/backend"
)

func prdCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "prd",
		Short: "Create, list, and read PRDs.",
	}
	c.AddCommand(prdCreateCmd(), prdListCmd(), prdViewCmd(), prdIssuesCmd())
	return c
}

func prdCreateCmd() *cobra.Command {
	var (
		title    string
		body     string
		bodyFile string
		labels   []string
		assignee string
		jsonOut  bool
	)
	c := &cobra.Command{
		Use:   "create",
		Short: "Create a new PRD. Body is markdown; it is translated to the tracker's native format.",
		Example: "  cat <<'EOF' | thomctl prd create --title \"Improve onboarding\" -f -\n" +
			"  ## Goal\n" +
			"  ...\n" +
			"  EOF\n" +
			"  thomctl prd create --title \"Improve onboarding\" -f prd.md --label engine",
		RunE: func(cmd *cobra.Command, args []string) error {
			if title == "" {
				return errors.New("--title is required")
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
			opts := backend.CreateOpts{
				Summary:  title,
				BodyMD:   b,
				Labels:   append([]string{cfg.ReadyLabel}, labels...),
				Assignee: assignee,
			}
			key, err := be.CreatePRD(opts)
			if err != nil {
				return err
			}
			if jsonOut {
				return emitJSON(map[string]string{"key": key})
			}
			fmt.Println(key)
			return nil
		},
	}
	c.Flags().StringVar(&title, "title", "", "PRD title (required)")
	c.Flags().StringVar(&body, "body", "", "PRD body (markdown). Mutually exclusive with --body-file.")
	c.Flags().StringVarP(&bodyFile, "body-file", "f", "", "Path to markdown file ('-' for stdin). Mutually exclusive with --body.")
	c.Flags().StringSliceVar(&labels, "label", nil, "Extra labels to apply (repeatable).")
	c.Flags().StringVar(&assignee, "assignee", "", "Assignee email or '@me'. Defaults to default_assignee from config. Pass '' explicitly to leave unassigned.")
	c.Flags().BoolVar(&jsonOut, "json", false, "Print JSON to stdout.")
	return c
}

func prdListCmd() *cobra.Command {
	var (
		labels  []string
		status  string
		limit   int
		all     bool
		jsonOut bool
	)
	c := &cobra.Command{
		Use:   "list",
		Short: "List PRDs. Completed PRDs are hidden by default; pass --all to include them.",
		RunE: func(cmd *cobra.Command, args []string) error {
			be, _, err := resolveBackend()
			if err != nil {
				return err
			}
			issues, err := be.ListPRDs(backend.ListOpts{
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
	c.Flags().StringSliceVar(&labels, "label", nil, "Filter by label (repeatable, AND-matched).")
	c.Flags().StringVar(&status, "status", "", "Filter by status (e.g. 'To Do', 'In Progress'). Overrides --all.")
	c.Flags().IntVar(&limit, "limit", 0, "Max results. 0 = paginate all.")
	c.Flags().BoolVar(&all, "all", false, "Include completed PRDs (Done / closed). Off by default.")
	c.Flags().BoolVar(&jsonOut, "json", false, "Print JSON to stdout.")
	return c
}

func prdIssuesCmd() *cobra.Command {
	var (
		labels  []string
		status  string
		limit   int
		hitl    bool
		afk     bool
		all     bool
		jsonOut bool
	)
	c := &cobra.Command{
		Use:   "issues <KEY>",
		Short: "List sub-issues of a PRD. Shortcut for `issue list --parent <KEY>` with completed hidden by default.",
		Args:  cobra.ExactArgs(1),
		Example: "  thomctl prd issues OPE-742\n" +
			"  thomctl prd issues OPE-742 --afk\n" +
			"  thomctl prd issues OPE-742 --all --json",
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
			issues, err := be.ListSubissues(args[0], backend.ListOpts{
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
	c.Flags().StringSliceVar(&labels, "label", nil, "Filter by label (repeatable).")
	c.Flags().StringVar(&status, "status", "", "Filter by status. Overrides --all.")
	c.Flags().IntVar(&limit, "limit", 0, "Max results. 0 = paginate all.")
	c.Flags().BoolVar(&hitl, "hitl", false, "Shortcut for --label hitl.")
	c.Flags().BoolVar(&afk, "afk", false, "Shortcut for --label afk.")
	c.Flags().BoolVar(&all, "all", false, "Include completed issues. Off by default.")
	c.Flags().BoolVar(&jsonOut, "json", false, "Print JSON to stdout.")
	return c
}

func prdViewCmd() *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "view <KEY>",
		Short: "Show a PRD's detail (summary, status, labels, description).",
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
