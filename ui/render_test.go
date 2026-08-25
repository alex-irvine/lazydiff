package ui

import (
	"testing"

	"github.com/alex-irvine/lazydiff/diff"
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