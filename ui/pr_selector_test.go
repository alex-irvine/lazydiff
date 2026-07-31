// ui/pr_selector_test.go
package ui

import (
	"testing"

	"github.com/alex-irvine/lazydiff/pr"
)

func makeTestPRs() []pr.PR {
	return []pr.PR{
		{Number: 42, Title: "feat: add login", Author: "alex", HeadRefName: "feat-login", BaseRefName: "main", Mergeable: "MERGEABLE"},
		{Number: 7, Title: "fix: typo", Author: "sam", HeadRefName: "fix-typo", BaseRefName: "main", Mergeable: "CONFLICTING"},
	}
}

func TestPRSelectorStartsAtFirstPR(t *testing.T) {
	ps := NewPRSelector(makeTestPRs())
	if ps.cursor != 0 || ps.Selected() == nil || ps.Selected().Number != 42 {
		t.Fatalf("selected = %+v", ps.Selected())
	}
}

func TestPRSelectorMoveClamps(t *testing.T) {
	ps := NewPRSelector(makeTestPRs())
	ps.Move(5)
	if ps.Selected().Number != 7 {
		t.Fatalf("after Move(5) selected = %+v", ps.Selected())
	}
	ps.Move(-5)
	if ps.Selected().Number != 42 {
		t.Fatalf("after Move(-5) selected = %+v", ps.Selected())
	}
}

func TestPRSelectorSelectByNumber(t *testing.T) {
	ps := NewPRSelector(makeTestPRs())
	ps.Select(7)
	if ps.selectedPR == nil || ps.selectedPR.Number != 7 || ps.selectedPR.Title != "fix: typo" {
		t.Fatalf("selectedPR = %+v", ps.selectedPR)
	}
}

func TestPRSelectorRowsReturnsCopy(t *testing.T) {
	ps := NewPRSelector(makeTestPRs())
	rows := ps.Rows()
	rows[0].Title = "mutated"
	if ps.prs[0].Title == "mutated" {
		t.Fatal("Rows returned the backing slice")
	}
}

func TestPRSelectorEmptyList(t *testing.T) {
	ps := NewPRSelector(nil)
	if ps.Selected() != nil {
		t.Fatal("expected nil selection on empty list")
	}
	ps.Move(1) // must not panic
}

func TestPRSelectorDiffCacheInitialized(t *testing.T) {
	ps := NewPRSelector(makeTestPRs())
	if ps.diffCache == nil {
		t.Fatal("diffCache should be initialized")
	}
}
