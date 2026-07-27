package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alex-irvine/lazydiff/agent"
	"github.com/alex-irvine/lazydiff/config"
	"github.com/alex-irvine/lazydiff/delta"
	"github.com/alex-irvine/lazydiff/diff"
	"github.com/alex-irvine/lazydiff/git"
	"github.com/alex-irvine/lazydiff/prompt"
	tea "github.com/charmbracelet/bubbletea"
)

func testFiles() []diff.File {
	return []diff.File{
		{ID: "file:a", Path: "a.go", Status: diff.Modified, Hunks: []diff.Hunk{{ID: "hunk:a:0", Header: "@@ -1 +1 @@"}, {ID: "hunk:a:1", Header: "@@ -4 +4 @@"}}},
		{ID: "file:b", Path: "b.go", Status: diff.Added, Hunks: []diff.Hunk{{ID: "hunk:b:0", Header: "@@ -0 +1 @@"}}},
	}
}

func TestTreeNavigationAndSelection(t *testing.T) {
	tree := NewTree(testFiles())
	if file, hunk, ok := tree.Selected(); !ok || file.Path != "a.go" || hunk != nil {
		t.Fatalf("initial selection = %v, %v, %v", file.Path, hunk, ok)
	}
	tree.Toggle()
	if len(tree.Rows()) != 4 {
		t.Fatalf("expanded rows = %d", len(tree.Rows()))
	}
	tree.Move(1)
	file, hunk, ok := tree.Selected()
	if !ok || file.Path != "a.go" || hunk == nil || hunk.ID != "hunk:a:0" {
		t.Fatalf("hunk selection = %+v, %+v, %v", file, hunk, ok)
	}
	tree.Move(100)
	file, hunk, ok = tree.Selected()
	if !ok || file.Path != "b.go" || hunk != nil {
		t.Fatalf("last selection = %+v, %+v, %v", file, hunk, ok)
	}
}

func TestTreePreservesSelectionAfterRefresh(t *testing.T) {
	tree := NewTree(testFiles())
	tree.Toggle()
	tree.Move(1)
	tree.SetFiles(testFiles())
	_, hunk, ok := tree.Selected()
	if !ok || hunk == nil || hunk.ID != "hunk:a:0" {
		t.Fatalf("selection not preserved: %+v, %v", hunk, ok)
	}
}

func TestTreeEmptyState(t *testing.T) {
	tree := NewTree(nil)
	if _, _, ok := tree.Selected(); ok || len(tree.Rows()) != 0 {
		t.Fatal("empty tree has selection")
	}
}

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

func TestCheckStateSurvivesSetFilesRebuild(t *testing.T) {
	tree := NewTree(testFiles())
	tree.Toggle()
	tree.Move(1) // cursor -> hunk:a:0
	tree.ToggleCheck()
	tree.SetFiles(testFiles())
	// SetFiles already restores cursor/selection precisely (see
	// TestTreePreservesSelectionAfterRefresh) — no extra navigation needed.
	_, hunk, _ := tree.Selected()
	if hunk == nil || hunk.ID != "hunk:a:0" {
		t.Fatalf("expected hunk:a:0 selected, got %+v", hunk)
	}
	node := tree.flatNodes[tree.cursor]
	if tree.CheckState(node) != Checked {
		t.Fatal("checked state lost after SetFiles rebuild")
	}
}

func TestComputeLayoutWideSplit(t *testing.T) {
	l := ComputeLayout(120, 40)
	bodyH := 39
	if l.Files.W != 33 || l.Agent.W != l.Files.W || l.Code.X != l.Files.W {
		t.Fatalf("columns = files=%+v code=%+v agent=%+v", l.Files, l.Code, l.Agent)
	}
	if l.Files.H != bodyH/2 || l.Agent.H != bodyH-bodyH/2 || l.Code.H != bodyH {
		t.Fatalf("heights = files=%+v code=%+v agent=%+v", l.Files, l.Code, l.Agent)
	}
	if l.Files.Y != 0 || l.Agent.Y != l.Files.H || l.Code.Y != 0 {
		t.Fatalf("positions = files=%+v code=%+v agent=%+v", l.Files, l.Code, l.Agent)
	}
	if l.Status.Y != bodyH || l.Status.H != 1 {
		t.Fatalf("status = %+v", l.Status)
	}
}

func TestComputeLayoutOddBodyGivesAgentExtraRow(t *testing.T) {
	l := ComputeLayout(120, 42)
	if l.Files.H != 20 || l.Agent.H != 21 || l.Files.H+l.Agent.H != 41 {
		t.Fatalf("odd body split = files=%+v agent=%+v", l.Files, l.Agent)
	}
}

func TestComputeLayoutNarrowStacksFilesCodeAgent(t *testing.T) {
	l := ComputeLayout(70, 24)
	if l.Files.X != 0 || l.Code.X != 0 || l.Agent.X != 0 {
		t.Fatalf("narrow X positions = files=%+v code=%+v agent=%+v", l.Files, l.Code, l.Agent)
	}
	if l.Files.W != 70 || l.Code.W != 70 || l.Agent.W != 70 {
		t.Fatalf("narrow widths = files=%+v code=%+v agent=%+v", l.Files, l.Code, l.Agent)
	}
	if !(l.Files.Y < l.Code.Y && l.Code.Y < l.Agent.Y) || l.Agent.Y+l.Agent.H != l.Status.Y {
		t.Fatalf("narrow order = files=%+v code=%+v agent=%+v status=%+v", l.Files, l.Code, l.Agent, l.Status)
	}
}

func TestComputeLayoutCapsLeftRail(t *testing.T) {
	for _, size := range []struct{ width, height int }{{120, 40}, {80, 24}, {70, 24}} {
		layout := ComputeLayout(size.width, size.height)
		if size.width >= 80 && layout.Files.W > size.width/3 {
			t.Fatalf("files width %d exceeds one-third of %d", layout.Files.W, size.width)
		}
		if layout.Status.Y+layout.Status.H != size.height {
			t.Fatalf("status does not end at terminal bottom: %+v", layout.Status)
		}
	}
}

type fakeLoader struct {
	snapshots []git.Snapshot
	index     int
}

func (f *fakeLoader) Snapshot(context.Context, git.Mode) (git.Snapshot, error) {
	snapshot := f.snapshots[f.index]
	if f.index < len(f.snapshots)-1 {
		f.index++
	}
	return snapshot, nil
}

type fakeRenderer struct{}

func (fakeRenderer) Render(_ context.Context, raw string, _ int) delta.Result {
	return delta.Result{Content: "ANSI:" + raw, Styled: true}
}

type fakeRunner struct {
	requests []agent.Request
	events   []agent.Event
}

func (f *fakeRunner) Run(_ context.Context, request agent.Request, emit func(agent.Event)) error {
	f.requests = append(f.requests, request)
	for _, event := range f.events {
		emit(event)
	}
	return nil
}

func makeSnapshot(id string) git.Snapshot {
	files := testFiles()
	files[0].Raw = "diff --git a/a.go b/a.go\n@@ -1 +1 @@\n-old\n+new\n"
	files[0].Hunks[0].Raw = "@@ -1 +1 @@\n-old\n+new\n"
	return git.Snapshot{ID: id, Mode: git.WorkingTree, RawDiff: files[0].Raw, Files: files}
}

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

func TestModelRefreshAndAnalysisContext(t *testing.T) {
	loader := &fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}
	runner := &fakeRunner{events: []agent.Event{{Kind: agent.Output, Text: "analysis line"}}}
	model := newTestModel(loader, runner)
	_, cmd := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if cmd == nil {
		t.Fatal("resize did not produce refresh command")
	}
	msg := cmd()
	model, _ = model.Update(msg)
	if model.snapshot.ID != "one" {
		t.Fatalf("snapshot = %+v", model.snapshot)
	}
	model.focus = FocusTree
	model.tree.Toggle()
	model.tree.Move(1)
	model, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	if cmd == nil {
		t.Fatal("detail key did not create command")
	}
	cmd()
	if len(runner.requests) != 1 || !strings.Contains(runner.requests[0].Prompt, "Selected diff:") || !strings.Contains(runner.requests[0].Prompt, "@@ -1 +1 @@") {
		t.Fatalf("requests = %+v", runner.requests)
	}
}

func TestModelMarksCompletedResultStaleAfterRefresh(t *testing.T) {
	loader := &fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one"), makeSnapshot("two")}, index: 1}
	runner := &fakeRunner{}
	model := newTestModel(loader, runner)
	model.termW, model.termH = 120, 40
	model.snapshot = makeSnapshot("one")
	model.haveSnap = true
	model.tree = NewTree(model.snapshot.Files)
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if cmd == nil {
		t.Fatal("overall key did not create command")
	}
	cmd()
	model, cmd = model.Update(refreshMsg{})
	if cmd == nil {
		t.Fatal("refresh message did not schedule refresh")
	}
	model, _ = model.Update(cmd())
	for _, result := range model.results {
		if !result.Stale {
			t.Fatal("result was not marked stale")
		}
	}
}

func TestModelCancellation(t *testing.T) {
	loader := &fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}
	runner := &blockingRunner{}
	model := newTestModel(loader, runner)
	model.snapshot = makeSnapshot("one")
	model.haveSnap = true
	model.tree = NewTree(model.snapshot.Files)
	model.termW, model.termH = 120, 40
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if cmd == nil {
		t.Fatal("analysis command missing")
	}
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	cmd()
	if runner.cancelled == false {
		t.Fatal("runner was not cancelled")
	}
}

type blockingRunner struct{ cancelled bool }

func (b *blockingRunner) Run(ctx context.Context, _ agent.Request, _ func(agent.Event)) error {
	<-ctx.Done()
	b.cancelled = true
	return ctx.Err()
}

var _ = time.Second
