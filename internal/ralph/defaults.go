package ralph

import _ "embed"

//go:embed defaults/prompt.md
var defaultPrompt string

//go:embed defaults/pr-prompt.md
var defaultPRPrompt string

//go:embed defaults/settings.json
var defaultSettings string

// DefaultPrompt is the loop prompt scaffolded into .thom/ralph/prompt.md.
func DefaultPrompt() string { return defaultPrompt }

// DefaultSettings is the claude --settings JSON scaffolded into
// .thom/ralph/settings.json.
func DefaultSettings() string { return defaultSettings }

// DefaultPRPrompt is the PR-drafting prompt. It is intentionally not
// user-overridable — the inputs are mechanical (PRD title/body, resolved
// sub-issues, commit log) and the output contract is strict.
func DefaultPRPrompt() string { return defaultPRPrompt }
