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
