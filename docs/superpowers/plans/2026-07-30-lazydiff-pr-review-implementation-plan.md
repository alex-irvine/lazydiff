# PR Review Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the user browse open pull requests, review a PR's diff, and perform review actions (approve, request changes, merge, close + delete branch) from lazydiff's TUI, all through the `gh` CLI.

**Architecture:** A new `pr.GitHub` wrapper (reusing `git.CommandRunner`) handles PR discovery and mutation. A new `ui.PRSelector` model mirrors `BranchSelector`. The `TreeMode` enum grows `TreeModePRSelector`/`TreeModePRDiff`; `SnapshotLoader` gains `SnapshotPR`; a new `PRReviewer` interface (satisfied by `pr.GitHub`) is wired alongside `Mutator`. `cmd/lazydiff/main.go` is the composition root: it constructs `pr.GitHub` from the origin remote URL and implements `SnapshotPR` (snapshot construction lives in the caller, keeping the `git` package free of `pr` imports — per spec open decision #2).

**Tech Stack:** Go 1.24 + Bubble Tea, no new external dependencies. `gh` CLI must be on `PATH` (already used by `version` for release checks/downloads).

**Spec drift notes (code is truth, spec text is stale here):**
- Current `[`/`]` cycle is `worktree ↔ branch selector` — `staged` was removed from the cycle in `a2d9b72` (parent of the spec commit). This plan extends the **actual** cycle: `worktree → branch selector → PR selector`. `TreeModeStaged` stays reachable only via `Snapshot(git.Staged)` internals, never via `[`/`]`.
- The tab bar currently has 2 tabs (`Worktree | Branch`); this plan adds a third: `Worktree | Branch | PRs`. (Spec's "fourth mode" text predates the staged removal.)
- `PRReviewer` additionally gains `ListPRs` (needed by the PR-list load command) and `pr.GitHub` gains `PR(ctx, number)` via `gh pr view` (needed by `SnapshotPR` for base/head ref names). Neither was in the spec's literal method table.
- `pr` now imports `git` (for `CommandRunner`) — one-way, no cycle; the "independent leaf" description in AGENTS.md becomes "depends only on git".

## Global Constraints

- No table-driven tests / no `t.Run` subtests — every case is its own `TestXxx` func.
- Fakes, not mocks — `fakeRunner` for `git.CommandRunner`, `fakeLoader`/`fakePRReviewer`/`fakeMutator`/`fakeOpener`/`fakeRenderer` for `ui` interfaces.
- `go test ./... -count=1` then `go vet ./...` then `go build ./...` then `git diff --check` to verify after all tasks.
- Conventional Commits (`feat(scope): ...`, `refactor(scope): ...`, `test(scope): ...`).
- Model methods always use value receiver and return `(Model, tea.Cmd)` — never mutate in place with pointer receiver.
- Every `pr.GitHub` method first validates the remote is `github.com` (`requireGitHub`) and returns a blocking error otherwise.
- All PR mutation/read calls go through `pr.GitHub`'s `CommandRunner` — nothing shells out directly from `ui`.
- `ga`/`gr`/`gm`/`gd` are two-key sequences via a `pendingPRKey` field; they only fire when `focus == FocusTree && treeMode == TreeModePRDiff && prSelector.selectedPR != nil`.

---

### Task 1: Export git.ExecRunner

**Files:**
- Modify: `git/repository.go` — rename `execRunner` → `ExecRunner` (exported)

`cmd/lazydiff/main.go` must construct a `pr.GitHub` with a real `CommandRunner`; `execRunner` is currently unexported, so export it.

- [ ] **Step 1: Rename `execRunner` to `ExecRunner`**

In `git/repository.go`: `type execRunner struct{}` → `type ExecRunner struct{}`; update the three receiver declarations (`Run`, `RunWithStdin`) and the two internal uses (`Open`, `Repository.run`'s nil-default).

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: no error

- [ ] **Step 3: Commit**

```bash
git add git/repository.go
git commit -m "refactor(git): export ExecRunner for reuse outside git"
```

---

### Task 2: pr.PR type and pr.GitHub wrapper

**Files:**
- Create: `pr/types.go`
- Create: `pr/gh.go`
- Create: `pr/gh_test.go`

**Interfaces:**
- Consumes: `git.CommandRunner` (existing), `ownerRepo` (existing unexported helper in `pr/url.go`)
- Produces: `pr.PR`, `pr.GitHub` with `NewGitHub`, `ListPRs`, `PR`, `PRDiff`, `Approve`, `RequestChanges`, `Merge`, `Close`, `DeleteBranch`

- [ ] **Step 1: Write failing tests**

```go
// pr/gh_test.go
package pr

import (
	"context"
	"fmt"
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
	return nil, fmt.Errorf("unexpected command %q", key)
}

func (f *fakeRunner) RunWithStdin(context.Context, interface{}, string, ...string) ([]byte, error) {
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
```

Note: `RunWithStdin` takes `io.Reader` — declare the import and use `io.Reader` in the fake's signature; the no-op body stays the same.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pr -run TestGitHub -v -count=1`
Expected: FAIL — package `pr` has no `GitHub`

- [ ] **Step 3: Write pr/types.go and pr/gh.go**

```go
// pr/types.go
package pr

type PR struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Author      string `json:"author"`
	HeadRefName string `json:"headRefName"`
	BaseRefName string `json:"baseRefName"`
	URL         string `json:"url"`
	Mergeable   string `json:"mergeable"`
	CreatedAt   string `json:"createdAt"`
}
```

```go
// pr/gh.go
package pr

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/alex-irvine/lazydiff/git"
)

// GitHub wraps the gh CLI for PR discovery and review actions. All methods
// require a github.com remote (validated per call); anything else errors.
type GitHub struct {
	Runner    git.CommandRunner
	remoteURL string
}

func NewGitHub(remoteURL string, runner git.CommandRunner) *GitHub {
	return &GitHub{Runner: runner, remoteURL: remoteURL}
}

func (g *GitHub) run(ctx context.Context, args ...string) ([]byte, error) {
	if g.Runner == nil {
		return nil, fmt.Errorf("gh runner unavailable")
	}
	return g.Runner.Run(ctx, "gh", args...)
}

func (g *GitHub) requireGitHub() error {
	host, _, _, err := ownerRepo(g.remoteURL)
	if err != nil {
		return fmt.Errorf("PR review requires a github.com remote: %w", err)
	}
	if host != "github.com" {
		return fmt.Errorf("remote host %q is not github.com; PR review requires github.com", host)
	}
	return nil
}

const prJSONFields = "number,title,author,headRefName,baseRefName,mergeable,url,createdAt"

func (g *GitHub) ListPRs(ctx context.Context, state string) ([]PR, error) {
	if err := g.requireGitHub(); err != nil {
		return nil, err
	}
	out, err := g.run(ctx, "pr", "list", "--state", state, "--json", prJSONFields)
	if err != nil {
		return nil, fmt.Errorf("gh pr list: %w", err)
	}
	var prs []PR
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("parse gh pr list: %w", err)
	}
	return prs, nil
}

func (g *GitHub) PR(ctx context.Context, number int) (PR, error) {
	if err := g.requireGitHub(); err != nil {
		return PR{}, err
	}
	out, err := g.run(ctx, "pr", "view", strconv.Itoa(number), "--json", prJSONFields)
	if err != nil {
		return PR{}, fmt.Errorf("gh pr view #%d: %w", number, err)
	}
	var p PR
	if err := json.Unmarshal(out, &p); err != nil {
		return PR{}, fmt.Errorf("parse gh pr view: %w", err)
	}
	return p, nil
}

func (g *GitHub) PRDiff(ctx context.Context, number int) (string, error) {
	if err := g.requireGitHub(); err != nil {
		return "", err
	}
	out, err := g.run(ctx, "pr", "diff", strconv.Itoa(number), "--patch")
	if err != nil {
		return "", fmt.Errorf("gh pr diff #%d: %w", number, err)
	}
	return string(out), nil
}

func (g *GitHub) Approve(ctx context.Context, number int, comment string) error {
	if err := g.requireGitHub(); err != nil {
		return err
	}
	args := []string{"pr", "review", strconv.Itoa(number), "--approve"}
	if comment != "" {
		args = append(args, "--body", comment)
	}
	if _, err := g.run(ctx, args...); err != nil {
		return fmt.Errorf("approve PR #%d: %w", number, err)
	}
	return nil
}

func (g *GitHub) RequestChanges(ctx context.Context, number int, body string) error {
	if err := g.requireGitHub(); err != nil {
		return err
	}
	if _, err := g.run(ctx, "pr", "review", strconv.Itoa(number), "--request-changes", "--body", body); err != nil {
		return fmt.Errorf("request changes on PR #%d: %w", number, err)
	}
	return nil
}

func (g *GitHub) Merge(ctx context.Context, number int) error {
	if err := g.requireGitHub(); err != nil {
		return err
	}
	if _, err := g.run(ctx, "pr", "merge", strconv.Itoa(number), "--merge"); err != nil {
		return fmt.Errorf("merge PR #%d: %w", number, err)
	}
	return nil
}

func (g *GitHub) Close(ctx context.Context, number int) error {
	if err := g.requireGitHub(); err != nil {
		return err
	}
	if _, err := g.run(ctx, "pr", "close", strconv.Itoa(number)); err != nil {
		return fmt.Errorf("close PR #%d: %w", number, err)
	}
	return nil
}

// DeleteBranch removes the PR's remote branch via `git push origin --delete`.
// Direct exec through the runner (not through git.Repository) per spec.
func (g *GitHub) DeleteBranch(ctx context.Context, branch string) error {
	if err := g.requireGitHub(); err != nil {
		return err
	}
	if _, err := g.Runner.Run(ctx, "git", "push", "origin", "--delete", branch); err != nil {
		return fmt.Errorf("delete remote branch %q: %w", branch, err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pr -run TestGitHub -v -count=1`
Expected: PASS

- [ ] **Step 5: Run all pr tests**

Run: `go test ./pr -count=1`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pr/types.go pr/gh.go pr/gh_test.go
git commit -m "feat(pr): add GitHub CLI wrapper for PR review"
```

---

### Task 3: ui.PRSelector model

**Files:**
- Create: `ui/pr_selector.go`
- Create: `ui/pr_selector_test.go`

**Interfaces:**
- Consumes: `pr.PR` (Task 2)
- Produces: `*PRSelector` with `NewPRSelector`, `Move`, `Select`, `Selected`, `Rows`, `Cursor`

- [ ] **Step 1: Write failing tests**

```go
// ui/pr_selector_test.go
package ui

import (
	"testing"

	"github.com/alex-irvine/lazydiff/pr"
)

func makeTestPRs() []pr.PR {
	return []pr.PR{
		{Number: 42, Title: "feat: add login", Author: "alex", HeadRefName: "feat-login", BaseRefName: "main", Mergeable: "MERGEABLE"},
		{Number: 7, Title: "fix: typo", Author: "sam", HeadRefName: "fix-typo", BaseRefName: "main", Mergeable: "CONFLICTING"},
	}
}

func TestPRSelectorStartsAtFirstPR(t *testing.T) {
	ps := NewPRSelector(makeTestPRs())
	if ps.cursor != 0 || ps.Selected() == nil || ps.Selected().Number != 42 {
		t.Fatalf("selected = %+v", ps.Selected())
	}
}

func TestPRSelectorMoveClamps(t *testing.T) {
	ps := NewPRSelector(makeTestPRs())
	ps.Move(5)
	if ps.Selected().Number != 7 {
		t.Fatalf("after Move(5) selected = %+v", ps.Selected())
	}
	ps.Move(-5)
	if ps.Selected().Number != 42 {
		t.Fatalf("after Move(-5) selected = %+v", ps.Selected())
	}
}

func TestPRSelectorSelectByNumber(t *testing.T) {
	ps := NewPRSelector(makeTestPRs())
	ps.Select(7)
	if ps.selectedPR == nil || ps.selectedPR.Number != 7 || ps.selectedPR.Title != "fix: typo" {
		t.Fatalf("selectedPR = %+v", ps.selectedPR)
	}
}

func TestPRSelectorRowsReturnsCopy(t *testing.T) {
	ps := NewPRSelector(makeTestPRs())
	rows := ps.Rows()
	rows[0].Title = "mutated"
	if ps.prs[0].Title == "mutated" {
		t.Fatal("Rows returned the backing slice")
	}
}

func TestPRSelectorEmptyList(t *testing.T) {
	ps := NewPRSelector(nil)
	if ps.Selected() != nil {
		t.Fatal("expected nil selection on empty list")
	}
	ps.Move(1) // must not panic
}

func TestPRSelectorDiffCacheInitialized(t *testing.T) {
	ps := NewPRSelector(makeTestPRs())
	if ps.diffCache == nil {
		t.Fatal("diffCache should be initialized")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./ui -run TestPRSelector -v -count=1`
Expected: FAIL — undefined `NewPRSelector`

- [ ] **Step 3: Write minimal implementation**

```go
// ui/pr_selector.go
package ui

import "github.com/alex-irvine/lazydiff/git"
import "github.com/alex-irvine/lazydiff/pr"

type PRSelector struct {
	prs        []pr.PR
	cursor     int
	selectedPR *pr.PR  // nil while browsing; set when the user opens a PR diff
	diffCache  map[int]git.Snapshot
	err        error  // last PR-list load error, shown inline
}

func NewPRSelector(prs []pr.PR) *PRSelector {
	return &PRSelector{
		prs:       append([]pr.PR(nil), prs...),
		diffCache: make(map[int]git.Snapshot),
	}
}

func (s *PRSelector) Move(delta int) {
	s.cursor += delta
	if s.cursor < 0 {
		s.cursor = 0
	}
	if s.cursor >= len(s.prs) {
		s.cursor = len(s.prs) - 1
	}
}

// Select marks the PR with the given number as the reviewed PR (no-op if
// not present in the list).
func (s *PRSelector) Select(number int) {
	for i := range s.prs {
		if s.prs[i].Number == number {
			p := s.prs[i]
			s.selectedPR = &p
			return
		}
	}
}

func (s *PRSelector) Selected() *pr.PR {
	if s.cursor < 0 || s.cursor >= len(s.prs) {
		return nil
	}
	return &s.prs[s.cursor]
}

func (s *PRSelector) Rows() []pr.PR {
	return append([]pr.PR(nil), s.prs...)
}

func (s *PRSelector) Cursor() int { return s.cursor }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./ui -run TestPRSelector -v -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add ui/pr_selector.go ui/pr_selector_test.go
git commit -m "feat(ui): add PRSelector model for PR list"
```

---

### Task 4: ui.ConfirmDialog for simple PR confirmations

**Files:**
- Modify: `ui/dialog.go` — add `DialogKind` values + `ConfirmDialog` struct
- Modify: `ui/dialog_test.go` — add tests

Spec open decision: new `ConfirmDialog` struct (no textarea — approve/merge/close have no text to edit). Stored in a separate `Model.confirm` field, distinct from the textarea-based `dialog`.

- [ ] **Step 1: Write failing tests**

```go
// ui/dialog_test.go — append
func TestConfirmDialogEscCancels(t *testing.T) {
	d := NewConfirmDialog(ApproveDialog, "Approve PR #42 (feat: add login)")
	if action, _ := d.Update(tea.KeyMsg{Type: tea.KeyEsc}); action != ActionCancel {
		t.Fatalf("esc action = %v", action)
	}
}

func TestConfirmDialogCtrlSConfirms(t *testing.T) {
	d := NewConfirmDialog(MergeDialog, "Merge PR #42 (feat: add login)")
	if action, _ := d.Update(tea.KeyMsg{Type: tea.KeyCtrlS}); action != ActionConfirm {
		t.Fatalf("ctrl+s action = %v", action)
	}
}

func TestConfirmDialogStoresTitleAndKind(t *testing.T) {
	d := NewConfirmDialog(ClosePRDialog, "Close PR #42 (feat: add login) + delete branch feat-login")
	if d.Kind != ClosePRDialog || d.Title != "Close PR #42 (feat: add login) + delete branch feat-login" {
		t.Fatalf("dialog = %+v", d)
	}
}

func TestRequestChangesDialogKindReadyImmediately(t *testing.T) {
	d := NewActionDialog(RequestChangesDialog)
	d.SetDraft("", nil)
	if !d.Ready || d.Err != nil {
		t.Fatalf("ready = %v, err = %v", d.Ready, d.Err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./ui -run "TestConfirmDialog|TestRequestChangesDialog" -v -count=1`
Expected: FAIL — undefined `NewConfirmDialog`, `ApproveDialog`

- [ ] **Step 3: Extend dialog.go**

```go
// Add to the DialogKind const block in ui/dialog.go:
const (
	CommitDialog DialogKind = iota
	PRDialog
	ApproveDialog
	RequestChangesDialog
	MergeDialog
	ClosePRDialog
)
```

```go
// ConfirmDialog is a simple title + hint modal with no editable text,
// used for approve/merge/close+delete confirmations. Err holds a
// mutation failure, if any (the dialog stays open so the user can retry
// or cancel).
type ConfirmDialog struct {
	Kind  DialogKind
	Title string
	Err   error
}

func NewConfirmDialog(kind DialogKind, title string) *ConfirmDialog {
	return &ConfirmDialog{Kind: kind, Title: title}
}

// Update maps esc/ctrl+s onto DialogAction; every other key is ignored.
func (d *ConfirmDialog) Update(msg tea.Msg) (DialogAction, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "esc":
			return ActionCancel, nil
		case "ctrl+s":
			return ActionConfirm, nil
		}
	}
	return ActionNone, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./ui -run "TestConfirmDialog|TestRequestChangesDialog" -v -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add ui/dialog.go ui/dialog_test.go
git commit -m "feat(ui): add ConfirmDialog for simple PR confirmations"
```

---

### Task 5: ui.Model wiring — PR modes, g-prefix keys, dialogs, PR diff loading

**Files:**
- Modify: `ui/model.go` — TreeMode additions, PRReviewer interface, Model fields, messages, key routing, refreshCmd, loadPRsCmd, dialog confirm/regenerate cases
- Modify: `ui/model_test.go` — fakeLoader.SnapshotPR, fakePRReviewer, newTestModel signature, updated cycle test, new PR tests

- [ ] **Step 1: Write failing tests (update fakes first)**

Update `fakeLoader` in `ui/model_test.go`:

```go
type fakeLoader struct {
	snapshots       []git.Snapshot
	branchSnapshots map[string]git.Snapshot
	prSnapshots     map[int]git.Snapshot
	index           int
	prCalls         int
}

func (f *fakeLoader) Snapshot(context.Context, git.Mode) (git.Snapshot, error) {
	snapshot := f.snapshots[f.index]
	if f.index < len(f.snapshots)-1 {
		f.index++
	}
	return snapshot, nil
}

func (f *fakeLoader) SnapshotBranch(_ context.Context, branch string) (git.Snapshot, error) {
	if f.branchSnapshots != nil {
		if s, ok := f.branchSnapshots[branch]; ok {
			return s, nil
		}
	}
	return f.snapshots[0], nil
}

func (f *fakeLoader) SnapshotPR(_ context.Context, number int) (git.Snapshot, error) {
	f.prCalls++
	if f.prSnapshots != nil {
		if s, ok := f.prSnapshots[number]; ok {
			return s, nil
		}
	}
	return f.snapshots[0], nil
}
```

Add `fakePRReviewer`:

```go
type fakePRReviewer struct {
	prs       []pr.PR
	listState string
	approved  []int
	commented []string
	requested []int
	merged    []int
	closed    []int
	deleted   []string
	err       error
}

func (f *fakePRReviewer) ListPRs(_ context.Context, state string) ([]pr.PR, error) {
	f.listState = state
	return f.prs, f.err
}

func (f *fakePRReviewer) Approve(_ context.Context, number int, comment string) error {
	f.approved = append(f.approved, number)
	f.commented = append(f.commented, comment)
	return f.err
}

func (f *fakePRReviewer) RequestChanges(_ context.Context, number int, body string) error {
	f.requested = append(f.requested, number)
	f.commented = append(f.commented, body)
	return f.err
}

func (f *fakePRReviewer) Merge(_ context.Context, number int) error {
	f.merged = append(f.merged, number)
	return f.err
}

func (f *fakePRReviewer) Close(_ context.Context, number int) error {
	f.closed = append(f.closed, number)
	return f.err
}

func (f *fakePRReviewer) DeleteBranch(_ context.Context, branch string) error {
	f.deleted = append(f.deleted, branch)
	return f.err
}
```

Update `newTestModel` (NewModel gains a `prReviewer PRReviewer` argument):

```go
func newTestModel(loader SnapshotLoader, runner agent.Runner) Model {
	cfg := config.Default()
	templates, err := prompt.Parse(prompt.Sources{ /* unchanged */ })
	if err != nil {
		panic(err)
	}
	return NewModel(git.Repository{Root: "/repo"}, cfg, loader, fakeRenderer{}, runner, templates, &fakeMutator{}, &fakeOpener{}, &fakePRReviewer{})
}
```

Add a test helper for a model sitting in PR diff mode:

```go
func modelInPRDiff(t *testing.T, loader SnapshotLoader, reviewer PRReviewer) Model {
	t.Helper()
	model := newTestModel(loader, &fakeRunner{})
	model.treeMode = TreeModePRDiff
	model.prSelector = NewPRSelector(makeTestPRs())
	model.prSelector.Select(42)
	model.prReviewer = reviewer
	return model
}
```

Now the new tests (each its own `TestXxx` func):

```go
func TestTreeModeCyclesThroughPRSelector(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if model.treeMode != TreeModeBranchSelector {
		t.Fatalf("after first ] treeMode = %d, want BranchSelector", model.treeMode)
	}
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if model.treeMode != TreeModePRSelector {
		t.Fatalf("after second ] treeMode = %d, want PRSelector", model.treeMode)
	}
	if cmd == nil {
		t.Fatal("expected loadPRsCmd on entering PR selector")
	}
	msg := cmd()
	prsMsg, ok := msg.(prsLoadedMsg)
	if !ok || len(prsMsg.PRs) != 2 {
		t.Fatalf("msg = %+v", msg)
	}
	model, _ = model.Update(prsMsg)
	if model.prSelector == nil || model.prSelector.Rows()[0].Number != 42 {
		t.Fatalf("prSelector = %+v", model.prSelector)
	}
	// backward wrap: [ from worktree goes to PR selector
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	if model.treeMode != TreeModePRSelector {
		t.Fatalf("backward treeMode = %d, want PRSelector", model.treeMode)
	}
}
```

Note: `TestTreeModeCyclesWithBracketKeys` (existing) must be rewritten to the new 3-mode cycle: `]` → BranchSelector → PRSelector → Worktree; `[` → PRSelector → BranchSelector → Worktree. Replace the old expectation that `]` from branch selector wraps straight to worktree.

```go
func TestPRSelectorEnterLoadsPRDiff(t *testing.T) {
	loader := &fakeLoader{
		snapshots: []git.Snapshot{makeSnapshot("one")},
		prSnapshots: map[int]git.Snapshot{
			42: makeSnapshot("pr42"),
		},
	}
	model := newTestModel(loader, &fakeRunner{})
	model.treeMode = TreeModePRSelector
	model.prSelector = NewPRSelector(makeTestPRs())
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.treeMode != TreeModePRDiff {
		t.Fatalf("treeMode = %d, want PRDiff", model.treeMode)
	}
	if model.prSelector.selectedPR == nil || model.prSelector.selectedPR.Number != 42 {
		t.Fatalf("selectedPR = %+v", model.prSelector.selectedPR)
	}
	if cmd == nil {
		t.Fatal("expected a cmd")
	}
	msg := cmd()
	diffMsg, ok := msg.(prDiffMsg)
	if !ok || diffMsg.Snapshot.ID != "pr42" {
		t.Fatalf("msg = %+v", msg)
	}
}

func TestPRDiffHCacheHitReturnsToSelector(t *testing.T) {
	loader := &fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}, prSnapshots: map[int]git.Snapshot{42: makeSnapshot("pr42")}}
	model := modelInPRDiff(t, loader, &fakePRReviewer{})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if model.treeMode != TreeModePRSelector {
		t.Fatalf("treeMode = %d, want PRSelector", model.treeMode)
	}
	// re-enter: refreshCmd must hit the cache, not the loader
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a cmd")
	}
	msg := cmd()
	if diffMsg, ok := msg.(prDiffMsg); !ok || diffMsg.Snapshot.ID != "pr42" {
		t.Fatalf("msg = %+v", msg)
	}
	if loader.prCalls != 0 {
		t.Fatalf("loader.prCalls = %d, want 0 (cache hit)", loader.prCalls)
	}
}

func TestPRDiffRefreshBypassesCache(t *testing.T) {
	loader := &fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}, prSnapshots: map[int]git.Snapshot{42: makeSnapshot("pr42")}}
	model := modelInPRDiff(t, loader, &fakePRReviewer{})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if len(model.prSelector.diffCache) != 0 {
		t.Fatal("r should clear the PR diff cache")
	}
}

func TestGKeyThenAOpensApproveConfirmDialog(t *testing.T) {
	model := modelInPRDiff(t, &fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakePRReviewer{})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if model.confirm == nil || model.confirm.Kind != ApproveDialog {
		t.Fatalf("confirm = %+v", model.confirm)
	}
	if !strings.Contains(model.confirm.Title, "Approve PR #42") {
		t.Fatalf("title = %q", model.confirm.Title)
	}
}

func TestGKeyThenROpensRequestChangesDialog(t *testing.T) {
	model := modelInPRDiff(t, &fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakePRReviewer{})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if model.dialog == nil || model.dialog.Kind != RequestChangesDialog || !model.dialog.Ready {
		t.Fatalf("dialog = %+v", model.dialog)
	}
}

// Global Constraints forbid table-driven tests/loops — each key gets its
// own TestXxx func with the body duplicated.
func TestGKeyThenMOpenMergeConfirmDialog(t *testing.T) {
	model := modelInPRDiff(t, &fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakePRReviewer{})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if model.confirm == nil || model.confirm.Kind != MergeDialog {
		t.Fatalf("confirm = %+v", model.confirm)
	}
}

func TestGKeyThenDOpenCloseConfirmDialog(t *testing.T) {
	model := modelInPRDiff(t, &fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakePRReviewer{})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if model.confirm == nil || model.confirm.Kind != ClosePRDialog {
		t.Fatalf("confirm = %+v", model.confirm)
	}
}

func TestGKeyInPRDiffStillScrollsWhenFocusIsDiff(t *testing.T) {
	model := modelInPRDiff(t, &fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakePRReviewer{})
	model.focus = FocusDiff
	model.diffScroll = 10
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if model.diffScroll != 0 {
		t.Fatalf("diffScroll = %d, want 0", model.diffScroll)
	}
	if model.pendingPRKey != "" {
		t.Fatal("pending key should not be set outside tree focus")
	}
}

func TestGKeyNotInPRDiffDoesNotSetPending(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model.snapshot = makeSnapshot("one")
	model.haveSnap = true
	model.tree = NewTree(model.snapshot.Files)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if model.pendingPRKey != "" {
		t.Fatal("pending key set outside PR diff mode")
	}
}

func TestConfirmApproveCallsReviewerApprove(t *testing.T) {
	reviewer := &fakePRReviewer{}
	model := modelInPRDiff(t, &fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, reviewer)
	model.confirm = NewConfirmDialog(ApproveDialog, "Approve PR #42 (feat: add login)")
	model, cmd := model.updateConfirmKey(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("confirm did not produce a command")
	}
	msg := cmd()
	result, ok := msg.(prActionMsg)
	if !ok || result.Err != nil {
		t.Fatalf("msg = %+v", msg)
	}
	if len(reviewer.approved) != 1 || reviewer.approved[0] != 42 {
		t.Fatalf("approved = %v", reviewer.approved)
	}
}

func TestConfirmMergeCallsReviewerMerge(t *testing.T) {
	reviewer := &fakePRReviewer{}
	model := modelInPRDiff(t, &fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, reviewer)
	model.confirm = NewConfirmDialog(MergeDialog, "Merge PR #42 (feat: add login)")
	model, cmd := model.updateConfirmKey(tea.KeyMsg{Type: tea.KeyCtrlS})
	msg := cmd()
	if result, ok := msg.(prActionMsg); !ok || result.Err != nil {
		t.Fatalf("msg = %+v", msg)
	}
	if len(reviewer.merged) != 1 || reviewer.merged[0] != 42 {
		t.Fatalf("merged = %v", reviewer.merged)
	}
}

func TestConfirmCloseCallsCloseThenDeleteBranch(t *testing.T) {
	reviewer := &fakePRReviewer{}
	model := modelInPRDiff(t, &fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, reviewer)
	model.confirm = NewConfirmDialog(ClosePRDialog, "Close PR #42 (feat: add login) + delete branch feat-login")
	model, cmd := model.updateConfirmKey(tea.KeyMsg{Type: tea.KeyCtrlS})
	msg := cmd()
	if result, ok := msg.(prActionMsg); !ok || result.Err != nil {
		t.Fatalf("msg = %+v", msg)
	}
	if len(reviewer.closed) != 1 || reviewer.closed[0] != 42 {
		t.Fatalf("closed = %v", reviewer.closed)
	}
	if len(reviewer.deleted) != 1 || reviewer.deleted[0] != "feat-login" {
		t.Fatalf("deleted = %v", reviewer.deleted)
	}
}

func TestConfirmRequestChangesCallsReviewerWithBody(t *testing.T) {
	reviewer := &fakePRReviewer{}
	model := modelInPRDiff(t, &fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, reviewer)
	model.dialog = NewActionDialog(RequestChangesDialog)
	model.dialog.SetDraft("needs tests", nil)
	model, cmd := model.updateDialogKey(tea.KeyMsg{Type: tea.KeyCtrlS})
	msg := cmd()
	if result, ok := msg.(prActionMsg); !ok || result.Err != nil {
		t.Fatalf("msg = %+v", msg)
	}
	if len(reviewer.requested) != 1 || reviewer.requested[0] != 42 || reviewer.commented[0] != "needs tests" {
		t.Fatalf("requested = %v, commented = %v", reviewer.requested, reviewer.commented)
	}
}

func TestPRActionFailureKeepsConfirmOpen(t *testing.T) {
	reviewer := &fakePRReviewer{err: fmt.Errorf("gh auth failed")}
	model := modelInPRDiff(t, &fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, reviewer)
	model.confirm = NewConfirmDialog(ApproveDialog, "Approve PR #42 (feat: add login)")
	model, cmd := model.updateConfirmKey(tea.KeyMsg{Type: tea.KeyCtrlS})
	model, _ = model.Update(cmd())
	if model.confirm == nil || model.confirm.Err == nil {
		t.Fatal("confirm dialog should stay open with error on failure")
	}
	if !strings.Contains(model.status, "gh auth failed") {
		t.Fatalf("status = %q", model.status)
	}
}

func TestPRActionSuccessClosesConfirmAndSetsStatus(t *testing.T) {
	reviewer := &fakePRReviewer{}
	model := modelInPRDiff(t, &fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, reviewer)
	model.confirm = NewConfirmDialog(ApproveDialog, "Approve PR #42 (feat: add login)")
	model, cmd := model.updateConfirmKey(tea.KeyMsg{Type: tea.KeyCtrlS})
	model, _ = model.Update(cmd())
	if model.confirm != nil {
		t.Fatal("confirm dialog should close on success")
	}
	if !strings.Contains(model.status, "approved PR #42") {
		t.Fatalf("status = %q", model.status)
	}
}

func TestPRDiffOOpensSelectedPRURL(t *testing.T) {
	model := modelInPRDiff(t, &fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakePRReviewer{})
	opener := &fakeOpener{}
	model.opener = opener
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if cmd == nil {
		t.Fatal("o did not produce a command")
	}
	cmd()
	if len(opener.urls) != 1 || !strings.Contains(opener.urls[0], "pull/42") {
		t.Fatalf("opened urls = %v", opener.urls)
	}
}

func TestPRsErrorMsgShowsErrorInSelector(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model.treeMode = TreeModePRSelector
	model, _ = model.Update(prsErrorMsg{Err: fmt.Errorf("gh not installed")})
	if model.prSelector == nil || model.prSelector.err == nil {
		t.Fatalf("prSelector = %+v", model.prSelector)
	}
	if !strings.Contains(model.status, "gh not installed") {
		t.Fatalf("status = %q", model.status)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./ui -run "TestTreeModeCyclesThroughPRSelector|TestPRSelectorEnter|TestPRDiff|TestGKey|TestConfirm|TestPRAction|TestPRsError" -v -count=1`
Expected: FAIL (missing symbols: TreeModePRSelector/PRDiff, prsLoadedMsg, prActionMsg, PRReviewer, pendingPRKey, `NewModel` signature)

- [ ] **Step 3: Implement model wiring**

Add to `ui/model.go`:

```go
// In the TreeMode const block:
const (
	TreeModeWorktree TreeMode = iota
	TreeModeStaged
	TreeModeBranchSelector
	TreeModeBranchDiff
	TreeModePRSelector
	TreeModePRDiff
)

// New message types:
type prsLoadedMsg struct{ PRs []pr.PR }
type prsErrorMsg struct{ Err error }
type prDiffMsg struct {
	Number   int
	Snapshot git.Snapshot
}
type prActionMsg struct {
	Label string
	Err   error
}

// PRReviewer is every PR action the PR-review flows need. pr.GitHub
// satisfies this; ui tests use fakePRReviewer.
type PRReviewer interface {
	ListPRs(context.Context, string) ([]pr.PR, error)
	Approve(context.Context, int, string) error
	RequestChanges(context.Context, int, string) error
	Merge(context.Context, int) error
	Close(context.Context, int) error
	DeleteBranch(context.Context, string) error
}

// Add to SnapshotLoader:
type SnapshotLoader interface {
	Snapshot(context.Context, git.Mode) (git.Snapshot, error)
	SnapshotBranch(context.Context, string) (git.Snapshot, error)
	SnapshotPR(context.Context, int) (git.Snapshot, error)  // NEW
}

// Add to Model struct:
	prSelector   *PRSelector
	prReviewer   PRReviewer
	pendingPRKey string
```

Update `NewModel` signature and body:

```go
func NewModel(repo git.Repository, cfg config.Config, loader SnapshotLoader, renderer Renderer, runner agent.Runner, templates prompt.Templates, mutator Mutator, opener pr.Opener, prReviewer PRReviewer) Model {
	return Model{
		repo: repo, cfg: cfg, loader: loader, renderer: renderer, runner: runner, templates: templates,
		mutator: mutator, opener: opener, prReviewer: prReviewer,
		mode: git.WorkingTree, treeMode: TreeModeWorktree, tree: NewTree(nil), focus: FocusTree, activeTab: DetailTab,
		results: make(map[string]*analysisResult), requests: make(map[string]context.CancelFunc),
		status: "loading repository",
	}
}
```

Add `Update` intercept for confirm dialog (before the `dialog` intercept):

```go
	if keyMsg, ok := msg.(tea.KeyMsg); ok && m.confirm != nil {
		return m.updateConfirmKey(keyMsg)
	}
```

Add message handlers in `Update` (place next to `branchesLoadedMsg`/`snapshotMsg`):

```go
	case prsLoadedMsg:
		m.prSelector = NewPRSelector(message.PRs)
		return m, nil
	case prsErrorMsg:
		if m.prSelector == nil {
			m.prSelector = NewPRSelector(nil)
		}
		m.prSelector.err = message.Err
		m.status = "pr list: " + message.Err.Error()
		return m, nil
	case prDiffMsg:
		if m.prSelector != nil {
			m.prSelector.diffCache[message.Number] = message.Snapshot
		}
		changed := m.applySnapshot(message.Snapshot)
		if changed {
			return m, tea.Batch(m.renderSelectedCmd(), tickCmd())
		}
		return m, tickCmd()
	case prActionMsg:
		if message.Err != nil {
			m.status = "pr action failed: " + message.Err.Error()
			if m.confirm != nil {
				m.confirm.Err = message.Err
			}
			return m, nil
		}
		m.confirm = nil
		m.dialog = nil
		m.status = message.Label
		return m, nil
```

Add `updateConfirmKey` and `confirmActionCmd`:

```go
func (m Model) updateConfirmKey(key tea.KeyMsg) (Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.confirm = nil
		return m, nil
	case "ctrl+s":
		return m, m.confirmActionCmd()
	}
	return m, nil
}

func (m Model) confirmActionCmd() tea.Cmd {
	if m.confirm == nil || m.prSelector == nil || m.prSelector.selectedPR == nil {
		return nil
	}
	reviewer := m.prReviewer
	kind := m.confirm.Kind
	number := m.prSelector.selectedPR.Number
	head := m.prSelector.selectedPR.HeadRefName
	return func() tea.Msg {
		ctx := context.Background()
		var label string
		var err error
		switch kind {
		case ApproveDialog:
			label = fmt.Sprintf("approved PR #%d", number)
			err = reviewer.Approve(ctx, number, "")
		case MergeDialog:
			label = fmt.Sprintf("merged PR #%d", number)
			err = reviewer.Merge(ctx, number)
		case ClosePRDialog:
			err = reviewer.Close(ctx, number)
			if err == nil {
				err = reviewer.DeleteBranch(ctx, head)
			}
			label = fmt.Sprintf("closed PR #%d + deleted branch %s", number, head)
		}
		return prActionMsg{Label: label, Err: err}
	}
}
```

Extend `confirmDialogCmd` (textarea dialog confirm) with the RequestChanges case:

```go
	case RequestChangesDialog:
		reviewer := m.prReviewer
		number := m.prSelector.selectedPR.Number
		text := m.dialog.Text()
		return m, func() tea.Msg {
			return prActionMsg{
				Label: fmt.Sprintf("requested changes on PR #%d", number),
				Err:   reviewer.RequestChanges(context.Background(), number, text),
			}
		}
```

Extend `regenerateDialogCmd`: `case RequestChangesDialog: return m, nil` (no AI regeneration for hand-written comments — ctrl+r is a no-op here).

Add `loadPRsCmd`:

```go
func (m Model) loadPRsCmd() tea.Cmd {
	reviewer := m.prReviewer
	return func() tea.Msg {
		if reviewer == nil {
			return prsErrorMsg{Err: fmt.Errorf("PR reviewer unavailable")}
		}
		prs, err := reviewer.ListPRs(context.Background(), "open")
		if err != nil {
			return prsErrorMsg{Err: err}
		}
		return prsLoadedMsg{PRs: prs}
	}
}
```

Add `openSelectedPRCmd`:

```go
func (m Model) openSelectedPRCmd() tea.Cmd {
	opener := m.opener
	p := m.prSelector.selectedPR
	return func() tea.Msg {
		return prResultMsg{Err: opener.Open(context.Background(), p.URL)}
	}
}
```

Rewrite the `[` / `]` cases in `updateKey` for the 3-mode cycle:

```go
	case "]":
		if m.focus == FocusAnalysis {
			if m.activeTab < RequestLogTab {
				m.activeTab++
			} else {
				m.activeTab = DetailTab
			}
		} else if m.focus == FocusTree {
			switch m.treeMode {
			case TreeModeWorktree:
				m.treeMode = TreeModeBranchSelector
				if m.branchSelector == nil {
					return m, m.loadBranchesCmd()
				}
			case TreeModeBranchSelector:
				m.treeMode = TreeModePRSelector
				if m.prSelector == nil {
					return m, m.loadPRsCmd()
				}
			default:
				m.treeMode = TreeModeWorktree
			}
		}
	case "[":
		if m.focus == FocusAnalysis {
			if m.activeTab > 0 {
				m.activeTab--
			} else {
				m.activeTab = RequestLogTab
			}
		} else if m.focus == FocusTree {
			switch m.treeMode {
			case TreeModePRSelector:
				m.treeMode = TreeModeBranchSelector
			case TreeModeBranchSelector:
				m.treeMode = TreeModeWorktree
			default:
				m.treeMode = TreeModePRSelector
				if m.prSelector == nil {
					return m, m.loadPRsCmd()
				}
			}
		}
```

Extend `j`/`k` navigation for the PR selector (mirror the branch selector branch):

```go
	case "up", "k":
		if m.focus == FocusTree && m.treeMode == TreeModePRSelector && m.prSelector != nil {
			m.prSelector.Move(-1)
		} else if m.focus == FocusTree && m.treeMode == TreeModeBranchSelector && m.branchSelector != nil {
			m.branchSelector.Move(-1)
		} else if m.focus == FocusTree {
			m.tree.Move(-1)
			m.diffScroll = 0
			return m, m.renderSelectedCmd()
		}
		// ... existing diff/analysis scroll logic unchanged
	case "down", "j":
		// same shape with Move(1)
```

Extend `enter` and `l` for the PR selector (same shape as the branch selector branch):

```go
	case "enter":
		if m.focus == FocusTree && m.treeMode == TreeModePRSelector && m.prSelector != nil {
			if p := m.prSelector.Selected(); p != nil {
				m.prSelector.Select(p.Number)
				m.treeMode = TreeModePRDiff
				return m, m.refreshCmd()
			}
		}
		if m.focus == FocusTree && m.treeMode == TreeModeBranchSelector && m.branchSelector != nil {
			// ... existing branch branch unchanged
		}
```

Extend `h` — PR diff returns to PR selector, PR selector returns to branch selector:

```go
	case "h":
		if m.focus == FocusTree && m.treeMode == TreeModePRDiff {
			m.treeMode = TreeModePRSelector
			return m, nil
		}
		if m.focus == FocusTree && m.treeMode == TreeModePRSelector {
			m.treeMode = TreeModeBranchSelector
			return m, nil
		}
		if m.focus == FocusTree && m.treeMode == TreeModeBranchDiff {
			m.treeMode = TreeModeBranchSelector
			return m, nil
		}
		// ... existing collapse branch unchanged
```

Extend `o`:

```go
	case "o":
		if m.treeMode == TreeModeBranchDiff && m.branchSelector != nil && m.branchSelector.selectedBranch != "" {
			return m, m.startPRForReviewedBranchCmd()
		}
		if m.treeMode == TreeModePRDiff && m.prSelector != nil && m.prSelector.selectedPR != nil {
			return m, m.openSelectedPRCmd()
		}
		return m, m.startPRCmd()
```

Extend `r`:

```go
	case "r":
		if m.treeMode == TreeModePRSelector {
			return m, m.loadPRsCmd()
		}
		if m.treeMode == TreeModePRDiff && m.prSelector != nil {
			clear(m.prSelector.diffCache)
		}
		return m, m.refreshCmd()
```

Rewrite the `g` case for the pending-key sequence. Add this at the very top of `updateKey` (before the switch):

```go
	if m.pendingPRKey != "" {
		m.pendingPRKey = ""
		switch key.String() {
		case "a":
			return m.openConfirmDialog(ApproveDialog)
		case "r":
			return m.openRequestChangesDialog()
		case "m":
			return m.openConfirmDialog(MergeDialog)
		case "d":
			return m.openConfirmDialog(ClosePRDialog)
		}
		// any other key: fall through to normal handling
	}
```

Replace the existing `case "g":` body:

```go
	case "g":
		if m.focus == FocusTree && m.treeMode == TreeModePRDiff && m.prSelector != nil && m.prSelector.selectedPR != nil {
			m.pendingPRKey = "g"
			return m, nil
		}
		if m.focus == FocusDiff {
			m.diffScroll = 0
		}
		if m.focus == FocusAnalysis {
			m.analysisScroll = 0
		}
```

Add the dialog-opening helpers:

```go
func (m Model) openConfirmDialog(kind DialogKind) (Model, tea.Cmd) {
	if m.prSelector == nil || m.prSelector.selectedPR == nil {
		return m, nil
	}
	p := m.prSelector.selectedPR
	var title string
	switch kind {
	case ApproveDialog:
		title = fmt.Sprintf("Approve PR #%d (%s)", p.Number, p.Title)
	case MergeDialog:
		title = fmt.Sprintf("Merge PR #%d (%s)", p.Number, p.Title)
	case ClosePRDialog:
		title = fmt.Sprintf("Close PR #%d (%s) + delete branch %s", p.Number, p.Title, p.HeadRefName)
	}
	m.confirm = NewConfirmDialog(kind, title)
	return m, nil
}

func (m Model) openRequestChangesDialog() (Model, tea.Cmd) {
	if m.prSelector == nil || m.prSelector.selectedPR == nil {
		return m, nil
	}
	m.dialog = NewActionDialog(RequestChangesDialog)
	m.dialog.SetDraft("", nil) // ready immediately; comment is hand-written
	return m, nil
}
```

Update `refreshCmd` for PR diff mode (cache-first):

```go
func (m Model) refreshCmd() tea.Cmd {
	if m.treeMode == TreeModePRDiff && m.prSelector != nil && m.prSelector.selectedPR != nil {
		loader := m.loader
		number := m.prSelector.selectedPR.Number
		if snap, ok := m.prSelector.diffCache[number]; ok {
			return func() tea.Msg { return prDiffMsg{Number: number, Snapshot: snap} }
		}
		return func() tea.Msg {
			snapshot, err := loader.SnapshotPR(context.Background(), number)
			if err != nil {
				return snapshotErrorMsg{Err: err}
			}
			return prDiffMsg{Number: number, Snapshot: snapshot}
		}
	}
	if m.treeMode == TreeModeBranchDiff && m.branchSelector != nil && m.branchSelector.selectedBranch != "" {
		// ... existing branch branch unchanged
	}
	// ... existing default branch unchanged
}
```

Add PR branches to `visiblePRs` (alongside `visibleBranches` in model.go):

```go
func (m Model) visiblePRs() []pr.PR {
	if m.prSelector == nil {
		return nil
	}
	rows := m.prSelector.Rows()
	if m.searchFilter == nil {
		return rows
	}
	var result []pr.PR
	for _, p := range rows {
		if m.searchFilter.MatchString(fmt.Sprintf("#%d %s", p.Number, p.Title)) {
			result = append(result, p)
		}
	}
	return result
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./ui -run "TestTreeModeCyclesThroughPRSelector|TestPRSelectorEnter|TestPRDiff|TestGKey|TestConfirm|TestPRAction|TestPRsError" -v -count=1`
Expected: PASS

- [ ] **Step 5: Run all ui tests**

Run: `go test ./ui -count=1`
Expected: all pass (fix the rewritten `TestTreeModeCyclesWithBracketKeys` first if red)

- [ ] **Step 6: Commit**

```bash
git add ui/model.go ui/model_test.go
git commit -m "feat(ui): add PR modes, g-prefix actions, and PR diff loading"
```

---

### Task 6: ui rendering — PR selector, confirm dialog, tab bar, status line, help text

**Files:**
- Modify: `ui/render.go` — renderPRSelector, renderConfirmDialog, tab bar third tab, status line, help text, `renderDialog` title/hint for RequestChangesDialog
- Modify: `ui/view_test.go` — add render tests

- [ ] **Step 1: Write failing tests**

```go
// ui/view_test.go — append
func TestRenderTreeShowsPRSelectorWhenInPRSelectorMode(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model.termW, model.termH = 120, 40
	model.layout = ComputeLayout(120, 40)
	model.treeMode = TreeModePRSelector
	model.prSelector = NewPRSelector(makeTestPRs())
	out := model.renderTree(model.layout.Files)
	if !strings.Contains(out, "#42") || !strings.Contains(out, "feat: add login") {
		t.Fatalf("expected PR rows in tree pane:\n%s", out)
	}
}

func TestRenderPRSelectorShowsNoOpenPRs(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model.termW, model.termH = 120, 40
	model.layout = ComputeLayout(120, 40)
	model.treeMode = TreeModePRSelector
	model.prSelector = NewPRSelector(nil)
	out := model.renderTree(model.layout.Files)
	if !strings.Contains(out, "no open pull requests") {
		t.Fatalf("expected empty message:\n%s", out)
	}
}

func TestRenderPRSelectorShowsInlineError(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model.termW, model.termH = 120, 40
	model.layout = ComputeLayout(120, 40)
	model.treeMode = TreeModePRSelector
	model.prSelector = NewPRSelector(nil)
	model.prSelector.err = fmt.Errorf("gh not installed")
	out := model.renderTree(model.layout.Files)
	if !strings.Contains(out, "gh not installed") {
		t.Fatalf("expected inline error:\n%s", out)
	}
}

func TestTabBarShowsPRsThirdTab(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model.treeMode = TreeModeWorktree
	bar := model.renderTabBar()
	if !strings.Contains(bar, "Worktree") || !strings.Contains(bar, "Branch") || !strings.Contains(bar, "PRs") {
		t.Fatalf("tab bar = %q", bar)
	}
	model.treeMode = TreeModePRSelector
	bar = model.renderTabBar()
	if !strings.Contains(bar, "PRs") {
		t.Fatalf("PR selector tab bar = %q", bar)
	}
	model.treeMode = TreeModePRDiff
	model.prSelector = NewPRSelector(makeTestPRs())
	model.prSelector.Select(42)
	bar = model.renderTabBar()
	if !strings.Contains(bar, "#42") {
		t.Fatalf("PR diff tab bar = %q", bar)
	}
}

func TestStatusLineShowsPRDiffInfo(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model.treeMode = TreeModePRDiff
	model.prSelector = NewPRSelector(makeTestPRs())
	model.prSelector.Select(42)
	line := model.statusLine()
	if !strings.Contains(line, "PR #42: feat: add login") {
		t.Fatalf("status line = %q", line)
	}
}

func TestStatusLineShowsPRSelectorMode(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model.treeMode = TreeModePRSelector
	line := model.statusLine()
	if !strings.Contains(line, "PR selector") {
		t.Fatalf("status line = %q", line)
	}
}

func TestRenderConfirmDialogShowsTitleAndError(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model.termW, model.termH = 120, 40
	model.layout = ComputeLayout(120, 40)
	model.confirm = NewConfirmDialog(ApproveDialog, "Approve PR #42 (feat: add login)")
	model.confirm.Err = fmt.Errorf("gh auth failed")
	out := model.renderConfirmDialog()
	if !strings.Contains(out, "Approve PR #42") || !strings.Contains(out, "gh auth failed") || !strings.Contains(out, "ctrl+s confirm") {
		t.Fatalf("confirm dialog:\n%s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./ui -run "TestRenderTreeShowsPRSelector|TestRenderPRSelector|TestTabBarShowsPRs|TestStatusLineShowsPR|TestRenderConfirmDialog" -v -count=1`
Expected: FAIL

- [ ] **Step 3: Implement rendering**

In `View()`, before the `m.dialog != nil` check:

```go
	if m.confirm != nil {
		return m.renderConfirmDialog()
	}
```

Add `renderPRSelector` and wire it into `renderTree` (before the branch selector check):

```go
func (m Model) renderTree(r Rect) string {
	if m.treeMode == TreeModePRSelector && m.prSelector != nil {
		return m.renderPRSelector(r)
	}
	if m.treeMode == TreeModeBranchSelector && m.branchSelector != nil {
		return m.renderBranchSelector(r)
	}
	// ... unchanged
```

```go
func (m Model) renderPRSelector(r Rect) string {
	title := delta.Truncate(m.renderTabBar(), max(1, r.W-2))
	lines := []string{title}
	rows := m.visiblePRs()
	if len(rows) == 0 {
		msg := "(no open pull requests)"
		if m.prSelector.err != nil {
			msg = "(error: " + m.prSelector.err.Error() + ")"
		}
		empty := delta.Truncate(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(msg), max(1, r.W-2))
		lines = append(lines, empty)
		return box(r, strings.Join(padLines(lines, r.H-2), "\n"), m.focus == FocusTree)
	}
	maxW := max(1, r.W-2)
	for i, p := range rows {
		prefix := "  "
		if i == m.prSelector.Cursor() {
			prefix = "▶ "
		}
		icon := "✓"
		if p.Mergeable == "CONFLICTING" {
			icon = "✗"
		} else if p.Mergeable == "UNKNOWN" {
			icon = "?"
		}
		style := lipgloss.Color("245")
		if i == m.prSelector.Cursor() {
			style = lipgloss.Color("51")
		}
		line := fmt.Sprintf("%s#%d %s  (%s, %s)", prefix, p.Number, p.Title, p.Author, icon)
		lines = append(lines, lipgloss.NewStyle().Foreground(style).Render(delta.Truncate(line, maxW)))
	}
	return box(r, strings.Join(padLines(lines, r.H-2), "\n"), m.focus == FocusTree)
}
```

Add `renderConfirmDialog` (mirrors `renderDialog` layout):

```go
func (m Model) renderConfirmDialog() string {
	width := m.termW - 10
	if width > 100 {
		width = 100
	}
	if width < 20 {
		width = 20
	}
	height := m.termH - 12
	if height > 20 {
		height = 20
	}
	if height < 3 {
		height = 3
	}
	var body strings.Builder
	body.WriteString(lipgloss.NewStyle().Bold(true).Render(m.confirm.Title))
	body.WriteString("\n\n")
	if m.confirm.Err != nil {
		body.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render("error: " + m.confirm.Err.Error()))
		body.WriteString("\n\n")
	}
	body.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render("ctrl+s confirm   esc cancel"))
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Width(width).Height(height)
	return lipgloss.Place(m.termW, m.termH, lipgloss.Center, lipgloss.Center, style.Render(body.String()))
}
```

In `renderDialog`, fix the title and hint for `RequestChangesDialog`:

```go
	title := "Commit Message"
	switch m.dialog.Kind {
	case PRDialog:
		title = "Pull Request"
	case RequestChangesDialog:
		title = "Request Changes"
	}
	// ... existing body ...
	hint := "ctrl+s confirm   esc cancel"
	if m.dialog.Kind != RequestChangesDialog {
		hint += "   ctrl+r regenerate"
	}
	body.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(hint))
```

Update `renderTabBar` for the third tab:

```go
func (m Model) renderTabBar() string {
	green := lipgloss.Color("42")
	dim := lipgloss.Color("245")
	active := lipgloss.NewStyle().Foreground(green).Bold(true).Render
	inactive := lipgloss.NewStyle().Foreground(dim).Render
	switch m.treeMode {
	case TreeModeWorktree, TreeModeStaged:
		return active("Worktree") + "  " + inactive("Branch") + "  " + inactive("PRs")
	case TreeModeBranchDiff:
		name := "Branch"
		if m.branchSelector != nil && m.branchSelector.selectedBranch != "" {
			name = m.branchSelector.selectedBranch
		}
		return inactive("Worktree") + "  " + active(name) + "  " + inactive("PRs")
	case TreeModeBranchSelector:
		return inactive("Worktree") + "  " + active("Branch") + "  " + inactive("PRs")
	case TreeModePRSelector:
		return inactive("Worktree") + "  " + inactive("Branch") + "  " + active("PRs")
	case TreeModePRDiff:
		name := "PRs"
		if m.prSelector != nil && m.prSelector.selectedPR != nil {
			name = fmt.Sprintf("#%d", m.prSelector.selectedPR.Number)
		}
		return inactive("Worktree") + "  " + inactive("Branch") + "  " + active(name)
	default:
		return ""
	}
}
```

Update `statusLine`:

```go
	modeLabel := m.treeMode.String()
	if m.treeMode == TreeModeBranchDiff && m.branchSelector != nil {
		modeLabel = "branch diff: " + m.branchSelector.selectedBranch
	} else if m.treeMode == TreeModePRDiff && m.prSelector != nil && m.prSelector.selectedPR != nil {
		p := m.prSelector.selectedPR
		modeLabel = fmt.Sprintf("PR #%d: %s", p.Number, p.Title)
	} else if m.treeMode == TreeModePRSelector {
		modeLabel = "PR selector"
	}
```

Add a PR Review section to `helpText` (after the Staging section):

```go
		section("PR Review"),
		key("[ga]", "Approve PR (PR diff view)"),
		key("[gr]", "Request changes (PR diff view)"),
		key("[gm]", "Merge PR (PR diff view)"),
		key("[gd]", "Close PR + delete branch (PR diff view)"),
		key("o", "Open selected PR in browser"),
		key("r", "Refresh PR list / PR diff"),
		"",
```

Extend `TreeMode.String()` with the two new modes:

```go
	case TreeModePRSelector:
		return "PR selector"
	case TreeModePRDiff:
		return "PR diff"
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./ui -run "TestRenderTreeShowsPRSelector|TestRenderPRSelector|TestTabBarShowsPRs|TestStatusLineShowsPR|TestRenderConfirmDialog" -v -count=1`
Expected: PASS

- [ ] **Step 5: Run all tests**

Run: `go test ./... -count=1`
Expected: all pass (may surface a stale compile error in cmd — fixed by Task 7's commit; see note below)

Note: after Task 5, `go test ./...` fails to compile `cmd/lazydiff/main.go` (NewModel signature changed, repositoryLoader lacks SnapshotPR). If that blocks this step, proceed to Task 7 and re-run everything there; alternatively make the Task 5 commit and Task 7 commit back-to-back before the full-suite run.

- [ ] **Step 6: Commit**

```bash
git add ui/render.go ui/view_test.go
git commit -m "feat(ui): render PR selector, PR diff, and PR tab bar"
```

---

### Task 7: cmd/lazydiff wiring — pr.GitHub, repositoryLoader.SnapshotPR

**Files:**
- Modify: `cmd/lazydiff/main.go` — construct `pr.GitHub` from origin remote URL, pass as `PRReviewer`, add `SnapshotPR` to `repositoryLoader`
- Create: `cmd/lazydiff/main_test.go` — SnapshotPR unit test with a fake CommandRunner

- [ ] **Step 1: Write the failing test**

```go
// cmd/lazydiff/main_test.go
package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/alex-irvine/lazydiff/git"
	"github.com/alex-irvine/lazydiff/pr"
)

type fakeCommandRunner struct {
	outputs map[string][]byte
}

func (f *fakeCommandRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	key := strings.Join(append([]string{name}, args...), " ")
	if out, ok := f.outputs[key]; ok {
		return out, nil
	}
	return nil, fmt.Errorf("unexpected command %q", key)
}

func (f *fakeCommandRunner) RunWithStdin(context.Context, interface{}, string, ...string) ([]byte, error) {
	return nil, nil
}

func TestSnapshotPRBuildsSnapshotFromGHDiff(t *testing.T) {
	runner := &fakeCommandRunner{outputs: map[string][]byte{
		"gh pr view 42 --json number,title,author,headRefName,baseRefName,mergeable,url,createdAt": []byte(`{"number":42,"title":"feat: add login","author":"alex","headRefName":"feat-login","baseRefName":"main","mergeable":"MERGEABLE","url":"https://github.com/alex-irvine/lazydiff/pull/42","createdAt":"2026-07-01T00:00:00Z"}`),
		"gh pr diff 42 --patch": []byte("diff --git a/login.go b/login.go\nnew file mode 100644\n--- /dev/null\n+++ b/login.go\n@@ -0,0 +1 @@\n+func login() {}\n"),
	}}
	gh := pr.NewGitHub("git@github.com:alex-irvine/lazydiff.git", runner)
	loader := repositoryLoader{repo: git.Repository{}, gh: gh}
	snapshot, err := loader.SnapshotPR(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Mode != git.Branch || snapshot.Base != "main...feat-login" {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if len(snapshot.Files) != 1 || snapshot.Files[0].Path != "login.go" {
		t.Fatalf("files = %+v", snapshot.Files)
	}
	if !strings.Contains(snapshot.RawDiff, "login.go") {
		t.Fatalf("raw = %q", snapshot.RawDiff)
	}
}

func TestSnapshotPRRejectsNonGitHubRemote(t *testing.T) {
	gh := pr.NewGitHub("git@gitlab.com:some/repo.git", &fakeCommandRunner{outputs: map[string][]byte{}})
	loader := repositoryLoader{repo: git.Repository{}, gh: gh}
	if _, err := loader.SnapshotPR(context.Background(), 42); err == nil || !strings.Contains(err.Error(), "github.com") {
		t.Fatalf("err = %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/lazydiff -run TestSnapshotPR -v -count=1`
Expected: FAIL — `repositoryLoader` has no `gh` field / `SnapshotPR`

- [ ] **Step 3: Wire cmd/lazydiff/main.go**

```go
// In runApp, after `repo, err := git.Open(ctx, root)`:
	remoteURL, remoteErr := repo.RemoteURL(ctx, "origin")
	if remoteErr != nil {
		remoteURL = "" // no origin — PR ops will error with a clear message
	}
	gh := pr.NewGitHub(remoteURL, git.ExecRunner{})

// Update the NewModel call (add prReviewer):
	model := ui.NewTeaModel(ui.NewModel(repo, cfg, loader, delta.Renderer{Command: "delta"}, runner, templates, repo, pr.NewOpener(), gh))
```

Update `repositoryLoader`:

```go
type repositoryLoader struct {
	repo git.Repository
	gh   *pr.GitHub
}

func (l repositoryLoader) Snapshot(ctx context.Context, mode git.Mode) (git.Snapshot, error) {
	return l.repo.Snapshot(ctx, mode)
}

func (l repositoryLoader) SnapshotBranch(ctx context.Context, branch string) (git.Snapshot, error) {
	return l.repo.SnapshotBranch(ctx, branch)
}

// SnapshotPR builds a git.Snapshot from gh pr view + gh pr diff. Snapshot
// construction lives here (the caller), keeping the git package free of pr
// imports (spec open decision #2).
func (l repositoryLoader) SnapshotPR(ctx context.Context, number int) (git.Snapshot, error) {
	p, err := l.gh.PR(ctx, number)
	if err != nil {
		return git.Snapshot{}, err
	}
	raw, err := l.gh.PRDiff(ctx, number)
	if err != nil {
		return git.Snapshot{}, err
	}
	files, parseErr := diff.Parse(raw)
	if parseErr != nil {
		return git.Snapshot{}, fmt.Errorf("parse PR #%d diff: %w", number, parseErr)
	}
	base := p.BaseRefName + "..." + p.HeadRefName
	hash := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s", git.Branch, base, raw)))
	return git.Snapshot{ID: fmt.Sprintf("%x", hash[:]), Mode: git.Branch, Base: base, RawDiff: raw, Files: files}, nil
}
```

Add imports: `crypto/sha256` and `github.com/alex-irvine/lazydiff/diff`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/lazydiff -run TestSnapshotPR -v -count=1`
Expected: PASS

- [ ] **Step 5: Run full suite (compiles now that NewModel is wired)**

Run: `go test ./... -count=1`
Expected: all pass

- [ ] **Step 6: Commit**

```bash
git add cmd/lazydiff/main.go cmd/lazydiff/main_test.go
git commit -m "feat(cmd): wire GitHub PR reviewer and SnapshotPR"
```

---

### Task 8: PTY integration test for PR review mode

**Files:**
- Modify: `integration/pty_linux_test.go` — add PR-mode flow test with a fake `gh` script

The fixture's scratch repo has no `origin` remote, so the test adds a github.com remote first (never pushed to). A fake `gh` script on PATH (same tools dir as the fake `delta`/`agent`) serves the PR list/view/diff and records `pr review` invocations to a log file the test asserts on.

- [ ] **Step 1: Write the test**

```go
// integration/pty_linux_test.go — append
func TestPTYPRReviewApproveFlow(t *testing.T) {
	fixture := newFixture(t)
	run(t, fixture.root, "git", "remote", "add", "origin", "git@github.com:alex-irvine/lazydiff.git")
	ghLog := filepath.Join(fixture.root, "gh.log")
	gh := filepath.Join(filepath.Dir(fixture.binary), "gh")
	writeExecutable(t, gh, `#!/bin/sh
case "$1 $2" in
"pr list") echo '[{"number":42,"title":"feat: add login","author":"alex","headRefName":"feat-login","baseRefName":"main","mergeable":"MERGEABLE","url":"https://github.com/alex-irvine/lazydiff/pull/42","createdAt":"2026-07-01T00:00:00Z"}]';;
"pr view") echo '{"number":42,"title":"feat: add login","author":"alex","headRefName":"feat-login","baseRefName":"main","mergeable":"MERGEABLE","url":"https://github.com/alex-irvine/lazydiff/pull/42","createdAt":"2026-07-01T00:00:00Z"}';;
"pr diff") printf 'diff --git a/login.go b/login.go\nnew file mode 100644\n--- /dev/null\n+++ b/login.go\n@@ -0,0 +1 @@\n+func login() {}\n';;
"pr review") echo "$@" >> "$GH_LOG";;
esac
`)
	defer os.Remove(gh)

	cmd := exec.Command(fixture.binary, "-config", fixture.config)
	cmd.Dir = fixture.root
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(fixture.delta)+":"+os.Getenv("PATH"), "GH_LOG="+ghLog)
	terminal, err := pty.Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	if err := pty.Setsize(terminal, &pty.Winsize{Cols: 120, Rows: 40}); err != nil {
		t.Fatal(err)
	}
	_ = readUntil(t, terminal, "delta-output", 3*time.Second)

	// worktree → branch selector → PR selector
	if _, err := terminal.Write([]byte("]")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := terminal.Write([]byte("]")); err != nil {
		t.Fatal(err)
	}
	output := readUntil(t, terminal, "#42", 3*time.Second)
	if !strings.Contains(output, "feat: add login") {
		t.Fatalf("expected PR row:\n%s", output)
	}

	// select PR #42 → PR diff
	if _, err := terminal.Write([]byte{13}); err != nil { // enter
		t.Fatal(err)
	}
	output = readUntil(t, terminal, "login.go", 3*time.Second)
	if !strings.Contains(output, "login.go") {
		t.Fatalf("expected PR diff:\n%s", output)
	}

	// h returns to PR selector, re-enter hits the diff cache
	if _, err := terminal.Write([]byte("h")); err != nil {
		t.Fatal(err)
	}
	output = readUntil(t, terminal, "PRs", 3*time.Second)
	if !strings.Contains(output, "#42") {
		t.Fatalf("expected PR list after h:\n%s", output)
	}
	if _, err := terminal.Write([]byte{13}); err != nil {
		t.Fatal(err)
	}
	output = readUntil(t, terminal, "login.go", 3*time.Second)

	// ga → confirm dialog → ctrl+s approve
	if _, err := terminal.Write([]byte("g")); err != nil {
		t.Fatal(err)
	}
	if _, err := terminal.Write([]byte("a")); err != nil {
		t.Fatal(err)
	}
	output = readUntil(t, terminal, "Approve PR #42", 3*time.Second)
	if !strings.Contains(output, "ctrl+s confirm") {
		t.Fatalf("expected confirm hint:\n%s", output)
	}
	if _, err := terminal.Write([]byte{19}); err != nil { // ctrl+s
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("lazydiff exit: %v", err)
	}
	logData, err := os.ReadFile(ghLog)
	if err != nil {
		t.Fatalf("read gh log: %v", err)
	}
	if !strings.Contains(string(logData), "pr review 42 --approve") {
		t.Fatalf("expected approve invocation, gh log = %q", logData)
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./integration -run TestPTYPRReview -v -count=1`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add integration/pty_linux_test.go
git commit -m "test(integration): cover PR review mode flow"
```

---

### Verification

- [ ] **Step 1: Run full suite**

```bash
go test ./... -count=1
go vet ./...
go build ./...
git diff --check
```

Expected: all pass, no vet warnings, clean build, no whitespace errors.

- [ ] **Step 2: Manual smoke test (optional but recommended)**

```bash
go build -o /tmp/lazydiff ./cmd/lazydiff
cd /tmp/lazydiff && /tmp/lazydiff
```

In a real repo with open PRs: `]` `]` to reach the PR selector, `enter` to open a PR diff, `ga` to try an approve, `esc` to cancel, `h` to return, `q` to quit. Confirm `gh` errors surface in the status line when `gh` is unauthenticated.
