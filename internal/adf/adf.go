// Package adf converts CommonMark markdown into Atlassian Document Format (ADF) JSON.
//
// ADF is the JSON-shaped rich-text format Jira Cloud uses for issue descriptions
// and comments. acli's --from-json payload expects the description field in ADF.
//
// Supported nodes: headings, paragraphs, hard/soft breaks, strong/em/strike,
// inline code, links, bullet/ordered lists (nested), code blocks (fenced),
// blockquotes, thematic breaks, and images (rendered as links — ADF media nodes
// require uploaded media IDs which we don't have).
package adf

import (
	"bytes"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	gmtext "github.com/yuin/goldmark/text"
)

// FromMarkdown parses md and returns an ADF doc node.
func FromMarkdown(md string) (map[string]any, error) {
	src := []byte(md)
	root := goldmark.DefaultParser().Parse(gmtext.NewReader(src))
	c := &converter{src: src}
	blocks := c.blocks(root)
	if blocks == nil {
		blocks = []any{}
	}
	return map[string]any{
		"version": 1,
		"type":    "doc",
		"content": blocks,
	}, nil
}

type converter struct {
	src []byte
}

func (c *converter) blocks(parent ast.Node) []any {
	var out []any
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		if b := c.block(n); b != nil {
			out = append(out, b)
		}
	}
	return out
}

func (c *converter) block(n ast.Node) any {
	switch n := n.(type) {
	case *ast.Heading:
		return map[string]any{
			"type":    "heading",
			"attrs":   map[string]any{"level": n.Level},
			"content": c.inlines(n),
		}
	case *ast.Paragraph:
		return map[string]any{
			"type":    "paragraph",
			"content": c.inlines(n),
		}
	case *ast.ThematicBreak:
		return map[string]any{"type": "rule"}
	case *ast.Blockquote:
		return map[string]any{
			"type":    "blockquote",
			"content": c.blocks(n),
		}
	case *ast.FencedCodeBlock:
		text := string(n.Text(c.src))
		node := map[string]any{
			"type":    "codeBlock",
			"content": []any{map[string]any{"type": "text", "text": text}},
		}
		if lang := string(n.Language(c.src)); lang != "" {
			node["attrs"] = map[string]any{"language": lang}
		}
		return node
	case *ast.CodeBlock:
		text := string(n.Text(c.src))
		return map[string]any{
			"type":    "codeBlock",
			"content": []any{map[string]any{"type": "text", "text": text}},
		}
	case *ast.List:
		kind := "bulletList"
		if n.IsOrdered() {
			kind = "orderedList"
		}
		return map[string]any{
			"type":    kind,
			"content": c.blocks(n),
		}
	case *ast.ListItem:
		return map[string]any{
			"type":    "listItem",
			"content": c.blocks(n),
		}
	case *ast.TextBlock:
		// Tight list items wrap their text in a TextBlock (no <p>). ADF still
		// requires a paragraph inside listItem, so wrap it.
		return map[string]any{
			"type":    "paragraph",
			"content": c.inlines(n),
		}
	case *ast.HTMLBlock:
		// Render raw HTML as a paragraph of plain text — Jira ADF has no
		// equivalent and stripping silently would lose content.
		return map[string]any{
			"type": "paragraph",
			"content": []any{
				map[string]any{"type": "text", "text": string(n.Text(c.src))},
			},
		}
	default:
		// Unknown block: best-effort flatten its inline content.
		if n.Type() == ast.TypeBlock {
			return map[string]any{
				"type":    "paragraph",
				"content": c.inlines(n),
			}
		}
		return nil
	}
}

func (c *converter) inlines(parent ast.Node) []any {
	var out []any
	c.walkInlines(parent, nil, &out)
	if out == nil {
		return []any{}
	}
	return out
}

func (c *converter) walkInlines(parent ast.Node, marks []map[string]any, out *[]any) {
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		c.inline(n, marks, out)
	}
}

func (c *converter) inline(n ast.Node, marks []map[string]any, out *[]any) {
	switch n := n.(type) {
	case *ast.Text:
		text := string(n.Segment.Value(c.src))
		if text != "" {
			node := map[string]any{"type": "text", "text": text}
			if len(marks) > 0 {
				node["marks"] = cloneMarks(marks)
			}
			*out = append(*out, node)
		}
		if n.HardLineBreak() {
			*out = append(*out, map[string]any{"type": "hardBreak"})
		} else if n.SoftLineBreak() {
			// Render soft breaks as a space to keep paragraphs readable in ADF.
			*out = append(*out, map[string]any{"type": "text", "text": " "})
		}
	case *ast.String:
		text := string(n.Value)
		if text != "" {
			node := map[string]any{"type": "text", "text": text}
			if len(marks) > 0 {
				node["marks"] = cloneMarks(marks)
			}
			*out = append(*out, node)
		}
	case *ast.CodeSpan:
		var buf bytes.Buffer
		for s := n.FirstChild(); s != nil; s = s.NextSibling() {
			if t, ok := s.(*ast.Text); ok {
				buf.Write(t.Segment.Value(c.src))
			}
		}
		// ADF spec: code excludes em/strong/strike/underline/subsup/textColor.
		// Link is allowed alongside code. Strip the excluded ones to avoid
		// INVALID_INPUT from Jira.
		filtered := stripExcludedByCode(marks)
		extra := append(filtered, map[string]any{"type": "code"})
		*out = append(*out, map[string]any{
			"type":  "text",
			"text":  buf.String(),
			"marks": extra,
		})
	case *ast.Emphasis:
		mark := "em"
		if n.Level == 2 {
			mark = "strong"
		}
		c.walkInlines(n, append(marks, map[string]any{"type": mark}), out)
	case *ast.Link:
		linkMark := map[string]any{
			"type":  "link",
			"attrs": map[string]any{"href": string(n.Destination)},
		}
		c.walkInlines(n, append(marks, linkMark), out)
	case *ast.AutoLink:
		url := string(n.URL(c.src))
		label := string(n.Label(c.src))
		if label == "" {
			label = url
		}
		linkMark := map[string]any{
			"type":  "link",
			"attrs": map[string]any{"href": url},
		}
		*out = append(*out, map[string]any{
			"type":  "text",
			"text":  label,
			"marks": append(cloneMarks(marks), linkMark),
		})
	case *ast.Image:
		// ADF media nodes need an uploaded media ID; render as a link instead.
		alt := flattenText(n, c.src)
		if alt == "" {
			alt = string(n.Destination)
		}
		linkMark := map[string]any{
			"type":  "link",
			"attrs": map[string]any{"href": string(n.Destination)},
		}
		*out = append(*out, map[string]any{
			"type":  "text",
			"text":  alt,
			"marks": append(cloneMarks(marks), linkMark),
		})
	case *ast.RawHTML:
		// Lose-no-content: render raw HTML inline as plain text.
		var buf bytes.Buffer
		for i := 0; i < n.Segments.Len(); i++ {
			seg := n.Segments.At(i)
			buf.Write(seg.Value(c.src))
		}
		txt := buf.String()
		if txt != "" {
			node := map[string]any{"type": "text", "text": txt}
			if len(marks) > 0 {
				node["marks"] = cloneMarks(marks)
			}
			*out = append(*out, node)
		}
	default:
		// Unknown inline — recurse.
		c.walkInlines(n, marks, out)
	}
}

func flattenText(n ast.Node, src []byte) string {
	var buf bytes.Buffer
	for s := n.FirstChild(); s != nil; s = s.NextSibling() {
		switch v := s.(type) {
		case *ast.Text:
			buf.Write(v.Segment.Value(src))
		case *ast.String:
			buf.Write(v.Value)
		default:
			buf.WriteString(flattenText(v, src))
		}
	}
	return buf.String()
}

// stripExcludedByCode removes marks that ADF's `code` mark excludes (em,
// strong, strike, underline, subsup, textColor). Link survives.
func stripExcludedByCode(marks []map[string]any) []map[string]any {
	excluded := map[string]bool{
		"em": true, "strong": true, "strike": true,
		"underline": true, "subsup": true, "textColor": true,
	}
	out := make([]map[string]any, 0, len(marks))
	for _, m := range marks {
		t, _ := m["type"].(string)
		if excluded[t] {
			continue
		}
		clone := make(map[string]any, len(m))
		for k, v := range m {
			clone[k] = v
		}
		out = append(out, clone)
	}
	return out
}

func cloneMarks(marks []map[string]any) []map[string]any {
	if len(marks) == 0 {
		return nil
	}
	out := make([]map[string]any, len(marks))
	for i, m := range marks {
		clone := make(map[string]any, len(m))
		for k, v := range m {
			clone[k] = v
		}
		out[i] = clone
	}
	return out
}

// ToPlain renders ADF inline content back to a plain-text string. Useful for
// list outputs where we want to show the first paragraph as a description.
func ToPlain(node map[string]any) string {
	var buf bytes.Buffer
	walkPlain(node, &buf)
	return buf.String()
}

func walkPlain(n any, buf *bytes.Buffer) {
	m, ok := n.(map[string]any)
	if !ok {
		return
	}
	if t, _ := m["type"].(string); t == "text" {
		if s, _ := m["text"].(string); s != "" {
			buf.WriteString(s)
		}
	}
	if t, _ := m["type"].(string); t == "hardBreak" {
		buf.WriteString("\n")
	}
	if c, ok := m["content"].([]any); ok {
		for _, child := range c {
			walkPlain(child, buf)
		}
		switch m["type"] {
		case "paragraph", "heading":
			buf.WriteString("\n")
		}
	}
}
