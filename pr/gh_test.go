// pr/gh_test.go
package pr

import (
	"context"
	"io"
	"strings"
	"testing"
)

type fakeRunner struct {
	outputs map[string][]byte
	runs    [][]string
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.runs = append(f.runs, append([]string{name}, args...))
	key := strings.Join(append([]string{name}, args...), " ")
	if out, ok := f.outputs[key]; ok {
		return out, nil
	}
	return []byte{}, nil
}

func (f *fakeRunner) RunWithStdin(_ context.Context, _ io.Reader, _ string, _ ...string) ([]byte, error) {
	return nil, nil
}

const testRemote = "git@github.com:alex-irvine/lazydiff.git"

func TestGitHubListPRsParsesJSON(t *testing.T) {
	runner := &fakeRunner{outputs: map[string][]byte{
		"gh pr list --state open --json number,title,author,headRefName,baseRefName,mergeable,url,createdAt": []byte(`[{"number":42,"title":"feat: add login","author":"alex","headRefName":"feat-login","baseRefName":"main","mergeable":"MERGEABLE","url":"https://github.com/alex-irvine/lazydiff/pull/42","createdAt":"2026-07-01T00:00:00Z"}]`),
	}}
	g := NewGitHub(testRemote, runner)
	prs, err := g.ListPRs(context.Background(), "open")
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 1 {
		t.Fatalf("prs = %+v", prs)
	}
	p := prs[0]
	if p.Number != 42 || p.Title != "feat: add login" || p.HeadRefName != "feat-login" || p.BaseRefName != "main" || p.Mergeable != "MERGEABLE" {
		t.Fatalf("pr = %+v", p)
	}
}

func TestGitHubListPRsRejectsNonGitHubRemote(t *testing.T) {
	g := NewGitHub("git@gitlab.com:some/repo.git", &fakeRunner{outputs: map[string][]byte{}})
	if _, err := g.ListPRs(context.Background(), "open"); err == nil || !strings.Contains(err.Error(), "github.com") {
		t.Fatalf("err = %v, want github.com rejection", err)
	}
}

func TestGitHubPRDiffReturnsRawPatch(t *testing.T) {
	runner := &fakeRunner{outputs: map[string][]byte{
		"gh pr diff 42 --patch": []byte("diff --git a/login.go b/login.go\n@@ -0,0 +1 @@\n+func login() {}\n"),
	}}
	g := NewGitHub(testRemote, runner)
	raw, err := g.PRDiff(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "login.go") {
		t.Fatalf("raw = %q", raw)
	}
}

func TestGitHubApproveWithCommentAddsBodyFlag(t *testing.T) {
	g := NewGitHub(testRemote, &fakeRunner{outputs: map[string][]byte{}})
	if err := g.Approve(context.Background(), 42, "LGTM"); err != nil {
		t.Fatal(err)
	}
	want := []string{"gh", "pr", "review", "42", "--approve", "--body", "LGTM"}
	got := g.Runner.(*fakeRunner).runs[0]
	if !equalStrings(got, want) {
		t.Fatalf("run = %v, want %v", got, want)
	}
}

func TestGitHubApproveWithoutCommentOmitsBody(t *testing.T) {
	g := NewGitHub(testRemote, &fakeRunner{outputs: map[string][]byte{}})
	if err := g.Approve(context.Background(), 42, ""); err != nil {
		t.Fatal(err)
	}
	want := []string{"gh", "pr", "review", "42", "--approve"}
	got := g.Runner.(*fakeRunner).runs[0]
	if !equalStrings(got, want) {
		t.Fatalf("run = %v, want %v", got, want)
	}
}

func TestGitHubRequestChangesPassesBody(t *testing.T) {
	g := NewGitHub(testRemote, &fakeRunner{outputs: map[string][]byte{}})
	if err := g.RequestChanges(context.Background(), 42, "needs tests"); err != nil {
		t.Fatal(err)
	}
	want := []string{"gh", "pr", "review", "42", "--request-changes", "--body", "needs tests"}
	got := g.Runner.(*fakeRunner).runs[0]
	if !equalStrings(got, want) {
		t.Fatalf("run = %v, want %v", got, want)
	}
}

func TestGitHubMergePassesMergeFlag(t *testing.T) {
	g := NewGitHub(testRemote, &fakeRunner{outputs: map[string][]byte{}})
	if err := g.Merge(context.Background(), 42); err != nil {
		t.Fatal(err)
	}
	want := []string{"gh", "pr", "merge", "42", "--merge"}
	got := g.Runner.(*fakeRunner).runs[0]
	if !equalStrings(got, want) {
		t.Fatalf("run = %v, want %v", got, want)
	}
}

func TestGitHubClose(t *testing.T) {
	g := NewGitHub(testRemote, &fakeRunner{outputs: map[string][]byte{}})
	if err := g.Close(context.Background(), 42); err != nil {
		t.Fatal(err)
	}
	want := []string{"gh", "pr", "close", "42"}
	got := g.Runner.(*fakeRunner).runs[0]
	if !equalStrings(got, want) {
		t.Fatalf("run = %v, want %v", got, want)
	}
}

func TestGitHubDeleteBranchRunsGitPush(t *testing.T) {
	g := NewGitHub(testRemote, &fakeRunner{outputs: map[string][]byte{}})
	if err := g.DeleteBranch(context.Background(), "feat-login"); err != nil {
		t.Fatal(err)
	}
	want := []string{"git", "push", "origin", "--delete", "feat-login"}
	got := g.Runner.(*fakeRunner).runs[0]
	if !equalStrings(got, want) {
		t.Fatalf("run = %v, want %v", got, want)
	}
}

func TestGitHubPRReturnsSinglePR(t *testing.T) {
	runner := &fakeRunner{outputs: map[string][]byte{
		"gh pr view 42 --json number,title,author,headRefName,baseRefName,mergeable,url,createdAt": []byte(`{"number":42,"title":"feat: add login","author":"alex","headRefName":"feat-login","baseRefName":"main","mergeable":"MERGEABLE","url":"https://github.com/alex-irvine/lazydiff/pull/42","createdAt":"2026-07-01T00:00:00Z"}`),
	}}
	g := NewGitHub(testRemote, runner)
	p, err := g.PR(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if p.Number != 42 || p.BaseRefName != "main" || p.HeadRefName != "feat-login" {
		t.Fatalf("pr = %+v", p)
	}
}

func equalStrings(a, b []string) bool {
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
