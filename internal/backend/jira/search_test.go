package jira

import (
	"testing"

	"github.com/ludusrusso/thom/internal/backend"
)

func TestSearchJQL(t *testing.T) {
	cases := []struct {
		name    string
		project string
		opts    backend.SearchOpts
		want    string
	}{
		{
			name:    "delegated by me across projects",
			project: "OPE",
			opts:    backend.SearchOpts{Reporter: "@me", Assignee: "!@me", AllProjects: true},
			want:    `assignee != currentUser() AND reporter = currentUser() AND statusCategory != Done ORDER BY updated DESC`,
		},
		{
			name:    "assigned to me in project since a date",
			project: "OPE",
			opts:    backend.SearchOpts{Assignee: "@me", UpdatedSince: "2026-10-30", IncludeClosed: true},
			want:    `project = "OPE" AND assignee = currentUser() AND updated >= "2026-10-30" ORDER BY updated DESC`,
		},
		{
			name:    "reporter by email with explicit status",
			project: "OPE",
			opts:    backend.SearchOpts{Reporter: "a@b.c", Status: "In corso", AllProjects: true},
			want:    `reporter = "a@b.c" AND status = "In corso" ORDER BY updated DESC`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := searchJQL(c.project, c.opts)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got  %s\nwant %s", got, c.want)
			}
		})
	}
}

func TestSearchJQLNeedsProjectUnlessAllProjects(t *testing.T) {
	if _, err := searchJQL("", backend.SearchOpts{Assignee: "@me"}); err == nil {
		t.Fatal("want error without project key")
	}
}
