package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/ludusrusso/thom/internal/config"
	"github.com/ludusrusso/thom/internal/ralph"
)

// defaultConfigYAML is the starter config.yaml `thomctl init` writes when no
// .thomctl.yaml exists to migrate. It is intentionally backend-less — the
// user must pick one and fill in the project key before commands work.
const defaultConfigYAML = `# thomctl project config. Picked up automatically when thomctl is invoked
# from anywhere under this directory.
backend: github

# GitHub backend has no required fields today — repo is resolved by ` + "`gh`" + `
# from cwd. Uncomment + populate when switching to Jira:
#
# backend: jira
# jira:
#   project_key: OPE
#   prd_issue_type: Epic
#   subissue_issue_type: Task
#   close_status: Done
#   site: https://acme.atlassian.net  # optional; enables clickable KEYs in list/view

prd_label: PRD
ready_label: ready-for-agent
default_assignee: "@me"
`

func initCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "init",
		Short: "Scaffold .thom/ with config + ralph defaults; migrate legacy .thomctl.yaml.",
		Long: "Creates `.thom/config.yaml` (migrating `.thomctl.yaml` if present), `.thom/ralph/prompt.md`, " +
			"and `.thom/ralph/settings.json`. Safe to re-run — existing files are left alone.",
		RunE: runInit,
	}
	return c
}

func runInit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	thomDir := filepath.Join(cwd, config.ConfigDir)
	configPath := filepath.Join(thomDir, config.ConfigName)
	legacyPath := filepath.Join(cwd, config.LegacyConfigName)

	if err := os.MkdirAll(thomDir, 0o755); err != nil {
		return err
	}

	newExists := fileExists(configPath)
	legacyExists := fileExists(legacyPath)
	switch {
	case newExists && legacyExists:
		return fmt.Errorf("both %s and %s exist; merge by hand and remove the legacy file", configPath, legacyPath)
	case legacyExists:
		if err := os.Rename(legacyPath, configPath); err != nil {
			return fmt.Errorf("migrate %s → %s: %w", legacyPath, configPath, err)
		}
		fmt.Printf("migrated %s → %s\n", legacyPath, configPath)
	case !newExists:
		if err := os.WriteFile(configPath, []byte(defaultConfigYAML), 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", configPath)
	default:
		fmt.Printf("%s already exists; leaving it alone\n", configPath)
	}

	ralphDir := filepath.Join(thomDir, "ralph")
	if err := os.MkdirAll(ralphDir, 0o755); err != nil {
		return err
	}
	for _, f := range []struct {
		path    string
		content string
	}{
		{filepath.Join(ralphDir, "prompt.md"), ralph.DefaultPrompt()},
		{filepath.Join(ralphDir, "settings.json"), ralph.DefaultSettings()},
	} {
		if fileExists(f.path) {
			fmt.Printf("%s already exists; leaving it alone\n", f.path)
			continue
		}
		if err := os.WriteFile(f.path, []byte(f.content), 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", f.path)
	}
	return nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return !errors.Is(err, os.ErrNotExist) && err == nil
}
