package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"goforge.dev/tbd/v2/internal/v2/gitops"
)

func TestSplitCommandLineKeepsQuotedArgs(t *testing.T) {
	got, err := splitCommandLine(`tbd feature --id JIRA-123 --desc "Add login" --flag='two words' escaped\ value`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"tbd", "feature", "--id", "JIRA-123", "--desc", "Add login", "--flag=two words", "escaped value"}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arg %d = %q want %q; all=%#v", i, got[i], want[i], got)
		}
	}
	if _, err := splitCommandLine(`tbd feature --desc "missing`); err == nil {
		t.Fatal("expected unterminated quote error")
	}
}

func TestVisualGraphSnapshotIncludesRefsAndWorkflow(t *testing.T) {
	dir := newV2Repo(t)
	if code, out, errOut := runIn(t, dir, "init", "--yes"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}
	if code, out, errOut := runIn(t, dir, "feature", "--id", "JIRA-123", "--desc", "Add login"); code != 0 {
		t.Fatalf("feature failed: %s %s", out, errOut)
	}
	write(t, dir, "login.txt", "login")
	if code, out, errOut := runIn(t, dir, "commit", "--no-edit"); code != 0 {
		t.Fatalf("commit failed: %s %s", out, errOut)
	}
	gitCmd(t, dir, "tag", "dev-deploy")

	e, err := gitops.Load(dir, &bytes.Buffer{}, &bytes.Buffer{}, false)
	if err != nil {
		t.Fatal(err)
	}
	graph, err := buildVisualGraph(e, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Commits) < 2 {
		t.Fatalf("expected multiple commits, got %d", len(graph.Commits))
	}
	if len(graph.Edges) == 0 {
		t.Fatal("expected parent edges")
	}
	assertVisualRef(t, graph, "HEAD", "head")
	assertVisualRef(t, graph, "feature/JIRA-123-add-login", "")
	assertVisualRef(t, graph, "dev-deploy", "deploy")
	if len(graph.Workflow.Items) != 1 || graph.Workflow.Items[0].ID != "JIRA-123" {
		t.Fatalf("workflow items not loaded: %#v", graph.Workflow.Items)
	}
}

func TestServeGraphAPI(t *testing.T) {
	dir := newV2Repo(t)
	if code, out, errOut := runIn(t, dir, "init", "--yes"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}
	e, err := gitops.Load(dir, &bytes.Buffer{}, &bytes.Buffer{}, false)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/graph?limit=20", nil)
	rec := httptest.NewRecorder()
	newServeMux(e, os.Args[0]).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var graph visualGraphSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &graph); err != nil {
		t.Fatal(err)
	}
	graphRoot, _ := filepath.EvalSymlinks(graph.Root)
	wantRoot, _ := filepath.EvalSymlinks(dir)
	if graphRoot != wantRoot {
		t.Fatalf("root=%q want %q", graph.Root, dir)
	}
	if graph.Config.TrunkName != "main" {
		t.Fatalf("trunk=%q", graph.Config.TrunkName)
	}
}

func assertVisualRef(t *testing.T, graph visualGraphSnapshot, name, role string) {
	t.Helper()
	for _, ref := range graph.Refs {
		if ref.Name == name && (role == "" || ref.Role == role) {
			return
		}
	}
	t.Fatalf("missing ref %q role %q in %#v", name, role, graph.Refs)
}
