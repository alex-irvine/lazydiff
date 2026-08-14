package ui

import "sort"

type BranchSelector struct {
	branches       []string
	currentBranch  string
	defaultBranch  string
	cursor         int
	selectedBranch string
	worktrees      map[string]string
}

func NewBranchSelector(branches []string, currentBranch, defaultBranch string, worktrees map[string]string) *BranchSelector {
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
		worktrees:     worktrees,
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

func (b *BranchSelector) WorktreePath(branch string) (string, bool) {
	if b.worktrees == nil {
		return "", false
	}
	path, ok := b.worktrees[branch]
	return path, ok
}
