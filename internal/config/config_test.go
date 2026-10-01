package config

import (
	"os"
	"path/filepath"
	"testing"
)

func loadFrom(t *testing.T, yaml string) Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("THOMCTL_CONFIG", path)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestLoadIDKeysQuotedOrBare(t *testing.T) {
	cfg := loadFrom(t, `
backend: jira
jira:
  project_key: OPE
  prd_issue_type_id: "10000"
  subissue_issue_type_id: 10016
  close_status_id: 10015
`)
	j := cfg.Jira
	if j.PRDIssueTypeID != "10000" || j.SubissueIssueTypeID != "10016" || j.CloseStatusID != "10015" {
		t.Fatalf("ids = %q %q %q", j.PRDIssueTypeID, j.SubissueIssueTypeID, j.CloseStatusID)
	}
}

func TestLoadNameOnlyConfigUnchanged(t *testing.T) {
	cfg := loadFrom(t, `
backend: jira
jira:
  project_key: OPE
  subissue_issue_type: Sottotask
  close_status: Completata
`)
	j := cfg.Jira
	if j.SubissueIssueType != "Sottotask" || j.CloseStatus != "Completata" || j.PRDIssueType != "Epic" {
		t.Fatalf("names = %+v", j)
	}
	if j.PRDIssueTypeID != "" || j.SubissueIssueTypeID != "" || j.CloseStatusID != "" {
		t.Fatalf("ids must default to empty, got %+v", j)
	}
}
