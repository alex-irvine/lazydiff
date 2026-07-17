package ui

import (
	"testing"

	"github.com/alex-irvine/lazydiff/diff"
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

func TestComputeLayoutCapsTreeAndStacksReview(t *testing.T) {
	for _, size := range []struct{ width, height int }{{120, 40}, {80, 24}, {70, 24}} {
		layout := ComputeLayout(size.width, size.height)
		if layout.Tree.W > size.width/3 {
			t.Fatalf("tree width %d exceeds one-third of %d", layout.Tree.W, size.width)
		}
		if layout.Status.Y+layout.Status.H != size.height {
			t.Fatalf("status does not end at terminal bottom: %+v", layout.Status)
		}
		if size.width >= 80 && (layout.Tree.H != layout.Diff.H+layout.Analysis.H || layout.Diff.X != layout.Analysis.X) {
			t.Fatalf("review stack not aligned: diff=%+v analysis=%+v", layout.Diff, layout.Analysis)
		}
	}
}
