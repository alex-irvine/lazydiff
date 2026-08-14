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
