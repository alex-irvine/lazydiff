package ui

import "sort"

type WorktreeEntry struct {
	Name string // directory basename (e.g. "feature")
	Path string // full worktree path
}

type WorktreeSelector struct {
	worktrees []WorktreeEntry
	current   string // name of the current/main worktree
	cursor    int
	selected  string
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
