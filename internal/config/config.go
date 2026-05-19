// Package config loads thomctl configuration from .thomctl.yaml.
//
// Search order:
//  1. $THOMCTL_CONFIG if set (file path)
//  2. .thomctl.yaml walking up from the current working directory to the
//     user's home directory (inclusive)
//  3. $XDG_CONFIG_HOME/thomctl/config.yaml (or ~/.config/thomctl/config.yaml)
//
// All configuration lives in yaml; there are no value-override env vars.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Backend string `yaml:"backend"`

	// Skill-convention knobs, shared across all backends.
	PRDLabel        string   `yaml:"prd_label"`
	ReadyLabel      string   `yaml:"ready_label"`
	DefaultLabels   []string `yaml:"default_labels"`
	DefaultAssignee string   `yaml:"default_assignee"`

	Jira   JiraConfig   `yaml:"jira"`
	Github GithubConfig `yaml:"github"`

	// Source is the absolute path of the file that was loaded, or "" when
	// defaults produced the config without a file.
	Source string `yaml:"-"`
}

type JiraConfig struct {
	ProjectKey        string `yaml:"project_key"`
	PRDIssueType      string `yaml:"prd_issue_type"`
	SubissueIssueType string `yaml:"subissue_issue_type"`
	CloseStatus       string `yaml:"close_status"`
}

// GithubConfig has no fields today: the repo is resolved by `gh` from the
// current directory, close maps to `--reason completed|"not planned"`, and
// issue type is derived from labels. Kept as a named struct so future
// GitHub-specific knobs have a home without changing the yaml shape.
type GithubConfig struct{}

func defaults() Config {
	return Config{
		Backend:         "jira",
		PRDLabel:        "PRD",
		ReadyLabel:      "ready-for-agent",
		DefaultAssignee: "@me",
		Jira: JiraConfig{
			PRDIssueType:      "Epic",
			SubissueIssueType: "Task",
			CloseStatus:       "Done",
		},
	}
}

// Load resolves the config from file. A missing file is not an error
// (defaults are used), but a malformed file is.
func Load() (Config, error) {
	cfg := defaults()

	path, err := findConfigFile()
	if err != nil {
		return cfg, err
	}
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("read %s: %w", path, err)
		}
		if err := yaml.Unmarshal(b, &cfg); err != nil {
			return cfg, fmt.Errorf("parse %s: %w", path, err)
		}
		cfg.Source = path
	}

	if cfg.Backend == "" {
		cfg.Backend = "jira"
	}
	if cfg.Backend == "jira" && cfg.Jira.ProjectKey == "" {
		return cfg, errors.New("jira.project_key not set in .thomctl.yaml")
	}
	return cfg, nil
}

func findConfigFile() (string, error) {
	if v := os.Getenv("THOMCTL_CONFIG"); v != "" {
		if _, err := os.Stat(v); err != nil {
			return "", fmt.Errorf("THOMCTL_CONFIG=%s: %w", v, err)
		}
		return v, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	home, _ := os.UserHomeDir()

	dir := cwd
	for {
		candidate := filepath.Join(dir, ".thomctl.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		// Stop one level above $HOME so we don't pick up unrelated files.
		if home != "" && dir == home {
			break
		}
		dir = parent
	}

	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" && home != "" {
		xdg = filepath.Join(home, ".config")
	}
	if xdg != "" {
		candidate := filepath.Join(xdg, "thomctl", "config.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", nil
}
