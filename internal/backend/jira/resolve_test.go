package jira

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/ludusrusso/thom/internal/backend"
	"github.com/ludusrusso/thom/internal/config"
)

// fakeACLI records every acli call and answers from a handler, so the
// backend can be exercised without a Jira site.
type fakeACLI struct {
	calls  [][]string
	handle func(args []string) ([]byte, error)
}

func (f *fakeACLI) run(args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	return f.handle(args)
}

func (f *fakeACLI) called(sub ...string) [][]string {
	var out [][]string
	for _, c := range f.calls {
		if len(c) >= len(sub) && equalSlices(c[:len(sub)], sub) {
			out = append(out, c)
		}
	}
	return out
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func argAfter(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// An Italian-profile caller sees the built-in names translated.
const italianProjectView = `{"key":"OPE","issueTypes":[
 {"id":"10015","name":"Task","subtask":false,"hierarchyLevel":0},
 {"id":"10016","name":"Sottotask","subtask":true,"hierarchyLevel":-1},
 {"id":"10000","name":"Epic","subtask":false,"hierarchyLevel":1}]}`

func newTestBackend(jc config.JiraConfig, f *fakeACLI) *Backend {
	if jc.ProjectKey == "" {
		jc.ProjectKey = "OPE"
	}
	return &Backend{cfg: config.Config{Jira: jc}, runner: f.run}
}

func TestCreateSubissueResolvesIssueTypeIDToCallerName(t *testing.T) {
	var payload map[string]any
	f := &fakeACLI{handle: func(args []string) ([]byte, error) {
		switch {
		case args[0] == "project":
			return []byte(italianProjectView), nil
		case args[0] == "workitem" && args[1] == "create":
			b, err := os.ReadFile(argAfter(args, "--from-json"))
			if err != nil {
				return nil, err
			}
			if err := json.Unmarshal(b, &payload); err != nil {
				return nil, err
			}
			return []byte(`{"key":"OPE-1"}`), nil
		}
		return nil, errors.New("unexpected call")
	}}
	be := newTestBackend(config.JiraConfig{
		SubissueIssueType:   "Sub-task", // English name, wrong for this caller
		SubissueIssueTypeID: "10016",
	}, f)

	key, err := be.CreateSubissue(backend.CreateOpts{Summary: "s", BodyMD: "b", ParentKey: "OPE-9"})
	if err != nil {
		t.Fatal(err)
	}
	if key != "OPE-1" {
		t.Fatalf("key = %q", key)
	}
	if got := payload["type"]; got != "Sottotask" {
		t.Fatalf("payload type = %v, want the caller's localised name Sottotask", got)
	}
}

func TestCreateWithoutIDSendsConfiguredNameAndSkipsLookup(t *testing.T) {
	var payload map[string]any
	f := &fakeACLI{handle: func(args []string) ([]byte, error) {
		if args[0] == "workitem" && args[1] == "create" {
			b, _ := os.ReadFile(argAfter(args, "--from-json"))
			_ = json.Unmarshal(b, &payload)
			return []byte(`{"key":"OPE-2"}`), nil
		}
		return nil, errors.New("unexpected call")
	}}
	be := newTestBackend(config.JiraConfig{PRDIssueType: "Epic"}, f)

	if _, err := be.CreatePRD(backend.CreateOpts{Summary: "s"}); err != nil {
		t.Fatal(err)
	}
	if payload["type"] != "Epic" {
		t.Fatalf("payload type = %v", payload["type"])
	}
	if n := len(f.called("project")); n != 0 {
		t.Fatalf("project view called %d times, want 0 for a name-only config", n)
	}
}

func TestIssueTypeIDNotInProjectListsAvailableTypes(t *testing.T) {
	f := &fakeACLI{handle: func(args []string) ([]byte, error) {
		return []byte(italianProjectView), nil
	}}
	be := newTestBackend(config.JiraConfig{SubissueIssueTypeID: "99999"}, f)

	_, err := be.CreateSubissue(backend.CreateOpts{Summary: "s", ParentKey: "OPE-9"})
	if err == nil {
		t.Fatal("want error for unknown issue type id")
	}
	for _, want := range []string{"subissue_issue_type_id=99999", `10016="Sottotask", subtask`, `10000="Epic", epic-level`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestProjectIssueTypesAreCached(t *testing.T) {
	f := &fakeACLI{handle: func(args []string) ([]byte, error) {
		return []byte(italianProjectView), nil
	}}
	be := newTestBackend(config.JiraConfig{}, f)
	for i := 0; i < 3; i++ {
		if _, err := be.issueTypeName("", "10016", "subissue_issue_type_id"); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(f.called("project")); n != 1 {
		t.Fatalf("project view called %d times, want 1", n)
	}
}

func TestListUsesIssueTypeIDInJQL(t *testing.T) {
	var jqls []string
	f := &fakeACLI{handle: func(args []string) ([]byte, error) {
		jqls = append(jqls, argAfter(args, "--jql"))
		return []byte(`[]`), nil
	}}
	be := newTestBackend(config.JiraConfig{
		PRDIssueType: "Epic", PRDIssueTypeID: "10000",
		SubissueIssueType: "Sub-task", SubissueIssueTypeID: "10016",
	}, f)

	if _, err := be.ListPRDs(backend.ListOpts{}); err != nil {
		t.Fatal(err)
	}
	if _, err := be.ListSubissues("", backend.ListOpts{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(jqls[0], "issuetype = 10000") || strings.Contains(jqls[0], `"Epic"`) {
		t.Errorf("PRD jql = %q", jqls[0])
	}
	if !strings.Contains(jqls[1], "issuetype = 10016") || strings.Contains(jqls[1], "Sub-task") {
		t.Errorf("subissue jql = %q", jqls[1])
	}
}

func TestListWithoutIDKeepsQuotedName(t *testing.T) {
	var jql string
	f := &fakeACLI{handle: func(args []string) ([]byte, error) {
		jql = argAfter(args, "--jql")
		return []byte(`[]`), nil
	}}
	be := newTestBackend(config.JiraConfig{PRDIssueType: "Epic"}, f)
	if _, err := be.ListPRDs(backend.ListOpts{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(jql, `issuetype = "Epic"`) {
		t.Fatalf("jql = %q", jql)
	}
}

// closeFake simulates an issue moving from "In corso" to whatever status
// acli is asked for, and answers `status = <id>` searches as an Italian
// caller would.
func closeFake(t *testing.T) (*fakeACLI, *string) {
	current := "In corso"
	f := &fakeACLI{}
	f.handle = func(args []string) ([]byte, error) {
		switch {
		case args[1] == "search":
			if jql := argAfter(args, "--jql"); jql != "status = 10015" {
				t.Errorf("status lookup jql = %q", jql)
			}
			return []byte(`[{"key":"OPE-7","fields":{"status":{"id":"10015","name":"Completata"}}}]`), nil
		case args[1] == "view":
			return []byte(`{"key":"OPE-5","fields":{"status":{"name":"` + current + `"}}}`), nil
		case args[1] == "transition":
			current = argAfter(args, "--status")
			return nil, nil
		}
		return nil, errors.New("unexpected call")
	}
	return f, &current
}

func TestCloseResolvesStatusIDToCallerName(t *testing.T) {
	f, current := closeFake(t)
	be := newTestBackend(config.JiraConfig{CloseStatus: "Done", CloseStatusID: "10015"}, f)

	if err := be.Close("OPE-5", "", ""); err != nil {
		t.Fatal(err)
	}
	if *current != "Completata" {
		t.Fatalf("transitioned to %q, want Completata", *current)
	}
}

func TestCloseExplicitStatusWinsOverID(t *testing.T) {
	f, current := closeFake(t)
	be := newTestBackend(config.JiraConfig{CloseStatusID: "10015"}, f)

	if err := be.Close("OPE-5", "Annullato", ""); err != nil {
		t.Fatal(err)
	}
	if *current != "Annullato" {
		t.Fatalf("transitioned to %q, want Annullato", *current)
	}
	if n := len(f.called("workitem", "search")); n != 0 {
		t.Fatalf("status lookup ran %d times, want 0 when --status is explicit", n)
	}
}

func TestCloseWithoutIDUsesConfiguredName(t *testing.T) {
	f, current := closeFake(t)
	be := newTestBackend(config.JiraConfig{CloseStatus: "Completata"}, f)

	if err := be.Close("OPE-5", "", ""); err != nil {
		t.Fatal(err)
	}
	if *current != "Completata" {
		t.Fatalf("transitioned to %q", *current)
	}
	if n := len(f.called("workitem", "search")); n != 0 {
		t.Fatalf("status lookup ran %d times, want 0 for a name-only config", n)
	}
}

func TestCloseStatusIDWithNoIssueInThatStatus(t *testing.T) {
	f := &fakeACLI{handle: func(args []string) ([]byte, error) {
		return []byte(`[]`), nil
	}}
	be := newTestBackend(config.JiraConfig{CloseStatusID: "10015"}, f)

	err := be.Close("OPE-5", "", "")
	if err == nil || !strings.Contains(err.Error(), "close_status_id=10015") {
		t.Fatalf("err = %v", err)
	}
	if n := len(f.called("workitem", "transition")); n != 0 {
		t.Fatal("must not transition when the status cannot be resolved")
	}
}

func TestNonNumericIDIsRejectedBeforeAnyCall(t *testing.T) {
	f := &fakeACLI{handle: func(args []string) ([]byte, error) {
		return []byte(`[]`), nil
	}}
	be := newTestBackend(config.JiraConfig{PRDIssueTypeID: `1 OR project = X`, CloseStatusID: "Done"}, f)

	if _, err := be.ListPRDs(backend.ListOpts{}); err == nil || !strings.Contains(err.Error(), "numeric") {
		t.Fatalf("ListPRDs err = %v", err)
	}
	if err := be.Close("OPE-5", "", ""); err == nil || !strings.Contains(err.Error(), "numeric") {
		t.Fatalf("Close err = %v", err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("acli called %d times, want 0", len(f.calls))
	}
}
