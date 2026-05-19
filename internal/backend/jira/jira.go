// Package jira implements backend.Backend by shelling out to the Atlassian
// `acli` CLI. We pass payloads as temp JSON files (acli --from-json) so we
// don't have to escape ADF on the command line.
package jira

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/ludusrusso/thom/internal/adf"
	"github.com/ludusrusso/thom/internal/backend"
	"github.com/ludusrusso/thom/internal/config"
)

type Backend struct {
	cfg config.JiraConfig
}

func New(cfg config.JiraConfig) *Backend {
	return &Backend{cfg: cfg}
}

// --- public API ---

func (b *Backend) CreatePRD(opts backend.CreateOpts) (string, error) {
	labels := mergeLabels(b.cfg.DefaultLabels, opts.Labels, b.cfg.PRDLabel)
	return b.create(opts, b.cfg.PRDIssueType, labels, "")
}

func (b *Backend) CreateSubissue(opts backend.CreateOpts) (string, error) {
	if opts.ParentKey == "" {
		return "", errors.New("parent key is required for sub-issues")
	}
	labels := mergeLabels(b.cfg.DefaultLabels, opts.Labels)
	return b.create(opts, b.cfg.SubissueIssueType, labels, opts.ParentKey)
}

func (b *Backend) ListPRDs(opts backend.ListOpts) ([]backend.Issue, error) {
	clauses := []string{
		fmt.Sprintf("project = %s", quote(b.cfg.ProjectKey)),
		fmt.Sprintf("issuetype = %s", quote(b.cfg.PRDIssueType)),
	}
	if b.cfg.PRDLabel != "" {
		clauses = append(clauses, fmt.Sprintf("labels = %s", quote(b.cfg.PRDLabel)))
	}
	for _, l := range opts.Labels {
		clauses = append(clauses, fmt.Sprintf("labels = %s", quote(l)))
	}
	if opts.Status != "" {
		clauses = append(clauses, fmt.Sprintf("status = %s", quote(opts.Status)))
	}
	return b.search(strings.Join(clauses, " AND ")+" ORDER BY updated DESC", opts.Limit)
}

func (b *Backend) ListSubissues(parentKey string, opts backend.ListOpts) ([]backend.Issue, error) {
	var clauses []string
	if parentKey != "" {
		clauses = append(clauses, fmt.Sprintf("parent = %s", parentKey))
	} else {
		clauses = append(clauses,
			fmt.Sprintf("project = %s", quote(b.cfg.ProjectKey)),
			fmt.Sprintf("issuetype = %s", quote(b.cfg.SubissueIssueType)),
		)
	}
	for _, l := range opts.Labels {
		clauses = append(clauses, fmt.Sprintf("labels = %s", quote(l)))
	}
	if opts.Status != "" {
		clauses = append(clauses, fmt.Sprintf("status = %s", quote(opts.Status)))
	}
	return b.search(strings.Join(clauses, " AND ")+" ORDER BY key ASC", opts.Limit)
}

func (b *Backend) View(key string) (backend.Issue, error) {
	out, err := b.run("workitem", "view", key,
		"--fields", "key,issuetype,summary,status,assignee,labels,parent,updated,description",
		"--json")
	if err != nil {
		return backend.Issue{}, err
	}
	var raw map[string]any
	if err := json.Unmarshal(out, &raw); err != nil {
		return backend.Issue{}, fmt.Errorf("decode view: %w", err)
	}
	return decodeIssue(raw), nil
}

func (b *Backend) Comment(key, bodyMD string) error {
	doc, err := adf.FromMarkdown(bodyMD)
	if err != nil {
		return err
	}
	path, cleanup, err := writeTempJSON(doc)
	if err != nil {
		return err
	}
	defer cleanup()
	_, err = b.run("workitem", "comment", "create", "--key", key, "--body-file", path)
	return err
}

func (b *Backend) AddLabel(key, label string) error {
	// acli edit --labels REPLACES labels. Fetch current set first.
	issue, err := b.View(key)
	if err != nil {
		return err
	}
	for _, l := range issue.Labels {
		if l == label {
			return nil
		}
	}
	all := append([]string{}, issue.Labels...)
	all = append(all, label)
	_, err = b.run("workitem", "edit", "--key", key, "--labels", strings.Join(all, ","), "--yes")
	return err
}

func (b *Backend) RemoveLabel(key, label string) error {
	_, err := b.run("workitem", "edit", "--key", key, "--remove-labels", label, "--yes")
	return err
}

// ListLinks reads issuelinks from `workitem view` (the dedicated `link list`
// command of acli drops the direction information for links where the
// queried key is the outward side — view --fields issuelinks is complete).
func (b *Backend) ListLinks(key string) ([]backend.Link, error) {
	out, err := b.run("workitem", "view", key, "--fields", "issuelinks", "--json")
	if err != nil {
		return nil, err
	}
	var raw struct {
		Fields struct {
			IssueLinks []struct {
				ID   string `json:"id"`
				Type struct {
					Name    string `json:"name"`
					Inward  string `json:"inward"`
					Outward string `json:"outward"`
				} `json:"type"`
				InwardIssue  *linkedIssue `json:"inwardIssue,omitempty"`
				OutwardIssue *linkedIssue `json:"outwardIssue,omitempty"`
			} `json:"issuelinks"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("decode issuelinks: %w", err)
	}
	links := make([]backend.Link, 0, len(raw.Fields.IssueLinks))
	for _, l := range raw.Fields.IssueLinks {
		// When the queried key is the OUTWARD side of the link, acli returns
		// inwardIssue (the other end). When it's the inward side, it returns
		// outwardIssue. Direction is from the queried key's perspective.
		switch {
		case l.InwardIssue != nil:
			links = append(links, backend.Link{
				ID:        l.ID,
				Type:      l.Type.Name,
				Direction: "out",
				Phrase:    l.Type.Outward,
				OtherKey:  l.InwardIssue.Key,
				Summary:   l.InwardIssue.Fields.Summary,
				Status:    l.InwardIssue.Fields.Status.Name,
			})
		case l.OutwardIssue != nil:
			links = append(links, backend.Link{
				ID:        l.ID,
				Type:      l.Type.Name,
				Direction: "in",
				Phrase:    l.Type.Inward,
				OtherKey:  l.OutwardIssue.Key,
				Summary:   l.OutwardIssue.Fields.Summary,
				Status:    l.OutwardIssue.Fields.Status.Name,
			})
		}
	}
	return links, nil
}

type linkedIssue struct {
	Key    string `json:"key"`
	Fields struct {
		Summary string `json:"summary"`
		Status  struct {
			Name string `json:"name"`
		} `json:"status"`
	} `json:"fields"`
}

func (b *Backend) AddLink(outKey, inKey, linkType string) error {
	_, err := b.run("workitem", "link", "create",
		"--out", outKey, "--in", inKey, "--type", linkType)
	return err
}

func (b *Backend) RemoveLink(linkID string) error {
	_, err := b.run("workitem", "link", "delete", "--id", linkID, "--yes")
	return err
}

func (b *Backend) Close(key, status, comment string) error {
	if status == "" {
		status = b.cfg.CloseStatus
	}
	if status == "" {
		return errors.New("no close status configured (set jira.close_status in .thomctl.yaml or pass --status)")
	}
	if comment != "" {
		if err := b.Comment(key, comment); err != nil {
			return err
		}
	}
	// acli's transition silently succeeds even when the target status is not a
	// valid transition from the current one — so we verify the status changed.
	before, err := b.View(key)
	if err != nil {
		return err
	}
	if statusEq(before.Status, status) {
		return nil // already there
	}
	if _, err := b.run("workitem", "transition", "--key", key, "--status", status, "--yes"); err != nil {
		return err
	}
	after, err := b.View(key)
	if err != nil {
		return err
	}
	if !statusEq(after.Status, status) {
		return fmt.Errorf("transition no-op: status still %q after asking for %q (workflow may not allow this transition from %q)", after.Status, status, before.Status)
	}
	return nil
}

func statusEq(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// --- internals ---

func (b *Backend) create(opts backend.CreateOpts, issueType string, labels []string, parentKey string) (string, error) {
	doc, err := adf.FromMarkdown(opts.BodyMD)
	if err != nil {
		return "", err
	}
	payload := map[string]any{
		"projectKey":  b.cfg.ProjectKey,
		"type":        issueType,
		"summary":     opts.Summary,
		"labels":      labels,
		"description": doc,
	}
	if parentKey != "" {
		payload["parentIssueId"] = parentKey
	}
	path, cleanup, err := writeTempJSON(payload)
	if err != nil {
		return "", err
	}
	defer cleanup()
	// acli's create silently ignores --assignee when --from-json is given, and
	// putting "@me" inside the JSON payload errors with "User not found".
	// So we create without assignee, then issue a separate `assign` call which
	// does understand "@me".
	out, err := b.run("workitem", "create", "--from-json", path, "--json")
	if err != nil {
		return "", err
	}
	var resp struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("decode create response: %w (body: %s)", err, string(out))
	}
	if resp.Key == "" {
		return "", fmt.Errorf("create returned no key (body: %s)", string(out))
	}
	if opts.Assignee != "" {
		if _, err := b.run("workitem", "assign", "--key", resp.Key, "--assignee", opts.Assignee); err != nil {
			return resp.Key, fmt.Errorf("created %s but failed to assign: %w", resp.Key, err)
		}
	}
	return resp.Key, nil
}

func (b *Backend) search(jql string, limit int) ([]backend.Issue, error) {
	args := []string{"workitem", "search",
		"--jql", jql,
		"--fields", "key,issuetype,summary,status,assignee,labels",
		"--json"}
	if limit > 0 {
		args = append(args, "--limit", fmt.Sprintf("%d", limit))
	} else {
		args = append(args, "--paginate")
	}
	out, err := b.run(args...)
	if err != nil {
		return nil, err
	}
	var raw []map[string]any
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("decode search: %w (body: %.200s)", err, string(out))
	}
	issues := make([]backend.Issue, 0, len(raw))
	for _, r := range raw {
		issues = append(issues, decodeIssue(r))
	}
	return issues, nil
}

func (b *Backend) run(args ...string) ([]byte, error) {
	full := append([]string{"jira"}, args...)
	cmd := exec.Command("acli", full...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("acli %s: %s", strings.Join(args, " "), msg)
	}
	return out, nil
}

func writeTempJSON(v any) (string, func(), error) {
	f, err := os.CreateTemp("", "thomctl-*.json")
	if err != nil {
		return "", nil, err
	}
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", nil, err
	}
	return f.Name(), func() { os.Remove(f.Name()) }, nil
}

func mergeLabels(parts ...any) []string {
	seen := map[string]bool{}
	var out []string
	add := func(l string) {
		l = strings.TrimSpace(l)
		if l == "" || seen[l] {
			return
		}
		seen[l] = true
		out = append(out, l)
	}
	for _, p := range parts {
		switch v := p.(type) {
		case string:
			add(v)
		case []string:
			for _, s := range v {
				add(s)
			}
		}
	}
	return out
}

func quote(s string) string {
	// Quote JQL string literals — Jira accepts both single and double quotes
	// but doesn't support escaping inside identifiers, so we go with double
	// quotes and reject inputs containing them.
	if strings.ContainsAny(s, `"`) {
		return s
	}
	return `"` + s + `"`
}

// decodeIssue accepts either a flat issue object (as returned by `acli
// workitem view`) or the search-shaped object with a "fields" sub-object.
// We merge fields up so the flat-decode path works uniformly.
func decodeIssue(r map[string]any) backend.Issue {
	flat := map[string]any{}
	for k, v := range r {
		flat[k] = v
	}
	if f, ok := r["fields"].(map[string]any); ok {
		for k, v := range f {
			if _, exists := flat[k]; !exists {
				flat[k] = v
			}
		}
	}
	issue := backend.Issue{
		Key:     getStr(flat, "key"),
		Summary: getStr(flat, "summary"),
	}
	if t, ok := flat["issuetype"].(map[string]any); ok {
		issue.Type = getStr(t, "name")
	} else {
		issue.Type = getStr(flat, "issuetype")
	}
	if s, ok := flat["status"].(map[string]any); ok {
		issue.Status = getStr(s, "name")
	} else {
		issue.Status = getStr(flat, "status")
	}
	if a, ok := flat["assignee"].(map[string]any); ok {
		issue.Assignee = firstNonEmpty(getStr(a, "displayName"), getStr(a, "emailAddress"))
	} else {
		issue.Assignee = getStr(flat, "assignee")
	}
	if p, ok := flat["parent"].(map[string]any); ok {
		issue.ParentKey = getStr(p, "key")
	} else {
		issue.ParentKey = getStr(flat, "parent")
	}
	if labels, ok := flat["labels"].([]any); ok {
		for _, l := range labels {
			if s, ok := l.(string); ok {
				issue.Labels = append(issue.Labels, s)
			}
		}
	}
	if desc, ok := flat["description"].(map[string]any); ok {
		issue.Description = adf.ToPlain(desc)
	} else {
		issue.Description = getStr(flat, "description")
	}
	return issue
}

func getStr(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}
