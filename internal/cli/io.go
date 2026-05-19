package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ludusrusso/thom/internal/backend"
)

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
	for _, i := range issues {
		mode := hitlAfkTag(i.Labels)
		labels := strings.Join(otherLabels(i.Labels), ", ")
		row := [6]string{i.Key, i.Type, i.Status, mode, labels, i.Summary}
		rows = append(rows, row)
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
	for _, r := range rows {
		fmt.Printf("%-*s  %-*s  %-*s  %-*s  %-*s  %s\n",
			maxKey, r[0], maxType, r[1], maxStatus, r[2],
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

func emitIssueDetail(i backend.Issue) {
	fmt.Printf("Key:      %s\n", i.Key)
	fmt.Printf("Type:     %s\n", i.Type)
	fmt.Printf("Status:   %s\n", i.Status)
	if i.ParentKey != "" {
		fmt.Printf("Parent:   %s\n", i.ParentKey)
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
