package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func testRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "base.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "base.txt")
	runGit(t, dir, "commit", "-m", "base")
	return dir
}

func TestOpenRejectsNonRepository(t *testing.T) {
	_, err := Open(context.Background(), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "git repository") {
		t.Fatalf("error = %v", err)
	}
}

func TestSnapshotWorkingAndStagedModes(t *testing.T) {
	dir := testRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "base.txt"), []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "staged.txt")
	runGit(t, dir, "config", "core.excludesfile", filepath.Join(dir, "ignore"))
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	working, err := r.Snapshot(context.Background(), WorkingTree)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(working.RawDiff, "base.txt") || !strings.Contains(working.RawDiff, "staged.txt") {
		t.Fatalf("working diff = %s", working.RawDiff)
	}
	staged, err := r.Snapshot(context.Background(), Staged)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(staged.RawDiff, "three") || !strings.Contains(staged.RawDiff, "staged.txt") {
		t.Fatalf("staged diff = %s", staged.RawDiff)
	}
	if working.ID == staged.ID {
		t.Fatal("different modes share snapshot ID")
	}
}

func TestSnapshotIncludesSmallUntrackedAndExcludesIgnored(t *testing.T) {
	dir := testRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("ignore\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("ignored.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := r.Snapshot(context.Background(), WorkingTree)
	if err != nil {
		t.Fatal(err)
	}
	paths := make(map[string]bool, len(snapshot.Files))
	for _, file := range snapshot.Files {
		paths[file.Path] = true
	}
	if !paths["new.txt"] || paths["ignored.txt"] {
		t.Fatalf("untracked paths = %v, diff = %s", paths, snapshot.RawDiff)
	}
}

func TestSnapshotBranchMode(t *testing.T) {
	dir := testRepo(t)
	runGit(t, dir, "checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(dir, "base.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "base.txt")
	runGit(t, dir, "commit", "-m", "feature")
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := r.Snapshot(context.Background(), Branch)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Base != "main...HEAD" || !strings.Contains(snapshot.RawDiff, "feature") {
		t.Fatalf("branch snapshot = %+v", snapshot)
	}
}

type fakeRunner struct {
	outputs map[string][]byte
}

func (f fakeRunner) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	key := strings.Join(args, " ")
	if output, ok := f.outputs[key]; ok {
		return output, nil
	}
	return nil, fmt.Errorf("missing fake command %q", key)
}

func TestDefaultBranchUsesRemoteHeadThenCandidates(t *testing.T) {
	r := Repository{Root: "/repo", runner: fakeRunner{outputs: map[string][]byte{
		"-C /repo symbolic-ref --quiet --short refs/remotes/origin/HEAD": []byte("origin/main\n"),
	}}}
	branch, err := r.DefaultBranch(context.Background())
	if err != nil || branch != "origin/main" {
		t.Fatalf("branch = %q, err = %v", branch, err)
	}
}
