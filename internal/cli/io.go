package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/ludusrusso/thom/internal/backend"
)

// linkify wraps text in an OSC 8 hyperlink escape so modern terminals render
// it as a clickable link. Falls back to plain text when stdout isn't a TTY,
// NO_COLOR is set (https://no-color.org), or no URL is available. The visible
// width of the wrapped string matches the plain text, so callers can compute
// column widths from the raw key.
func linkify(text, url string) string {
	if url == "" {
		return text
	}
	if os.Getenv("NO_COLOR") != "" {
		return text
	}
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return text
	}
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

// hitl/afk labels are the Pocock `to-issues` convention for marking
// tracer-bullet slices.

// readBody resolves --body / --body-file / "-". An empty body is allowed
// for commands that pass empty markdown straight through.
func readBody(body, bodyFile string) (string, error) {
	if body != "" && bodyFile != "" {
		return "", fmt.Errorf("--body and --body-file are mutually exclusive")
	}
	if body != "" {
		return body, nil
	}
	if bodyFile == "" {
		return "", nil
	}
	if bodyFile == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return string(b), nil
	}
	b, err := os.ReadFile(bodyFile)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func emitJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// hitlAfkTag returns "HITL" / "AFK" / "" depending on which Pocock-convention
// label the issue carries. The hitl/afk labels mark tracer-bullet slices for
// the `to-issues` skill: HITL needs human interaction, AFK is agent-runnable.
func hitlAfkTag(labels []string) string {
	for _, l := range labels {
		switch strings.ToLower(l) {
		case "hitl":
			return "HITL"
		case "afk":
			return "AFK"
		}
	}
	return ""
}

func emitIssueTable(issues []backend.Issue) {
	if len(issues) == 0 {
		return
	}
	maxKey, maxStatus, maxType, maxTag, maxLabels :=
		len("KEY"), len("STATUS"), len("TYPE"), len("MODE"), len("LABELS")
	rows := make([][6]string, 0, len(issues))
	urls := make([]string, 0, len(issues))
	for _, i := range issues {
		mode := hitlAfkTag(i.Labels)
		labels := strings.Join(otherLabels(i.Labels), ", ")
		row := [6]string{i.Key, i.Type, i.Status, mode, labels, i.Summary}
		rows = append(rows, row)
		urls = append(urls, i.URL)
		if l := len(row[0]); l > maxKey {
			maxKey = l
		}
		if l := len(row[1]); l > maxType {
			maxType = l
		}
		if l := len(row[2]); l > maxStatus {
			maxStatus = l
		}
		if l := len(row[3]); l > maxTag {
			maxTag = l
		}
		if l := len(row[4]); l > maxLabels {
			maxLabels = l
		}
	}
	fmt.Printf("%-*s  %-*s  %-*s  %-*s  %-*s  %s\n",
		maxKey, "KEY", maxType, "TYPE", maxStatus, "STATUS",
		maxTag, "MODE", maxLabels, "LABELS", "SUMMARY")
	for idx, r := range rows {
		// Pad to maxKey first, THEN wrap with OSC 8 — otherwise the escape
		// bytes count toward the padded width and the columns shift right.
		keyCell := linkify(fmt.Sprintf("%-*s", maxKey, r[0]), urls[idx])
		fmt.Printf("%s  %-*s  %-*s  %-*s  %-*s  %s\n",
			keyCell, maxType, r[1], maxStatus, r[2],
			maxTag, r[3], maxLabels, r[4], r[5])
	}
}

// otherLabels returns every label except the HITL/AFK marker (already shown
// in the dedicated MODE column).
func otherLabels(labels []string) []string {
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		switch strings.ToLower(l) {
		case "hitl", "afk":
			continue
		}
		out = append(out, l)
	}
	return out
}

func emitIssueDetail(i backend.Issue, be backend.Backend) {
	fmt.Printf("Key:      %s\n", linkify(i.Key, i.URL))
	fmt.Printf("Type:     %s\n", i.Type)
	fmt.Printf("Status:   %s\n", i.Status)
	if i.ParentKey != "" {
		fmt.Printf("Parent:   %s\n", linkify(i.ParentKey, be.IssueURL(i.ParentKey)))
	}
	if i.Assignee != "" {
		fmt.Printf("Assignee: %s\n", i.Assignee)
	}
	if len(i.Labels) > 0 {
		fmt.Printf("Labels:   %s\n", strings.Join(i.Labels, ", "))
	}
	fmt.Printf("Summary:  %s\n", i.Summary)
	if i.Description != "" {
		fmt.Printf("\n%s\n", strings.TrimRight(i.Description, "\n"))
	}
}

func emitSearchTable(issues []backend.Issue) {
	if len(issues) == 0 {
		return
	}
	header := [6]string{"KEY", "TYPE", "STATUS", "ASSIGNEE", "REPORTER", "SUMMARY"}
	rows := make([][6]string, 0, len(issues))
	var width [6]int
	for i, h := range header {
		width[i] = len(h)
	}
	for _, is := range issues {
		row := [6]string{is.Key, is.Type, is.Status, is.Assignee, is.Reporter, is.Summary}
		rows = append(rows, row)
		for i, cell := range row[:5] {
			width[i] = max(width[i], len(cell))
		}
	}
	printRow := func(row [6]string, key string) {
		fmt.Printf("%s%s", key, strings.Repeat(" ", width[0]-len(row[0])+2))
		for i := 1; i < 5; i++ {
			fmt.Printf("%-*s  ", width[i], row[i])
		}
		fmt.Println(row[5])
	}
	printRow(header, header[0])
	for n, row := range rows {
		printRow(row, linkify(row[0], issues[n].URL))
	}
}
