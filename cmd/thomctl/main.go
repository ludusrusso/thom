// Command thomctl is the RedCarbon issue-tracker wrapper. It exposes a stable,
// tracker-agnostic surface (PRDs and sub-issues) so agent skills don't have to
// learn the underlying tracker's API.
//
// Today the only backend is Jira (via the Atlassian `acli` CLI).
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
