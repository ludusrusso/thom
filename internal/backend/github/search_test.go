package github

import (
	"reflect"
	"testing"

	"github.com/ludusrusso/thom/internal/backend"
)

func TestSearchArgsQualifiers(t *testing.T) {
	args := searchArgs("", backend.SearchOpts{Reporter: "@me", Assignee: "!@me", UpdatedSince: "2026-10-30", AllProjects: true})
	var query []string
	for i, a := range args {
		if a == "--" {
			query = args[i+1:]
		}
	}
	want := []string{"-assignee:@me", "author:@me", "updated:>=2026-10-30", "state:open"}
	if !reflect.DeepEqual(query, want) {
		t.Errorf("got %v, want %v", query, want)
	}
}

func TestSearchArgsRepoScopeAndAllStates(t *testing.T) {
	args := searchArgs("o/r", backend.SearchOpts{Assignee: "@me", IncludeClosed: true})
	want := []string{"repo:o/r", "assignee:@me"}
	if got := args[len(args)-2:]; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
