// Package cli wires the cobra command tree for thomctl.
//
// The surface is intentionally narrow:
//
//	thomctl prd create   --title ... --body-file ...
//	thomctl prd list
//	thomctl prd view     KEY
//	thomctl issue create --parent KEY --title ... --body-file ...
//	thomctl issue list   --parent KEY
//	thomctl issue view   KEY
//	thomctl issue comment KEY --body-file ...
//	thomctl issue label add|remove KEY LABEL
//	thomctl issue close  KEY [--comment ...]
//
// Most commands accept `--json` to produce machine-readable output for agents.
package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ludusrusso/thom/internal/backend"
	"github.com/ludusrusso/thom/internal/backend/github"
	"github.com/ludusrusso/thom/internal/backend/jira"
	"github.com/ludusrusso/thom/internal/config"
)

func Root() *cobra.Command {
	root := &cobra.Command{
		Use:           "thomctl",
		Short:         "Tracker-agnostic CLI for PRDs and sub-issues.",
		Long:          "thomctl wraps the configured issue tracker (today: Jira via acli) behind a small, stable command surface designed for agent skills.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(prdCmd(), issueCmd(), configCmd(), llmCmd(), initCmd(), ralphCmd())
	return root
}

// resolveBackend loads the config and instantiates the matching backend.
// Callers should propagate any error directly to the user.
func resolveBackend() (backend.Backend, config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, cfg, err
	}
	switch cfg.Backend {
	case "jira":
		return jira.New(cfg), cfg, nil
	case "github":
		return github.New(cfg), cfg, nil
	default:
		return nil, cfg, fmt.Errorf("unknown backend %q (supported: \"jira\", \"github\")", cfg.Backend)
	}
}

func configCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "config",
		Short: "Inspect the resolved configuration.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			fmt.Printf("backend          = %s\n", cfg.Backend)
			if cfg.Source != "" {
				fmt.Printf("source           = %s\n", cfg.Source)
			} else {
				fmt.Printf("source           = (defaults)\n")
			}
			fmt.Printf("prd_label        = %s\n", cfg.PRDLabel)
			fmt.Printf("ready_label      = %s\n", cfg.ReadyLabel)
			fmt.Printf("default_labels   = %v\n", cfg.DefaultLabels)
			fmt.Printf("default_assignee = %s\n", cfg.DefaultAssignee)
			switch cfg.Backend {
			case "jira":
				fmt.Printf("jira.project_key         = %s\n", cfg.Jira.ProjectKey)
				fmt.Printf("jira.prd_issue_type      = %s\n", withID(cfg.Jira.PRDIssueType, cfg.Jira.PRDIssueTypeID))
				fmt.Printf("jira.subissue_issue_type = %s\n", withID(cfg.Jira.SubissueIssueType, cfg.Jira.SubissueIssueTypeID))
				fmt.Printf("jira.close_status        = %s\n", withID(cfg.Jira.CloseStatus, cfg.Jira.CloseStatusID))
				fmt.Printf("jira.site                = %s\n", cfg.Jira.Site)
			case "github":
				fmt.Printf("github                   = (repo resolved from cwd by gh)\n")
			}
			return nil
		},
	}
	return c
}

// withID renders a name-or-ID config pair. When an ID is set it is what
// thomctl uses (resolved to the caller's localised name at runtime), so it
// is shown first and the name is marked as unused.
func withID(name, id string) string {
	if id == "" {
		return name
	}
	return fmt.Sprintf("id %s (name %q ignored; resolved per user)", id, name)
}

// mustOneOf returns the first non-empty value, or an error using name.
func mustOneOf(name string, vs ...string) (string, error) {
	for _, v := range vs {
		if v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("--%s is required", name)
}

var errMissingBody = errors.New("either --body or --body-file is required")
