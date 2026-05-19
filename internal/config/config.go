// Package config loads thomctl configuration from .thomctl.yaml.
//
// Search order:
//  1. $THOMCTL_CONFIG if set
//  2. .thomctl.yaml walking up from the current working directory to the
//     user's home directory (inclusive)
//  3. $XDG_CONFIG_HOME/thomctl/config.yaml (or ~/.config/thomctl/config.yaml)
//
// Env vars override file values. See Load for the full list.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Backend string     `yaml:"backend"`
	Jira    JiraConfig `yaml:"jira"`

	// Source is the absolute path of the file that was loaded, or "" when
	// defaults + env vars produced the config without a file.
	Source string `yaml:"-"`
}

type JiraConfig struct {
	ProjectKey        string   `yaml:"project_key"`
	PRDIssueType      string   `yaml:"prd_issue_type"`
	SubissueIssueType string   `yaml:"subissue_issue_type"`
	PRDLabel          string   `yaml:"prd_label"`
	DefaultLabels     []string `yaml:"default_labels"`
	ReadyLabel        string   `yaml:"ready_label"`
	DefaultAssignee   string   `yaml:"default_assignee"`
	CloseStatus       string   `yaml:"close_status"`
}

func defaults() Config {
	return Config{
		Backend: "jira",
		Jira: JiraConfig{
			PRDIssueType:      "Epic",
			SubissueIssueType: "Task",
			PRDLabel:          "PRD",
			ReadyLabel:        "ready-for-agent",
			DefaultAssignee:   "@me",
			CloseStatus:       "Done",
		},
	}
}

// Load resolves the config from file + env. A missing file is not an error
// (defaults + env are used), but a malformed file is.
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

	applyEnv(&cfg)

	if cfg.Backend == "" {
		cfg.Backend = "jira"
	}
	if cfg.Backend == "jira" && cfg.Jira.ProjectKey == "" {
		return cfg, errors.New("jira.project_key not set (configure .thomctl.yaml or set THOMCTL_JIRA_PROJECT_KEY)")
	}
	return cfg, nil
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("THOMCTL_BACKEND"); v != "" {
		cfg.Backend = v
	}
	if v := os.Getenv("THOMCTL_JIRA_PROJECT_KEY"); v != "" {
		cfg.Jira.ProjectKey = v
	}
	if v := os.Getenv("THOMCTL_JIRA_PRD_TYPE"); v != "" {
		cfg.Jira.PRDIssueType = v
	}
	if v := os.Getenv("THOMCTL_JIRA_SUBISSUE_TYPE"); v != "" {
		cfg.Jira.SubissueIssueType = v
	}
	if v := os.Getenv("THOMCTL_JIRA_PRD_LABEL"); v != "" {
		cfg.Jira.PRDLabel = v
	}
	if v := os.Getenv("THOMCTL_JIRA_READY_LABEL"); v != "" {
		cfg.Jira.ReadyLabel = v
	}
	if v := os.Getenv("THOMCTL_JIRA_DEFAULT_LABELS"); v != "" {
		cfg.Jira.DefaultLabels = splitCSV(v)
	}
	if v := os.Getenv("THOMCTL_JIRA_DEFAULT_ASSIGNEE"); v != "" {
		cfg.Jira.DefaultAssignee = v
	}
	if v := os.Getenv("THOMCTL_JIRA_CLOSE_STATUS"); v != "" {
		cfg.Jira.CloseStatus = v
	}
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
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
