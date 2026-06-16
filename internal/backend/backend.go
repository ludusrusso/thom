// Package backend defines the tracker-agnostic Backend interface used by the
// CLI. Today only the Jira implementation exists; the interface is shaped so
// GitHub, GitLab, or local-markdown backends can be plugged in later without
// touching the CLI surface.
package backend

// Issue is the normalised representation of any tracker item.
type Issue struct {
	Key       string   `json:"key"`
	Type      string   `json:"type"`
	Summary   string   `json:"summary"`
	Status    string   `json:"status"`
	Labels    []string `json:"labels,omitempty"`
	ParentKey string   `json:"parent_key,omitempty"`
	URL       string   `json:"url,omitempty"`
	Assignee  string   `json:"assignee,omitempty"`

	// Description is plain-text (rendered from the tracker's native format).
	// Returned only by View, not by list operations.
	Description string `json:"description,omitempty"`
}

// CreateOpts is the input for creating a PRD or sub-issue.
type CreateOpts struct {
	Summary   string
	BodyMD    string
	Labels    []string
	ParentKey string // empty for top-level PRDs
	Assignee  string // empty = unassigned, "@me" = self
}

// EditOpts carries optional field updates for an existing issue. A nil pointer
// means "leave unchanged"; a non-nil pointer applies the value (including the
// empty string, to clear a field). At least one field must be set.
type EditOpts struct {
	Summary *string // new title/summary
	BodyMD  *string // new description, as markdown
}

// ListOpts filters list operations.
type ListOpts struct {
	Labels []string // AND-matched
	Status string   // tracker-native status string; empty = any
	Limit  int      // 0 = backend default

	// IncludeClosed: when false (default) and Status is empty, list
	// operations exclude completed/closed items. Explicit Status takes
	// precedence — passing Status="Done" returns Done items regardless.
	IncludeClosed bool
}

// Link is one directed relationship between two issues. Direction is "in"
// (the queried issue is on the inward side — e.g. "is blocked by Other") or
// "out" (the queried issue is on the outward side — e.g. "blocks Other").
type Link struct {
	ID        string `json:"id"`
	Type      string `json:"type"`      // canonical type name (Blocks, Relates, ...)
	Direction string `json:"direction"` // "in" or "out"
	OtherKey  string `json:"other_key"` // the issue on the *other* end
	Phrase    string `json:"phrase"`    // human phrase, e.g. "is blocked by", "blocks"
	Summary   string `json:"summary,omitempty"`
	Status    string `json:"status,omitempty"`
}

type Backend interface {
	// CreatePRD creates a top-level PRD-style issue. Returns the new key.
	CreatePRD(opts CreateOpts) (string, error)

	// CreateSubissue creates an issue under an existing PRD.
	CreateSubissue(opts CreateOpts) (string, error)

	// ListPRDs lists top-level PRDs (filtered by the backend's PRD label).
	ListPRDs(opts ListOpts) ([]Issue, error)

	// ListSubissues lists issues whose parent is parentKey. If parentKey is
	// empty, lists sub-issue-type items across the whole project (used by
	// triage workflows that filter by label rather than by parent).
	ListSubissues(parentKey string, opts ListOpts) ([]Issue, error)

	// View returns full detail for a single issue.
	View(key string) (Issue, error)

	// Edit updates mutable fields of an existing issue (summary, description).
	// Fields left nil in opts are unchanged. Returns an error if opts sets no
	// fields.
	Edit(key string, opts EditOpts) error

	// Comment posts a markdown comment on key.
	Comment(key, bodyMD string) error

	// AddLabel adds a label to key.
	AddLabel(key, label string) error

	// RemoveLabel removes a label from key.
	RemoveLabel(key, label string) error

	// Close transitions key to a terminal state. status is the tracker-native
	// target status; pass empty to use the backend's configured default.
	// Comment is optional.
	Close(key, status, comment string) error

	// Transition moves key to an arbitrary tracker-native status (e.g. the
	// in-progress status). status is required. Comment is optional. Backends
	// without arbitrary statuses (e.g. GitHub, whose issues are only
	// open/closed) return an error. Use Close for terminal transitions.
	Transition(key, status, comment string) error

	// ListLinks returns all issue links involving key.
	ListLinks(key string) ([]Link, error)

	// AddLink creates a directional link: outKey <linkType> inKey. The type is
	// the canonical type name (e.g. "Blocks", "Relates", "Duplicate").
	AddLink(outKey, inKey, linkType string) error

	// RemoveLink deletes a single link by its tracker-native ID.
	RemoveLink(linkID string) error

	// IssueURL returns the browser URL for an issue key without fetching it.
	// Used to linkify keys we don't already have an Issue for (e.g. a parent
	// key shown in detail view). Returns "" when no URL can be derived
	// (Jira: site unset; GitHub: repo slug not resolvable; key malformed).
	IssueURL(key string) string
}
