package git

import (
	"context"
	"fmt"
	"io"
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

func TestExecRunnerRunWithStdinPipesInput(t *testing.T) {
	output, err := ExecRunner{}.RunWithStdin(context.Background(), strings.NewReader("hello\n"), "cat")
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "hello\n" {
		t.Fatalf("output = %q", output)
	}
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

func (f fakeRunner) RunWithStdin(_ context.Context, _ io.Reader, _ string, args ...string) ([]byte, error) {
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

func TestDefaultBranchChecksOriginMasterBeforeMaster(t *testing.T) {
	r := Repository{Root: "/repo", runner: fakeRunner{outputs: map[string][]byte{
		"-C /repo rev-parse --verify origin/master": []byte("abc123\n"),
		"-C /repo rev-parse --verify master":        []byte("def456\n"),
	}}}
	branch, err := r.DefaultBranch(context.Background())
	if err != nil || branch != "origin/master" {
		t.Fatalf("branch = %q, err = %v", branch, err)
	}
}

func TestCurrentBranchTrimsOutput(t *testing.T) {
	r := Repository{Root: "/repo", runner: fakeRunner{outputs: map[string][]byte{
		"-C /repo rev-parse --abbrev-ref HEAD": []byte("feature/869d6rn69-thing\n"),
	}}}
	branch, err := r.CurrentBranch(context.Background())
	if err != nil || branch != "feature/869d6rn69-thing" {
		t.Fatalf("branch = %q, err = %v", branch, err)
	}
}

func TestRemoteURLTrimsOutput(t *testing.T) {
	r := Repository{Root: "/repo", runner: fakeRunner{outputs: map[string][]byte{
		"-C /repo remote get-url origin": []byte("git@github.com:alex-irvine/lazydiff.git\n"),
	}}}
	url, err := r.RemoteURL(context.Background(), "origin")
	if err != nil || url != "git@github.com:alex-irvine/lazydiff.git" {
		t.Fatalf("url = %q, err = %v", url, err)
	}
}

func TestRemoteURLWrapsFailure(t *testing.T) {
	r := Repository{Root: "/repo", runner: fakeRunner{outputs: map[string][]byte{}}}
	if _, err := r.RemoteURL(context.Background(), "origin"); err == nil {
		t.Fatal("expected error for missing remote")
	}
}

func TestBranchesListsLocalBranches(t *testing.T) {
	dir := testRepo(t)
	runGit(t, dir, "checkout", "-b", "feature-a")
	runGit(t, dir, "checkout", "-b", "feature-b")
	runGit(t, dir, "checkout", "main")
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	branches, err := r.Branches(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(branches) != 3 {
		t.Fatalf("expected 3 branches, got %v", branches)
	}
}

func TestBranchesReturnsErrorOnGitFailure(t *testing.T) {
	r := Repository{Root: "/bad", runner: fakeRunner{outputs: map[string][]byte{}}}
	_, err := r.Branches(context.Background())
	if err == nil {
		t.Fatal("expected error for missing repository")
	}
}

func TestBranchesViaFakeRunner(t *testing.T) {
	r := Repository{Root: "/repo", runner: fakeRunner{outputs: map[string][]byte{
		"-C /repo branch --format=%(refname:short)": []byte("main\nfeature-a\nfeature-b\n"),
	}}}
	branches, err := r.Branches(context.Background())
	if err != nil || len(branches) != 3 || branches[0] != "main" {
		t.Fatalf("branches = %v, err = %v", branches, err)
	}
}

func TestSnapshotBranchDiffAgainstDefault(t *testing.T) {
	dir := testRepo(t)
	runGit(t, dir, "checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "feature.txt")
	runGit(t, dir, "commit", "-m", "feature")
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := r.SnapshotBranch(context.Background(), "feature")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(snapshot.RawDiff, "feature.txt") {
		t.Fatalf("branch snapshot should include feature.txt:\n%s", snapshot.RawDiff)
	}
	if !strings.Contains(snapshot.Base, "main...feature") {
		t.Fatalf("expected base main...feature, got %q", snapshot.Base)
	}
}

func TestSnapshotBranchViaFakeRunner(t *testing.T) {
	r := Repository{Root: "/repo", runner: fakeRunner{outputs: map[string][]byte{
		"-C /repo symbolic-ref --quiet --short refs/remotes/origin/HEAD": []byte("origin/main\n"),
		"-C /repo diff --no-color --binary origin/main...feature":        []byte("diff --git a/x b/x\n@@ -1 +1 @@\n-old\n+new\n"),
	}}}
	snapshot, err := r.SnapshotBranch(context.Background(), "feature")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Base != "origin/main...feature" {
		t.Fatalf("base = %q", snapshot.Base)
	}
	if len(snapshot.Files) == 0 {
		t.Fatal("expected parsed files")
	}
}

func TestWorktreesListsBranchesWithPaths(t *testing.T) {
	dir := testRepo(t)
	runGit(t, dir, "checkout", "-b", "wt-branch")
	runGit(t, dir, "checkout", "main")
	wtDir := filepath.Join(t.TempDir(), "wt")
	runGit(t, dir, "worktree", "add", wtDir, "wt-branch")
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	worktrees, err := r.Worktrees(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	path, ok := worktrees["wt-branch"]
	if !ok {
		t.Fatalf("expected worktree for wt-branch, got %v", worktrees)
	}
	if path != wtDir {
		t.Fatalf("worktree path = %q, expected %s", path, wtDir)
	}
}

func TestWorktreesViaFakeRunner(t *testing.T) {
	output := "worktree /wt/feature\nHEAD abc123\nbranch refs/heads/feature\n\nworktree /repo\nHEAD def456\nbranch refs/heads/main\n"
	r := Repository{Root: "/repo", runner: fakeRunner{outputs: map[string][]byte{
		"-C /repo worktree list --porcelain": []byte(output),
	}}}
	worktrees, err := r.Worktrees(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(worktrees) != 2 {
		t.Fatalf("expected 2 worktrees, got %v", worktrees)
	}
	if worktrees["feature"] != "/wt/feature" {
		t.Fatalf("feature path = %q", worktrees["feature"])
	}
	if worktrees["main"] != "/repo" {
		t.Fatalf("main path = %q", worktrees["main"])
	}
}

func TestWorktreeSnapshotRunsDiffFromWorktreeDir(t *testing.T) {
	dir := testRepo(t)
	runGit(t, dir, "checkout", "-b", "wt-branch")
	runGit(t, dir, "checkout", "main")
	wtDir := filepath.Join(t.TempDir(), "wt")
	runGit(t, dir, "worktree", "add", wtDir, "wt-branch")
	if err := os.WriteFile(filepath.Join(wtDir, "wt.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, wtDir, "add", "wt.txt")
	runGit(t, wtDir, "commit", "-m", "add wt.txt")
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := r.WorktreeSnapshot(context.Background(), wtDir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(snapshot.RawDiff, "wt.txt") {
		t.Fatalf("expected wt.txt in diff:\n%s", snapshot.RawDiff)
	}
}

func TestWorktreeSnapshotExcludesMainDrift(t *testing.T) {
	dir := testRepo(t)
	runGit(t, dir, "checkout", "-b", "wt-branch")
	runGit(t, dir, "checkout", "main")
	wtDir := filepath.Join(t.TempDir(), "wt")
	runGit(t, dir, "worktree", "add", wtDir, "wt-branch")

	// Commit on wt-branch inside the worktree
	if err := os.WriteFile(filepath.Join(wtDir, "wt.txt"), []byte("hello from wt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, wtDir, "add", "wt.txt")
	runGit(t, wtDir, "commit", "-m", "add wt.txt")

	// Commit on main in main repo after branch was created
	if err := os.WriteFile(filepath.Join(dir, "main-drift.txt"), []byte("drift on main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "main-drift.txt")
	runGit(t, dir, "commit", "-m", "add main-drift.txt on main")

	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := r.WorktreeSnapshot(context.Background(), wtDir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(snapshot.RawDiff, "wt.txt") {
		t.Fatalf("expected wt.txt in diff:\n%s", snapshot.RawDiff)
	}
	if strings.Contains(snapshot.RawDiff, "main-drift.txt") {
		t.Fatalf("expected diff to NOT contain reverse diff of main-drift.txt, got:\n%s", snapshot.RawDiff)
	}
}

func TestWorktreeSnapshotIncludesUntrackedInWorktree(t *testing.T) {
	dir := testRepo(t)
	runGit(t, dir, "checkout", "-b", "wt-branch")
	runGit(t, dir, "checkout", "main")
	wtDir := filepath.Join(t.TempDir(), "wt")
	runGit(t, dir, "worktree", "add", wtDir, "wt-branch")

	if err := os.WriteFile(filepath.Join(wtDir, "untracked.txt"), []byte("new untracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := r.WorktreeSnapshot(context.Background(), wtDir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(snapshot.RawDiff, "untracked.txt") {
		t.Fatalf("expected untracked.txt in diff:\n%s", snapshot.RawDiff)
	}
}

func TestWorktreesEmptyOutput(t *testing.T) {
	r := Repository{Root: "/repo", runner: fakeRunner{outputs: map[string][]byte{
		"-C /repo worktree list --porcelain": []byte("worktree /repo\nHEAD abc123\nbranch refs/heads/main\n"),
	}}}
	worktrees, err := r.Worktrees(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(worktrees) != 1 {
		t.Fatalf("expected 1 worktree, got %v", worktrees)
	}
}
