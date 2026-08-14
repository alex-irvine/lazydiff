# Worktree Selector Pane Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Transform the Worktree pane from a working-tree diff view into a worktree selector that lists all worktrees by directory name, allowing selection and file editing in each worktree's actual directory.

**Architecture:** Add a `WorktreeSelector` (mirrors `BranchSelector`) and a new `TreeModeWorktreeDiff` mode. The `]` cycle becomes WorktreeSelector → BranchSelector → PRSelector. Selecting a worktree loads its working-tree snapshot via `git -C <worktree-path>`. The `[e]` key opens files from the selected worktree's directory.

**Tech Stack:** Go, Bubble Tea, git CLI

## Global Constraints

- Go 1.24, no new dependencies
- All mutations via `git` CLI (no libgit2)
- Fake-based testing (no real git in unit tests except `git/repository_test.go`)
- Conventional Commits
- `go test ./... -count=1 && go vet ./... && go build ./...` must pass

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `ui/worktree_selector.go` | Create | `WorktreeSelector` struct + methods |
| `ui/worktree_selector_test.go` | Create | Unit tests for WorktreeSelector |
| `git/repository.go` | Modify | Add `WorktreeSnapshot(ctx, worktreePath)` |
| `git/repository_test.go` | Modify | Test `WorktreeSnapshot` |
| `ui/model.go` | Modify | Wire WorktreeSelector, new mode, key handlers |
| `ui/render.go` | Modify | Render worktree list, update tab bar |
| `ui/model_test.go` | Modify | Tests for worktree selection flow |
| `ui/view_test.go` | Modify | Test worktree list rendering |

---

### Task 1: Create WorktreeSelector

**Files:**
- Create: `ui/worktree_selector.go`
- Create: `ui/worktree_selector_test.go`

**Interfaces:**
- Produces: `WorktreeSelector` struct with `Move`, `Select`, `Selected`, `SelectedPath`, `Rows`, `Cursor`, `HasWorktree` methods

- [ ] **Step 1: Create `ui/worktree_selector.go`**

```go
package ui

import "sort"

type WorktreeEntry struct {
	Name string // directory basename (e.g. "feature")
	Path string // full worktree path
}

type WorktreeSelector struct {
	worktrees   []WorktreeEntry
	current     string // name of the current/main worktree
	cursor      int
	selected    string
}

func NewWorktreeSelector(entries []WorktreeEntry, current string) *WorktreeSelector {
	sorted := make([]WorktreeEntry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Name == current {
			return true
		}
		if sorted[j].Name == current {
			return false
		}
		return sorted[i].Name < sorted[j].Name
	})
	return &WorktreeSelector{
		worktrees: sorted,
		current:   current,
	}
}

func (w *WorktreeSelector) Move(delta int) {
	w.cursor += delta
	if w.cursor < 0 {
		w.cursor = 0
	}
	if w.cursor >= len(w.worktrees) {
		w.cursor = len(w.worktrees) - 1
	}
}

func (w *WorktreeSelector) Select(name string) {
	w.selected = name
}

func (w *WorktreeSelector) Selected() string {
	if w.cursor < 0 || w.cursor >= len(w.worktrees) {
		return ""
	}
	return w.worktrees[w.cursor].Name
}

func (w *WorktreeSelector) SelectedPath() string {
	if w.cursor < 0 || w.cursor >= len(w.worktrees) {
		return ""
	}
	return w.worktrees[w.cursor].Path
}

func (w *WorktreeSelector) Rows() []WorktreeEntry {
	return append([]WorktreeEntry(nil), w.worktrees...)
}

func (w *WorktreeSelector) Cursor() int { return w.cursor }

func (w *WorktreeSelector) HasWorktree(name string) bool {
	for _, e := range w.worktrees {
		if e.Name == name {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Create `ui/worktree_selector_test.go`**

```go
package ui

import "testing"

func TestWorktreeSelectorCurrentFirst(t *testing.T) {
	entries := []WorktreeEntry{
		{Name: "feature", Path: "/wt/feature"},
		{Name: "repo", Path: "/repo"},
		{Name: "fix", Path: "/wt/fix"},
	}
	ws := NewWorktreeSelector(entries, "repo")
	rows := ws.Rows()
	if len(rows) != 3 || rows[0].Name != "repo" {
		t.Fatalf("expected repo first, got %v", rows)
	}
}

func TestWorktreeSelectorMove(t *testing.T) {
	entries := []WorktreeEntry{
		{Name: "main", Path: "/main"},
		{Name: "feature", Path: "/wt/feature"},
	}
	ws := NewWorktreeSelector(entries, "main")
	ws.Move(1)
	if ws.Selected() != "feature" {
		t.Fatalf("expected feature, got %q", ws.Selected())
	}
}

func TestWorktreeSelectorSelectedPath(t *testing.T) {
	entries := []WorktreeEntry{
		{Name: "main", Path: "/main"},
		{Name: "feature", Path: "/wt/feature"},
	}
	ws := NewWorktreeSelector(entries, "main")
	ws.Move(1)
	if ws.SelectedPath() != "/wt/feature" {
		t.Fatalf("expected /wt/feature, got %q", ws.SelectedPath())
	}
}

func TestWorktreeSelectorHasWorktree(t *testing.T) {
	entries := []WorktreeEntry{
		{Name: "main", Path: "/main"},
		{Name: "feature", Path: "/wt/feature"},
	}
	ws := NewWorktreeSelector(entries, "main")
	if !ws.HasWorktree("feature") {
		t.Fatal("expected HasWorktree feature")
	}
	if ws.HasWorktree("nonexistent") {
		t.Fatal("expected !HasWorktree nonexistent")
	}
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./ui/... -run TestWorktreeSelector -v -count=1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add ui/worktree_selector.go ui/worktree_selector_test.go
git commit -m "feat(ui): add WorktreeSelector for worktree pane"
```

---

### Task 2: Add WorktreeSnapshot to git.Repository

**Files:**
- Modify: `git/repository.go`
- Modify: `git/repository_test.go`

**Interfaces:**
- Produces: `Repository.WorktreeSnapshot(ctx, worktreePath string) (Snapshot, error)`

- [ ] **Step 1: Add WorktreeSnapshot to `git/repository.go`**

Add after `SnapshotBranch`:

```go
func (r Repository) WorktreeSnapshot(ctx context.Context, worktreePath string) (Snapshot, error) {
	args := []string{"-C", worktreePath, "diff", "--no-color", "--binary", "HEAD"}
	raw, err := r.runner.Run(ctx, "git", args...)
	if err != nil && len(raw) == 0 {
		return Snapshot{}, fmt.Errorf("worktree diff at %s: %w", worktreePath, err)
	}
	rawText := string(raw)
	files, parseErr := diff.Parse(rawText)
	if parseErr != nil {
		return Snapshot{}, fmt.Errorf("parse worktree diff at %s: %w", worktreePath, parseErr)
	}
	hash := sha256.Sum256([]byte(fmt.Sprintf("worktree\x00%s\x00%s", worktreePath, rawText)))
	return Snapshot{ID: fmt.Sprintf("%x", hash[:]), Mode: WorkingTree, Base: "HEAD", RawDiff: rawText, Files: files}, nil
}
```

- [ ] **Step 2: Add test to `git/repository_test.go`**

```go
func TestWorktreeSnapshotRunsDiffFromWorktreeDir(t *testing.T) {
	dir := testRepo(t)
	runGit(t, dir, "checkout", "-b", "wt-branch")
	runGit(t, dir, "checkout", "main")
	wtDir := filepath.Join(t.TempDir(), "wt")
	runGit(t, dir, "worktree", "add", wtDir, "wt-branch")
	// Make a change in the worktree
	if err := os.WriteFile(filepath.Join(wtDir, "wt.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, wtDir, "add", "wt.txt")
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
```

- [ ] **Step 3: Run tests**

Run: `go test ./git/... -run TestWorktreeSnapshot -v -count=1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add git/repository.go git/repository_test.go
git commit -m "feat(git): add WorktreeSnapshot for loading diff from a worktree directory"
```

---

### Task 3: Add TreeModeWorktreeDiff and wire into model

**Files:**
- Modify: `ui/model.go`

**Interfaces:**
- Consumes: `WorktreeSelector` (Task 1), `Repository.WorktreeSnapshot` (Task 2)
- Produces: `TreeModeWorktreeDiff` constant, `worktreeSelector` field on Model, `loadWorktreeSnapshotCmd`

- [ ] **Step 1: Add TreeModeWorktreeDiff constant**

In `ui/model.go`, after `TreeModeWorktree`:

```go
TreeModeWorktree TreeMode = iota
TreeModeWorktreeDiff
TreeModeStaged
```

- [ ] **Step 2: Add worktreeSelector field to Model**

In the `Model` struct, after `branchSelector`:

```go
branchSelector    *BranchSelector
worktreeSelector  *WorktreeSelector
selectedWorktree  string // path of the selected worktree
```

- [ ] **Step 3: Add worktreeSnapshot case to branchesLoadedMsg handler**

After the existing `branchesLoadedMsg` handler that creates the BranchSelector, also create the WorktreeSelector:

```go
case branchesLoadedMsg:
	m.branchSelector = NewBranchSelector(message.Branches, message.Current, message.Default, message.Worktrees)
	// Build worktree entries from the worktrees map
	var wtEntries []WorktreeEntry
	for branch, path := range message.Worktrees {
		name := filepath.Base(path)
		wtEntries = append(wtEntries, WorktreeEntry{Name: name, Path: path})
	}
	if len(wtEntries) == 0 {
		// At minimum, the main worktree exists
		wtEntries = append(wtEntries, WorktreeEntry{Name: filepath.Base(m.repo.Root), Path: m.repo.Root})
	}
	m.worktreeSelector = NewWorktreeSelector(wtEntries, filepath.Base(m.repo.Root))
	return m, nil
```

- [ ] **Step 4: Add loadWorktreeSnapshotCmd**

```go
func (m Model) loadWorktreeSnapshotCmd(path string) tea.Cmd {
	loader := m.loader
	repo := m.repo
	return func() tea.Msg {
		snapshot, err := repo.WorktreeSnapshot(context.Background(), path)
		if err != nil {
			return snapshotErrorMsg{Err: err}
		}
		return snapshotMsg{Snapshot: snapshot}
	}
}
```

- [ ] **Step 5: Update `]` key handler to start at WorktreeSelector**

The first `]` press from Worktree mode should enter WorktreeSelector (it already does since TreeModeWorktree maps to `default` in the `]` switch). No change needed — `TreeModeWorktree` already falls through to the default case which sets `m.treeMode = TreeModeBranchSelector`. We need to change the cycle order.

Replace the `]` handler:

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
		case TreeModeWorktree, TreeModeWorktreeDiff:
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
			if m.worktreeSelector == nil {
				return m, m.loadBranchesCmd()
			}
		}
	}
```

- [ ] **Step 6: Update `[` key handler**

Replace the `[` handler:

```go
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
			if m.worktreeSelector == nil {
				return m, m.loadBranchesCmd()
			}
		default:
			m.treeMode = TreeModePRSelector
			if m.prSelector == nil {
				return m, m.loadPRsCmd()
			}
		}
	}
```

- [ ] **Step 7: Update `h` key handler for worktree back-navigation**

Add after the `TreeModeBranchDiff` → `TreeModeBranchSelector` case:

```go
if m.focus == FocusTree && m.treeMode == TreeModeWorktreeDiff {
	m.treeMode = TreeModeWorktree
	return m, nil
}
```

- [ ] **Step 8: Update `l`/enter key for worktree selection**

Add a new case in the `l` handler for worktree selector:

```go
if m.focus == FocusTree && m.treeMode == TreeModeWorktree && m.worktreeSelector != nil {
	name := m.worktreeSelector.Selected()
	if name != "" {
		m.worktreeSelector.Select(name)
		m.selectedWorktree = m.worktreeSelector.SelectedPath()
		m.treeMode = TreeModeWorktreeDiff
		return m, m.loadWorktreeSnapshotCmd(m.selectedWorktree)
	}
}
```

- [ ] **Step 9: Update `e` key handler for worktree diff mode**

After the existing `TreeModeWorktree` case for `e`:

```go
if m.focus == FocusTree && m.treeMode == TreeModeWorktreeDiff && m.selectedWorktree != "" {
	if file, _, ok := m.tree.Selected(); ok {
		path := filepath.Join(m.selectedWorktree, file.Path)
		return m, m.openEditorCmd(path)
	}
}
```

- [ ] **Step 10: Update `c` (commit) to only work in TreeModeWorktree**

The existing `c` handler already checks `m.treeMode == TreeModeWorktree`. No change needed — commits only work from the main worktree's working tree mode.

- [ ] **Step 11: Run build**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 12: Commit**

```bash
git add ui/model.go
git commit -m "feat(ui): wire WorktreeSelector into model with TreeModeWorktreeDiff"
```

---

### Task 4: Update rendering for worktree pane

**Files:**
- Modify: `ui/render.go`

**Interfaces:**
- Consumes: `WorktreeSelector` (Task 1), `TreeModeWorktreeDiff` (Task 3)

- [ ] **Step 1: Add renderWorktreeSelector method**

Add after `renderBranchSelector`:

```go
func (m Model) renderWorktreeSelector(r Rect) string {
	title := delta.Truncate(m.renderTabBar(), max(1, r.W-2))
	lines := []string{title}
	rows := m.worktreeSelector.Rows()
	if len(rows) == 0 {
		empty := delta.Truncate(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("(no worktrees)"), max(1, r.W-2))
		lines = append(lines, empty)
		return box(r, strings.Join(padLines(lines, r.H-2), "\n"), m.focus == FocusTree)
	}
	maxW := max(1, r.W-2)
	for i, entry := range rows {
		prefix := "  "
		if i == m.worktreeSelector.Cursor() {
			prefix = "▶ "
		}
		style := lipgloss.Color("245")
		if i == m.worktreeSelector.Cursor() {
			style = lipgloss.Color("51")
		} else if entry.Name == m.worktreeSelector.current {
			style = lipgloss.Color("228")
		}
		line := delta.Truncate(prefix+entry.Name, maxW)
		lines = append(lines, lipgloss.NewStyle().Foreground(style).Render(line))
	}
	return box(r, strings.Join(padLines(lines, r.H-2), "\n"), m.focus == FocusTree)
}
```

- [ ] **Step 2: Update renderTree to dispatch to worktree selector**

In `renderTree`, before the `TreeModeBranchSelector` check:

```go
if m.treeMode == TreeModeWorktree && m.worktreeSelector != nil {
	return m.renderWorktreeSelector(r)
}
```

Note: `TreeModeWorktree` now shows the worktree list. The old working-tree-diff behavior moves to `TreeModeWorktreeDiff`. Since `renderTree` falls through to the default tree rendering for unknown modes, `TreeModeWorktreeDiff` will automatically show the file tree (same as before).

- [ ] **Step 3: Update renderTabBar for worktree modes**

Replace the `TreeModeWorktree` case in `renderTabBar`:

```go
case TreeModeWorktree:
	return active("[1] Worktree") + "  " + inactive("Branch") + "  " + inactive("PRs")
case TreeModeWorktreeDiff:
	name := "Worktree"
	if m.selectedWorktree != "" {
		name = filepath.Base(m.selectedWorktree)
	}
	return active(name) + "  " + inactive("Branch") + "  " + inactive("PRs")
```

- [ ] **Step 4: Update status line for worktree diff mode**

In the status line (the `String()` method or status rendering), add:

```go
case TreeModeWorktreeDiff:
	return "worktree diff"
```

- [ ] **Step 5: Run build and tests**

Run: `go build ./... && go test ./ui/... -count=1`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add ui/render.go
git commit -m "feat(ui): render worktree selector list and update tab bar"
```

---

### Task 5: Update tests for worktree selection flow

**Files:**
- Modify: `ui/model_test.go`
- Modify: `ui/view_test.go`

- [ ] **Step 1: Add test for worktree selector creation on branchesLoadedMsg**

In `ui/model_test.go`:

```go
func TestWorktreeSelectorCreatedOnBranchesLoaded(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	wt := map[string]string{"feature": "/wt/feature", "main": "/repo"}
	msg := branchesLoadedMsg{
		Branches:  []string{"main", "feature"},
		Current:   "main",
		Default:   "main",
		Worktrees: wt,
	}
	model, _ = model.Update(msg)
	if model.worktreeSelector == nil {
		t.Fatal("expected worktreeSelector to be created")
	}
	rows := model.worktreeSelector.Rows()
	if len(rows) != 2 {
		t.Fatalf("expected 2 worktree entries, got %d", len(rows))
	}
}
```

- [ ] **Step 2: Add test for entering worktree diff mode**

```go
func TestWorktreeSelectorEnterLoadsDiff(t *testing.T) {
	loader := &fakeLoader{
		snapshots: []git.Snapshot{makeSnapshot("wt-diff")},
	}
	model := newTestModel(loader, &fakeRunner{})
	wt := map[string]string{"feature": "/wt/feature"}
	msg := branchesLoadedMsg{
		Branches:  []string{"main"},
		Current:   "main",
		Default:   "main",
		Worktrees: wt,
	}
	model, _ = model.Update(msg)
	model.treeMode = TreeModeWorktree
	model.worktreeSelector.Move(1) // select "feature"
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.treeMode != TreeModeWorktreeDiff {
		t.Fatalf("treeMode = %d, want WorktreeDiff", model.treeMode)
	}
	if cmd == nil {
		t.Fatal("expected a command to load snapshot")
	}
}
```

- [ ] **Step 3: Add test for `h` back from worktree diff**

```go
func TestHKeyFromWorktreeDiffGoesToSelector(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model.treeMode = TreeModeWorktreeDiff
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if model.treeMode != TreeModeWorktree {
		t.Fatalf("treeMode = %d, want Worktree", model.treeMode)
	}
}
```

- [ ] **Step 4: Add view test for worktree selector rendering**

In `ui/view_test.go`:

```go
func TestRenderTreeShowsWorktreeSelectorWhenInWorktreeMode(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model.termW, model.termH = 120, 40
	model.layout = ComputeLayout(120, 40)
	model.treeMode = TreeModeWorktree
	entries := []WorktreeEntry{
		{Name: "repo", Path: "/repo"},
		{Name: "feature", Path: "/wt/feature"},
	}
	model.worktreeSelector = NewWorktreeSelector(entries, "repo")
	out := model.renderTree(model.layout.Files)
	if !strings.Contains(out, "feature") {
		t.Fatalf("expected worktree list in tree pane:\n%s", out)
	}
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./ui/... -count=1 -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add ui/model_test.go ui/view_test.go
git commit -m "test(ui): add tests for worktree selector and diff mode"
```

---

### Task 6: Handle edge cases and final polish

**Files:**
- Modify: `ui/model.go`
- Modify: `git/repository.go`

- [ ] **Step 1: Ensure worktree entries exist even when git worktree list returns empty**

In the `branchesLoadedMsg` handler, if `message.Worktrees` is nil/empty, create at least the main worktree entry (already handled in Task 3 Step 3).

- [ ] **Step 2: Handle worktree directory not found gracefully**

In `WorktreeSnapshot`, if the worktree path doesn't exist, return a clear error:

```go
if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
	return Snapshot{}, fmt.Errorf("worktree directory not found: %s", worktreePath)
}
```

- [ ] **Step 3: Remove (wt) prefix from branch selector rendering**

Since worktrees now have their own pane, remove the `(wt)` prefix from `renderBranchSelector` in `ui/render.go`. The branch selector should just show branch names.

- [ ] **Step 4: Run full test suite**

Run: `go test ./... -count=1 && go vet ./... && go build ./...`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add ui/model.go ui/render.go git/repository.go
git commit -m "fix(ui): handle edge cases for worktree selector, remove redundant (wt) prefix"
```

---

## Summary

After this plan:
- `]` cycles: **Worktree** → **Branch** → **PRs**
- **Worktree pane** lists all worktrees by directory name (main first)
- Selecting a worktree shows its file diff (editable via `[e]` in the worktree's actual directory)
- **Branch pane** shows branches only (no worktree markers needed)
- `h` backs out of worktree diff → worktree list
- Commits (`c`) still only work from the main worktree's working tree mode
