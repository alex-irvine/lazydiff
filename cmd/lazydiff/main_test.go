package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alex-irvine/lazydiff/git"
	"github.com/alex-irvine/lazydiff/pr"
)

func TestRunVersionDoesNotNeedRepository(t *testing.T) {
	t.Chdir(t.TempDir())
	var stdout bytes.Buffer
	if err := run(context.Background(), []string{"lazydiff", "--version"}, strings.NewReader(""), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "lazydiff dev\n" {
		t.Fatalf("version output = %q", got)
	}
}

func TestRunShortVersionFlag(t *testing.T) {
	t.Chdir(t.TempDir())
	var stdout bytes.Buffer
	if err := run(context.Background(), []string{"lazydiff", "-version"}, strings.NewReader(""), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "lazydiff dev\n" {
		t.Fatalf("output = %q", stdout.String())
	}
}

func TestRunRejectsInvalidRepository(t *testing.T) {
	t.Chdir(t.TempDir())
	err := run(context.Background(), []string{"lazydiff", "-config", filepath.Join(t.TempDir(), "missing.toml")}, strings.NewReader(""), os.Stdout, os.Stderr)
	if err == nil || !strings.Contains(err.Error(), "git repository") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunConfigPathIsAccepted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`[agent]
provider = "generic"
command = "cat"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path); err != nil {
		t.Fatal(err)
	}
}

type fakeGHRunner struct {
	outputs map[string][]byte
}

func (f *fakeGHRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	key := strings.Join(append([]string{name}, args...), " ")
	if out, ok := f.outputs[key]; ok {
		return out, nil
	}
	return nil, fmt.Errorf("unexpected command %q", key)
}

func (f *fakeGHRunner) RunWithStdin(context.Context, io.Reader, string, ...string) ([]byte, error) {
	return nil, nil
}

const testPRJSON = `{"number":42,"title":"feat: add login","author":"alex","headRefName":"feat-login","baseRefName":"main","mergeable":"MERGEABLE","url":"https://github.com/alex-irvine/lazydiff/pull/42","createdAt":"2026-07-01T00:00:00Z"}`

func TestSnapshotPRBuildsSnapshotFromGHDiff(t *testing.T) {
	runner := &fakeGHRunner{outputs: map[string][]byte{
		"gh pr view 42 --json number,title,author,headRefName,baseRefName,mergeable,url,createdAt": []byte(testPRJSON),
		"gh pr diff 42 --patch": []byte("diff --git a/login.go b/login.go\nnew file mode 100644\n--- /dev/null\n+++ b/login.go\n@@ -0,0 +1 @@\n+func login() {}\n"),
	}}
	loader := repositoryLoader{gh: pr.NewGitHub("git@github.com:alex-irvine/lazydiff.git", runner)}
	snapshot, err := loader.SnapshotPR(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Mode != git.Branch {
		t.Fatalf("Mode = %v, want git.Branch", snapshot.Mode)
	}
	if snapshot.Base != "main...feat-login" {
		t.Fatalf("Base = %q, want %q", snapshot.Base, "main...feat-login")
	}
	if len(snapshot.Files) != 1 || snapshot.Files[0].Path != "login.go" {
		t.Fatalf("Files = %+v", snapshot.Files)
	}
	if snapshot.ID == "" {
		t.Fatal("ID is empty")
	}
}

func TestSnapshotPRIDIsDeterministicForSameContent(t *testing.T) {
	runner := &fakeGHRunner{outputs: map[string][]byte{
		"gh pr view 42 --json number,title,author,headRefName,baseRefName,mergeable,url,createdAt": []byte(testPRJSON),
		"gh pr diff 42 --patch": []byte("diff --git a/login.go b/login.go\n@@ -0,0 +1 @@\n+func login() {}\n"),
	}}
	loader := repositoryLoader{gh: pr.NewGitHub("git@github.com:alex-irvine/lazydiff.git", runner)}
	first, err := loader.SnapshotPR(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	second, err := loader.SnapshotPR(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("ID not deterministic: %q vs %q", first.ID, second.ID)
	}
}

func TestSnapshotPRIDChangesWhenDiffContentChanges(t *testing.T) {
	runner := &fakeGHRunner{outputs: map[string][]byte{
		"gh pr view 42 --json number,title,author,headRefName,baseRefName,mergeable,url,createdAt": []byte(testPRJSON),
		"gh pr diff 42 --patch": []byte("diff --git a/login.go b/login.go\n@@ -0,0 +1 @@\n+func login() {}\n"),
	}}
	loader := repositoryLoader{gh: pr.NewGitHub("git@github.com:alex-irvine/lazydiff.git", runner)}
	before, err := loader.SnapshotPR(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	// Same PR (same Number/CreatedAt), new commit pushed: diff content changes.
	runner.outputs["gh pr diff 42 --patch"] = []byte("diff --git a/login.go b/login.go\n@@ -0,0 +1 @@\n+func login() { return }\n")
	after, err := loader.SnapshotPR(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if before.ID == after.ID {
		t.Fatalf("ID unchanged (%q) after diff content changed — stale AI analysis would show as current", before.ID)
	}
}

func TestSnapshotPRRejectsNonGitHubRemote(t *testing.T) {
	loader := repositoryLoader{gh: pr.NewGitHub("git@gitlab.com:some/repo.git", &fakeGHRunner{outputs: map[string][]byte{}})}
	if _, err := loader.SnapshotPR(context.Background(), 42); err == nil || !strings.Contains(err.Error(), "github.com") {
		t.Fatalf("err = %v, want github.com rejection", err)
	}
}

func TestSnapshotPRReturnsErrorWhenGHUnconfigured(t *testing.T) {
	loader := repositoryLoader{}
	if _, err := loader.SnapshotPR(context.Background(), 42); err == nil {
		t.Fatal("expected an error when gh reviewer is unconfigured")
	}
}
