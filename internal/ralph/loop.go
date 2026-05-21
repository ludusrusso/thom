package ralph

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ludusrusso/thom/internal/backend"
	"github.com/ludusrusso/thom/internal/config"
)

// loop drives the inner cycle: list open sub-issues, fail fast if no progress
// over two iterations, otherwise invoke Claude with the rendered prompt and
// repeat.
func loop(opts Opts, worktreeDir, prompt, settingsPath string) error {
	rendered := renderPrompt(prompt, opts.PRD)

	var prevSet map[string]struct{}
	iteration := 0
	for {
		issues, err := opts.Backend.ListSubissues(opts.PRD, backend.ListOpts{})
		if err != nil {
			return fmt.Errorf("list sub-issues: %w", err)
		}
		open := openSet(issues)
		if len(open) == 0 {
			fmt.Println("=== All sub-issues are closed. ===")
			return nil
		}

		// No-progress guard: same open set two iterations in a row means
		// Claude isn't closing anything. Bail before burning more tokens.
		if prevSet != nil && setsEqual(open, prevSet) {
			return fmt.Errorf("no progress in 2 iterations (open: %s); aborting", joinSorted(open))
		}
		prevSet = open

		iteration++
		if iteration > 1 {
			fmt.Print("\n\n\n")
		}
		fmt.Printf("=== Iteration %d: %d open sub-issue(s) remaining ===\n", iteration, len(open))

		if err := invokeClaude(worktreeDir, settingsPath, rendered, opts.AFK); err != nil {
			return fmt.Errorf("claude iteration %d: %w", iteration, err)
		}
	}
}

// loadPromptAndSettings reads the user-overridable prompt + settings from
// <repoRoot>/.thom/ralph/. If either is missing, materialises the embedded
// default at the user-facing path (so the file is editable and visible) and
// then reads it. settingsPath is returned as an absolute path that Claude
// can resolve from worktreeDir.
func loadPromptAndSettings(repoRoot, worktreeDir string) (prompt, settingsPath string, err error) {
	ralphDir := filepath.Join(repoRoot, config.ConfigDir, "ralph")
	promptPath := filepath.Join(ralphDir, "prompt.md")
	settingsPath = filepath.Join(ralphDir, "settings.json")

	if err := ensureFile(promptPath, DefaultPrompt()); err != nil {
		return "", "", err
	}
	if err := ensureFile(settingsPath, DefaultSettings()); err != nil {
		return "", "", err
	}

	b, err := os.ReadFile(promptPath)
	if err != nil {
		return "", "", err
	}
	_ = worktreeDir // settingsPath is absolute, claude can resolve it from any cwd
	return string(b), settingsPath, nil
}

func ensureFile(path, content string) error {
	if fileExists(path) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func renderPrompt(prompt, prd string) string {
	return strings.ReplaceAll(prompt, "{{PRD}}", prd)
}

// invokeClaude runs the claude CLI. In interactive mode, claude attaches to
// the TTY directly. In AFK mode, we run with -p --output-format stream-json
// and decode assistant text events to stdout — same UX as rc-frontend's
// afk.sh, but without the bash-and-jq pipeline.
func invokeClaude(workdir, settingsPath, prompt string, afk bool) error {
	if afk {
		return claudeStream(workdir, settingsPath, prompt)
	}
	return claudeInteractive(workdir, settingsPath, prompt)
}

func claudeInteractive(workdir, settingsPath, prompt string) error {
	cmd := exec.Command("claude", "--settings", settingsPath, prompt)
	cmd.Dir = workdir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func claudeStream(workdir, settingsPath, prompt string) error {
	cmd := exec.Command(
		"claude",
		"-p", "--verbose",
		"--output-format", "stream-json",
		"--settings", settingsPath,
		prompt,
	)
	cmd.Dir = workdir
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := renderStream(stdout, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "stream decode: %v\n", err)
	}
	return cmd.Wait()
}

// renderStream consumes claude's stream-json output line by line and prints
// just the assistant's text content — mirroring the jq filter in rc-frontend's
// afk.sh. Non-JSON lines and unrecognised event types are skipped silently.
func renderStream(r io.Reader, w io.Writer) error {
	sc := bufio.NewScanner(r)
	// stream-json events can be longer than the default 64 KiB token buffer
	// (tool results, long assistant turns). Bump the cap.
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var ev streamEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if ev.Type != "assistant" {
			continue
		}
		printed := false
		for _, c := range ev.Message.Content {
			if c.Type == "text" && c.Text != "" {
				fmt.Fprint(w, c.Text)
				printed = true
			}
		}
		if printed {
			fmt.Fprintln(w)
		}
	}
	return sc.Err()
}

type streamEvent struct {
	Type    string `json:"type"`
	Message struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
}

func openSet(issues []backend.Issue) map[string]struct{} {
	open := make(map[string]struct{}, len(issues))
	for _, i := range issues {
		// ListSubissues with default ListOpts excludes closed items; defensive
		// in case a backend ever returns mixed results.
		if isClosedStatus(i.Status) {
			continue
		}
		open[i.Key] = struct{}{}
	}
	return open
}

func isClosedStatus(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "done", "closed", "completata", "completato":
		return true
	}
	return false
}

func setsEqual(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

func joinSorted(set map[string]struct{}) string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}
