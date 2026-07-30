# Branch Diff Review Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add tree-mode cycling (worktree/staged/branch-selector), regex search, and branch-diff review to lazydiff's left pane.

**Architecture:** New `git.Branches`/`git.SnapshotBranch` methods feed a `ui.BranchSelector` model. The main `ui.Model` gains a `TreeMode` enum that controls left-pane rendering and key routing, replacing the retired `m`-key mode cycle. `[/]` become context-sensitive. A `SnapShotBranch` method is added to the `SnapshotLoader` interface.

**Tech Stack:** Go 1.24 + Bubble Tea, no new external dependencies. Search input reuses `bubbles/textinput` (already in `go.mod`).

## Global Constraints

- No table-driven tests / no `t.Run` subtests — every case is its own `TestXxx` func.
- Fakes, not mocks — `fakeRunner` for `git.CommandRunner`, `fakeLoader`/`fakeRunner`/`fakeMutator`/`fakeOpener` for `ui` interfaces.
- `go test ./... -count=1` then `go vet ./...` then `go build ./...` to verify after all tasks.
- Conventional Commits (`feat(scope): ...`, `test(scope): ...`, `docs(scope): ...`).
- Branch diff diff uses three-dot syntax: `git diff <default-branch>...<selected-branch>`.
- Model methods always use value receiver and return `(Model, tea.Cmd)` — never mutate in place with pointer receiver.

---

### Task 1: git.Branches and git.SnapshotBranch

**Files:**
- Modify: `git/repository.go` — add `Branches`
- Modify: `git/snapshot.go` — add `SnapshotBranch`
- Test: `git/repository_test.go` — add tests

**Interfaces:**
- Consumes: `Repository` (existing), `CommandRunner` (existing)
- Produces: `Repository.Branches(ctx) ([]string, error)`, `Repository.SnapshotBranch(ctx, branch) (Snapshot, error)`

- [ ] **Step 1: Write failing test for Branches**

```go
// Add to git/repository_test.go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./git -run TestBranches -v -count=1`
Expected: FAIL — `Branches undefined`

- [ ] **Step 3: Write minimal Branches implementation**

```go
// Add to git/repository.go
func (r Repository) Branches(ctx context.Context) ([]string, error) {
    output, err := r.run(ctx, "branch", "--format=%(refname:short)")
    if err != nil {
        return nil, fmt.Errorf("list branches: %w", err)
    }
    var branches []string
    for _, b := range strings.Split(strings.TrimSpace(string(output)), "\n") {
        b = strings.TrimSpace(b)
        if b != "" {
            branches = append(branches, b)
        }
    }
    return branches, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./git -run TestBranches -v -count=1`
Expected: PASS

- [ ] **Step 5: Write failing test for SnapshotBranch**

```go
// Add to git/repository_test.go
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
```

- [ ] **Step 6: Run test to verify it fails**

Run: `go test ./git -run TestSnapshotBranch -v -count=1`
Expected: FAIL

- [ ] **Step 7: Write minimal SnapshotBranch implementation**

```go
// Add to git/snapshot.go
func (r Repository) SnapshotBranch(ctx context.Context, branch string) (Snapshot, error) {
    base, err := r.DefaultBranch(ctx)
    if err != nil {
        return Snapshot{}, fmt.Errorf("resolve base for branch diff: %w", err)
    }
    ref := base + "..." + branch
    raw, err := r.run(ctx, "diff", "--no-color", "--binary", ref)
    if err != nil && len(raw) == 0 {
        return Snapshot{}, fmt.Errorf("diff %s: %w", ref, err)
    }
    rawText := string(raw)
    files, parseErr := diff.Parse(rawText)
    if parseErr != nil {
        return Snapshot{}, fmt.Errorf("parse %s diff: %w", ref, parseErr)
    }
    hash := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s", Branch, ref, rawText)))
    return Snapshot{ID: fmt.Sprintf("%x", hash[:]), Mode: Branch, Base: ref, RawDiff: rawText, Files: files}, nil
}
```

- [ ] **Step 8: Run test to verify it passes**

Run: `go test ./git -run TestSnapshotBranch -v -count=1`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add git/repository.go git/snapshot.go git/repository_test.go
git commit -m "feat(git): add Branches and SnapshotBranch methods"
```

---

### Task 2: ui.BranchSelector

**Files:**
- Create: `ui/branch_selector.go`
- Create: `ui/branch_selector_test.go`

**Interfaces:**
- Consumes: nothing (standalone model)
- Produces: `*BranchSelector` with `NewBranchSelector(branches, currentBranch)`, `Move(delta)`, `Selected()`, `Rows()`, `Select(branch)`

- [ ] **Step 1: Write failing tests**

```go
// ui/branch_selector_test.go
package ui

import (
    "testing"
)

func TestBranchSelectorDefaultBranchFirst(t *testing.T) {
    bs := NewBranchSelector([]string{"feature-a", "main", "feature-b"}, "feature-b", "main")
    rows := bs.Rows()
    if len(rows) != 3 || rows[0] != "main" {
        t.Fatalf("expected main first, got %v", rows)
    }
}

func TestBranchSelectorCurrentBranchHighlighted(t *testing.T) {
    bs := NewBranchSelector([]string{"main", "feature-a", "feature-b"}, "feature-a", "main")
    if bs.currentBranch != "feature-a" {
        t.Fatalf("currentBranch = %q", bs.currentBranch)
    }
}

func TestBranchSelectorMoveCursor(t *testing.T) {
    bs := NewBranchSelector([]string{"main", "feature"}, "main", "main")
    if bs.cursor != 0 {
        t.Fatalf("initial cursor = %d", bs.cursor)
    }
    bs.Move(1)
    if bs.cursor != 1 {
        t.Fatalf("after Move(1) cursor = %d", bs.cursor)
    }
}

func TestBranchSelectorSelectedEmptyInitially(t *testing.T) {
    bs := NewBranchSelector([]string{"main", "feature"}, "main", "main")
    if bs.selectedBranch != "" {
        t.Fatal("selectedBranch should be empty")
    }
}

func TestBranchSelectorSelect(t *testing.T) {
    bs := NewBranchSelector([]string{"main", "feature"}, "main", "main")
    bs.Select("feature")
    if bs.selectedBranch != "feature" {
        t.Fatalf("selectedBranch = %q", bs.selectedBranch)
    }
}

func TestBranchSelectorRowsReturnsAllBranches(t *testing.T) {
    bs := NewBranchSelector([]string{"main", "zebra", "alpha"}, "main", "main")
    rows := bs.Rows()
    // default first, then alpha, zebra
    if len(rows) != 3 || rows[0] != "main" || rows[1] != "alpha" || rows[2] != "zebra" {
        t.Fatalf("rows = %v", rows)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./ui -run TestBranchSelector -v -count=1`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

```go
// ui/branch_selector.go
package ui

import "sort"

type BranchSelector struct {
    branches       []string
    currentBranch  string
    defaultBranch  string
    cursor         int
    selectedBranch string
}

func NewBranchSelector(branches []string, currentBranch, defaultBranch string) *BranchSelector {
    sorted := make([]string, 0, len(branches))
    for _, b := range branches {
        if b != defaultBranch {
            sorted = append(sorted, b)
        }
    }
    sort.Strings(sorted)
    sorted = append([]string{defaultBranch}, sorted...)
    return &BranchSelector{
        branches:      sorted,
        currentBranch: currentBranch,
        defaultBranch: defaultBranch,
        cursor:        0,
    }
}

func (b *BranchSelector) Move(delta int) {
    b.cursor += delta
    if b.cursor < 0 {
        b.cursor = 0
    }
    if b.cursor >= len(b.branches) {
        b.cursor = len(b.branches) - 1
    }
}

func (b *BranchSelector) Select(branch string) {
    b.selectedBranch = branch
}

func (b *BranchSelector) Selected() string {
    if b.cursor < 0 || b.cursor >= len(b.branches) {
        return ""
    }
    return b.branches[b.cursor]
}

func (b *BranchSelector) Rows() []string {
    return append([]string(nil), b.branches...)
}

func (b *BranchSelector) Cursor() int { return b.cursor }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./ui -run TestBranchSelector -v -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add ui/branch_selector.go ui/branch_selector_test.go
git commit -m "feat(ui): add BranchSelector model for inline branch list"
```

---

### Task 3: ui.TreeMode enum, Model wiring, and key routing

**Files:**
- Modify: `ui/model.go` — add TreeMode, treeMode field, branchSelector, search fields; context-sensitive key routing
- Modify: `ui/model_test.go` — update fakeLoader, add new tests; add `SnapshotBranch` to SnapshotLoader interface
- Modify: `cmd/lazydiff/main.go` — add SnapshotBranch to repositoryLoader (minimal, just calls repo.SnapshotBranch)

Note: The existing `SnapshotLoader` interface must grow a `SnapshotBranch` method. Every existing implementer (fakeLoader, repositoryLoader) needs to implement it.

- [ ] **Step 1: Write failing tests for TreeMode routing and branch selection flow**

First, update fakeLoader to support SnapshotBranch:

```go
// In ui/model_test.go, update fakeLoader:
type fakeLoader struct {
    snapshots      []git.Snapshot
    branchSnapshots map[string]git.Snapshot
    index          int
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
    // fallback: return first snapshot
    return f.snapshots[0], nil
}
```

Now the test:

```go
func TestTreeModeCyclesWithBracketKeys(t *testing.T) {
    model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
    if model.treeMode != TreeModeWorktree {
        t.Fatalf("initial treeMode = %d, want Worktree", model.treeMode)
    }
    // ] cycles forward
    model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
    if model.treeMode != TreeModeStaged {
        t.Fatalf("after first ] treeMode = %d, want Staged", model.treeMode)
    }
    model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
    if model.treeMode != TreeModeBranchSelector {
        t.Fatalf("after second ] treeMode = %d, want BranchSelector", model.treeMode)
    }
    model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
    if model.treeMode != TreeModeWorktree {
        t.Fatalf("after third ] treeMode = %d, want Worktree", model.treeMode)
    }
    // [ cycles backward
    model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'[']}})
    if model.treeMode != TreeModeBranchSelector {
        t.Fatalf("backward treeMode = %d, want BranchSelector", model.treeMode)
    }
}

func TestBracketKeysStillCycleAnalysisTabsWhenFocusIsAnalysis(t *testing.T) {
    model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
    model.focus = FocusAnalysis
    model.activeTab = DetailTab
    model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
    if model.activeTab != OverallTab {
        t.Fatalf("activeTab = %d, want OverallTab", model.activeTab)
    }
}

func TestBranchSelectorEnterLoadsBranchDiff(t *testing.T) {
    loader := &fakeLoader{
        snapshots: []git.Snapshot{makeSnapshot("one")},
        branchSnapshots: map[string]git.Snapshot{
            "feature": makeSnapshot("branch"),
        },
    }
    model := newTestModel(loader, &fakeRunner{})
    model.treeMode = TreeModeBranchSelector
    model.branchSelector = NewBranchSelector([]string{"main", "feature"}, "main", "main")
    model.branchSelector.Move(1) // cursor on "feature"

    model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
    if model.treeMode != TreeModeBranchDiff {
        t.Fatalf("treeMode = %d, want BranchDiff", model.treeMode)
    }
    if model.branchSelector.selectedBranch != "feature" {
        t.Fatalf("selectedBranch = %q", model.branchSelector.selectedBranch)
    }
    if cmd == nil {
        t.Fatal("expected a cmd")
    }
    msg := cmd()
    snapMsg, ok := msg.(snapshotMsg)
    if !ok {
        t.Fatalf("expected snapshotMsg, got %T", msg)
    }
    if snapMsg.Snapshot.ID != "branch" {
        t.Fatalf("snapshot ID = %q, want 'branch'", snapMsg.Snapshot.ID)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./ui -run "TestTreeMode|TestBracketKeys|TestBranchSelectorEnter" -v -count=1`
Expected: FAIL

- [ ] **Step 3: Implement TreeMode, model fields, and key routing**

Add to `ui/model.go` (before `Focus` type):

```go
type TreeMode int

const (
    TreeModeWorktree TreeMode = iota
    TreeModeStaged
    TreeModeBranchSelector
    TreeModeBranchDiff
)

type branchesLoadedMsg struct {
    Branches []string
    Current  string
    Default  string
}
type branchesErrorMsg struct{ Err error }
```

Add to `SnapshotLoader` interface:

```go
type SnapshotLoader interface {
    Snapshot(context.Context, git.Mode) (git.Snapshot, error)
    SnapshotBranch(context.Context, string) (git.Snapshot, error)
}
```

Add fields to `Model` struct (after `mode git.Mode`):

```go
    treeMode         TreeMode
    branchSelector   *BranchSelector
    searchActive     bool
    searchQuery      string
```

In `NewModel`, set initial treeMode:

```go
    treeMode: TreeModeWorktree,
```

Add `loadBranchesCmd`:

```go
func (m Model) loadBranchesCmd() tea.Cmd {
    repo := m.repo
    return func() tea.Msg {
        branches, err := repo.Branches(context.Background())
        if err != nil {
            return branchesErrorMsg{Err: err}
        }
        current, err := repo.CurrentBranch(context.Background())
        if err != nil {
            return branchesErrorMsg{Err: err}
        }
        def, err := repo.DefaultBranch(context.Background())
        if err != nil {
            return branchesErrorMsg{Err: err}
        }
        return branchesLoadedMsg{Branches: branches, Current: current, Default: def}
    }
}
```

Add `startPRForReviewedBranchCmd` — similar to `startPRCmd` but uses the reviewed branch:

```go
func (m Model) startPRForReviewedBranchCmd() tea.Cmd {
    mutator, loader, cfg, templates := m.mutator, m.loader, m.cfg, m.templates
    branch := m.branchSelector.selectedBranch
    repoRoot := m.repo.Root
    return func() tea.Msg {
        ctx := context.Background()
        base, err := mutator.DefaultBranch(ctx)
        if err != nil {
            return prPrepMsg{Err: err}
        }
        if branch == base {
            return prPrepMsg{Err: fmt.Errorf("cannot open a pull request from the default branch %q", base)}
        }
        snapshot, err := loader.SnapshotBranch(ctx, branch)
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
```

Add `branchesLoadedMsg` and `branchesErrorMsg` handlers to `Update`:

```go
case branchesLoadedMsg:
    m.branchSelector = NewBranchSelector(message.Branches, message.Current, message.Default)
    return m, nil
case branchesErrorMsg:
    m.status = "branch list: " + message.Err.Error()
    return m, nil
```

Modify `updateKey` — change the `case "["` and `case "]"` handling to be context-sensitive. Replace the existing `[`/`]` cases:

```go
case "[":
    if m.focus == FocusAnalysis {
        if m.activeTab > 0 {
            m.activeTab--
        } else {
            m.activeTab = RequestLogTab
        }
    } else if m.focus == FocusTree {
        m.treeMode--
        if m.treeMode == TreeModeBranchDiff {
            m.treeMode = TreeModeStaged
        } else if m.treeMode < 0 {
            m.treeMode = TreeModeBranchSelector
        }
    }
case "]":
    if m.focus == FocusAnalysis {
        if m.activeTab < RequestLogTab {
            m.activeTab++
        } else {
            m.activeTab = DetailTab
        }
    } else if m.focus == FocusTree {
        if m.treeMode == TreeModeBranchDiff {
            m.treeMode = TreeModeWorktree
        } else {
            m.treeMode = (m.treeMode + 1) % 3
            if m.treeMode == TreeModeBranchSelector && m.branchSelector == nil {
                return m, m.loadBranchesCmd()
            }
        }
    }
```

Modify `case " "` and `case "ctrl+a"` to check `treeMode` instead of `mode`:

```go
case " ":
    if m.focus == FocusTree && m.treeMode == TreeModeWorktree {
        m.tree.ToggleCheck()
    }
case "ctrl+a":
    if m.focus == FocusTree && m.treeMode == TreeModeWorktree {
        m.tree.ToggleCheckAll()
    }
```

Modify `case "c"` to check `treeMode`:

```go
case "c":
    if m.treeMode == TreeModeWorktree && len(m.tree.StagingPlan()) > 0 {
        return m, m.startCommitCmd()
    }
```

Handle `enter` and `l` in branch selector:

```go
case "enter":
    if m.focus == FocusTree && m.treeMode == TreeModeBranchSelector && m.branchSelector != nil {
        branch := m.branchSelector.Selected()
        if branch != "" {
            m.branchSelector.Select(branch)
            m.treeMode = TreeModeBranchDiff
            return m, m.refreshCmd()
        }
    }
```

Modify `case "l"` to handle branch selector:

```go
case "l":
    if m.focus == FocusTree && m.treeMode == TreeModeBranchSelector && m.branchSelector != nil {
        branch := m.branchSelector.Selected()
        if branch != "" {
            m.branchSelector.Select(branch)
            m.treeMode = TreeModeBranchDiff
            return m, m.refreshCmd()
        }
    }
    if m.focus == FocusTree {
        m.tree.ExpandOrDescend()
        m.diffScroll = 0
        return m, m.renderSelectedCmd()
    }
```

Handle `h` returning from branch diff to branch selector:

```go
case "h":
    if m.focus == FocusTree && m.treeMode == TreeModeBranchDiff {
        m.treeMode = TreeModeBranchSelector
        return m, nil
    }
    if m.focus == FocusTree {
        m.tree.CollapseOrParent()
        m.diffScroll = 0
        return m, m.renderSelectedCmd()
    }
```

Modify `case "o"` to handle branch diff mode:

```go
case "o":
    if m.treeMode == TreeModeBranchDiff && m.branchSelector != nil && m.branchSelector.selectedBranch != "" {
        return m, m.startPRForReviewedBranchCmd()
    }
    return m, m.startPRCmd()
```

Modify the `refreshCmd` to handle branch diff mode:

```go
func (m Model) refreshCmd() tea.Cmd {
    if m.treeMode == TreeModeBranchDiff && m.branchSelector != nil && m.branchSelector.selectedBranch != "" {
        loader := m.loader
        branch := m.branchSelector.selectedBranch
        return func() tea.Msg {
            snapshot, err := loader.SnapshotBranch(context.Background(), branch)
            if err != nil {
                return snapshotErrorMsg{Err: err}
            }
            return snapshotMsg{Snapshot: snapshot}
        }
    }
    loader, mode := m.loader, m.mode
    return func() tea.Msg {
        if loader == nil {
            return snapshotErrorMsg{Err: fmt.Errorf("snapshot loader unavailable")}
        }
        snapshot, err := loader.Snapshot(context.Background(), mode)
        if err != nil {
            return snapshotErrorMsg{Err: err}
        }
        return snapshotMsg{Snapshot: snapshot}
    }
}
```

Remove the `m`-key handler (delete the `case "m":` block).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./ui -run "TestTreeMode|TestBracketKeys|TestBranchSelectorEnter" -v -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add ui/model.go ui/model_test.go
git commit -m "feat(ui): add TreeMode, branch selector state, and context-sensitive [/] keys"
```

---

### Task 4: cmd/lazydiff/main.go — repositoryLoader SnapshotBranch

**Files:**
- Modify: `cmd/lazydiff/main.go` — add `SnapshotBranch` method

- [ ] **Step 1: Write the implementation**

In `cmd/lazydiff/main.go`, modify the `repositoryLoader`:

```go
type repositoryLoader struct{ repo git.Repository }

func (l repositoryLoader) Snapshot(ctx context.Context, mode git.Mode) (git.Snapshot, error) {
    return l.repo.Snapshot(ctx, mode)
}

func (l repositoryLoader) SnapshotBranch(ctx context.Context, branch string) (git.Snapshot, error) {
    return l.repo.SnapshotBranch(ctx, branch)
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: no error

- [ ] **Step 3: Commit**

```bash
git add cmd/lazydiff/main.go
git commit -m "fix(cmd): implement SnapshotBranch on repositoryLoader"
```

---

### Task 5: ui rendering — branch selector, status line, help text

**Files:**
- Modify: `ui/render.go` — render branch selector in left pane, update status line, update help text

- [ ] **Step 1: Write the tests**

```go
// ui/view_test.go
func TestRenderTreeShowsBranchSelectorWhenInBranchSelectorMode(t *testing.T) {
    model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
    model.termW, model.termH = 120, 40
    model.layout = ComputeLayout(120, 40)
    model.treeMode = TreeModeBranchSelector
    model.branchSelector = NewBranchSelector([]string{"main", "feature"}, "feature")
    out := model.renderTree(model.layout.Files)
    if !strings.Contains(out, "feature") {
        t.Fatalf("expected branch list in tree pane:\n%s", out)
    }
}

func TestRenderTreeShowsBranchDiffLabelWhenInBranchDiffMode(t *testing.T) {
    model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
    model.termW, model.termH = 120, 40
    model.layout = ComputeLayout(120, 40)
    model.treeMode = TreeModeBranchDiff
    model.snapshot = makeSnapshot("one")
    model.haveSnap = true
    model.tree = NewTree(model.snapshot.Files)
    out := model.renderTree(model.layout.Files)
    if !strings.Contains(out, "[1]") || !strings.Contains(out, "BRANCH DIFF") {
        t.Fatalf("expected BRANCH DIFF title in tree pane:\n%s", out)
    }
}

func TestStatusLineShowsTreeMode(t *testing.T) {
    model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
    model.termW, model.termH = 120, 40
    model.layout = ComputeLayout(120, 40)
    model.snapshot = makeSnapshot("one")
    model.haveSnap = true
    model.tree = NewTree(model.snapshot.Files)
    model.diffStyled = true

    // Worktree
    model.treeMode = TreeModeWorktree
    line := model.statusLine()
    if !strings.Contains(line, "worktree") {
        t.Fatalf("expected worktree in status line:\n%s", line)
    }

    // Staged
    model.treeMode = TreeModeStaged
    line = model.statusLine()
    if !strings.Contains(line, "staged") {
        t.Fatalf("expected staged in status line:\n%s", line)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./ui -run "TestRenderTreeShowsBranchSelector|TestRenderTreeShowsBranchDiff|TestStatusLineShowsTreeMode" -v -count=1`
Expected: FAIL

- [ ] **Step 3: Modify renderTree**

Replace `renderTree` in `ui/render.go` to handle branch selector and branch diff modes:

```go
func (m Model) renderTree(r Rect) string {
    if m.treeMode == TreeModeBranchSelector && m.branchSelector != nil {
        return m.renderBranchSelector(r)
    }
    title := "CHANGED FILES"
    if m.treeMode == TreeModeBranchDiff {
        title = "BRANCH DIFF"
    }
    titleRendered := delta.Truncate(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245")).Render("[1] "+title), max(1, r.W-2))
    lines := []string{titleRendered}
    ...
    // rest of existing renderTree unchanged
}

// Add new method:
func (m Model) renderBranchSelector(r Rect) string {
    title := delta.Truncate(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245")).Render("[1] BRANCHES"), max(1, r.W-2))
    lines := []string{title}
    rows := m.branchSelector.Rows()
    if len(rows) == 0 {
        empty := delta.Truncate(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("(no local branches)"), max(1, r.W-2))
        lines = append(lines, empty)
        return box(r, strings.Join(padLines(lines, r.H-2), "\n"), m.focus == FocusTree)
    }
    contentH := r.H - 3
    if contentH < 1 {
        contentH = 1
    }
    // simple clamping (no scroll for now — small branch lists)
    maxW := max(1, r.W-2)
    for i, branch := range rows {
        prefix := "  "
        if i == m.branchSelector.Cursor() {
            prefix = "▶ "
        }
        active := branch == m.branchSelector.currentBranch
        style := lipgloss.Color("245")
        if i == m.branchSelector.Cursor() {
            style = lipgloss.Color("51")
        } else if branch == m.branchSelector.currentBranch {
            style = lipgloss.Color("228")
        }
        line := delta.Truncate(prefix+branch, maxW)
        lines = append(lines, lipgloss.NewStyle().Foreground(style).Render(line))
    }
    return box(r, strings.Join(padLines(lines, r.H-2), "\n"), m.focus == FocusTree)
}
```

Update `statusLine`:

```go
func (m Model) statusLine() string {
    deltaState := "delta fallback"
    if m.diffStyled {
        deltaState = "delta active"
    }
    modeLabel := m.treeMode.String()
    if m.treeMode == TreeModeBranchDiff && m.branchSelector != nil {
        modeLabel = "branch diff: " + m.branchSelector.selectedBranch
    } else if m.treeMode == TreeModeBranchSelector {
        modeLabel = "branch selector"
    }
    updateHint := ""
    if m.showUpdateModal || m.showUpdating {
        updateHint = ""
    } else if m.updateVersion != "" {
        updateHint = "  [u] update v" + m.updateVersion
    }
    return fmt.Sprintf("mode: %s  %s  %s  %s%s  %s", modeLabel, deltaState, m.status, "[1-3] pane  [space] check  [c] commit  [o] PR  [?] help  [q] quit", updateHint, version.Current)
}
```

Add a `String()` method on TreeMode:

```go
func (m TreeMode) String() string {
    switch m {
    case TreeModeWorktree:
        return "worktree"
    case TreeModeStaged:
        return "staged"
    case TreeModeBranchSelector:
        return "branch selector"
    case TreeModeBranchDiff:
        return "branch diff"
    default:
        return "unknown"
    }
}
```

Update `helpText` — remove the `m` key reference and note `[/]` for left-pane cycling:

Replace the section "General" to reflect changes:

In the help text:
- change `key("m", "Toggle diff mode")` to `key("[/]", "Cycle left pane")`
- add context note: `dim.Render("  [/] cycles analysis tabs when focus is on the analysis pane")`

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./ui -run "TestRenderTreeShowsBranchSelector|TestRenderTreeShowsBranchDiff|TestStatusLineShowsTreeMode" -v -count=1`
Expected: PASS

- [ ] **Step 5: Run all existing tests to check for regressions**

Run: `go test ./... -count=1`
If tests fail, fix issues until they pass.

- [ ] **Step 6: Commit**

```bash
git add ui/render.go ui/view_test.go
git commit -m "feat(ui): render branch selector, branch diff, and update status line"
```

---

### Task 6: Regex search for left pane

**Files:**
- Modify: `ui/model.go` — add search state, `/` key input, `n`/`N`/`esc` handling
- Modify: `ui/render.go` — filter rows by search, render search input
- New: `ui/search.go` — search helper
- Modify: `ui/model_test.go` — test search filtering
- Modify: `ui/view_test.go` — test search rendering

- [ ] **Step 1: Write failing tests**

```go
// ui/model_test.go
func TestSearchFiltersFilesInTree(t *testing.T) {
    model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
    model.termW, model.termH = 120, 40
    model.layout = ComputeLayout(120, 40)
    model.snapshot = makeSnapshot("one")
    model.haveSnap = true
    model.tree = NewTree(model.snapshot.Files)

    // Start search
    model.searchActive = true
    model.searchQuery = "a.go"
    model.applySearchFilter()
    visible := model.visibleNodes()
    if len(visible) == 0 {
        t.Fatal("expected at least one visible node")
    }
    // The a.go file (and its hunks) should be visible
    found := false
    for _, n := range visible {
        if n.File != nil && n.File.Path == "a.go" {
            found = true
            break
        }
    }
    if !found {
        t.Fatal("expected a.go to be visible after search")
    }
}

func TestSearchFiltersBranchNames(t *testing.T) {
    model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
    model.treeMode = TreeModeBranchSelector
    model.branchSelector = NewBranchSelector([]string{"main", "feature-x", "hotfix-y"}, "main")
    model.searchActive = true
    model.searchQuery = "feature"
    model.applySearchFilter()
    visible := model.visibleBranches()
    if len(visible) != 1 || visible[0] != "feature-x" {
        t.Fatalf("visible branches = %v, want [feature-x]", visible)
    }
}
```

```go
// ui/view_test.go
func TestRenderTreeShowsSearchPromptWhenSearchActive(t *testing.T) {
    model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
    model.termW, model.termH = 120, 40
    model.layout = ComputeLayout(120, 40)
    model.snapshot = makeSnapshot("one")
    model.haveSnap = true
    model.tree = NewTree(model.snapshot.Files)
    model.searchActive = true
    model.searchQuery = "test"
    out := model.renderTree(model.layout.Files)
    if !strings.Contains(out, "test") || !strings.Contains(out, "/") {
        t.Fatalf("expected search indicator in tree:\n%s", out)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./ui -run TestSearch -v -count=1`
Expected: FAIL

- [ ] **Step 3: Write search helper and model wiring**

Create `ui/search.go`:

```go
package ui

import (
    "regexp"
)

func compileSearch(query string) (*regexp.Regexp, error) {
    if query == "" {
        return nil, nil
    }
    return regexp.Compile("(?i)" + query)
}

func (m Model) applySearchFilter() Model {
    if !m.searchActive || m.searchQuery == "" {
        m.searchFilter = nil
        return m
    }
    re, err := compileSearch(m.searchQuery)
    if err != nil {
        m.searchFilter = nil
        m.status = "search: " + err.Error()
        return m
    }
    m.searchFilter = re
    return m
}

func (m Model) visibleNodes() []*TreeNode {
    if m.searchFilter == nil {
        return m.tree.Rows()
    }
    all := m.tree.Rows()
    var result []*TreeNode
    for _, n := range all {
        if m.searchFilter.MatchString(nodeSearchLabel(n)) {
            result = append(result, n)
        }
    }
    return result
}

func (m Model) visibleBranches() []string {
    if m.branchSelector == nil {
        return nil
    }
    rows := m.branchSelector.Rows()
    if m.searchFilter == nil {
        return rows
    }
    var result []string
    for _, b := range rows {
        if m.searchFilter.MatchString(b) {
            result = append(result, b)
        }
    }
    return result
}

func nodeSearchLabel(n *TreeNode) string {
    if n.File != nil {
        return n.File.DisplayPath()
    }
    return n.Label
}
```

Add to `Model` struct: `searchFilter *regexp.Regexp`, plus import `regexp`.

In `Update`, add search intercept before dialog check:

```go
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
    if m.searchActive {
        if keyMsg, ok := msg.(tea.KeyMsg); ok {
            return m.updateSearchKey(keyMsg)
        }
    }
    if keyMsg, ok := msg.(tea.KeyMsg); ok && m.dialog != nil {
        return m.updateDialogKey(keyMsg)
    }
    // ... rest unchanged
}
```

Add `updateSearchKey`:

```go
func (m Model) updateSearchKey(key tea.KeyMsg) (Model, tea.Cmd) {
    switch key.String() {
    case "esc":
        m.searchActive = false
        m.searchQuery = ""
        m.searchFilter = nil
        return m, nil
    case "enter":
        m.searchActive = false
        return m, nil
    case "n":
        visible := m.visibleNodes()
        for i, n := range visible {
            if n.ID() == m.tree.selectedID && i < len(visible)-1 {
                m.tree.selectNode(visible[i+1])
                break
            }
        }
        return m, nil
    case "N":
        visible := m.visibleNodes()
        for i, n := range visible {
            if n.ID() == m.tree.selectedID && i > 0 {
                m.tree.selectNode(visible[i-1])
                break
            }
        }
        return m, nil
    case "backspace":
        if len(m.searchQuery) > 0 {
            m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
        }
        return m.applySearchFilter(), nil
    default:
        m.searchQuery += key.String()
        return m.applySearchFilter(), nil
    }
}
```

In `updateKey`, add the `/` handler:

```go
case "/":
    if m.focus == FocusTree {
        m.searchActive = true
        m.searchQuery = ""
        return m, nil
    }
```

In `renderTree`, replace `nodes := m.tree.Rows()` with `nodes := m.visibleNodes()`.

In `renderBranchSelector`, replace `rows := m.branchSelector.Rows()` with `rows := m.visibleBranches()`.

- [ ] **Step 5: Run tests**

Run: `go test ./ui -run TestSearch -v -count=1`
Expected: PASS

- [ ] **Step 6: Run all tests**

Run: `go test ./... -count=1`
Expected: all pass

- [ ] **Step 7: Commit**

```bash
git add ui/search.go ui/model.go ui/render.go ui/model_test.go ui/view_test.go
git commit -m "feat(ui): add regex search for left pane (files and branches)"
```

---

### Task 7: PTY integration test for branch diff mode

**Files:**
- Modify: `integration/pty_linux_test.go` — add test for branch diff mode

- [ ] **Step 1: Write failing test**

```go
func TestBranchDiffModeSelectsBranchAndShowsDiff(t *testing.T) {
    fixture := newFixture(t)
    defer fixture.close()
    fixture.addCommitFile("base.txt", "hello")
    fixture.git("checkout", "-b", "feature")
    fixture.addCommitFile("feat.txt", "new feature")
    fixture.git("checkout", "main")
    fixture.launch()
    fixture.send("]")        // worktree → staged
    fixture.send("]")        // staged → branch selector
    fixture.match("feature") // branch appears in selector
    fixture.send("j")        // move to feature
    fixture.send("enter")    // select feature → branch diff
    fixture.match("BRANCH DIFF")
    fixture.match("feat.txt")
    fixture.send("h")        // back to branch selector
    fixture.match("BRANCHES")
    fixture.quit()
}
```

- [ ] **Step 2: Add match method to fixture**

Add a `match` helper that waits for expected text to appear (using the same PTY output-gathering pattern as existing tests).

- [ ] **Step 3: Run test**

Run: `go test ./integration -run TestBranchDiffMode -v -count=1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add integration/pty_linux_test.go
git commit -m "test(integration): cover branch diff mode flow"
```

---

### Verification

- [ ] **Step 1: Run full test suite**

```bash
go test ./... -count=1
go vet ./...
go build ./...
git diff --check
```

Expected: all pass, no vet warnings, clean build, no whitespace errors.
