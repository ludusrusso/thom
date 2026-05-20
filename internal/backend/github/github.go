// Package github implements backend.Backend by shelling out to the `gh` CLI.
// Repo is resolved by `gh` from the current working directory (or
// `gh repo set-default`), so thomctl carries no repo config. Parent ↔
// sub-issue uses GitHub's native sub-issues feature via GraphQL (see
// docs/adr/0001-github-parent-linkage-via-native-subissues.md). Issue
// dependencies (blocks / blocked-by) use the REST dependencies endpoints.
// "Relates to" and "Duplicate of" have no native analog and are rejected.
package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ludusrusso/thom/internal/backend"
	"github.com/ludusrusso/thom/internal/config"
)

type Backend struct {
	cfg config.Config

	// repoSlug is "owner/name", populated on first use of the REST endpoints
	// that need it explicitly (the `gh` subcommands resolve repo themselves).
	repoSlug string

	// labelsCache holds the names of labels known to exist in the repo, used
	// to avoid asking gh to add a label that doesn't exist yet (which errors).
	// Missing labels are auto-created.
	labelsCache map[string]struct{}
}

func New(cfg config.Config) *Backend {
	return &Backend{cfg: cfg}
}

// --- public API ---

func (b *Backend) CreatePRD(opts backend.CreateOpts) (string, error) {
	labels := mergeLabels(b.cfg.DefaultLabels, opts.Labels, b.cfg.PRDLabel)
	return b.createIssue(opts.Summary, opts.BodyMD, labels, opts.Assignee)
}

func (b *Backend) CreateSubissue(opts backend.CreateOpts) (string, error) {
	if opts.ParentKey == "" {
		return "", errors.New("parent key is required for sub-issues")
	}
	parentNum, err := normalizeKey(opts.ParentKey)
	if err != nil {
		return "", err
	}
	labels := mergeLabels(b.cfg.DefaultLabels, opts.Labels)
	childKey, err := b.createIssue(opts.Summary, opts.BodyMD, labels, opts.Assignee)
	if err != nil {
		return "", err
	}
	childNum, _ := strconv.Atoi(childKey)
	if err := b.addSubIssue(parentNum, childNum); err != nil {
		return childKey, fmt.Errorf("created #%s but failed to set parent: %w", childKey, err)
	}
	return childKey, nil
}

func (b *Backend) ListPRDs(opts backend.ListOpts) ([]backend.Issue, error) {
	args := []string{"issue", "list",
		"--label", b.cfg.PRDLabel,
		"--json", listJSONFields,
	}
	for _, l := range opts.Labels {
		args = append(args, "--label", l)
	}
	args = append(args, "--state", chooseState(opts))
	args = append(args, "--limit", listLimit(opts.Limit))
	out, err := b.run(args...)
	if err != nil {
		return nil, err
	}
	return b.decodeIssueList(out)
}

func (b *Backend) ListSubissues(parentKey string, opts backend.ListOpts) ([]backend.Issue, error) {
	if parentKey == "" {
		return b.listProjectWide(opts)
	}
	parentNum, err := normalizeKey(parentKey)
	if err != nil {
		return nil, err
	}
	parentID, err := b.nodeID(parentNum)
	if err != nil {
		return nil, err
	}
	query := `query($id: ID!) {
      node(id: $id) {
        ... on Issue {
          subIssues(first: 100) {
            nodes {
              number title state url
              labels(first: 50) { nodes { name } }
              assignees(first: 5) { nodes { login } }
            }
          }
        }
      }
    }`
	out, err := b.runGraphQL(query, map[string]string{"id": parentID})
	if err != nil {
		return nil, err
	}
	var raw struct {
		Data struct {
			Node struct {
				SubIssues struct {
					Nodes []graphqlIssue `json:"nodes"`
				} `json:"subIssues"`
			} `json:"node"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("decode subIssues: %w", err)
	}
	issues := make([]backend.Issue, 0, len(raw.Data.Node.SubIssues.Nodes))
	for _, n := range raw.Data.Node.SubIssues.Nodes {
		issue := n.toIssue(b.cfg.PRDLabel)
		issue.ParentKey = strconv.Itoa(parentNum)
		if !matchesFilters(issue, opts) {
			continue
		}
		issues = append(issues, issue)
	}
	return issues, nil
}

func (b *Backend) listProjectWide(opts backend.ListOpts) ([]backend.Issue, error) {
	args := []string{"issue", "list",
		"--search", "-label:" + b.cfg.PRDLabel,
		"--json", listJSONFields,
	}
	for _, l := range opts.Labels {
		args = append(args, "--label", l)
	}
	args = append(args, "--state", chooseState(opts))
	args = append(args, "--limit", listLimit(opts.Limit))
	out, err := b.run(args...)
	if err != nil {
		return nil, err
	}
	return b.decodeIssueList(out)
}

func (b *Backend) View(key string) (backend.Issue, error) {
	n, err := normalizeKey(key)
	if err != nil {
		return backend.Issue{}, err
	}
	out, err := b.run("issue", "view", strconv.Itoa(n),
		"--json", "number,title,state,body,labels,assignees,url")
	if err != nil {
		return backend.Issue{}, err
	}
	var raw ghIssue
	if err := json.Unmarshal(out, &raw); err != nil {
		return backend.Issue{}, fmt.Errorf("decode issue view: %w", err)
	}
	issue := raw.toIssue(b.cfg.PRDLabel)
	issue.Description = raw.Body

	// Parent linkage isn't exposed by `gh issue view --json`; fetch via GraphQL.
	id, err := b.nodeID(n)
	if err != nil {
		return issue, err
	}
	pq := `query($id: ID!) { node(id: $id) { ... on Issue { parent { number } } } }`
	pout, err := b.runGraphQL(pq, map[string]string{"id": id})
	if err != nil {
		return issue, err
	}
	var pres struct {
		Data struct {
			Node struct {
				Parent *struct {
					Number int `json:"number"`
				} `json:"parent"`
			} `json:"node"`
		} `json:"data"`
	}
	if err := json.Unmarshal(pout, &pres); err != nil {
		return issue, fmt.Errorf("decode parent: %w", err)
	}
	if pres.Data.Node.Parent != nil {
		issue.ParentKey = strconv.Itoa(pres.Data.Node.Parent.Number)
	}
	return issue, nil
}

func (b *Backend) Comment(key, bodyMD string) error {
	n, err := normalizeKey(key)
	if err != nil {
		return err
	}
	_, err = b.runStdin(bodyMD, "issue", "comment", strconv.Itoa(n), "--body-file", "-")
	return err
}

func (b *Backend) AddLabel(key, label string) error {
	n, err := normalizeKey(key)
	if err != nil {
		return err
	}
	if err := b.ensureLabels([]string{label}); err != nil {
		return err
	}
	_, err = b.run("issue", "edit", strconv.Itoa(n), "--add-label", label)
	return err
}

func (b *Backend) RemoveLabel(key, label string) error {
	n, err := normalizeKey(key)
	if err != nil {
		return err
	}
	_, err = b.run("issue", "edit", strconv.Itoa(n), "--remove-label", label)
	return err
}

func (b *Backend) Close(key, status, comment string) error {
	n, err := normalizeKey(key)
	if err != nil {
		return err
	}
	reason, err := mapCloseReason(status)
	if err != nil {
		return err
	}
	if comment != "" {
		if err := b.Comment(key, comment); err != nil {
			return err
		}
	}
	_, err = b.run("issue", "close", strconv.Itoa(n), "--reason", reason)
	return err
}

func (b *Backend) ListLinks(key string) ([]backend.Link, error) {
	n, err := normalizeKey(key)
	if err != nil {
		return nil, err
	}
	slug, err := b.slug()
	if err != nil {
		return nil, err
	}
	blockedBy, err := b.fetchDependencyList(slug, n, "blocked_by")
	if err != nil {
		return nil, err
	}
	blocking, err := b.fetchDependencyList(slug, n, "blocking")
	if err != nil {
		return nil, err
	}
	links := make([]backend.Link, 0, len(blockedBy)+len(blocking))
	for _, dep := range blockedBy {
		// Other side blocks us → we are inward.
		links = append(links, backend.Link{
			ID:        fmt.Sprintf("%d-blocks-%d", dep.Number, n),
			Type:      "Blocks",
			Direction: "in",
			Phrase:    "is blocked by",
			OtherKey:  strconv.Itoa(dep.Number),
			Summary:   dep.Title,
			Status:    dep.State,
		})
	}
	for _, dep := range blocking {
		// We block them → we are outward.
		links = append(links, backend.Link{
			ID:        fmt.Sprintf("%d-blocks-%d", n, dep.Number),
			Type:      "Blocks",
			Direction: "out",
			Phrase:    "blocks",
			OtherKey:  strconv.Itoa(dep.Number),
			Summary:   dep.Title,
			Status:    dep.State,
		})
	}
	return links, nil
}

func (b *Backend) AddLink(outKey, inKey, linkType string) error {
	if !isBlocksType(linkType) {
		return fmt.Errorf("link type %q not supported on github (only Blocks / blocks)", linkType)
	}
	outNum, err := normalizeKey(outKey)
	if err != nil {
		return err
	}
	inNum, err := normalizeKey(inKey)
	if err != nil {
		return err
	}
	slug, err := b.slug()
	if err != nil {
		return err
	}
	// Express as: inKey is blocked by outKey.
	outDBID, err := b.databaseID(slug, outNum)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("repos/%s/issues/%d/dependencies/blocked_by", slug, inNum)
	body := fmt.Sprintf(`{"issue_id":%d}`, outDBID)
	_, err = b.runStdin(body, "api", "--method", "POST", path, "--input", "-",
		"--header", "Accept: application/vnd.github+json")
	return err
}

func (b *Backend) RemoveLink(linkID string) error {
	outNum, inNum, err := parseLinkID(linkID)
	if err != nil {
		return err
	}
	slug, err := b.slug()
	if err != nil {
		return err
	}
	outDBID, err := b.databaseID(slug, outNum)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("repos/%s/issues/%d/dependencies/blocked_by", slug, inNum)
	body := fmt.Sprintf(`{"issue_id":%d}`, outDBID)
	_, err = b.runStdin(body, "api", "--method", "DELETE", path, "--input", "-",
		"--header", "Accept: application/vnd.github+json")
	return err
}

// --- internals ---

const listJSONFields = "number,title,state,labels,assignees,url"

func (b *Backend) createIssue(title, bodyMD string, labels []string, assignee string) (string, error) {
	if err := b.ensureLabels(labels); err != nil {
		return "", err
	}
	args := []string{"issue", "create", "--title", title, "--body-file", "-"}
	for _, l := range labels {
		args = append(args, "--label", l)
	}
	if assignee != "" {
		args = append(args, "--assignee", assignee)
	}
	out, err := b.runStdin(bodyMD, args...)
	if err != nil {
		return "", err
	}
	num, err := numberFromURL(strings.TrimSpace(string(out)))
	if err != nil {
		return "", fmt.Errorf("parse `gh issue create` output: %w (body: %q)", err, string(out))
	}
	return strconv.Itoa(num), nil
}

func (b *Backend) addSubIssue(parentNum, childNum int) error {
	parentID, err := b.nodeID(parentNum)
	if err != nil {
		return err
	}
	childID, err := b.nodeID(childNum)
	if err != nil {
		return err
	}
	mutation := `mutation($p: ID!, $c: ID!) {
      addSubIssue(input: { issueId: $p, subIssueId: $c }) {
        issue { number }
      }
    }`
	_, err = b.runGraphQL(mutation, map[string]string{"p": parentID, "c": childID})
	return err
}

func (b *Backend) nodeID(number int) (string, error) {
	out, err := b.run("issue", "view", strconv.Itoa(number), "--json", "id")
	if err != nil {
		return "", err
	}
	var r struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return "", fmt.Errorf("decode node id: %w", err)
	}
	if r.ID == "" {
		return "", fmt.Errorf("no node id returned for #%d", number)
	}
	return r.ID, nil
}

// databaseID returns the issue's internal database id used by REST endpoints
// like the dependency API (which takes `issue_id`, not the issue number).
func (b *Backend) databaseID(slug string, number int) (int64, error) {
	out, err := b.run("api", fmt.Sprintf("repos/%s/issues/%d", slug, number),
		"--jq", ".id")
	if err != nil {
		return 0, err
	}
	id, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse database id %q: %w", string(out), err)
	}
	return id, nil
}

func (b *Backend) slug() (string, error) {
	if b.repoSlug != "" {
		return b.repoSlug, nil
	}
	out, err := b.run("repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner")
	if err != nil {
		return "", err
	}
	slug := strings.TrimSpace(string(out))
	if slug == "" {
		return "", errors.New("could not resolve repo: run inside a git checkout or set `gh repo set-default`")
	}
	b.repoSlug = slug
	return slug, nil
}

func (b *Backend) ensureLabels(labels []string) error {
	if len(labels) == 0 {
		return nil
	}
	if b.labelsCache == nil {
		out, err := b.run("label", "list", "--json", "name", "--limit", "500")
		if err != nil {
			return fmt.Errorf("list labels: %w", err)
		}
		var existing []struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(out, &existing); err != nil {
			return fmt.Errorf("decode labels: %w", err)
		}
		b.labelsCache = make(map[string]struct{}, len(existing))
		for _, l := range existing {
			b.labelsCache[l.Name] = struct{}{}
		}
	}
	for _, l := range labels {
		if _, ok := b.labelsCache[l]; ok {
			continue
		}
		if _, err := b.run("label", "create", l, "--color", "ededed", "--force"); err != nil {
			return fmt.Errorf("create label %q: %w", l, err)
		}
		b.labelsCache[l] = struct{}{}
	}
	return nil
}

type dependencyIssue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
}

func (b *Backend) fetchDependencyList(slug string, n int, kind string) ([]dependencyIssue, error) {
	path := fmt.Sprintf("repos/%s/issues/%d/dependencies/%s", slug, n, kind)
	out, err := b.run("api", path, "--header", "Accept: application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	var deps []dependencyIssue
	if err := json.Unmarshal(out, &deps); err != nil {
		return nil, fmt.Errorf("decode dependencies/%s: %w", kind, err)
	}
	return deps, nil
}

// --- exec helpers ---

func (b *Backend) run(args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("gh %s: %s", strings.Join(args, " "), msg)
	}
	return out, nil
}

func (b *Backend) runStdin(stdin string, args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	cmd.Stdin = strings.NewReader(stdin)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("gh %s: %s", strings.Join(args, " "), msg)
	}
	return out, nil
}

// runGraphQL executes `gh api graphql` with the given query and string-typed
// variables. All values are passed via -f (string fields); ID values fit this
// since GraphQL IDs serialize as strings.
func (b *Backend) runGraphQL(query string, vars map[string]string) ([]byte, error) {
	args := []string{"api", "graphql", "-f", "query=" + query}
	for k, v := range vars {
		args = append(args, "-f", k+"="+v)
	}
	out, err := b.run(args...)
	if err != nil {
		return nil, err
	}
	// gh returns the raw GraphQL response. Surface any `errors` array as a Go error.
	var probe struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(out, &probe); err == nil && len(probe.Errors) > 0 {
		msgs := make([]string, 0, len(probe.Errors))
		for _, e := range probe.Errors {
			msgs = append(msgs, e.Message)
		}
		return nil, fmt.Errorf("graphql: %s", strings.Join(msgs, "; "))
	}
	return out, nil
}

// --- decoding ---

// ghIssue is the shape `gh issue view/list --json ...` returns.
type ghIssue struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	State     string `json:"state"`
	Body      string `json:"body,omitempty"`
	URL       string `json:"url"`
	Labels    []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Assignees []struct {
		Login string `json:"login"`
	} `json:"assignees"`
}

func (g ghIssue) toIssue(prdLabel string) backend.Issue {
	labels := make([]string, 0, len(g.Labels))
	for _, l := range g.Labels {
		labels = append(labels, l.Name)
	}
	assignee := ""
	if len(g.Assignees) > 0 {
		assignee = g.Assignees[0].Login
	}
	return backend.Issue{
		Key:      strconv.Itoa(g.Number),
		Type:     deriveType(labels, prdLabel),
		Summary:  g.Title,
		Status:   g.State,
		Labels:   labels,
		URL:      g.URL,
		Assignee: assignee,
	}
}

// graphqlIssue is the shape returned by GraphQL queries (labels/assignees are
// connections with a "nodes" wrapper).
type graphqlIssue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	URL    string `json:"url"`
	Labels struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels"`
	Assignees struct {
		Nodes []struct {
			Login string `json:"login"`
		} `json:"nodes"`
	} `json:"assignees"`
}

func (g graphqlIssue) toIssue(prdLabel string) backend.Issue {
	labels := make([]string, 0, len(g.Labels.Nodes))
	for _, l := range g.Labels.Nodes {
		labels = append(labels, l.Name)
	}
	assignee := ""
	if len(g.Assignees.Nodes) > 0 {
		assignee = g.Assignees.Nodes[0].Login
	}
	return backend.Issue{
		Key:      strconv.Itoa(g.Number),
		Type:     deriveType(labels, prdLabel),
		Summary:  g.Title,
		Status:   g.State,
		Labels:   labels,
		URL:      g.URL,
		Assignee: assignee,
	}
}

func (b *Backend) decodeIssueList(out []byte) ([]backend.Issue, error) {
	var raw []ghIssue
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("decode issue list: %w (body: %.200s)", err, string(out))
	}
	issues := make([]backend.Issue, 0, len(raw))
	for _, r := range raw {
		issues = append(issues, r.toIssue(b.cfg.PRDLabel))
	}
	return issues, nil
}

// --- pure helpers ---

// normalizeKey accepts "742" or "#742" and returns 742.
func normalizeKey(key string) (int, error) {
	key = strings.TrimSpace(key)
	key = strings.TrimPrefix(key, "#")
	if key == "" {
		return 0, errors.New("empty issue key")
	}
	n, err := strconv.Atoi(key)
	if err != nil {
		return 0, fmt.Errorf("invalid github issue key %q (expected number or #number)", key)
	}
	return n, nil
}

// numberFromURL extracts N from .../issues/N (the create command's output).
func numberFromURL(url string) (int, error) {
	url = strings.TrimSpace(url)
	i := strings.LastIndex(url, "/")
	if i < 0 || i == len(url)-1 {
		return 0, fmt.Errorf("no number in url %q", url)
	}
	n, err := strconv.Atoi(url[i+1:])
	if err != nil {
		return 0, fmt.Errorf("not a number in url %q: %w", url, err)
	}
	return n, nil
}

// mapState turns the user's status filter into a `gh issue list --state` value.
// Empty means "no filter" → "all"; chooseState decides whether the empty case
// should fall back to "open" (when IncludeClosed is false).
func mapState(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "all", "any":
		return "all"
	case "open":
		return "open"
	case "closed":
		return "closed"
	default:
		// Unknown filter value; let gh tell the user.
		return status
	}
}

// chooseState picks the `--state` arg from a ListOpts. Explicit Status wins;
// otherwise IncludeClosed=false hides closed issues (the default).
func chooseState(opts backend.ListOpts) string {
	if opts.Status != "" {
		return mapState(opts.Status)
	}
	if opts.IncludeClosed {
		return "all"
	}
	return "open"
}

func listLimit(limit int) string {
	if limit > 0 {
		return strconv.Itoa(limit)
	}
	return "1000"
}

// mapCloseReason translates the tracker-agnostic `status` arg into `gh issue
// close --reason`. Empty defaults to completed. Anything outside the two
// accepted reasons is an explicit error.
func mapCloseReason(status string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "completed", "done":
		return "completed", nil
	case "not planned", "not_planned", "not-planned", "cancelled", "canceled":
		return "not planned", nil
	default:
		return "", fmt.Errorf("github close only supports 'completed' or 'not planned' (got %q)", status)
	}
}

func isBlocksType(t string) bool {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "blocks", "block":
		return true
	}
	return false
}

// parseLinkID inverts the synthesized "<out>-blocks-<in>" format.
func parseLinkID(id string) (out int, in int, err error) {
	parts := strings.Split(id, "-")
	if len(parts) != 3 || parts[1] != "blocks" {
		return 0, 0, fmt.Errorf("invalid github link id %q (expected <out>-blocks-<in>)", id)
	}
	out, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid out number in link id %q", id)
	}
	in, err = strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid in number in link id %q", id)
	}
	return out, in, nil
}

func deriveType(labels []string, prdLabel string) string {
	for _, l := range labels {
		if l == prdLabel {
			return "PRD"
		}
	}
	return "Issue"
}

func matchesFilters(issue backend.Issue, opts backend.ListOpts) bool {
	if opts.Status != "" {
		want := mapState(opts.Status)
		if want != "all" && !strings.EqualFold(issue.Status, want) {
			return false
		}
	} else if !opts.IncludeClosed && strings.EqualFold(issue.Status, "closed") {
		return false
	}
	if len(opts.Labels) > 0 {
		have := make(map[string]struct{}, len(issue.Labels))
		for _, l := range issue.Labels {
			have[l] = struct{}{}
		}
		for _, want := range opts.Labels {
			if _, ok := have[want]; !ok {
				return false
			}
		}
	}
	return true
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
