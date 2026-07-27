# lazydiff Stage / Commit / PR Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a user check files/hunks in lazydiff's tree, stage them, get an AI-written commit message (with a ClickUp ticket trailer parsed from the branch name), commit, and later push + open a browser at a pre-filled GitHub PR form (title re-prefixed `CU-<ticket>:`).

**Architecture:** New git mutation methods (`add`/`apply --cached`/`commit`/`push`, all via `os/exec`, no shell). A `Mutator` interface (mirroring the existing `SnapshotLoader` seam) lets `ui` package fake these in tests. A new tri-state checkbox model on the existing file/hunk tree drives a `StagingPlan`. A new `ActionDialog` sub-model (wrapping `bubbles/textarea`) shows AI-generated text for edit/confirm before anything mutates. A new `pr` package handles ticket-regex extraction, GitHub compare-URL construction, and opening it in a browser — no `gh` CLI, no GitHub API.

**Tech Stack:** Go 1.24, Bubble Tea/Lip Gloss (existing), new dependency `github.com/charmbracelet/bubbles` (textarea only).

**Spec:** `docs/superpowers/specs/2026-07-27-lazydiff-stage-commit-pr-design.md` — read it first; this plan does not repeat its rationale, only its concrete shape.

## Global Constraints

- Go 1.24.2, stdlib `testing` only — no testify, no other test framework.
- No shell interpolation for any subprocess: always `exec.CommandContext(ctx, name, args...)` with args as a slice, never a shell string.
- The AI agent subprocess stays read-only always — it only ever produces text; lazydiff's own Go code performs every git mutation.
- PR creation is GitHub-only (`github.com` host validated) and never uses `gh` CLI or the GitHub API — only `git push` plus opening a URL in the user's browser.
- Every new exported type/function needs a doc comment if it doesn't already have an obvious one-line purpose from its name (match the terse style already used in this codebase — most existing functions have zero or one-line comments).
- Verification gate for every task and for the plan as a whole: `go test ./... -count=1 && go vet ./... && go build ./...` must pass from the repo root before moving on.
- Bubbletea/bubbles API calls in this plan (`textarea.New`, `.Focus`, `.SetValue`, `.Value`, `tea.KeyCtrlS`, `tea.KeyCtrlR`, `tea.KeyCtrlA`, `tea.KeyEsc`) are written from strong but not 100%-certain recollection of these libraries' exact surface. If any of them don't compile as written, run `go doc github.com/charmbracelet/bubbles/textarea` / `go doc github.com/charmbracelet/bubbletea` to find the real name and adjust — the *behavior* described (focus a multi-line editable box, detect esc/ctrl+s/ctrl+r, read/write its full text value) is what matters, not the exact identifier.

---

### Task 1: `RunWithStdin` on `CommandRunner`

**Files:**
- Modify: `git/repository.go` (add `RunWithStdin` to the `CommandRunner` interface, implement on `execRunner`, add a `Repository.runWithStdin` helper)
- Modify: `git/repository_test.go` (the existing `fakeRunner` must implement the new interface method or the package stops compiling)
- Test: `git/repository_test.go` (new test for `execRunner.RunWithStdin`)

**Interfaces:**
- Consumes: nothing new.
- Produces: `CommandRunner.RunWithStdin(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error)`; `Repository.runWithStdin(ctx context.Context, stdin io.Reader, args ...string) ([]byte, error)` (unexported, mirrors existing `run`). Tasks 4 and 5 depend on `runWithStdin`.

- [ ] **Step 1: Write the failing test**

Add to `git/repository_test.go` (add `"io"` and `"strings"` are already imported; `"strings"` already is, add `"io"`):

```go
func TestExecRunnerRunWithStdinPipesInput(t *testing.T) {
	output, err := execRunner{}.RunWithStdin(context.Background(), strings.NewReader("hello\n"), "cat")
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "hello\n" {
		t.Fatalf("output = %q", output)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./git/... -run TestExecRunnerRunWithStdinPipesInput -v`
Expected: FAIL — `execRunner` has no method `RunWithStdin` (compile error).

- [ ] **Step 3: Implement `RunWithStdin`**

In `git/repository.go`, add `"io"` to the import block, then change:

```go
type CommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}
```

to:

```go
type CommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
	RunWithStdin(context.Context, io.Reader, string, ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}

func (execRunner) RunWithStdin(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = stdin
	return cmd.Output()
}
```

Then add a `runWithStdin` helper right after the existing `run` method:

```go
func (r Repository) runWithStdin(ctx context.Context, stdin io.Reader, args ...string) ([]byte, error) {
	if r.runner == nil {
		r.runner = execRunner{}
	}
	return r.runner.RunWithStdin(ctx, stdin, "git", append([]string{"-C", r.Root}, args...)...)
}
```

- [ ] **Step 4: Fix the existing `fakeRunner` so the package still compiles**

In `git/repository_test.go`, add `"io"` to imports, then add a method next to the existing `fakeRunner.Run`:

```go
func (f fakeRunner) RunWithStdin(_ context.Context, _ io.Reader, _ string, args ...string) ([]byte, error) {
	key := strings.Join(args, " ")
	if output, ok := f.outputs[key]; ok {
		return output, nil
	}
	return nil, fmt.Errorf("missing fake command %q", key)
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./git/... -run TestExecRunnerRunWithStdinPipesInput -v`
Expected: PASS

- [ ] **Step 6: Run full package test to confirm nothing else broke**

Run: `go test ./git/... -v`
Expected: all PASS (existing `TestDefaultBranchUsesRemoteHeadThenCandidates` etc. still pass since `fakeRunner` still satisfies `CommandRunner`)

- [ ] **Step 7: Commit**

```bash
git add git/repository.go git/repository_test.go
git commit -m "feat(git): add stdin support to CommandRunner"
```

---

### Task 2: `CurrentBranch` and `RemoteURL`

**Files:**
- Modify: `git/repository.go` (two new read-only methods, next to `DefaultBranch`)
- Test: `git/repository_test.go`

**Interfaces:**
- Consumes: `Repository.run` (existing).
- Produces: `Repository.CurrentBranch(ctx context.Context) (string, error)`; `Repository.RemoteURL(ctx context.Context, remote string) (string, error)`. Later PR-flow tasks depend on both.

- [ ] **Step 1: Write the failing tests**

Add to `git/repository_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./git/... -run 'TestCurrentBranchTrimsOutput|TestRemoteURL' -v`
Expected: FAIL — no method `CurrentBranch`/`RemoteURL` (compile error).

- [ ] **Step 3: Implement**

In `git/repository.go`, add after `DefaultBranch`:

```go
func (r Repository) CurrentBranch(ctx context.Context) (string, error) {
	output, err := r.run(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("resolve current branch: %w", err)
	}
	branch := strings.TrimSpace(string(output))
	if branch == "" {
		return "", fmt.Errorf("current branch is empty")
	}
	return branch, nil
}

func (r Repository) RemoteURL(ctx context.Context, remote string) (string, error) {
	output, err := r.run(ctx, "remote", "get-url", remote)
	if err != nil {
		return "", fmt.Errorf("resolve remote %q url: %w", remote, err)
	}
	url := strings.TrimSpace(string(output))
	if url == "" {
		return "", fmt.Errorf("remote %q url is empty", remote)
	}
	return url, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./git/... -run 'TestCurrentBranchTrimsOutput|TestRemoteURL' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add git/repository.go git/repository_test.go
git commit -m "feat(git): add CurrentBranch and RemoteURL"
```

---

### Task 3: `StageFile`

**Files:**
- Create: `git/mutate.go`
- Create: `git/mutate_test.go`

**Interfaces:**
- Consumes: `Repository.run` (existing, unexported — `mutate.go` is in the same package so this is fine).
- Produces: `Repository.StageFile(ctx context.Context, oldPath, path string) error`. Consumed by the `ui` package's `StagingPlan` execution in Task 18.

- [ ] **Step 1: Write the failing tests**

Create `git/mutate_test.go`:

```go
package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alex-irvine/lazydiff/diff"
)

func TestStageFileStagesModifiedFile(t *testing.T) {
	dir := testRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "base.txt"), []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.StageFile(context.Background(), "", "base.txt"); err != nil {
		t.Fatal(err)
	}
	staged, err := r.Snapshot(context.Background(), Staged)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(staged.RawDiff, "three") {
		t.Fatalf("staged diff = %s", staged.RawDiff)
	}
}

func TestStageFileStagesDeletion(t *testing.T) {
	dir := testRepo(t)
	if err := os.Remove(filepath.Join(dir, "base.txt")); err != nil {
		t.Fatal(err)
	}
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.StageFile(context.Background(), "", "base.txt"); err != nil {
		t.Fatal(err)
	}
	staged, err := r.Snapshot(context.Background(), Staged)
	if err != nil {
		t.Fatal(err)
	}
	if len(staged.Files) != 1 || staged.Files[0].Status != diff.Deleted {
		t.Fatalf("staged files = %+v", staged.Files)
	}
}

func TestStageFileStagesRenameAcrossBothPaths(t *testing.T) {
	dir := testRepo(t)
	if err := os.Rename(filepath.Join(dir, "base.txt"), filepath.Join(dir, "renamed.txt")); err != nil {
		t.Fatal(err)
	}
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.StageFile(context.Background(), "base.txt", "renamed.txt"); err != nil {
		t.Fatal(err)
	}
	staged, err := r.Snapshot(context.Background(), Staged)
	if err != nil {
		t.Fatal(err)
	}
	if len(staged.Files) != 1 || staged.Files[0].Status != diff.Renamed {
		t.Fatalf("staged files = %+v", staged.Files)
	}
}

func TestStageFileDedupesEmptyAndEqualPaths(t *testing.T) {
	dir := testRepo(t)
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.StageFile(context.Background(), "", ""); err == nil {
		t.Fatal("expected error when both paths are empty")
	}
}
```

This uses `testRepo`/`runGit` helpers already defined in `git/repository_test.go` — same package, no import needed for those.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./git/... -run TestStageFile -v`
Expected: FAIL — no method `StageFile` (compile error).

- [ ] **Step 3: Implement**

Create `git/mutate.go`:

```go
package git

import (
	"context"
	"fmt"
)

// StageFile stages a whole file via `git add -A`, which handles
// modifications, additions, deletions, and renames uniformly. Pass both
// oldPath and path for a rename (empty/duplicate paths are deduped).
func (r Repository) StageFile(ctx context.Context, oldPath, path string) error {
	paths := uniqueNonEmpty(oldPath, path)
	if len(paths) == 0 {
		return fmt.Errorf("stage file: no path given")
	}
	args := append([]string{"add", "-A", "--"}, paths...)
	if _, err := r.run(ctx, args...); err != nil {
		return fmt.Errorf("stage %v: %w", paths, err)
	}
	return nil
}

func uniqueNonEmpty(values ...string) []string {
	seen := make(map[string]bool, len(values))
	var result []string
	for _, v := range values {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		result = append(result, v)
	}
	return result
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./git/... -run TestStageFile -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add git/mutate.go git/mutate_test.go
git commit -m "feat(git): add StageFile"
```

---

### Task 4: `StagePatch`

**Files:**
- Modify: `git/mutate.go`
- Modify: `git/mutate_test.go`

**Interfaces:**
- Consumes: `Repository.runWithStdin` (Task 1).
- Produces: `Repository.StagePatch(ctx context.Context, patch string) error`. Consumed by `ui`'s `StagingPlan` execution (Task 18) together with `diff.BuildPatch` (Task 7).

- [ ] **Step 1: Write the failing test**

Add to `git/mutate_test.go` (add `"strings"` already imported above):

```go
func TestStagePatchAppliesToIndexOnly(t *testing.T) {
	dir := testRepo(t)
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	patch := "diff --git a/base.txt b/base.txt\n" +
		"--- a/base.txt\n" +
		"+++ b/base.txt\n" +
		"@@ -1,2 +1,3 @@\n" +
		" one\n" +
		" two\n" +
		"+three\n"
	if err := r.StagePatch(context.Background(), patch); err != nil {
		t.Fatal(err)
	}
	staged, err := r.Snapshot(context.Background(), Staged)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(staged.RawDiff, "+three") {
		t.Fatalf("staged diff = %s", staged.RawDiff)
	}
}

func TestStagePatchRejectsInvalidPatch(t *testing.T) {
	dir := testRepo(t)
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.StagePatch(context.Background(), "not a patch"); err == nil {
		t.Fatal("expected error for invalid patch")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./git/... -run TestStagePatch -v`
Expected: FAIL — no method `StagePatch` (compile error).

- [ ] **Step 3: Implement**

Add to `git/mutate.go` (add `"strings"` to imports):

```go
// StagePatch applies patch to the index only (git apply --cached), leaving
// the working tree untouched. patch must be a valid unified diff for
// exactly one file, e.g. built by diff.BuildPatch.
func (r Repository) StagePatch(ctx context.Context, patch string) error {
	if _, err := r.runWithStdin(ctx, strings.NewReader(patch), "apply", "--cached"); err != nil {
		return fmt.Errorf("apply patch: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./git/... -run TestStagePatch -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add git/mutate.go git/mutate_test.go
git commit -m "feat(git): add StagePatch"
```

---

### Task 5: `Commit`

**Files:**
- Modify: `git/mutate.go`
- Modify: `git/mutate_test.go`

**Interfaces:**
- Consumes: `Repository.runWithStdin` (Task 1).
- Produces: `Repository.Commit(ctx context.Context, message string) error`. Consumed by `ui`'s commit-confirm flow (Task 19).

- [ ] **Step 1: Write the failing tests**

Add to `git/mutate_test.go`:

```go
func TestCommitCreatesCommitFromStagedChanges(t *testing.T) {
	dir := testRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.StageFile(context.Background(), "", "new.txt"); err != nil {
		t.Fatal(err)
	}
	if err := r.Commit(context.Background(), "subject line\n\nbody line\n\nCU-869d6rn69"); err != nil {
		t.Fatal(err)
	}
	log := runGit(t, dir, "log", "-1", "--pretty=%B")
	if !strings.Contains(log, "subject line") || !strings.Contains(log, "CU-869d6rn69") {
		t.Fatalf("commit message = %q", log)
	}
	working, err := r.Snapshot(context.Background(), WorkingTree)
	if err != nil {
		t.Fatal(err)
	}
	if len(working.Files) != 0 {
		t.Fatalf("expected clean working tree after commit, got %+v", working.Files)
	}
}

func TestCommitRejectsEmptyMessage(t *testing.T) {
	dir := testRepo(t)
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Commit(context.Background(), "   "); err == nil {
		t.Fatal("expected error for empty commit message")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./git/... -run TestCommit -v`
Expected: FAIL — no method `Commit` (compile error).

- [ ] **Step 3: Implement**

Add to `git/mutate.go`:

```go
// Commit creates a commit from whatever is currently staged, with message
// piped to `git commit --file -` (avoids shell-escaping issues with
// multi-line messages).
func (r Repository) Commit(ctx context.Context, message string) error {
	if strings.TrimSpace(message) == "" {
		return fmt.Errorf("commit message must not be empty")
	}
	if _, err := r.runWithStdin(ctx, strings.NewReader(message), "commit", "--file", "-"); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./git/... -run TestCommit -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add git/mutate.go git/mutate_test.go
git commit -m "feat(git): add Commit"
```

---

### Task 6: `Push`

**Files:**
- Modify: `git/mutate.go`
- Modify: `git/mutate_test.go`

**Interfaces:**
- Consumes: `Repository.run` (existing).
- Produces: `Repository.Push(ctx context.Context, remote, branch string) error`. Consumed by `ui`'s PR-confirm flow (Task 21).

- [ ] **Step 1: Write the failing tests**

Add to `git/mutate_test.go`:

```go
func TestPushInvokesUpstreamPush(t *testing.T) {
	r := Repository{Root: "/repo", runner: fakeRunner{outputs: map[string][]byte{
		"-C /repo push -u origin feature/869d6rn69-thing": {},
	}}}
	if err := r.Push(context.Background(), "origin", "feature/869d6rn69-thing"); err != nil {
		t.Fatal(err)
	}
}

func TestPushWrapsFailure(t *testing.T) {
	r := Repository{Root: "/repo", runner: fakeRunner{outputs: map[string][]byte{}}}
	if err := r.Push(context.Background(), "origin", "main"); err == nil || !strings.Contains(err.Error(), "push") {
		t.Fatalf("err = %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./git/... -run TestPush -v`
Expected: FAIL — no method `Push` (compile error).

- [ ] **Step 3: Implement**

Add to `git/mutate.go`:

```go
// Push always passes -u; harmless when upstream already exists, so no
// separate upstream-detection step is needed.
func (r Repository) Push(ctx context.Context, remote, branch string) error {
	if _, err := r.run(ctx, "push", "-u", remote, branch); err != nil {
		return fmt.Errorf("push %s %s: %w", remote, branch, err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./git/... -run TestPush -v`
Expected: PASS

- [ ] **Step 5: Run the whole `git` package once more**

Run: `go test ./git/... -v -count=1`
Expected: all PASS (Tasks 1-6 combined)

- [ ] **Step 6: Commit**

```bash
git add git/mutate.go git/mutate_test.go
git commit -m "feat(git): add Push"
```

---

### Task 7: `diff.BuildPatch`

**Files:**
- Create: `diff/patch.go`
- Create: `diff/patch_test.go`

**Interfaces:**
- Consumes: `File`, `Hunk` (existing, `diff/model.go`); the `fixture` const already defined in `diff/parse_test.go` (same package, visible to the new test file).
- Produces: `diff.BuildPatch(file File, hunks []Hunk) string`. Consumed by `ui`'s `StagingPlan` execution (Task 18) together with `git.StagePatch` (Task 4).

- [ ] **Step 1: Write the failing tests**

Create `diff/patch_test.go`:

```go
package diff

import (
	"strings"
	"testing"
)

func TestBuildPatchWithSubsetOfHunks(t *testing.T) {
	files, err := Parse(fixture)
	if err != nil {
		t.Fatal(err)
	}
	file := files[0] // agent/runner.go, 2 hunks
	patch := BuildPatch(file, file.Hunks[1:])
	if strings.Contains(patch, `import "fmt"`) {
		t.Fatalf("patch should omit first hunk: %q", patch)
	}
	if !strings.Contains(patch, `fmt.Println("run")`) {
		t.Fatalf("patch should include second hunk: %q", patch)
	}
	if !strings.HasPrefix(patch, "diff --git a/agent/runner.go") {
		t.Fatalf("patch missing preamble: %q", patch)
	}
	if !strings.Contains(patch, "--- a/agent/runner.go") || !strings.Contains(patch, "+++ b/agent/runner.go") {
		t.Fatalf("patch missing file headers: %q", patch)
	}
}

func TestBuildPatchAllHunksReturnsFullRaw(t *testing.T) {
	files, err := Parse(fixture)
	if err != nil {
		t.Fatal(err)
	}
	file := files[0]
	if got := BuildPatch(file, file.Hunks); got != file.Raw {
		t.Fatal("expected full raw diff when all hunks selected")
	}
}

func TestBuildPatchNoHunksReturnsFullRaw(t *testing.T) {
	files, err := Parse(fixture)
	if err != nil {
		t.Fatal(err)
	}
	file := files[0]
	if got := BuildPatch(file, nil); got != file.Raw {
		t.Fatal("expected full raw diff when no hunks given")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./diff/... -run TestBuildPatch -v`
Expected: FAIL — no function `BuildPatch` (compile error).

- [ ] **Step 3: Implement**

Create `diff/patch.go`:

```go
package diff

import "strings"

// BuildPatch reconstructs a minimal valid unified diff patch for file,
// containing only the given subset of its hunks. Passing all of file's
// hunks (or none) returns the file's full raw diff unchanged. This is the
// same technique `git add -p` uses internally: the file's existing preamble
// (diff/mode/rename headers, ---/+++ lines) plus only the selected hunks'
// raw text.
func BuildPatch(file File, hunks []Hunk) string {
	if len(hunks) == 0 || len(hunks) == len(file.Hunks) {
		return file.Raw
	}
	preamble := file.Raw
	if len(file.Hunks) > 0 {
		if idx := strings.Index(file.Raw, file.Hunks[0].Raw); idx >= 0 {
			preamble = file.Raw[:idx]
		}
	}
	var patch strings.Builder
	patch.WriteString(preamble)
	for _, hunk := range hunks {
		patch.WriteString(hunk.Raw)
	}
	return patch.String()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./diff/... -v -count=1`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add diff/patch.go diff/patch_test.go
git commit -m "feat(diff): add BuildPatch for partial-hunk staging"
```

---

### Task 8: Config additions — `[pr]` section and two new prompt fields

**Files:**
- Modify: `config/config.go`
- Modify: `config/config_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `Config.PR.TicketPattern string`; `Config.Agent.Prompts.CommitMessage string`; `Config.Agent.Prompts.PRDescription string`. Consumed by Task 9 (`prompt` package) and Task 18/20 (`ui` commit/PR flows, which read `cfg.PR.TicketPattern` and the two prompt strings via `cfg.Agent.Prompts`).

- [ ] **Step 1: Write the failing tests**

Add to `config/config_test.go` (add `"regexp"` to imports):

```go
func TestLoadDefaultsIncludePRTicketPattern(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatal(err)
	}
	re, err := regexp.Compile(cfg.PR.TicketPattern)
	if err != nil {
		t.Fatalf("default ticket_pattern does not compile: %v", err)
	}
	match := re.FindStringSubmatch("feature/869d6rn69-add-login")
	if len(match) != 2 || match[1] != "869d6rn69" {
		t.Fatalf("default ticket_pattern match = %v", match)
	}
	if !strings.Contains(cfg.Agent.Prompts.CommitMessage, "{{staged_diff}}") {
		t.Fatal("default commit_message prompt missing staged_diff placeholder")
	}
	if !strings.Contains(cfg.Agent.Prompts.PRDescription, "{{branch_diff}}") {
		t.Fatal("default pr_description prompt missing branch_diff placeholder")
	}
}

func TestLoadOverlaysPRSection(t *testing.T) {
	path := writeConfig(t, `[pr]
ticket_pattern = "[A-Z]+-\\d+"

[agent.prompts]
commit_message = "{{staged_diff}}"
pr_description = "{{branch_diff}}"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PR.TicketPattern != `[A-Z]+-\d+` {
		t.Fatalf("ticket_pattern = %q", cfg.PR.TicketPattern)
	}
	if cfg.Agent.Prompts.CommitMessage != "{{staged_diff}}" || cfg.Agent.Prompts.PRDescription != "{{branch_diff}}" {
		t.Fatalf("prompts = %+v", cfg.Agent.Prompts)
	}
}

func TestLoadRejectsInvalidTicketPattern(t *testing.T) {
	path := writeConfig(t, `[pr]
ticket_pattern = "("
`)
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "ticket_pattern") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRejectsCommitMessageMissingRequiredPlaceholder(t *testing.T) {
	path := writeConfig(t, `[agent.prompts]
commit_message = "no placeholder here"
`)
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "staged_diff") {
		t.Fatalf("error = %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./config/... -run 'TestLoadDefaultsIncludePRTicketPattern|TestLoadOverlaysPRSection|TestLoadRejectsInvalidTicketPattern|TestLoadRejectsCommitMessageMissingRequiredPlaceholder' -v`
Expected: FAIL — `Config` has no field `PR` (compile error).

- [ ] **Step 3: Implement**

In `config/config.go`, change the `Config`/`AgentConfig`/`PromptConfig`/`fileConfig`/`fileAgentConfig`/`filePromptConfig` structs:

```go
type Config struct {
	Agent AgentConfig
	PR    PRConfig
}

type AgentConfig struct {
	Provider           string
	Command            string
	Args               []string
	ReadOnly           bool
	AllowExternalTools bool
	Prompts            PromptConfig
}

type PromptConfig struct {
	Overall       string
	Detail        string
	CommitMessage string
	PRDescription string
}

type PRConfig struct {
	TicketPattern string
}

type fileConfig struct {
	Agent fileAgentConfig `toml:"agent"`
	PR    filePRConfig    `toml:"pr"`
}

type fileAgentConfig struct {
	Provider           *string          `toml:"provider"`
	Command            *string          `toml:"command"`
	Args               *[]string        `toml:"args"`
	ReadOnly           *bool            `toml:"read_only"`
	AllowExternalTools *bool            `toml:"allow_external_tools"`
	Prompts            filePromptConfig `toml:"prompts"`
}

type filePromptConfig struct {
	Overall       *string `toml:"overall"`
	Detail        *string `toml:"detail"`
	CommitMessage *string `toml:"commit_message"`
	PRDescription *string `toml:"pr_description"`
}

type filePRConfig struct {
	TicketPattern *string `toml:"ticket_pattern"`
}
```

Update `allowedPlaceholders`:

```go
var allowedPlaceholders = map[string]struct{}{
	"repository":    {},
	"mode":          {},
	"overall_diff":  {},
	"selection":     {},
	"selected_diff": {},
	"staged_diff":   {},
	"branch_diff":   {},
	"ticket":        {},
	"branch":        {},
	"base_branch":   {},
}
```

Update `Default()`:

```go
func Default() Config {
	return Config{
		Agent: AgentConfig{
			Provider:           "generic",
			Command:            "claude",
			Args:               []string{"--model", "haiku-latest"},
			ReadOnly:           true,
			AllowExternalTools: false,
			Prompts: PromptConfig{
				Overall:       defaultOverallPrompt,
				Detail:        defaultDetailPrompt,
				CommitMessage: defaultCommitMessagePrompt,
				PRDescription: defaultPRDescriptionPrompt,
			},
		},
		PR: PRConfig{TicketPattern: defaultTicketPattern},
	}
}
```

Update `Load()` — add after the existing `overlay.Prompts.Detail` block, still inside the same function:

```go
	if overlay.Prompts.CommitMessage != nil {
		cfg.Agent.Prompts.CommitMessage = *overlay.Prompts.CommitMessage
	}
	if overlay.Prompts.PRDescription != nil {
		cfg.Agent.Prompts.PRDescription = *overlay.Prompts.PRDescription
	}
	if file.PR.TicketPattern != nil {
		cfg.PR.TicketPattern = *file.PR.TicketPattern
	}
```

(`file` is the existing `var file fileConfig` already in scope in `Load()`.)

Update `Validate()`:

```go
func (c Config) Validate() error {
	if c.Agent.Provider != "copilot" && c.Agent.Provider != "generic" && c.Agent.Provider != "claude" {
		return fmt.Errorf("agent provider %q is invalid; use copilot, generic, or claude", c.Agent.Provider)
	}
	if strings.TrimSpace(c.Agent.Command) == "" {
		return errors.New("agent command must not be empty")
	}
	if err := validateTemplate("overall", c.Agent.Prompts.Overall, "overall_diff"); err != nil {
		return err
	}
	if err := validateTemplate("detail", c.Agent.Prompts.Detail, "overall_diff", "selection", "selected_diff"); err != nil {
		return err
	}
	if err := validateTemplate("commit_message", c.Agent.Prompts.CommitMessage, "staged_diff"); err != nil {
		return err
	}
	if err := validateTemplate("pr_description", c.Agent.Prompts.PRDescription, "branch_diff"); err != nil {
		return err
	}
	if _, err := regexp.Compile(c.PR.TicketPattern); err != nil {
		return fmt.Errorf("pr.ticket_pattern is invalid: %w", err)
	}
	return nil
}
```

Add new default consts next to `defaultOverallPrompt`/`defaultDetailPrompt`:

```go
const defaultTicketPattern = `(?:^|[-/_])([0-9a-z]{6,10})(?:[-_]|$)`

const defaultCommitMessagePrompt = `You are writing a Git commit message in read-only mode; you do not run any commands.

Repository: {{repository}}

Staged diff:
{{staged_diff}}

Write a concise commit message: a short subject line (50 characters or fewer, no trailing period), a blank line, then a body explaining what changed and why. Return only the commit message text, nothing else.`

const defaultPRDescriptionPrompt = `You are writing a GitHub pull request title and description in read-only mode; you do not run any commands.

Repository: {{repository}}
Branch: {{branch}}
Base branch: {{base_branch}}

Branch diff:
{{branch_diff}}

Write a concise PR title as the first line (no prefix, no trailing period), then a blank line, then a free-form Markdown description of what changed and why. Return only the title and description text, nothing else.`
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./config/... -v -count=1`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add config/config.go config/config_test.go
git commit -m "feat(config): add [pr] section and commit/PR prompt fields"
```

---

### Task 9: `prompt.Parse` refactor for commit/PR templates

**Files:**
- Modify: `prompt/template.go` (rewrite — see below)
- Modify: `prompt/template_test.go`
- Modify: `cmd/lazydiff/main.go:59` (fix call site)
- Modify: `ui/model_test.go` (fix `newTestModel`'s call site — **only** the `prompt.Parse` call; do not touch `NewModel`'s own argument list yet, that changes in Task 17)

**Interfaces:**
- Consumes: nothing new.
- Produces: `prompt.Sources{Overall, Detail, CommitMessage, PRDescription string}`; `prompt.Parse(Sources) (Templates, error)` (replaces the old two-string-argument signature); `Templates.RenderCommitMessage(Context) (string, error)`; `Templates.RenderPRDescription(Context) (string, error)`; `Context` gains `StagedDiff`, `BranchDiff`, `Ticket`, `Branch`, `BaseBranch` fields. Consumed by Task 18 (`RenderCommitMessage`) and Task 20 (`RenderPRDescription`).

- [ ] **Step 1: Write the failing tests**

Replace the full contents of `prompt/template_test.go`:

```go
package prompt

import (
	"strings"
	"testing"
)

func testSources(overall, detail string) Sources {
	return Sources{
		Overall:       overall,
		Detail:        detail,
		CommitMessage: "{{staged_diff}}",
		PRDescription: "{{branch_diff}}",
	}
}

func TestParseAndRenderTemplates(t *testing.T) {
	templates, err := Parse(testSources(
		"Repo={{repository}} Mode={{mode}}\n{{overall_diff}}",
		"Target={{selection}}\n{{overall_diff}}\nSelected={{selected_diff}}",
	))
	if err != nil {
		t.Fatal(err)
	}
	ctx := Context{
		Repository:   "/tmp/repo",
		Mode:         "working tree / HEAD",
		OverallDiff:  "diff --git a/a.go b/a.go\n{{literal}}\n",
		Selection:    "a.go hunk 1",
		SelectedDiff: "@@ -1 +1 @@\n-old\n+new\n",
	}
	overall, err := templates.RenderOverall(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(overall, "/tmp/repo") || !strings.Contains(overall, "{{literal}}") {
		t.Fatalf("overall = %q", overall)
	}
	detail, err := templates.RenderDetail(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(detail, "a.go hunk 1") || !strings.Contains(detail, "+new") {
		t.Fatalf("detail = %q", detail)
	}
}

func TestParseRejectsUnknownPlaceholder(t *testing.T) {
	_, err := Parse(testSources("{{unknown}} {{overall_diff}}", "{{overall_diff}} {{selection}} {{selected_diff}}"))
	if err == nil || !strings.Contains(err.Error(), "placeholder") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseRejectsMissingRequiredFields(t *testing.T) {
	_, err := Parse(testSources("{{repository}}", "{{overall_diff}} {{selection}} {{selected_diff}}"))
	if err == nil || !strings.Contains(err.Error(), "overall_diff") {
		t.Fatalf("error = %v", err)
	}
	_, err = Parse(testSources("{{overall_diff}}", "{{overall_diff}} {{selection}}"))
	if err == nil || !strings.Contains(err.Error(), "selected_diff") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseRejectsMalformedTemplate(t *testing.T) {
	_, err := Parse(testSources("{{overall_diff}", "{{overall_diff}} {{selection}} {{selected_diff}}"))
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("error = %v", err)
	}
}

func TestRenderCommitMessageAndPRDescription(t *testing.T) {
	templates, err := Parse(Sources{
		Overall:       "{{overall_diff}}",
		Detail:        "{{overall_diff}} {{selection}} {{selected_diff}}",
		CommitMessage: "Ticket={{ticket}}\n{{staged_diff}}",
		PRDescription: "Branch={{branch}} Base={{base_branch}}\n{{branch_diff}}",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := Context{
		StagedDiff: "+new line\n",
		BranchDiff: "+branch change\n",
		Ticket:     "869d6rn69",
		Branch:     "feature/869d6rn69-thing",
		BaseBranch: "main",
	}
	commitMessage, err := templates.RenderCommitMessage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(commitMessage, "Ticket=869d6rn69") || !strings.Contains(commitMessage, "+new line") {
		t.Fatalf("commit message = %q", commitMessage)
	}
	prDescription, err := templates.RenderPRDescription(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prDescription, "Branch=feature/869d6rn69-thing") || !strings.Contains(prDescription, "Base=main") || !strings.Contains(prDescription, "+branch change") {
		t.Fatalf("pr description = %q", prDescription)
	}
}

func TestParseRejectsMissingCommitMessageRequiredField(t *testing.T) {
	_, err := Parse(Sources{
		Overall:       "{{overall_diff}}",
		Detail:        "{{overall_diff}} {{selection}} {{selected_diff}}",
		CommitMessage: "no placeholder here",
		PRDescription: "{{branch_diff}}",
	})
	if err == nil || !strings.Contains(err.Error(), "staged_diff") {
		t.Fatalf("error = %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./prompt/... -v`
Expected: FAIL — `Parse` called with one argument of type `Sources` doesn't match old two-string signature (compile error).

- [ ] **Step 3: Implement — rewrite `prompt/template.go`**

Replace the full contents of `prompt/template.go`:

```go
package prompt

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"text/template"
)

type Context struct {
	Repository   string
	Mode         string
	OverallDiff  string
	Selection    string
	SelectedDiff string
	StagedDiff   string
	BranchDiff   string
	Ticket       string
	Branch       string
	BaseBranch   string
}

// Sources holds the raw (unparsed) template text for every prompt lazydiff
// can render.
type Sources struct {
	Overall       string
	Detail        string
	CommitMessage string
	PRDescription string
}

type Templates struct {
	overall       *template.Template
	detail        *template.Template
	commitMessage *template.Template
	prDescription *template.Template
}

var placeholderPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}`)

var allowedPlaceholders = map[string]struct{}{
	"repository":    {},
	"mode":          {},
	"overall_diff":  {},
	"selection":     {},
	"selected_diff": {},
	"staged_diff":   {},
	"branch_diff":   {},
	"ticket":        {},
	"branch":        {},
	"base_branch":   {},
}

func Parse(sources Sources) (Templates, error) {
	if err := validate("overall", sources.Overall, "overall_diff"); err != nil {
		return Templates{}, err
	}
	if err := validate("detail", sources.Detail, "overall_diff", "selection", "selected_diff"); err != nil {
		return Templates{}, err
	}
	if err := validate("commit_message", sources.CommitMessage, "staged_diff"); err != nil {
		return Templates{}, err
	}
	if err := validate("pr_description", sources.PRDescription, "branch_diff"); err != nil {
		return Templates{}, err
	}
	funcs := template.FuncMap{}
	for name := range allowedPlaceholders {
		funcs[name] = func() string { return "" }
	}
	overallTemplate, err := template.New("overall").Funcs(funcs).Parse(normalizePlaceholders(sources.Overall))
	if err != nil {
		return Templates{}, fmt.Errorf("overall template malformed: %w", err)
	}
	detailTemplate, err := template.New("detail").Funcs(funcs).Parse(normalizePlaceholders(sources.Detail))
	if err != nil {
		return Templates{}, fmt.Errorf("detail template malformed: %w", err)
	}
	commitMessageTemplate, err := template.New("commit_message").Funcs(funcs).Parse(normalizePlaceholders(sources.CommitMessage))
	if err != nil {
		return Templates{}, fmt.Errorf("commit_message template malformed: %w", err)
	}
	prDescriptionTemplate, err := template.New("pr_description").Funcs(funcs).Parse(normalizePlaceholders(sources.PRDescription))
	if err != nil {
		return Templates{}, fmt.Errorf("pr_description template malformed: %w", err)
	}
	return Templates{
		overall:       overallTemplate,
		detail:        detailTemplate,
		commitMessage: commitMessageTemplate,
		prDescription: prDescriptionTemplate,
	}, nil
}

func (t Templates) RenderOverall(ctx Context) (string, error) { return render(t.overall, ctx) }

func (t Templates) RenderDetail(ctx Context) (string, error) { return render(t.detail, ctx) }

func (t Templates) RenderCommitMessage(ctx Context) (string, error) {
	return render(t.commitMessage, ctx)
}

func (t Templates) RenderPRDescription(ctx Context) (string, error) {
	return render(t.prDescription, ctx)
}

func render(t *template.Template, ctx Context) (string, error) {
	if t == nil {
		return "", fmt.Errorf("prompt template is nil")
	}
	var out bytes.Buffer
	if err := t.Execute(&out, map[string]string{
		"repository":    ctx.Repository,
		"mode":          ctx.Mode,
		"overall_diff":  ctx.OverallDiff,
		"selection":     ctx.Selection,
		"selected_diff": ctx.SelectedDiff,
		"staged_diff":   ctx.StagedDiff,
		"branch_diff":   ctx.BranchDiff,
		"ticket":        ctx.Ticket,
		"branch":        ctx.Branch,
		"base_branch":   ctx.BaseBranch,
	}); err != nil {
		return "", fmt.Errorf("render prompt: %w", err)
	}
	return out.String(), nil
}

func validate(name, source string, required ...string) error {
	if strings.TrimSpace(source) == "" {
		return fmt.Errorf("%s template must not be empty", name)
	}
	for _, match := range placeholderPattern.FindAllStringSubmatch(source, -1) {
		if _, ok := allowedPlaceholders[match[1]]; !ok {
			return fmt.Errorf("%s template contains unknown placeholder %q", name, match[1])
		}
	}
	if strings.Contains(source, "{{") || strings.Contains(source, "}}") {
		funcs := template.FuncMap{}
		for placeholder := range allowedPlaceholders {
			funcs[placeholder] = func() string { return "" }
		}
		if _, err := template.New(name).Funcs(funcs).Parse(normalizePlaceholders(source)); err != nil {
			return fmt.Errorf("%s template malformed: %w", name, err)
		}
	}
	for _, requiredPlaceholder := range required {
		found := false
		for _, match := range placeholderPattern.FindAllStringSubmatch(source, -1) {
			if match[1] == requiredPlaceholder {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%s template must include {{%s}}", name, requiredPlaceholder)
		}
	}
	return nil
}

func normalizePlaceholders(source string) string {
	return placeholderPattern.ReplaceAllString(source, "{{.$1}}")
}
```

- [ ] **Step 4: Fix `cmd/lazydiff/main.go:59`**

Change:

```go
	templates, err := prompt.Parse(cfg.Agent.Prompts.Overall, cfg.Agent.Prompts.Detail)
```

to:

```go
	templates, err := prompt.Parse(prompt.Sources{
		Overall:       cfg.Agent.Prompts.Overall,
		Detail:        cfg.Agent.Prompts.Detail,
		CommitMessage: cfg.Agent.Prompts.CommitMessage,
		PRDescription: cfg.Agent.Prompts.PRDescription,
	})
```

- [ ] **Step 5: Fix `ui/model_test.go`'s `newTestModel` helper**

Change only the `prompt.Parse` call inside `newTestModel` (leave the `NewModel(...)` call itself with its current 6 arguments untouched — that changes in Task 17):

```go
func newTestModel(loader SnapshotLoader, runner agent.Runner) Model {
	cfg := config.Default()
	templates, err := prompt.Parse(prompt.Sources{
		Overall:       cfg.Agent.Prompts.Overall,
		Detail:        cfg.Agent.Prompts.Detail,
		CommitMessage: cfg.Agent.Prompts.CommitMessage,
		PRDescription: cfg.Agent.Prompts.PRDescription,
	})
	if err != nil {
		panic(err)
	}
	return NewModel(git.Repository{Root: "/repo"}, cfg, loader, fakeRenderer{}, runner, templates)
}
```

- [ ] **Step 6: Run test to verify it passes, and that the whole repo still builds**

Run: `go build ./... && go test ./... -count=1`
Expected: all PASS/builds — this touches three packages (`prompt`, `cmd/lazydiff`, `ui`), so the full build is the real gate here, not just `./prompt/...`.

- [ ] **Step 7: Commit**

```bash
git add prompt/template.go prompt/template_test.go cmd/lazydiff/main.go ui/model_test.go
git commit -m "feat(prompt): add commit_message and pr_description templates"
```

---

### Task 10: `pr.ExtractTicket` and `pr.FormatTitle`

**Files:**
- Create: `pr/ticket.go`
- Create: `pr/ticket_test.go`

**Interfaces:**
- Consumes: nothing new (this is the first file in a brand-new package).
- Produces: `pr.ExtractTicket(pattern, branch string) (string, error)`; `pr.FormatTitle(ticket, title string) string`. Consumed by Task 18 (commit flow) and Task 20 (PR flow).

- [ ] **Step 1: Write the failing tests**

Create `pr/ticket_test.go`:

```go
package pr

import "testing"

const testDefaultPattern = `(?:^|[-/_])([0-9a-z]{6,10})(?:[-_]|$)`

func TestExtractTicketDefaultPatternMatchesClickUpID(t *testing.T) {
	ticket, err := ExtractTicket(testDefaultPattern, "feature/869d6rn69-add-login")
	if err != nil {
		t.Fatal(err)
	}
	if ticket != "869d6rn69" {
		t.Fatalf("ticket = %q", ticket)
	}
}

func TestExtractTicketAtStartOfBranchNoPrefix(t *testing.T) {
	ticket, err := ExtractTicket(testDefaultPattern, "869d6rn69-fix-bug")
	if err != nil {
		t.Fatal(err)
	}
	if ticket != "869d6rn69" {
		t.Fatalf("ticket = %q", ticket)
	}
}

func TestExtractTicketNoMatchReturnsEmpty(t *testing.T) {
	ticket, err := ExtractTicket(testDefaultPattern, "quick-fix-typo")
	if err != nil {
		t.Fatal(err)
	}
	if ticket != "" {
		t.Fatalf("expected no match, got %q", ticket)
	}
}

func TestExtractTicketJIRAStyleOverridePattern(t *testing.T) {
	ticket, err := ExtractTicket(`[A-Z]+-\d+`, "feature/ENG-1234-add-login")
	if err != nil {
		t.Fatal(err)
	}
	if ticket != "ENG-1234" {
		t.Fatalf("ticket = %q", ticket)
	}
}

func TestExtractTicketInvalidPattern(t *testing.T) {
	if _, err := ExtractTicket("(", "branch"); err == nil {
		t.Fatal("expected error for invalid pattern")
	}
}

func TestFormatTitleWithTicket(t *testing.T) {
	if got, want := FormatTitle("869d6rn69", "Add OAuth login"), "CU-869d6rn69: Add OAuth login"; got != want {
		t.Fatalf("title = %q, want %q", got, want)
	}
}

func TestFormatTitleWithoutTicket(t *testing.T) {
	if got, want := FormatTitle("", "Add OAuth login"), "Add OAuth login"; got != want {
		t.Fatalf("title = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pr/... -v`
Expected: FAIL — package `pr` doesn't exist yet (build error).

- [ ] **Step 3: Implement**

Create `pr/ticket.go`:

```go
package pr

import (
	"fmt"
	"regexp"
)

// ExtractTicket applies pattern (a Go regexp) against branch and returns the
// extracted ticket id. If the pattern has a capture group, group 1 is
// returned; otherwise the whole match is returned. Returns ("", nil) when
// the pattern does not match anywhere in branch.
func ExtractTicket(pattern, branch string) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("compile ticket pattern %q: %w", pattern, err)
	}
	match := re.FindStringSubmatch(branch)
	if match == nil {
		return "", nil
	}
	if len(match) > 1 {
		return match[1], nil
	}
	return match[0], nil
}

// FormatTitle builds the final PR title: the ticket re-prefixed as CU-<id>
// when present (ClickUp's GitHub integration only auto-links default
// hash-style ids in this form, not bare), otherwise the AI title unchanged.
func FormatTitle(ticket, title string) string {
	if ticket == "" {
		return title
	}
	return "CU-" + ticket + ": " + title
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pr/... -v -count=1`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add pr/ticket.go pr/ticket_test.go
git commit -m "feat(pr): add ticket extraction and title formatting"
```

---

### Task 11: `pr.CompareURL`

**Files:**
- Create: `pr/url.go`
- Create: `pr/url_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `pr.CompareURL(remoteURL, base, head, title, body string) (string, error)`. Consumed by Task 21 (PR-confirm flow).

- [ ] **Step 1: Write the failing tests**

Create `pr/url_test.go`:

```go
package pr

import (
	"net/url"
	"strings"
	"testing"
)

func TestCompareURLFromSSHRemote(t *testing.T) {
	got, err := CompareURL("git@github.com:alex-irvine/lazydiff.git", "main", "feature/x", "My title", "My body")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "https://github.com/alex-irvine/lazydiff/compare/main...feature/x?") {
		t.Fatalf("url = %q", got)
	}
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("title") != "My title" || parsed.Query().Get("body") != "My body" || parsed.Query().Get("expand") != "1" {
		t.Fatalf("query = %v", parsed.Query())
	}
}

func TestCompareURLFromHTTPSRemoteWithoutGitSuffix(t *testing.T) {
	got, err := CompareURL("https://github.com/alex-irvine/lazydiff", "main", "feature/x", "t", "b")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "https://github.com/alex-irvine/lazydiff/compare/main...feature/x?") {
		t.Fatalf("url = %q", got)
	}
}

func TestCompareURLRejectsNonGitHubHost(t *testing.T) {
	_, err := CompareURL("git@gitlab.com:group/project.git", "main", "feature/x", "t", "b")
	if err == nil || !strings.Contains(err.Error(), "github.com") {
		t.Fatalf("err = %v", err)
	}
}

func TestCompareURLTruncatesOversizedBody(t *testing.T) {
	body := strings.Repeat("a", 10000)
	got, err := CompareURL("git@github.com:owner/repo.git", "main", "feature/x", "title", body)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > maxCompareURLLength {
		t.Fatalf("url length = %d, want <= %d", len(got), maxCompareURLLength)
	}
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(parsed.Query().Get("body"), "truncated by lazydiff") {
		t.Fatalf("body missing truncation note: %q", parsed.Query().Get("body"))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pr/... -run TestCompareURL -v`
Expected: FAIL — no function `CompareURL` (compile error).

- [ ] **Step 3: Implement**

Create `pr/url.go`:

```go
package pr

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var remoteURLPattern = regexp.MustCompile(`(?:^git@([^:]+):|^https?://([^/]+)/)([^/]+)/(.+?)(?:\.git)?/?$`)

// ownerRepo extracts the host, owner, and repo name from a git remote URL,
// supporting both SSH (git@host:owner/repo.git) and HTTPS
// (https://host/owner/repo[.git]) forms.
func ownerRepo(remoteURL string) (host, owner, repo string, err error) {
	match := remoteURLPattern.FindStringSubmatch(strings.TrimSpace(remoteURL))
	if match == nil {
		return "", "", "", fmt.Errorf("unrecognized remote url %q", remoteURL)
	}
	host = match[1]
	if host == "" {
		host = match[2]
	}
	return host, match[3], match[4], nil
}

const maxCompareURLLength = 6000
const bodyTruncationNote = "\n\n_(description truncated by lazydiff — full text in the request log tab)_"

// CompareURL builds a GitHub compare-page URL that pre-fills the create-PR
// form with title and body, mirroring lazygit's approach. remoteURL is the
// origin remote's URL (SSH or HTTPS); the host must be github.com. If the
// resulting URL would exceed a conservative safety threshold, body is
// truncated with a note appended (title is never truncated).
func CompareURL(remoteURL, base, head, title, body string) (string, error) {
	host, owner, repo, err := ownerRepo(remoteURL)
	if err != nil {
		return "", err
	}
	if host != "github.com" {
		return "", fmt.Errorf("remote host %q is not github.com; PR flow is GitHub-only", host)
	}
	build := func(b string) string {
		query := url.Values{}
		query.Set("expand", "1")
		query.Set("title", title)
		query.Set("body", b)
		return fmt.Sprintf("https://github.com/%s/%s/compare/%s...%s?%s", owner, repo, base, head, query.Encode())
	}
	if full := build(body); len(full) <= maxCompareURLLength {
		return full, nil
	}
	overhead := len(build(bodyTruncationNote))
	budget := maxCompareURLLength - overhead
	if budget < 0 {
		budget = 0
	}
	if budget < len(body) {
		body = body[:budget]
	}
	return build(body + bodyTruncationNote), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pr/... -v -count=1`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add pr/url.go pr/url_test.go
git commit -m "feat(pr): add GitHub compare-URL construction"
```

---

### Task 12: `pr.Opener`

**Files:**
- Create: `pr/open.go`
- Create: `pr/open_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `pr.Opener` interface (`Open(ctx context.Context, rawURL string) error`); `pr.NewOpener() Opener` (real implementation). Consumed by Task 17 (`Model.opener` field, real value wired in `cmd/lazydiff/main.go`) and faked in `ui` package tests from Task 17 onward.

- [ ] **Step 1: Write the failing tests**

Create `pr/open_test.go`:

```go
package pr

import "testing"

func TestOpenerCommandLinux(t *testing.T) {
	name, args, err := openerCommand("linux", "https://example.com")
	if err != nil || name != "xdg-open" || len(args) != 1 || args[0] != "https://example.com" {
		t.Fatalf("name=%q args=%v err=%v", name, args, err)
	}
}

func TestOpenerCommandDarwin(t *testing.T) {
	name, args, err := openerCommand("darwin", "https://example.com")
	if err != nil || name != "open" || len(args) != 1 || args[0] != "https://example.com" {
		t.Fatalf("name=%q args=%v err=%v", name, args, err)
	}
}

func TestOpenerCommandWindowsAvoidsShell(t *testing.T) {
	name, args, err := openerCommand("windows", "https://example.com?a=1&b=2")
	if err != nil || name != "rundll32" || len(args) != 2 || args[0] != "url.dll,FileProtocolHandler" || args[1] != "https://example.com?a=1&b=2" {
		t.Fatalf("name=%q args=%v err=%v", name, args, err)
	}
}

func TestOpenerCommandUnsupportedOS(t *testing.T) {
	if _, _, err := openerCommand("plan9", "https://example.com"); err == nil {
		t.Fatal("expected error for unsupported OS")
	}
}
```

Note: `openerCommand` is a pure function taking `goos` as a plain string parameter (not reading `runtime.GOOS` internally), which is exactly what makes all three platform branches unit-testable in one CI run regardless of which OS actually runs the tests. `Open` itself (which really execs) is not directly unit-tested here — it's a two-line wrapper around `openerCommand` + `exec.CommandContext(...).Start()`, and `ui` package tests from Task 17 onward use a fake `Opener`, never the real one.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pr/... -run TestOpenerCommand -v`
Expected: FAIL — no function `openerCommand` (compile error).

- [ ] **Step 3: Implement**

Create `pr/open.go`:

```go
package pr

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
)

// Opener opens a URL in the user's default browser.
type Opener interface {
	Open(ctx context.Context, rawURL string) error
}

type osOpener struct{ goos string }

// NewOpener returns the real, OS-appropriate Opener.
func NewOpener() Opener {
	return osOpener{goos: runtime.GOOS}
}

func (o osOpener) Open(ctx context.Context, rawURL string) error {
	name, args, err := openerCommand(o.goos, rawURL)
	if err != nil {
		return err
	}
	if err := exec.CommandContext(ctx, name, args...).Start(); err != nil {
		return fmt.Errorf("open %q: %w", rawURL, err)
	}
	return nil
}

func openerCommand(goos, rawURL string) (string, []string, error) {
	switch goos {
	case "linux":
		return "xdg-open", []string{rawURL}, nil
	case "darwin":
		return "open", []string{rawURL}, nil
	case "windows":
		// Invoked directly (not via `cmd /c start`), which would treat the
		// URL's `&` query-param separators as command separators and
		// truncate it.
		return "rundll32", []string{"url.dll,FileProtocolHandler", rawURL}, nil
	default:
		return "", nil, fmt.Errorf("unsupported OS %q for opening a browser", goos)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pr/... -v -count=1`
Expected: all PASS (Tasks 10-12 combined)

- [ ] **Step 5: Commit**

```bash
git add pr/open.go pr/open_test.go
git commit -m "feat(pr): add per-OS browser opener"
```

---

### Task 13: Tri-state checkbox data model and toggle cascade

**Files:**
- Modify: `ui/tree.go`
- Modify: `ui/model_test.go` (tree tests already live here, e.g. `TestTreeNavigationAndSelection` — same convention, no new file)

**Interfaces:**
- Consumes: `TreeModel.flatNodes`, `TreeModel.roots`, `TreeNode.IsLeaf`/`ID` (all existing).
- Produces: `CheckState` type (`Unchecked`/`Checked`/`Indeterminate`); `TreeModel.CheckState(node *TreeNode) CheckState`; `TreeModel.ToggleCheck()`; `TreeModel.ToggleCheckAll()`. Consumed by Task 14 (`StagingPlan`), Task 15 (rendering), and Task 17 (`space`/`ctrl+a` keys).

- [ ] **Step 1: Write the failing tests**

Add to `ui/model_test.go`:

```go
func TestToggleCheckOnHunkSetsOnlyThatHunk(t *testing.T) {
	tree := NewTree(testFiles())
	tree.Toggle() // expand a.go
	tree.Move(1)  // cursor -> hunk:a:0
	tree.ToggleCheck()
	_, hunk, _ := tree.Selected()
	if hunk == nil || hunk.ID != "hunk:a:0" {
		t.Fatalf("expected cursor on hunk:a:0, got %+v", hunk)
	}
	node := tree.flatNodes[tree.cursor]
	if tree.CheckState(node) != Checked {
		t.Fatal("hunk not checked")
	}
	fileNode := tree.flatNodes[0]
	if tree.CheckState(fileNode) != Indeterminate {
		t.Fatalf("file state = %v, want Indeterminate", tree.CheckState(fileNode))
	}
}

func TestToggleCheckOnFileChecksAllItsHunksThenUnchecks(t *testing.T) {
	tree := NewTree(testFiles())
	tree.ToggleCheck() // cursor starts on a.go's (collapsed) file node
	fileNode := tree.flatNodes[0]
	if tree.CheckState(fileNode) != Checked {
		t.Fatalf("file state = %v, want Checked", tree.CheckState(fileNode))
	}
	tree.ToggleCheck()
	if tree.CheckState(fileNode) != Unchecked {
		t.Fatalf("file state = %v, want Unchecked", tree.CheckState(fileNode))
	}
}

func TestToggleCheckAllChecksEveryLeafThenUnchecksAll(t *testing.T) {
	tree := NewTree(testFiles())
	tree.ToggleCheckAll()
	for _, root := range tree.roots {
		if tree.CheckState(root) != Checked {
			t.Fatalf("root %q not fully checked", root.Label)
		}
	}
	tree.ToggleCheckAll()
	for _, root := range tree.roots {
		if tree.CheckState(root) != Unchecked {
			t.Fatalf("root %q not fully unchecked", root.Label)
		}
	}
}

func TestCheckStateSurvivesSetFilesRebuild(t *testing.T) {
	tree := NewTree(testFiles())
	tree.Toggle()
	tree.Move(1) // cursor -> hunk:a:0
	tree.ToggleCheck()
	tree.SetFiles(testFiles())
	tree.Toggle()
	tree.Move(1)
	_, hunk, _ := tree.Selected()
	if hunk == nil || hunk.ID != "hunk:a:0" {
		t.Fatalf("expected hunk:a:0 selected, got %+v", hunk)
	}
	node := tree.flatNodes[tree.cursor]
	if tree.CheckState(node) != Checked {
		t.Fatal("checked state lost after SetFiles rebuild")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./ui/... -run 'TestToggleCheck|TestCheckStateSurvivesSetFilesRebuild' -v`
Expected: FAIL — no method `ToggleCheck`/`CheckState`/`ToggleCheckAll` (compile error).

- [ ] **Step 3: Implement**

Add to `ui/tree.go`:

```go
type CheckState int

const (
	Unchecked CheckState = iota
	Checked
	Indeterminate
)

// CheckState reports whether every, some, or none of node's leaves (hunks,
// or the file itself for a no-hunks file) are checked.
func (t *TreeModel) CheckState(node *TreeNode) CheckState {
	total, checked := t.countLeaves(node)
	switch {
	case total == 0, checked == 0:
		return Unchecked
	case checked == total:
		return Checked
	default:
		return Indeterminate
	}
}

func (t *TreeModel) countLeaves(node *TreeNode) (total, checked int) {
	if node.IsLeaf() {
		if t.checked[node.ID()] {
			return 1, 1
		}
		return 1, 0
	}
	for _, child := range node.Children {
		childTotal, childChecked := t.countLeaves(child)
		total += childTotal
		checked += childChecked
	}
	return total, checked
}

// ToggleCheck toggles the check state of the node under the cursor. Checking
// a directory or a file with hunks cascades to every descendant leaf.
func (t *TreeModel) ToggleCheck() {
	if len(t.flatNodes) == 0 || t.cursor >= len(t.flatNodes) {
		return
	}
	if t.checked == nil {
		t.checked = make(map[string]bool)
	}
	node := t.flatNodes[t.cursor]
	t.setChecked(node, t.CheckState(node) != Checked)
}

func (t *TreeModel) setChecked(node *TreeNode, value bool) {
	if node.IsLeaf() {
		t.checked[node.ID()] = value
		return
	}
	for _, child := range node.Children {
		t.setChecked(child, value)
	}
}

// ToggleCheckAll checks every leaf in the tree if any are unchecked;
// unchecks all if every leaf is already checked.
func (t *TreeModel) ToggleCheckAll() {
	if len(t.roots) == 0 {
		return
	}
	if t.checked == nil {
		t.checked = make(map[string]bool)
	}
	allChecked := true
	for _, root := range t.roots {
		if t.CheckState(root) != Checked {
			allChecked = false
			break
		}
	}
	for _, root := range t.roots {
		t.setChecked(root, !allChecked)
	}
}
```

Also add a `checked map[string]bool` field to the `TreeModel` struct definition:

```go
type TreeModel struct {
	roots        []*TreeNode
	flatNodes    []*TreeNode
	cursor       int
	selectedID   string
	scrollOffset int
	checked      map[string]bool
}
```

No changes are needed to `SetFiles`/`buildTree`/`flatten` — `checked` is keyed by content-derived leaf IDs and lives on `TreeModel` itself (not on the per-rebuild `TreeNode`s), so it survives a `SetFiles` rebuild automatically, the same way `t.cursor`/`t.selectedID` already do. Stale entries for leaves that no longer exist are simply never looked up again — harmless.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./ui/... -v -count=1`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add ui/tree.go ui/model_test.go
git commit -m "feat(ui): add tri-state checkbox model to the file tree"
```

---

### Task 14: `TreeModel.StagingPlan`

**Files:**
- Modify: `ui/tree.go`
- Modify: `ui/model_test.go`

**Interfaces:**
- Consumes: `TreeModel.CheckState`, `TreeModel.checked` (Task 13); `diff.File`/`diff.Hunk` (existing).
- Produces: `StageAction{File diff.File, PartialHunks []diff.Hunk}`; `TreeModel.StagingPlan() []StageAction`. Consumed by Task 18 (commit flow).

- [ ] **Step 1: Write the failing tests**

Add to `ui/model_test.go`:

```go
func TestStagingPlanBucketsWholeFiles(t *testing.T) {
	tree := NewTree(testFiles())
	tree.ToggleCheck() // cursor 0 = a.go (collapsed), checks all its hunks
	tree.Move(1)       // cursor -> b.go
	tree.ToggleCheck()
	plan := tree.StagingPlan()
	if len(plan) != 2 {
		t.Fatalf("plan = %+v", plan)
	}
	if plan[0].File.Path != "a.go" || len(plan[0].PartialHunks) != 0 {
		t.Fatalf("a.go action = %+v", plan[0])
	}
	if plan[1].File.Path != "b.go" || len(plan[1].PartialHunks) != 0 {
		t.Fatalf("b.go action = %+v", plan[1])
	}
}

func TestStagingPlanPartialHunkSelection(t *testing.T) {
	tree := NewTree(testFiles())
	tree.Toggle() // expand a.go
	tree.Move(1)  // cursor -> hunk:a:0
	tree.ToggleCheck()
	plan := tree.StagingPlan()
	if len(plan) != 1 || plan[0].File.Path != "a.go" || len(plan[0].PartialHunks) != 1 || plan[0].PartialHunks[0].ID != "hunk:a:0" {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestStagingPlanSkipsUncheckedFiles(t *testing.T) {
	tree := NewTree(testFiles())
	if plan := tree.StagingPlan(); len(plan) != 0 {
		t.Fatalf("plan = %+v, want empty", plan)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./ui/... -run TestStagingPlan -v`
Expected: FAIL — no method `StagingPlan` (compile error).

- [ ] **Step 3: Implement**

Add `"github.com/alex-irvine/lazydiff/diff"` is already imported in `ui/tree.go`. Add to `ui/tree.go`:

```go
type StageAction struct {
	File diff.File
	// PartialHunks is empty for "stage the whole file"; when non-empty it
	// carries the strict subset of File.Hunks that were checked.
	PartialHunks []diff.Hunk
}

// StagingPlan returns one StageAction per file that has at least one checked
// leaf, in file order.
func (t *TreeModel) StagingPlan() []StageAction {
	var plan []StageAction
	for _, root := range t.roots {
		t.collectStageActions(root, &plan)
	}
	return plan
}

func (t *TreeModel) collectStageActions(node *TreeNode, plan *[]StageAction) {
	if node.File != nil && node.Hunk == nil {
		switch t.CheckState(node) {
		case Unchecked:
			return
		case Checked:
			*plan = append(*plan, StageAction{File: *node.File})
		case Indeterminate:
			var partial []diff.Hunk
			for _, hunk := range node.File.Hunks {
				if t.checked[hunk.ID] {
					partial = append(partial, hunk)
				}
			}
			*plan = append(*plan, StageAction{File: *node.File, PartialHunks: partial})
		}
		return
	}
	for _, child := range node.Children {
		t.collectStageActions(child, plan)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./ui/... -v -count=1`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add ui/tree.go ui/model_test.go
git commit -m "feat(ui): add StagingPlan"
```

---

### Task 15: Checkbox glyph rendering

**Files:**
- Modify: `ui/render.go` (add `git` import, edit `renderTree`)
- Modify: `ui/view_test.go`

**Interfaces:**
- Consumes: `TreeModel.CheckState` (Task 13); `Model.mode` (existing).
- Produces: no new symbols — this is a pure rendering change. Nothing later depends on it.

- [ ] **Step 1: Write the failing tests**

Add to `ui/view_test.go`:

```go
func TestRenderTreeShowsCheckboxesInWorkingTreeMode(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model.termW, model.termH = 120, 40
	model.layout = ComputeLayout(120, 40)
	model.snapshot = makeSnapshot("one")
	model.haveSnap = true
	model.mode = git.WorkingTree
	model.tree = NewTree(model.snapshot.Files)
	model.tree.ToggleCheck()
	out := model.renderTree(model.layout.Files)
	if !strings.Contains(out, "[x]") {
		t.Fatalf("expected a checked box in tree render:\n%s", out)
	}
}

func TestRenderTreeHidesCheckboxesOutsideWorkingTreeMode(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model.termW, model.termH = 120, 40
	model.layout = ComputeLayout(120, 40)
	model.snapshot = makeSnapshot("one")
	model.haveSnap = true
	model.mode = git.Branch
	model.tree = NewTree(model.snapshot.Files)
	out := model.renderTree(model.layout.Files)
	if strings.Contains(out, "[x]") || strings.Contains(out, "[ ]") || strings.Contains(out, "[-]") {
		t.Fatalf("did not expect checkboxes outside working tree mode:\n%s", out)
	}
}
```

(Check `ui/view_test.go`'s existing imports; add `"strings"`, `"github.com/alex-irvine/lazydiff/git"` if not already present there — `git` is likely already imported since other files in the package use it, but this file's own import list may not yet include it.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./ui/... -run TestRenderTree -v`
Expected: FAIL — `[x]` never appears (test failure, not compile error, since `renderTree`/`CheckState`/`m.mode` all already exist).

- [ ] **Step 3: Implement**

In `ui/render.go`, add `"github.com/alex-irvine/lazydiff/git"` to the import block. Change the loop body in `renderTree` from:

```go
	for _, node := range visible {
		id := node.ID()
		active := id == m.tree.selectedID
		prefix := "  "
		if active {
			prefix = "▶ "
		}
		indent := strings.Repeat("  ", node.Level)
		var icon string
		if node.Hunk != nil {
			icon = "  "
		} else if node.File != nil {
			icon = "📄 "
		} else if node.Expanded {
			icon = "📂 "
		} else {
			icon = "📁 "
		}
		fullLine := prefix + indent + icon + node.Label
```

to:

```go
	for _, node := range visible {
		id := node.ID()
		active := id == m.tree.selectedID
		prefix := "  "
		if active {
			prefix = "▶ "
		}
		indent := strings.Repeat("  ", node.Level)
		checkbox := ""
		if m.mode == git.WorkingTree {
			switch m.tree.CheckState(node) {
			case Checked:
				checkbox = "[x] "
			case Indeterminate:
				checkbox = "[-] "
			default:
				checkbox = "[ ] "
			}
		}
		var icon string
		if node.Hunk != nil {
			icon = "  "
		} else if node.File != nil {
			icon = "📄 "
		} else if node.Expanded {
			icon = "📂 "
		} else {
			icon = "📁 "
		}
		fullLine := prefix + indent + checkbox + icon + node.Label
```

(The rest of the loop — `truncated := delta.Truncate(fullLine, maxW)` onward — is unchanged.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./ui/... -v -count=1`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add ui/render.go ui/view_test.go
git commit -m "feat(ui): render checkbox glyphs in working tree mode"
```

---

### Task 16: `ActionDialog` (edit/confirm dialog sub-model)

**Files:**
- Modify: `go.mod`, `go.sum` (new dependency)
- Create: `ui/dialog.go`
- Create: `ui/dialog_test.go`

**Interfaces:**
- Consumes: `github.com/charmbracelet/bubbles/textarea` (new dependency); `tea.KeyMsg`/`tea.Msg` (existing).
- Produces: `DialogKind` (`CommitDialog`/`PRDialog`); `ActionDialog` struct; `NewActionDialog(kind DialogKind) *ActionDialog`; `(*ActionDialog).SetDraft(text string, err error)`; `(*ActionDialog).Update(msg tea.Msg) (DialogAction, tea.Cmd)`; `(*ActionDialog).View() string`; `(*ActionDialog).Text() string`; `DialogAction` (`ActionNone`/`ActionConfirm`/`ActionCancel`/`ActionRegenerate`). Consumed by Task 17 onward (`Model.dialog *ActionDialog`).

- [ ] **Step 1: Add the dependency**

Run: `go get github.com/charmbracelet/bubbles@latest`
Expected: `go.mod`/`go.sum` updated with a new direct dependency `github.com/charmbracelet/bubbles`.

- [ ] **Step 2: Write the failing tests**

Create `ui/dialog_test.go`:

```go
package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestActionDialogConfirmCancelRegenerateKeys(t *testing.T) {
	dialog := NewActionDialog(CommitDialog)
	dialog.SetDraft("subject\n\nbody", nil)
	if action, _ := dialog.Update(tea.KeyMsg{Type: tea.KeyEsc}); action != ActionCancel {
		t.Fatalf("esc action = %v", action)
	}
	if action, _ := dialog.Update(tea.KeyMsg{Type: tea.KeyCtrlS}); action != ActionConfirm {
		t.Fatalf("ctrl+s action = %v", action)
	}
	if action, _ := dialog.Update(tea.KeyMsg{Type: tea.KeyCtrlR}); action != ActionRegenerate {
		t.Fatalf("ctrl+r action = %v", action)
	}
}

func TestActionDialogTypingEditsText(t *testing.T) {
	dialog := NewActionDialog(CommitDialog)
	dialog.SetDraft("initial", nil)
	dialog.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})
	if !strings.Contains(dialog.Text(), "!") {
		t.Fatalf("text = %q, want to contain typed character", dialog.Text())
	}
}

func TestActionDialogNotReadyUntilSetDraft(t *testing.T) {
	dialog := NewActionDialog(PRDialog)
	if dialog.Ready {
		t.Fatal("dialog should not be ready before SetDraft")
	}
	dialog.SetDraft("title\n\nbody", nil)
	if !dialog.Ready {
		t.Fatal("dialog should be ready after SetDraft")
	}
}
```

Per the Global Constraints note on library API uncertainty: if `tea.KeyCtrlS`/`tea.KeyCtrlR`/`tea.KeyEsc` don't exist under those exact names, run `go doc github.com/charmbracelet/bubbletea` (look for the `Key*` constants) and substitute the real ones — the test's intent (esc cancels, ctrl+s confirms, ctrl+r regenerates, plain typing edits the text) stays the same.

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./ui/... -run TestActionDialog -v`
Expected: FAIL — no type `ActionDialog`/`DialogKind`/`CommitDialog` etc. (compile error).

- [ ] **Step 4: Implement**

Create `ui/dialog.go`:

```go
package ui

import (
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

type DialogKind int

const (
	CommitDialog DialogKind = iota
	PRDialog
)

// ActionDialog shows AI-generated text (a commit message, or a PR
// title+body) for the user to review and edit before anything mutates.
// Ready is false while the AI request is still in flight; Err holds a
// generation error, if any (the dialog still opens and lets the user
// hand-write the text in that case).
type ActionDialog struct {
	Kind     DialogKind
	Textarea textarea.Model
	Ready    bool
	Err      error
}

func NewActionDialog(kind DialogKind) *ActionDialog {
	ta := textarea.New()
	ta.Placeholder = "generating..."
	ta.Focus()
	return &ActionDialog{Kind: kind, Textarea: ta}
}

// SetDraft populates the dialog with generated text (or an error) once the
// AI request finishes.
func (d *ActionDialog) SetDraft(text string, err error) {
	d.Ready = true
	d.Err = err
	d.Textarea.SetValue(text)
}

type DialogAction int

const (
	ActionNone DialogAction = iota
	ActionConfirm
	ActionCancel
	ActionRegenerate
)

// Update intercepts esc/ctrl+s/ctrl+r; every other message is forwarded to
// the inner textarea for normal text editing.
func (d *ActionDialog) Update(msg tea.Msg) (DialogAction, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "esc":
			return ActionCancel, nil
		case "ctrl+s":
			return ActionConfirm, nil
		case "ctrl+r":
			return ActionRegenerate, nil
		}
	}
	var cmd tea.Cmd
	d.Textarea, cmd = d.Textarea.Update(msg)
	return ActionNone, cmd
}

func (d *ActionDialog) View() string {
	return d.Textarea.View()
}

func (d *ActionDialog) Text() string {
	return d.Textarea.Value()
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./ui/... -v -count=1`
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum ui/dialog.go ui/dialog_test.go
git commit -m "feat(ui): add ActionDialog edit/confirm sub-model"
```

---

### Task 17: `Mutator` interface and Model wiring skeleton

This task wires the plumbing (new interface, new `Model` fields, modal key capture, key remap) **without** the commit/PR flow logic itself — that's Tasks 18-21. Keeping this separate means no task references a function that doesn't exist yet.

**Files:**
- Modify: `ui/model.go`
- Modify: `ui/model_test.go`
- Modify: `cmd/lazydiff/main.go`

**Interfaces:**
- Consumes: `git.Repository` (already has every method `Mutator` needs, once Tasks 2-6 are done — Go's structural typing means it satisfies the interface with no explicit declaration); `pr.Opener` (Task 12); `ActionDialog` (Task 16).
- Produces: `Mutator` interface; `Model.mutator Mutator`, `Model.opener pr.Opener`, `Model.dialog *ActionDialog` fields; `NewModel`'s signature gains two trailing parameters. Consumed by Tasks 18-21.

- [ ] **Step 1: Write the failing tests**

Add to `ui/model_test.go`. First, two new test doubles next to the existing `fakeRunner`/`fakeLoader`:

```go
type fakeMutator struct {
	currentBranch string
	defaultBranch string
	remoteURL     string
	staged        []string
	commits       []string
	pushed        []string
	err           error
}

func (f *fakeMutator) CurrentBranch(context.Context) (string, error) { return f.currentBranch, f.err }
func (f *fakeMutator) DefaultBranch(context.Context) (string, error) { return f.defaultBranch, f.err }
func (f *fakeMutator) RemoteURL(context.Context, string) (string, error) { return f.remoteURL, f.err }

func (f *fakeMutator) StageFile(_ context.Context, oldPath, path string) error {
	f.staged = append(f.staged, "file:"+oldPath+":"+path)
	return f.err
}

func (f *fakeMutator) StagePatch(_ context.Context, patch string) error {
	f.staged = append(f.staged, "patch:"+patch)
	return f.err
}

func (f *fakeMutator) Commit(_ context.Context, message string) error {
	f.commits = append(f.commits, message)
	return f.err
}

func (f *fakeMutator) Push(_ context.Context, remote, branch string) error {
	f.pushed = append(f.pushed, remote+"/"+branch)
	return f.err
}

type fakeOpener struct {
	urls []string
	err  error
}

func (f *fakeOpener) Open(_ context.Context, rawURL string) error {
	f.urls = append(f.urls, rawURL)
	return f.err
}
```

Then update `newTestModel` to construct and pass them:

```go
func newTestModel(loader SnapshotLoader, runner agent.Runner) Model {
	cfg := config.Default()
	templates, err := prompt.Parse(prompt.Sources{
		Overall:       cfg.Agent.Prompts.Overall,
		Detail:        cfg.Agent.Prompts.Detail,
		CommitMessage: cfg.Agent.Prompts.CommitMessage,
		PRDescription: cfg.Agent.Prompts.PRDescription,
	})
	if err != nil {
		panic(err)
	}
	return NewModel(git.Repository{Root: "/repo"}, cfg, loader, fakeRenderer{}, runner, templates, &fakeMutator{}, &fakeOpener{})
}
```

Then the new tests:

```go
func TestSpaceTogglesCheckInsteadOfExpand(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model.termW, model.termH = 120, 40
	model.snapshot = makeSnapshot("one")
	model.haveSnap = true
	model.mode = git.WorkingTree
	model.tree = NewTree(model.snapshot.Files)
	model.focus = FocusTree
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	node := model.tree.flatNodes[0]
	if model.tree.CheckState(node) != Checked {
		t.Fatal("space did not check the file node")
	}
	if node.Expanded {
		t.Fatal("space should no longer expand/collapse the node")
	}
}

func TestCtrlACheckAllInWorkingTreeMode(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model.termW, model.termH = 120, 40
	model.snapshot = makeSnapshot("one")
	model.haveSnap = true
	model.mode = git.WorkingTree
	model.tree = NewTree(model.snapshot.Files)
	model.focus = FocusTree
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	for _, root := range model.tree.roots {
		if model.tree.CheckState(root) != Checked {
			t.Fatalf("ctrl+a did not check root %q", root.Label)
		}
	}
}

func TestXCancelsActiveAnalysis(t *testing.T) {
	runner := &blockingRunner{}
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, runner)
	model.snapshot = makeSnapshot("one")
	model.haveSnap = true
	model.tree = NewTree(model.snapshot.Files)
	model.termW, model.termH = 120, 40
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	cmd()
	if runner.cancelled == false {
		t.Fatal("x did not cancel the running analysis")
	}
}

func TestDialogCapturesKeysBeforeTreeNavigation(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model.snapshot = makeSnapshot("one")
	model.haveSnap = true
	model.tree = NewTree(model.snapshot.Files)
	model.tree.Toggle()
	startCursor := model.tree.cursor
	model.dialog = NewActionDialog(CommitDialog)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if model.tree.cursor != startCursor {
		t.Fatal("key reached tree navigation while dialog was open")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./ui/... -v`
Expected: FAIL — `NewModel` called with 8 arguments doesn't match the current 6-parameter signature; no field `mutator`/`opener`/`dialog` (compile error).

- [ ] **Step 3: Implement — `Mutator` interface and `Model` fields**

In `ui/model.go`, add next to the existing `SnapshotLoader` interface (around line 35):

```go
// Mutator is every git operation the commit/PR flows need beyond reading a
// snapshot (which stays on SnapshotLoader). git.Repository satisfies this
// once Tasks 2-6 add these methods — no explicit declaration needed.
type Mutator interface {
	CurrentBranch(context.Context) (string, error)
	DefaultBranch(context.Context) (string, error)
	RemoteURL(context.Context, string) (string, error)
	StageFile(ctx context.Context, oldPath, path string) error
	StagePatch(ctx context.Context, patch string) error
	Commit(ctx context.Context, message string) error
	Push(ctx context.Context, remote, branch string) error
}
```

Add `"github.com/alex-irvine/lazydiff/pr"` to the import block. Add fields to the `Model` struct:

```go
type Model struct {
	repo      git.Repository
	cfg       config.Config
	loader    SnapshotLoader
	renderer  Renderer
	runner    agent.Runner
	templates prompt.Templates
	mutator   Mutator
	opener    pr.Opener
	dialog    *ActionDialog

	// ... rest of the existing fields unchanged
```

Change `NewModel`:

```go
func NewModel(repo git.Repository, cfg config.Config, loader SnapshotLoader, renderer Renderer, runner agent.Runner, templates prompt.Templates, mutator Mutator, opener pr.Opener) Model {
	return Model{
		repo: repo, cfg: cfg, loader: loader, renderer: renderer, runner: runner, templates: templates,
		mutator: mutator, opener: opener,
		mode: git.WorkingTree, tree: NewTree(nil), focus: FocusTree, activeTab: DetailTab,
		results: make(map[string]*analysisResult), requests: make(map[string]context.CancelFunc),
		status: "loading repository",
	}
}
```

- [ ] **Step 4: Implement — modal key capture**

Change the top of `Update`:

```go
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok && m.dialog != nil {
		return m.updateDialogKey(keyMsg)
	}
	switch message := msg.(type) {
```

(everything below the `switch` stays exactly as-is for now — the `commitPrepMsg`/`commitDraftMsg`/etc. cases from Tasks 18-21 will be added to this same switch later, and since they aren't `tea.KeyMsg`, they still reach the switch even while `m.dialog != nil`).

Add a new method near `updateKey`:

```go
func (m Model) updateDialogKey(key tea.KeyMsg) (Model, tea.Cmd) {
	action, cmd := m.dialog.Update(key)
	switch action {
	case ActionCancel:
		m.dialog = nil
		return m, nil
	case ActionConfirm, ActionRegenerate:
		// Task 19/21 (commit confirm) and Task 20 (regenerate) fill this in;
		// for now these are unreachable in practice because no key sets
		// m.dialog to non-nil until Task 18.
		return m, nil
	}
	return m, cmd
}
```

- [ ] **Step 5: Implement — key remap**

In `updateKey`, remove the existing `case " ":` block entirely (it called `m.tree.Toggle()`) and the existing `case "c": m.cancelActive()` line, then add:

```go
	case " ":
		if m.focus == FocusTree && m.mode == git.WorkingTree {
			m.tree.ToggleCheck()
		}
	case "ctrl+a":
		if m.focus == FocusTree && m.mode == git.WorkingTree {
			m.tree.ToggleCheckAll()
		}
	case "x":
		m.cancelActive()
```

(Leave `h`/`l`/`tab`/`shift+tab`/etc. untouched — they already cover expand/collapse navigation, which is why removing space's old behavior loses nothing.)

- [ ] **Step 6: Fix `cmd/lazydiff/main.go`'s `NewModel` call site**

Add `"github.com/alex-irvine/lazydiff/pr"` to imports. Change:

```go
	model := ui.NewTeaModel(ui.NewModel(repo, cfg, loader, delta.Renderer{Command: "delta"}, runner, templates))
```

to:

```go
	model := ui.NewTeaModel(ui.NewModel(repo, cfg, loader, delta.Renderer{Command: "delta"}, runner, templates, repo, pr.NewOpener()))
```

(`repo` is passed twice: once as the `git.Repository` value the `Model` already stored for `.Root` access, once again satisfying the `ui.Mutator` interface — `git.Repository` has every method `Mutator` requires once Tasks 2-6 are in place.)

- [ ] **Step 7: Run test to verify it passes, and that the whole repo still builds**

Run: `go build ./... && go test ./... -count=1`
Expected: all PASS/builds.

- [ ] **Step 8: Commit**

```bash
git add ui/model.go ui/model_test.go cmd/lazydiff/main.go
git commit -m "feat(ui): add Mutator interface, dialog field, and modal key capture"
```

---

### Task 18: Commit flow trigger (`c`)

**Files:**
- Modify: `ui/model.go`
- Modify: `ui/model_test.go`

**Interfaces:**
- Consumes: `TreeModel.StagingPlan` (Task 14); `diff.BuildPatch` (Task 7); `Mutator.StageFile`/`StagePatch`/`CurrentBranch` (Tasks 2-6, via `Model.mutator`); `SnapshotLoader.Snapshot` (existing, via `Model.loader`); `pr.ExtractTicket` (Task 10); `Templates.RenderCommitMessage` (Task 9); `ActionDialog`/`NewActionDialog` (Task 16); `agent.Runner` (existing, via `Model.runner`).
- Produces: `commitPrepMsg{Ticket, Prompt string, Err error}`; `commitDraftMsg{Ticket, Text string, Err error}`; `Model.startCommitCmd() tea.Cmd`; `Model.runCommitAgentCmd(renderedPrompt, ticket string) tea.Cmd`; `splitSubjectBody(text string) (subject, body string)`. Consumed by Task 19 (regenerate reuses `startCommitCmd`).

- [ ] **Step 1: Write the failing tests**

Add to `ui/model_test.go`:

```go
func makeStagedSnapshot(id string) git.Snapshot {
	return git.Snapshot{ID: id, Mode: git.Staged, RawDiff: "staged-content\n"}
}

func TestStartCommitCmdStagesAndPreparesPrompt(t *testing.T) {
	loader := &fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one"), makeStagedSnapshot("staged-one")}}
	model := newTestModel(loader, &fakeRunner{})
	model.snapshot = makeSnapshot("one")
	model.haveSnap = true
	model.tree = NewTree(model.snapshot.Files)
	model.tree.ToggleCheck() // checks a.go (cursor starts there)
	mutator := &fakeMutator{currentBranch: "feature/869d6rn69-thing"}
	model.mutator = mutator
	msg := model.startCommitCmd()()
	prep, ok := msg.(commitPrepMsg)
	if !ok || prep.Err != nil {
		t.Fatalf("msg = %+v", msg)
	}
	if prep.Ticket != "869d6rn69" {
		t.Fatalf("ticket = %q", prep.Ticket)
	}
	if !strings.Contains(prep.Prompt, "staged-content") {
		t.Fatalf("prompt = %q", prep.Prompt)
	}
	if len(mutator.staged) != 1 || mutator.staged[0] != "file::a.go" {
		t.Fatalf("staged = %v", mutator.staged)
	}
}

func TestRunCommitAgentCmdParsesOutput(t *testing.T) {
	runner := &fakeRunner{events: []agent.Event{{Kind: agent.Output, Text: "Add OAuth login"}, {Kind: agent.Output, Text: "Adds login via OAuth provider."}}}
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, runner)
	msg := model.runCommitAgentCmd("rendered prompt", "869d6rn69")()
	draft, ok := msg.(commitDraftMsg)
	if !ok || draft.Err != nil {
		t.Fatalf("msg = %+v", msg)
	}
	if draft.Ticket != "869d6rn69" || !strings.Contains(draft.Text, "Add OAuth login") {
		t.Fatalf("draft = %+v", draft)
	}
}

func TestCommitDraftMsgPopulatesDialogWithTicketTrailer(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model.dialog = NewActionDialog(CommitDialog)
	model, _ = model.Update(commitDraftMsg{Ticket: "869d6rn69", Text: "Add OAuth login\nAdds login via OAuth provider."})
	if !model.dialog.Ready {
		t.Fatal("dialog not marked ready")
	}
	text := model.dialog.Text()
	if !strings.Contains(text, "Add OAuth login") || !strings.Contains(text, "CU-869d6rn69") {
		t.Fatalf("dialog text = %q", text)
	}
}

func TestCKeyStartsCommitFlowWhenItemsChecked(t *testing.T) {
	loader := &fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one"), makeStagedSnapshot("staged-one")}}
	model := newTestModel(loader, &fakeRunner{})
	model.snapshot = makeSnapshot("one")
	model.haveSnap = true
	model.tree = NewTree(model.snapshot.Files)
	model.mode = git.WorkingTree
	model.mutator = &fakeMutator{}
	model.tree.ToggleCheck()
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if cmd == nil {
		t.Fatal("c did not start the commit flow")
	}
	msg := cmd()
	if _, ok := msg.(commitPrepMsg); !ok {
		t.Fatalf("msg = %+v, want commitPrepMsg", msg)
	}
}

func TestCKeyNoopWithNothingChecked(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model.snapshot = makeSnapshot("one")
	model.haveSnap = true
	model.tree = NewTree(model.snapshot.Files)
	model.mode = git.WorkingTree
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if cmd != nil {
		t.Fatal("c should be a no-op when nothing is checked")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./ui/... -run 'TestStartCommitCmd|TestRunCommitAgentCmd|TestCommitDraftMsg|TestCKey' -v`
Expected: FAIL — no method `startCommitCmd`/`runCommitAgentCmd`, no type `commitPrepMsg`/`commitDraftMsg` (compile error).

- [ ] **Step 3: Implement**

Add `"github.com/alex-irvine/lazydiff/diff"` to `ui/model.go`'s imports if not already present (it likely is not, since `model.go` currently only uses `diff.Hunk` via the `resultKey` helper's parameter type — check the existing import block; add it if missing). Add to `ui/model.go`:

```go
type commitPrepMsg struct {
	Ticket string
	Prompt string
	Err    error
}

type commitDraftMsg struct {
	Ticket string
	Text   string
	Err    error
}

func (m Model) startCommitCmd() tea.Cmd {
	mutator, loader, cfg, templates, plan := m.mutator, m.loader, m.cfg, m.templates, m.tree.StagingPlan()
	repoRoot := m.repo.Root
	return func() tea.Msg {
		ctx := context.Background()
		for _, action := range plan {
			var err error
			if len(action.PartialHunks) == 0 {
				err = mutator.StageFile(ctx, action.File.OldPath, action.File.Path)
			} else {
				err = mutator.StagePatch(ctx, diff.BuildPatch(action.File, action.PartialHunks))
			}
			if err != nil {
				return commitPrepMsg{Err: err}
			}
		}
		staged, err := loader.Snapshot(ctx, git.Staged)
		if err != nil {
			return commitPrepMsg{Err: err}
		}
		branch, err := mutator.CurrentBranch(ctx)
		if err != nil {
			return commitPrepMsg{Err: err}
		}
		ticket, err := pr.ExtractTicket(cfg.PR.TicketPattern, branch)
		if err != nil {
			return commitPrepMsg{Err: err}
		}
		rendered, err := templates.RenderCommitMessage(prompt.Context{
			Repository: repoRoot,
			Mode:       staged.Mode.String(),
			StagedDiff: staged.RawDiff,
			Ticket:     ticket,
		})
		if err != nil {
			return commitPrepMsg{Err: err}
		}
		return commitPrepMsg{Ticket: ticket, Prompt: rendered}
	}
}

func (m Model) runCommitAgentCmd(renderedPrompt, ticket string) tea.Cmd {
	runner, repoRoot := m.runner, m.repo.Root
	return func() tea.Msg {
		var output strings.Builder
		err := runner.Run(context.Background(), agent.Request{RepoRoot: repoRoot, Prompt: renderedPrompt}, func(event agent.Event) {
			if event.Kind == agent.Output {
				if output.Len() > 0 {
					output.WriteByte('\n')
				}
				output.WriteString(event.Text)
			}
		})
		return commitDraftMsg{Ticket: ticket, Text: output.String(), Err: err}
	}
}

func splitSubjectBody(text string) (string, string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", ""
	}
	lines := strings.SplitN(trimmed, "\n", 2)
	subject := strings.TrimSpace(lines[0])
	body := ""
	if len(lines) > 1 {
		body = strings.TrimSpace(lines[1])
	}
	return subject, body
}
```

Add two new cases to `Update`'s switch (anywhere among the existing `case`s, e.g. right after `case updatePerformedMsg:`):

```go
	case commitPrepMsg:
		if message.Err != nil {
			m.status = "commit prep error: " + message.Err.Error()
			return m, nil
		}
		m.dialog = NewActionDialog(CommitDialog)
		return m, m.runCommitAgentCmd(message.Prompt, message.Ticket)
	case commitDraftMsg:
		if m.dialog == nil || m.dialog.Kind != CommitDialog {
			return m, nil
		}
		text := message.Text
		if message.Err == nil {
			subject, body := splitSubjectBody(text)
			if message.Ticket != "" {
				body = strings.TrimRight(body, "\n") + "\n\nCU-" + message.Ticket
			}
			text = subject + "\n\n" + body
		}
		m.dialog.SetDraft(text, message.Err)
		return m, nil
```

Add to `updateKey`'s switch:

```go
	case "c":
		if m.mode == git.WorkingTree && len(m.tree.StagingPlan()) > 0 {
			return m, m.startCommitCmd()
		}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./ui/... -v -count=1`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add ui/model.go ui/model_test.go
git commit -m "feat(ui): trigger commit flow on c"
```

---

### Task 19: Commit flow confirm and regenerate

**Files:**
- Modify: `ui/model.go` (replace the Task 17 `updateDialogKey` stub)
- Modify: `ui/model_test.go`

**Interfaces:**
- Consumes: `Mutator.Commit` (Task 5, via `Model.mutator`); `ActionDialog.Ready`/`.Err`/`.Text()`/`.Kind` (Task 16); `startCommitCmd` (Task 18).
- Produces: `commitResultMsg{Err error}`; `Model.confirmDialogCmd() (Model, tea.Cmd)`; `Model.regenerateDialogCmd() (Model, tea.Cmd)`. Task 21 (PR confirm) extends both of these with a `PRDialog` case.

- [ ] **Step 1: Write the failing tests**

Add to `ui/model_test.go` (add `"fmt"` to imports if not already present):

```go
func TestConfirmCommitDialogCallsMutatorCommit(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	mutator := &fakeMutator{}
	model.mutator = mutator
	model.dialog = NewActionDialog(CommitDialog)
	model.dialog.SetDraft("subject\n\nbody", nil)
	model, cmd := model.updateDialogKey(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("confirm did not produce a command")
	}
	msg := cmd()
	result, ok := msg.(commitResultMsg)
	if !ok || result.Err != nil {
		t.Fatalf("msg = %+v", msg)
	}
	if len(mutator.commits) != 1 || mutator.commits[0] != "subject\n\nbody" {
		t.Fatalf("commits = %v", mutator.commits)
	}
}

func TestCommitResultMsgClosesDialogAndRefreshes(t *testing.T) {
	loader := &fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one"), makeSnapshot("two")}}
	model := newTestModel(loader, &fakeRunner{})
	model.dialog = NewActionDialog(CommitDialog)
	model, cmd := model.Update(commitResultMsg{})
	if model.dialog != nil {
		t.Fatal("dialog should close after a successful commit")
	}
	if cmd == nil {
		t.Fatal("expected a refresh command after commit")
	}
}

func TestCommitResultMsgErrorKeepsDialogOpen(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model.dialog = NewActionDialog(CommitDialog)
	model, _ = model.Update(commitResultMsg{Err: fmt.Errorf("commit hook rejected")})
	if model.dialog == nil {
		t.Fatal("dialog should stay open on commit failure")
	}
	if !strings.Contains(model.status, "commit hook rejected") {
		t.Fatalf("status = %q", model.status)
	}
}

func TestRegenerateCommitDialogRerunsStartCommitCmd(t *testing.T) {
	loader := &fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one"), makeStagedSnapshot("staged-one")}}
	model := newTestModel(loader, &fakeRunner{})
	model.snapshot = makeSnapshot("one")
	model.haveSnap = true
	model.tree = NewTree(model.snapshot.Files)
	model.tree.ToggleCheck()
	model.mutator = &fakeMutator{}
	model.dialog = NewActionDialog(CommitDialog)
	model.dialog.SetDraft("stale draft", nil)
	model, cmd := model.updateDialogKey(tea.KeyMsg{Type: tea.KeyCtrlR})
	if cmd == nil {
		t.Fatal("regenerate did not produce a command")
	}
	msg := cmd()
	if _, ok := msg.(commitPrepMsg); !ok {
		t.Fatalf("msg = %+v, want commitPrepMsg", msg)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./ui/... -run 'TestConfirmCommitDialog|TestCommitResultMsg|TestRegenerateCommitDialog' -v`
Expected: FAIL — no type `commitResultMsg` (compile error); `updateDialogKey`'s current stub returns `nil` for confirm/regenerate either way, so even once it compiles the behavior is wrong until Step 3.

- [ ] **Step 3: Implement**

In `ui/model.go`, replace the Task 17 stub:

```go
func (m Model) updateDialogKey(key tea.KeyMsg) (Model, tea.Cmd) {
	action, cmd := m.dialog.Update(key)
	switch action {
	case ActionCancel:
		m.dialog = nil
		return m, nil
	case ActionConfirm, ActionRegenerate:
		// Task 19/21 (commit confirm) and Task 20 (regenerate) fill this in;
		// for now these are unreachable in practice because no key sets
		// m.dialog to non-nil until Task 18.
		return m, nil
	}
	return m, cmd
}
```

with:

```go
func (m Model) updateDialogKey(key tea.KeyMsg) (Model, tea.Cmd) {
	action, cmd := m.dialog.Update(key)
	switch action {
	case ActionCancel:
		m.dialog = nil
		return m, nil
	case ActionConfirm:
		return m.confirmDialogCmd()
	case ActionRegenerate:
		return m.regenerateDialogCmd()
	}
	return m, cmd
}

func (m Model) confirmDialogCmd() (Model, tea.Cmd) {
	if m.dialog == nil || !m.dialog.Ready || m.dialog.Err != nil {
		return m, nil
	}
	text := m.dialog.Text()
	switch m.dialog.Kind {
	case CommitDialog:
		mutator := m.mutator
		return m, func() tea.Msg { return commitResultMsg{Err: mutator.Commit(context.Background(), text)} }
	}
	return m, nil
}

func (m Model) regenerateDialogCmd() (Model, tea.Cmd) {
	if m.dialog == nil {
		return m, nil
	}
	switch m.dialog.Kind {
	case CommitDialog:
		return m, m.startCommitCmd()
	}
	return m, nil
}
```

Add the message type and a new `Update` case:

```go
type commitResultMsg struct{ Err error }
```

```go
	case commitResultMsg:
		if message.Err != nil {
			m.status = "commit failed: " + message.Err.Error()
			return m, nil
		}
		m.dialog = nil
		m.status = "committed"
		return m, m.refreshCmd()
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./ui/... -v -count=1`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add ui/model.go ui/model_test.go
git commit -m "feat(ui): confirm/regenerate commit dialog"
```

---

### Task 20: PR flow trigger (`o`)

**Files:**
- Modify: `ui/model.go`
- Modify: `ui/model_test.go`

**Interfaces:**
- Consumes: `Mutator.CurrentBranch`/`DefaultBranch` (Tasks 2, 6 wait — `DefaultBranch` is pre-existing on `git.Repository`, exposed here via `Mutator`); `SnapshotLoader.Snapshot` with `git.Branch` mode (existing); `pr.ExtractTicket` (Task 10); `Templates.RenderPRDescription` (Task 9); `pr.FormatTitle` (Task 10).
- Produces: `prPrepMsg{Ticket, Prompt string, Err error}`; `prDraftMsg{Ticket, Text string, Err error}`; `Model.startPRCmd() tea.Cmd`; `Model.runPRAgentCmd(renderedPrompt, ticket string) tea.Cmd`. Consumed by Task 21 (regenerate reuses `startPRCmd`).

- [ ] **Step 1: Write the failing tests**

Add to `ui/model_test.go`:

```go
func TestStartPRCmdBlocksOnDefaultBranch(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model.mutator = &fakeMutator{currentBranch: "main", defaultBranch: "main"}
	msg := model.startPRCmd()()
	prep, ok := msg.(prPrepMsg)
	if !ok || prep.Err == nil || !strings.Contains(prep.Err.Error(), "default branch") {
		t.Fatalf("msg = %+v", msg)
	}
}

func TestStartPRCmdPreparesPromptFromBranchDiff(t *testing.T) {
	loader := &fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one"), {ID: "branch-one", Mode: git.Branch, RawDiff: "branch-content\n"}}}
	model := newTestModel(loader, &fakeRunner{})
	model.mutator = &fakeMutator{currentBranch: "feature/869d6rn69-thing", defaultBranch: "main"}
	msg := model.startPRCmd()()
	prep, ok := msg.(prPrepMsg)
	if !ok || prep.Err != nil {
		t.Fatalf("msg = %+v", msg)
	}
	if prep.Ticket != "869d6rn69" || !strings.Contains(prep.Prompt, "branch-content") {
		t.Fatalf("prep = %+v", prep)
	}
}

func TestPRDraftMsgFormatsTitleWithTicket(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model.dialog = NewActionDialog(PRDialog)
	model, _ = model.Update(prDraftMsg{Ticket: "869d6rn69", Text: "Add OAuth login\nAdds login via OAuth provider."})
	text := model.dialog.Text()
	if !strings.HasPrefix(text, "CU-869d6rn69: Add OAuth login") {
		t.Fatalf("dialog text = %q", text)
	}
}

func TestOKeyStartsPRFlow(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model.mutator = &fakeMutator{currentBranch: "feature/x", defaultBranch: "main"}
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if cmd == nil {
		t.Fatal("o did not start the pr flow")
	}
	msg := cmd()
	if _, ok := msg.(prPrepMsg); !ok {
		t.Fatalf("msg = %+v, want prPrepMsg", msg)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./ui/... -run 'TestStartPRCmd|TestPRDraftMsg|TestOKeyStartsPRFlow' -v`
Expected: FAIL — no method `startPRCmd`, no type `prPrepMsg`/`prDraftMsg` (compile error).

- [ ] **Step 3: Implement**

Add to `ui/model.go`:

```go
type prPrepMsg struct {
	Ticket string
	Prompt string
	Err    error
}

type prDraftMsg struct {
	Ticket string
	Text   string
	Err    error
}

func (m Model) startPRCmd() tea.Cmd {
	mutator, loader, cfg, templates := m.mutator, m.loader, m.cfg, m.templates
	repoRoot := m.repo.Root
	return func() tea.Msg {
		ctx := context.Background()
		branch, err := mutator.CurrentBranch(ctx)
		if err != nil {
			return prPrepMsg{Err: err}
		}
		base, err := mutator.DefaultBranch(ctx)
		if err != nil {
			return prPrepMsg{Err: err}
		}
		if branch == base {
			return prPrepMsg{Err: fmt.Errorf("cannot open a pull request from the default branch %q", base)}
		}
		snapshot, err := loader.Snapshot(ctx, git.Branch)
		if err != nil {
			return prPrepMsg{Err: err}
		}
		ticket, err := pr.ExtractTicket(cfg.PR.TicketPattern, branch)
		if err != nil {
			return prPrepMsg{Err: err}
		}
		rendered, err := templates.RenderPRDescription(prompt.Context{
			Repository: repoRoot,
			Branch:     branch,
			BaseBranch: base,
			BranchDiff: snapshot.RawDiff,
			Ticket:     ticket,
		})
		if err != nil {
			return prPrepMsg{Err: err}
		}
		return prPrepMsg{Ticket: ticket, Prompt: rendered}
	}
}

func (m Model) runPRAgentCmd(renderedPrompt, ticket string) tea.Cmd {
	runner, repoRoot := m.runner, m.repo.Root
	return func() tea.Msg {
		var output strings.Builder
		err := runner.Run(context.Background(), agent.Request{RepoRoot: repoRoot, Prompt: renderedPrompt}, func(event agent.Event) {
			if event.Kind == agent.Output {
				if output.Len() > 0 {
					output.WriteByte('\n')
				}
				output.WriteString(event.Text)
			}
		})
		return prDraftMsg{Ticket: ticket, Text: output.String(), Err: err}
	}
}
```

Add `"fmt"` to `ui/model.go`'s imports if not already present. Add two `Update` cases next to the commit ones:

```go
	case prPrepMsg:
		if message.Err != nil {
			m.status = "pr prep error: " + message.Err.Error()
			return m, nil
		}
		m.dialog = NewActionDialog(PRDialog)
		return m, m.runPRAgentCmd(message.Prompt, message.Ticket)
	case prDraftMsg:
		if m.dialog == nil || m.dialog.Kind != PRDialog {
			return m, nil
		}
		text := message.Text
		if message.Err == nil {
			title, body := splitSubjectBody(text)
			title = pr.FormatTitle(message.Ticket, title)
			text = title + "\n\n" + body
		}
		m.dialog.SetDraft(text, message.Err)
		return m, nil
```

Add to `updateKey`'s switch:

```go
	case "o":
		return m, m.startPRCmd()
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./ui/... -v -count=1`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add ui/model.go ui/model_test.go
git commit -m "feat(ui): trigger PR flow on o"
```

---

### Task 21: PR flow confirm and regenerate

**Files:**
- Modify: `ui/model.go` (extend `confirmDialogCmd`/`regenerateDialogCmd` from Task 19 with a `PRDialog` case)
- Modify: `ui/model_test.go`

**Interfaces:**
- Consumes: `Mutator.Push`/`RemoteURL`/`DefaultBranch`/`CurrentBranch` (Tasks 2, 6); `pr.CompareURL` (Task 11); `pr.Opener.Open` (Task 12, via `Model.opener`).
- Produces: `prResultMsg{Err error}`; `Model.confirmPRCmd(title, body string) tea.Cmd`. Nothing later depends on this — it's the last piece of the PR flow.

- [ ] **Step 1: Write the failing tests**

Add to `ui/model_test.go`:

```go
func TestConfirmPRDialogPushesBuildsURLAndOpens(t *testing.T) {
	loader := &fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}
	model := newTestModel(loader, &fakeRunner{})
	mutator := &fakeMutator{currentBranch: "feature/869d6rn69-thing", defaultBranch: "main", remoteURL: "git@github.com:alex-irvine/lazydiff.git"}
	opener := &fakeOpener{}
	model.mutator = mutator
	model.opener = opener
	model.dialog = NewActionDialog(PRDialog)
	model.dialog.SetDraft("CU-869d6rn69: Add OAuth login\n\nAdds login via OAuth provider.", nil)
	model, cmd := model.updateDialogKey(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("confirm did not produce a command")
	}
	msg := cmd()
	result, ok := msg.(prResultMsg)
	if !ok || result.Err != nil {
		t.Fatalf("msg = %+v", msg)
	}
	if len(mutator.pushed) != 1 || mutator.pushed[0] != "origin/feature/869d6rn69-thing" {
		t.Fatalf("pushed = %v", mutator.pushed)
	}
	if len(opener.urls) != 1 || !strings.Contains(opener.urls[0], "alex-irvine/lazydiff/compare/main...feature/869d6rn69-thing") {
		t.Fatalf("opened urls = %v", opener.urls)
	}
}

func TestPRResultMsgClosesDialogOnSuccess(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model.dialog = NewActionDialog(PRDialog)
	model, _ = model.Update(prResultMsg{})
	if model.dialog != nil {
		t.Fatal("dialog should close after a successful PR open")
	}
}

func TestPRResultMsgErrorKeepsDialogOpen(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model.dialog = NewActionDialog(PRDialog)
	model, _ = model.Update(prResultMsg{Err: fmt.Errorf("push rejected")})
	if model.dialog == nil {
		t.Fatal("dialog should stay open on push failure")
	}
	if !strings.Contains(model.status, "push rejected") {
		t.Fatalf("status = %q", model.status)
	}
}

func TestRegeneratePRDialogRerunsStartPRCmd(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one"), {ID: "b", Mode: git.Branch, RawDiff: "x"}}}, &fakeRunner{})
	model.mutator = &fakeMutator{currentBranch: "feature/x", defaultBranch: "main"}
	model.dialog = NewActionDialog(PRDialog)
	model.dialog.SetDraft("stale draft", nil)
	model, cmd := model.updateDialogKey(tea.KeyMsg{Type: tea.KeyCtrlR})
	if cmd == nil {
		t.Fatal("regenerate did not produce a command")
	}
	msg := cmd()
	if _, ok := msg.(prPrepMsg); !ok {
		t.Fatalf("msg = %+v, want prPrepMsg", msg)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./ui/... -run 'TestConfirmPRDialog|TestPRResultMsg|TestRegeneratePRDialog' -v`
Expected: FAIL — no type `prResultMsg`, no method `confirmPRCmd` (compile error).

- [ ] **Step 3: Implement**

Add to `ui/model.go`:

```go
type prResultMsg struct{ Err error }

func (m Model) confirmPRCmd(title, body string) tea.Cmd {
	mutator, opener := m.mutator, m.opener
	return func() tea.Msg {
		ctx := context.Background()
		branch, err := mutator.CurrentBranch(ctx)
		if err != nil {
			return prResultMsg{Err: err}
		}
		if err := mutator.Push(ctx, "origin", branch); err != nil {
			return prResultMsg{Err: err}
		}
		remoteURL, err := mutator.RemoteURL(ctx, "origin")
		if err != nil {
			return prResultMsg{Err: err}
		}
		base, err := mutator.DefaultBranch(ctx)
		if err != nil {
			return prResultMsg{Err: err}
		}
		compareURL, err := pr.CompareURL(remoteURL, base, branch, title, body)
		if err != nil {
			return prResultMsg{Err: err}
		}
		if err := opener.Open(ctx, compareURL); err != nil {
			return prResultMsg{Err: err}
		}
		return prResultMsg{}
	}
}
```

Extend `confirmDialogCmd` (Task 19) with a `PRDialog` case:

```go
func (m Model) confirmDialogCmd() (Model, tea.Cmd) {
	if m.dialog == nil || !m.dialog.Ready || m.dialog.Err != nil {
		return m, nil
	}
	text := m.dialog.Text()
	switch m.dialog.Kind {
	case CommitDialog:
		mutator := m.mutator
		return m, func() tea.Msg { return commitResultMsg{Err: mutator.Commit(context.Background(), text)} }
	case PRDialog:
		title, body := splitSubjectBody(text)
		return m, m.confirmPRCmd(title, body)
	}
	return m, nil
}
```

Extend `regenerateDialogCmd` (Task 19) with a `PRDialog` case:

```go
func (m Model) regenerateDialogCmd() (Model, tea.Cmd) {
	if m.dialog == nil {
		return m, nil
	}
	switch m.dialog.Kind {
	case CommitDialog:
		return m, m.startCommitCmd()
	case PRDialog:
		return m, m.startPRCmd()
	}
	return m, nil
}
```

Add a new `Update` case:

```go
	case prResultMsg:
		if message.Err != nil {
			m.status = "pr failed: " + message.Err.Error()
			return m, nil
		}
		m.dialog = nil
		m.status = "opened pull request in browser"
		return m, nil
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./ui/... -v -count=1`
Expected: all PASS

- [ ] **Step 5: Run the whole repo once more**

Run: `go build ./... && go test ./... -count=1 && go vet ./...`
Expected: all PASS — this is the first point where the entire feature's `ui`-level logic is complete end-to-end.

- [ ] **Step 6: Commit**

```bash
git add ui/model.go ui/model_test.go
git commit -m "feat(ui): confirm/regenerate PR dialog (push, compare URL, open browser)"
```

---

### Task 22: Help text and status line

**Files:**
- Modify: `ui/render.go` (`helpText`, `statusLine`)
- Modify: `ui/model_test.go`

**Interfaces:**
- Consumes: nothing new — pure string content update.
- Produces: nothing new — last task that touches discoverability text, done last so it reflects every key added in Tasks 13-21.

- [ ] **Step 1: Write the failing test**

Add to `ui/model_test.go`:

```go
func TestHelpTextReflectsNewKeybindings(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	help := model.helpText()
	for _, want := range []string{
		"Toggle check for staging",
		"Check / uncheck all",
		"Stage checked items and commit",
		"Push and open pull request",
		"Cancel running analysis",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("help text missing %q:\n%s", want, help)
		}
	}
	if strings.Contains(help, "Toggle expand directory") {
		t.Fatal("help text still references the removed space binding")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./ui/... -run TestHelpTextReflectsNewKeybindings -v`
Expected: FAIL — help text still says "Toggle expand directory" and is missing the new lines.

- [ ] **Step 3: Implement**

In `ui/render.go`, replace `helpText`'s `lines` slice:

```go
	lines := []string{
		section("Navigation"),
		key("1 / 2 / 3", "Focus files / diff / analysis pane"),
		key("tab", "Cycle focus forward"),
		key("j / k", "Navigate tree / scroll diff"),
		key("space", "Toggle expand directory"),
		key("h / l", "Collapse / expand tree node"),
		key("g / G", "Scroll to top / bottom"),
		"",
		section("Analysis"),
		key("a / A", "Overall / detail review"),
		key("[/]", "Switch analysis tab"),
		key("c", "Cancel running analysis"),
		"",
		section("General"),
		key("m", "Toggle diff mode"),
		key("r", "Refresh snapshot"),
		key("u", "Check for update"),
		key("?", "Close this help"),
		key("q", "Quit"),
		"",
		dim.Render("  lazydiff " + version.Current),
	}
```

with:

```go
	lines := []string{
		section("Navigation"),
		key("1 / 2 / 3", "Focus files / diff / analysis pane"),
		key("tab", "Cycle focus forward"),
		key("j / k", "Navigate tree / scroll diff"),
		key("h / l", "Collapse / expand tree node"),
		key("g / G", "Scroll to top / bottom"),
		"",
		section("Staging"),
		key("space", "Toggle check for staging (working tree mode)"),
		key("ctrl+a", "Check / uncheck all"),
		key("c", "Stage checked items and commit"),
		key("o", "Push and open pull request"),
		"",
		section("Analysis"),
		key("a / A", "Overall / detail review"),
		key("[/]", "Switch analysis tab"),
		key("x", "Cancel running analysis"),
		"",
		section("General"),
		key("m", "Toggle diff mode"),
		key("r", "Refresh snapshot"),
		key("u", "Check for update"),
		key("?", "Close this help"),
		key("q", "Quit"),
		"",
		dim.Render("  lazydiff " + version.Current),
	}
```

In `statusLine`, change:

```go
	return fmt.Sprintf("mode: %s  %s  %s  %s%s  %s", m.mode, deltaState, m.status, "[1-3] pane  [/] tab  [a] overall  [A] detail  [m] mode  [?] help  [q] quit", updateHint, version.Current)
```

to:

```go
	return fmt.Sprintf("mode: %s  %s  %s  %s%s  %s", m.mode, deltaState, m.status, "[1-3] pane  [space] check  [c] commit  [o] PR  [?] help  [q] quit", updateHint, version.Current)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./ui/... -v -count=1`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add ui/render.go ui/model_test.go
git commit -m "docs(ui): update help text and status line for staging keys"
```

---

### Task 23: PTY integration tests

**Files:**
- Modify: `integration/pty_linux_test.go`

**Interfaces:**
- Consumes: the fully-wired real `lazydiff` binary (Tasks 1-22, all packages together); the existing `newFixture`/`run`/`readUntil` helpers.
- Produces: nothing new — this is the final end-to-end validation.

This task is scoped to what's realistically testable through a real compiled binary in a real (but network-less) temp repo:

- **Commit flow end-to-end**: no remote needed — staging and committing are pure local git operations. This is testable in full, including a real `git log` assertion afterward.
- **PR flow**: pushing to a real `github.com` remote isn't achievable in CI (no network, no credentials), and `pr.CompareURL` deliberately rejects non-`github.com` hosts, so a fixture-local bare repo as `origin` can't reach the "browser opens with the right URL" assertion either. That claim is already covered by `pr/url_test.go` (Task 11) and `TestConfirmPRDialogPushesBuildsURLAndOpens` (Task 21) with fakes. At the PTY level, this task instead verifies the dialog opens and cancels cleanly on a non-default branch, which exercises the real keybinding → real staged-diff-fetch → real agent subprocess → real dialog render path without needing a reachable remote.

- [ ] **Step 1: Write the failing test — commit flow**

Add to `integration/pty_linux_test.go`:

```go
func TestPTYCheckFileCommitFlow(t *testing.T) {
	fixture := newFixture(t)
	cmd := exec.Command(fixture.binary, "-config", fixture.config)
	cmd.Dir = fixture.root
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(fixture.delta)+":"+os.Getenv("PATH"))
	terminal, err := pty.Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	if err := pty.Setsize(terminal, &pty.Winsize{Cols: 120, Rows: 40}); err != nil {
		t.Fatal(err)
	}
	_ = readUntil(t, terminal, "delta-output", 3*time.Second)
	if _, err := terminal.Write([]byte(" ")); err != nil { // check main.go (cursor starts there)
		t.Fatal(err)
	}
	if _, err := terminal.Write([]byte("c")); err != nil { // stage + generate commit message
		t.Fatal(err)
	}
	output := readUntil(t, terminal, "analysis-output", 5*time.Second)
	if !strings.Contains(output, "analysis-output") {
		t.Fatalf("commit dialog did not show the generated draft:\n%s", output)
	}
	if _, err := terminal.Write([]byte{19}); err != nil { // ctrl+s
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond) // let the commit actually land before quitting
	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	log := runOutput(t, fixture.root, "log", "--oneline", "-1", "--pretty=%B")
	if !strings.Contains(log, "analysis-output") {
		t.Fatalf("expected a new commit with the fake agent's output, git log = %q", log)
	}
}

func runOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}
```

`ctrl+s` is byte value 19 (0x13) in raw terminal input — bubbletea reads it directly off the PTY, no separate translation needed, matching how this same file already writes raw bytes like `"q"`/`"A"` for other keys.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./integration/... -run TestPTYCheckFileCommitFlow -v`
Expected: FAIL — before Tasks 1-22 are all done, `space`/`c` don't do this yet; run this only after Task 22, where it should already pass since it's pure verification of already-implemented behavior. (If run earlier for TDD purposes on its own, it fails because there's no commit-dialog behavior yet to produce "analysis-output" a second time in this way.)

- [ ] **Step 3: Write the failing test — PR dialog open/cancel**

Add to `integration/pty_linux_test.go`:

```go
func TestPTYOpenPRDialogAndCancel(t *testing.T) {
	fixture := newFixture(t)
	run(t, fixture.root, "git", "checkout", "-b", "feature/869d6rn69-thing")
	cmd := exec.Command(fixture.binary, "-config", fixture.config)
	cmd.Dir = fixture.root
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(fixture.delta)+":"+os.Getenv("PATH"))
	terminal, err := pty.Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	if err := pty.Setsize(terminal, &pty.Winsize{Cols: 120, Rows: 40}); err != nil {
		t.Fatal(err)
	}
	_ = readUntil(t, terminal, "delta-output", 3*time.Second)
	if _, err := terminal.Write([]byte("o")); err != nil {
		t.Fatal(err)
	}
	output := readUntil(t, terminal, "analysis-output", 5*time.Second)
	if !strings.Contains(output, "analysis-output") {
		t.Fatalf("pr dialog did not show the generated draft:\n%s", output)
	}
	if _, err := terminal.Write([]byte{27}); err != nil { // esc
		t.Fatal(err)
	}
	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("lazydiff exit: %v", err)
	}
}
```

`esc` is byte value 27 (0x1b).

- [ ] **Step 4: Run tests to verify they fail appropriately, then pass**

Run: `go build -o /tmp/lazydiff-pty-check ./cmd/lazydiff && go test ./integration/... -v -count=1`
Expected: both new tests PASS (all of Tasks 1-22 are already in place by this point in the plan, so there's no red step here beyond confirming the assertions are correctly written — if either fails, the bug is in Tasks 1-22, not in this test file).

- [ ] **Step 5: Run the full verification gate from the README/CI**

Run: `go test ./... -count=1 && go vet ./... && go build ./... && git diff --check`
Expected: all PASS — this matches the exact sequence `.github/workflows/ci.yml` runs, confirming the whole feature is CI-clean.

- [ ] **Step 6: Commit**

```bash
git add integration/pty_linux_test.go
git commit -m "test(integration): cover commit flow and PR dialog end-to-end"
```

---

## Plan Self-Review

**Spec coverage:** Every section of `2026-07-27-lazydiff-stage-commit-pr-design.md` maps to a task — Data Model (13-14), Staging Mechanism (1-6, 7), Commit Flow (18-19), PR Flow (20-21), Config Additions (8-9), TUI Interaction/Keymap (17, 22), Architecture package summary (all), Error Handling (each flow's `Err`-carrying message types + status-line surfacing, checked task-by-task above), Testing Strategy (unit tests throughout, subprocess-integration in Tasks 3-6, PTY in Task 23).

**Placeholder scan:** No task defers logic to "later" — Task 17 deliberately stops short of confirm/regenerate behavior, but does so by *not adding* the `c`/`o` keys at all yet (nothing compiles-but-does-nothing), and Task 19's replaced stub is shown as an explicit before/after diff, not a TODO.

**Type consistency:** `Mutator` (Task 17) is satisfied structurally by `git.Repository` once Tasks 2-6 exist — verified by `main.go` passing `repo` directly as the argument, which only compiles if the method set matches exactly (same param/return types throughout: `context.Context` first, `string`/`string, string`/`string` params, `(string, error)` or `error` returns). `commitPrepMsg`/`commitDraftMsg`/`commitResultMsg` and `prPrepMsg`/`prDraftMsg`/`prResultMsg` are each introduced once (Tasks 18-21) and never redefined. `splitSubjectBody` (Task 18) is reused as-is by the PR flow (Task 20) rather than duplicated.

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-07-27-lazydiff-stage-commit-pr-implementation-plan.md`. Two execution options:

1. **Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.
2. **Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints.

Which approach?
