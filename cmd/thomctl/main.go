// Command thomctl is a tracker-agnostic issue-tracker wrapper. It exposes a
// stable surface (PRDs and sub-issues) so agent skills don't have to learn
// the underlying tracker's API.
//
// Supported backends: Jira (via the Atlassian `acli` CLI) and GitHub (via
// the `gh` CLI). Select with `backend: jira` or `backend: github` in
// `.thomctl.yaml`.
package main

import (
	"fmt"
	"os"

	"github.com/ludusrusso/thom/internal/cli"
)

func main() {
	if err := cli.Root().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
