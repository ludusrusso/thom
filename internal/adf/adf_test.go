package adf

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFromMarkdown_Smoke(t *testing.T) {
	doc, err := FromMarkdown(strings.TrimSpace(`
# Heading

A paragraph with **bold** and *em* and ` + "`code`" + ` and a [link](https://example.com).

- one
- two
  - nested

1. ordered
2. items

> quote line

` + "```go\nfunc x() {}\n```" + `

---
`))
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	for _, want := range []string{
		`"type":"doc"`,
		`"type":"heading"`,
		`"type":"strong"`,
		`"type":"em"`,
		`"type":"code"`,
		`"type":"link"`,
		`"href":"https://example.com"`,
		`"type":"bulletList"`,
		`"type":"orderedList"`,
		`"type":"blockquote"`,
		`"type":"codeBlock"`,
		`"language":"go"`,
		`"type":"rule"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in: %s", want, s)
		}
	}
}

func TestToPlain(t *testing.T) {
	doc, err := FromMarkdown("# Title\n\nHello **world**.")
	if err != nil {
		t.Fatal(err)
	}
	out := ToPlain(doc)
	if !strings.Contains(out, "Title") || !strings.Contains(out, "Hello world") {
		t.Errorf("unexpected plain output: %q", out)
	}
}

// Code marks in ADF cannot coexist with em/strong/strike/underline — Jira
// returns INVALID_INPUT otherwise. Regression test for that filtering.
func TestFromMarkdown_CodeExcludesEm(t *testing.T) {
	doc, err := FromMarkdown("_an `thomctl` tool_")
	if err != nil {
		t.Fatal(err)
	}
	node := findText(doc, "thomctl")
	if node == nil {
		t.Fatal("no `thomctl` text node found")
	}
	marks, _ := node["marks"].([]map[string]any)
	hasCode, hasEm := false, false
	for _, m := range marks {
		switch m["type"] {
		case "code":
			hasCode = true
		case "em", "strong", "strike", "underline":
			hasEm = true
		}
	}
	if !hasCode {
		t.Errorf("code mark missing on `thomctl` node: %v", node)
	}
	if hasEm {
		t.Errorf("excluded mark coexists with code: %v", node)
	}
}

func findText(n any, want string) map[string]any {
	m, ok := n.(map[string]any)
	if !ok {
		return nil
	}
	if t, _ := m["type"].(string); t == "text" {
		if s, _ := m["text"].(string); s == want {
			return m
		}
	}
	if c, ok := m["content"].([]any); ok {
		for _, child := range c {
			if r := findText(child, want); r != nil {
				return r
			}
		}
	}
	return nil
}

func TestFromMarkdown_Empty(t *testing.T) {
	doc, err := FromMarkdown("")
	if err != nil {
		t.Fatal(err)
	}
	if doc["type"] != "doc" {
		t.Fatalf("want doc, got %v", doc["type"])
	}
	content, ok := doc["content"].([]any)
	if !ok || len(content) != 0 {
		t.Fatalf("want empty content, got %v", doc["content"])
	}
}
