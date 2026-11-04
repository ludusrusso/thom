package jira

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Jira translates the names of its built-in issue types and statuses per
// user profile language, and acli only accepts the name as the calling user
// sees it. IDs are language-independent, so when the config carries an ID we
// resolve it to the caller's own name here, right before calling acli.

type issueType struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Subtask        bool   `json:"subtask"`
	HierarchyLevel int    `json:"hierarchyLevel"`
}

// issueTypeName returns the issue-type name to send to acli. With no id it is
// the configured name, unchanged. With an id it is the name of that type in
// the caller's language, read from the project's issue types.
func (b *Backend) issueTypeName(name, id, key string) (string, error) {
	if id == "" {
		return name, nil
	}
	if err := validateID(id, key); err != nil {
		return "", err
	}
	types, err := b.projectIssueTypes()
	if err != nil {
		return "", err
	}
	for _, t := range types {
		if t.ID == id {
			return t.Name, nil
		}
	}
	return "", fmt.Errorf("jira.%s=%s is not an issue type of project %s; available: %s",
		key, id, b.cfg.Jira.ProjectKey, describeIssueTypes(types))
}

// issueTypeClause is the JQL filter for an issue type. JQL accepts the ID
// directly, so no lookup is needed when one is configured.
func issueTypeClause(name, id, key string) (string, error) {
	if id == "" {
		return fmt.Sprintf("issuetype = %s", quote(name)), nil
	}
	if err := validateID(id, key); err != nil {
		return "", err
	}
	return "issuetype = " + id, nil
}

// projectIssueTypes loads (once) the project's issue types as the caller
// sees them: `acli jira project view --json` carries id, localised name,
// subtask flag and hierarchy level for each.
func (b *Backend) projectIssueTypes() ([]issueType, error) {
	if b.issueTypes != nil {
		return b.issueTypes, nil
	}
	out, err := b.run("project", "view", "--key", b.cfg.Jira.ProjectKey, "--json")
	if err != nil {
		return nil, err
	}
	var raw struct {
		IssueTypes []issueType `json:"issueTypes"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("decode project view: %w", err)
	}
	if len(raw.IssueTypes) == 0 {
		return nil, fmt.Errorf("project %s returned no issue types", b.cfg.Jira.ProjectKey)
	}
	b.issueTypes = raw.IssueTypes
	return b.issueTypes, nil
}

// statusName returns the caller's localised name for a status ID. acli has
// no status listing, but JQL accepts `status = <id>`, and any issue in that
// status reports its name in the caller's language. Status IDs are global,
// so the search is not restricted to the configured project: a project
// whose first issue has not been closed yet still resolves.
func (b *Backend) statusName(id, key string) (string, error) {
	if err := validateID(id, key); err != nil {
		return "", err
	}
	issues, err := b.search("status = "+id, 1)
	if err != nil {
		return "", err
	}
	if len(issues) == 0 || issues[0].Status == "" {
		return "", fmt.Errorf("jira.%s=%s: no visible issue has this status, so its name cannot be resolved; pass --status <name> or set jira.close_status instead", key, id)
	}
	return issues[0].Status, nil
}

// validateID rejects anything but a numeric Jira ID. IDs are interpolated
// into JQL unquoted, so this also keeps the query well-formed.
func validateID(id, key string) error {
	if id == "" {
		return fmt.Errorf("jira.%s is empty", key)
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return fmt.Errorf("jira.%s=%q must be a numeric Jira ID", key, id)
		}
	}
	return nil
}

func describeIssueTypes(types []issueType) string {
	parts := make([]string, 0, len(types))
	for _, t := range types {
		kind := ""
		switch {
		case t.Subtask:
			kind = ", subtask"
		case t.HierarchyLevel > 0:
			kind = ", epic-level"
		}
		parts = append(parts, fmt.Sprintf("%s=%q%s", t.ID, t.Name, kind))
	}
	return strings.Join(parts, "; ")
}
