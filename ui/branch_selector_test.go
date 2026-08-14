package ui

import (
	"testing"
)

func TestBranchSelectorDefaultBranchFirst(t *testing.T) {
	bs := NewBranchSelector([]string{"feature-a", "main", "feature-b"}, "feature-b", "main", nil)
	rows := bs.Rows()
	if len(rows) != 3 || rows[0] != "main" {
		t.Fatalf("expected main first, got %v", rows)
	}
}

func TestBranchSelectorCurrentBranchHighlighted(t *testing.T) {
	bs := NewBranchSelector([]string{"main", "feature-a", "feature-b"}, "feature-a", "main", nil)
	if bs.currentBranch != "feature-a" {
		t.Fatalf("currentBranch = %q", bs.currentBranch)
	}
}

func TestBranchSelectorMoveCursor(t *testing.T) {
	bs := NewBranchSelector([]string{"main", "feature"}, "main", "main", nil)
	if bs.cursor != 0 {
		t.Fatalf("initial cursor = %d", bs.cursor)
	}
	bs.Move(1)
	if bs.cursor != 1 {
		t.Fatalf("after Move(1) cursor = %d", bs.cursor)
	}
}

func TestBranchSelectorSelectedEmptyInitially(t *testing.T) {
	bs := NewBranchSelector([]string{"main", "feature"}, "main", "main", nil)
	if bs.selectedBranch != "" {
		t.Fatal("selectedBranch should be empty")
	}
}

func TestBranchSelectorSelect(t *testing.T) {
	bs := NewBranchSelector([]string{"main", "feature"}, "main", "main", nil)
	bs.Select("feature")
	if bs.selectedBranch != "feature" {
		t.Fatalf("selectedBranch = %q", bs.selectedBranch)
	}
}

func TestBranchSelectorRowsReturnsAllBranches(t *testing.T) {
	bs := NewBranchSelector([]string{"main", "zebra", "alpha"}, "main", "main", nil)
	rows := bs.Rows()
	if len(rows) != 3 || rows[0] != "main" || rows[1] != "alpha" || rows[2] != "zebra" {
		t.Fatalf("rows = %v", rows)
	}
}

func TestBranchSelectorWorktreePathReturnsPath(t *testing.T) {
	wt := map[string]string{"feature": "/worktrees/feature"}
	bs := NewBranchSelector([]string{"main", "feature"}, "main", "main", wt)
	path, ok := bs.WorktreePath("feature")
	if !ok || path != "/worktrees/feature" {
		t.Fatalf("expected /worktrees/feature, got %q, %v", path, ok)
	}
}

func TestBranchSelectorWorktreePathMissing(t *testing.T) {
	wt := map[string]string{"feature": "/worktrees/feature"}
	bs := NewBranchSelector([]string{"main", "feature"}, "main", "main", wt)
	_, ok := bs.WorktreePath("main")
	if ok {
		t.Fatal("expected no worktree for main")
	}
}

func TestBranchSelectorWorktreePathNil(t *testing.T) {
	bs := NewBranchSelector([]string{"main"}, "main", "main", nil)
	_, ok := bs.WorktreePath("main")
	if ok {
		t.Fatal("expected no worktree with nil map")
	}
}
