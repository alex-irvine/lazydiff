package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/alex-irvine/lazydiff/diff"
	"github.com/alex-irvine/lazydiff/git"
	"github.com/charmbracelet/lipgloss"
)

func TestFileStatusGlyphAdded(t *testing.T) {
	letter, color := fileStatusGlyph(diff.Added)
	if letter != "A" || color != lipgloss.Color("42") {
		t.Fatalf("added glyph = %q, %v; want A, 42", letter, color)
	}
}

func TestFileStatusGlyphModified(t *testing.T) {
	letter, color := fileStatusGlyph(diff.Modified)
	if letter != "M" || color != lipgloss.Color("214") {
		t.Fatalf("modified glyph = %q, %v; want M, 214", letter, color)
	}
}

func TestFileStatusGlyphBinary(t *testing.T) {
	letter, color := fileStatusGlyph(diff.Binary)
	if letter != "M" || color != lipgloss.Color("214") {
		t.Fatalf("binary glyph = %q, %v; want M, 214", letter, color)
	}
}

func TestFileStatusGlyphDeleted(t *testing.T) {
	letter, color := fileStatusGlyph(diff.Deleted)
	if letter != "D" || color != lipgloss.Color("203") {
		t.Fatalf("deleted glyph = %q, %v; want D, 203", letter, color)
	}
}

func TestFileStatusGlyphRenamed(t *testing.T) {
	letter, color := fileStatusGlyph(diff.Renamed)
	if letter != "R" || color != lipgloss.Color("39") {
		t.Fatalf("renamed glyph = %q, %v; want R, 39", letter, color)
	}
}
func TestChangeContextSendsTheWholeDiffWhenItFitsTheBudget(t *testing.T) {
	snapshot := makeSnapshot("one")
	if got := changeContext(snapshot, "file:a"); got != snapshot.RawDiff {
		t.Fatalf("change context = %q; want the raw diff", got)
	}
}

func TestChangeContextFallsBackToAnOutlineWhenTheDiffIsTooLarge(t *testing.T) {
	snapshot := git.Snapshot{
		RawDiff: strings.Repeat("x", changeContextBudget+1),
		Files: []diff.File{
			{ID: "file:a", Path: "a.go", Status: diff.Modified, Raw: "@@ -1 +1 @@\n-old\n+new\n", Hunks: []diff.Hunk{{Header: "@@ -1 +1 @@ func Explain("}}},
			{ID: "file:b", Path: "b.go", Status: diff.Added, Raw: "@@ -0 +1 @@\n+added\n"},
		},
	}
	outline := changeContext(snapshot, "file:a")
	if !strings.Contains(outline, "- modified a.go (+1/-1)  <- the file in view") {
		t.Fatalf("outline = %q", outline)
	}
	if !strings.Contains(outline, "@@ -1 +1 @@ func Explain(") {
		t.Fatal("outline dropped the hunk headers that carry function context")
	}
	if !strings.Contains(outline, "- added b.go (+1/-0)") {
		t.Fatalf("outline = %q", outline)
	}
}

func TestChangeContextWithNoFiles(t *testing.T) {
	if got := changeContext(git.Snapshot{}, "file:a"); got != "(this file is the whole change)" {
		t.Fatalf("change context = %q", got)
	}
}

func TestAnalysisLinesShowSpinnerWhileActive(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model.snapshot = makeSnapshot("one")
	model.haveSnap = true
	model.tree = NewTree(model.snapshot.Files)
	model.termW, model.termH = 120, 40
	model.activeTab = DetailTab
	model.results[activeResultKey(model)] = &analysisResult{Active: true, Started: time.Now().Add(-3 * time.Second)}
	joined := strings.Join(model.analysisLines(), "\n")
	if !strings.Contains(joined, spinnerFrames[0]) || !strings.Contains(joined, "Explaining a.go") {
		t.Fatalf("analysis lines = %q", joined)
	}
	if !strings.Contains(joined, "3s") || !strings.Contains(joined, "x to cancel") {
		t.Fatalf("analysis lines = %q", joined)
	}
}

func TestSpinnerTickStopsWhenNothingIsActive(t *testing.T) {
	model := newTestModel(&fakeLoader{snapshots: []git.Snapshot{makeSnapshot("one")}}, &fakeRunner{})
	model.spinnerActive = true
	model, cmd := model.Update(spinnerTickMsg{})
	if cmd != nil || model.spinnerActive {
		t.Fatal("spinner kept ticking with no active analysis")
	}
}
