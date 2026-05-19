// Command dump-adf is a debug tool: read markdown from a file (or stdin) and
// print the ADF JSON thomctl would send to Jira. Not part of the public CLI.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/ludusrusso/thom/internal/adf"
)

func main() {
	var b []byte
	var err error
	if len(os.Args) > 1 && os.Args[1] != "-" {
		b, err = os.ReadFile(os.Args[1])
	} else {
		b, err = io.ReadAll(os.Stdin)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	doc, err := adf.FromMarkdown(string(b))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	_ = enc.Encode(doc)
}
