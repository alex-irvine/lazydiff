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
	if len(rows) != 3 || rows[0] != "main" || rows[1] != "alpha" || rows[2] != "zebra" {
		t.Fatalf("rows = %v", rows)
	}
}
