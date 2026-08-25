# File Status Icons Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the tree's per-file `📄` icon with a colored lazygit-style status letter (green `A` added, amber `M` modified/binary, red `D` deleted, blue `R` renamed).

**Architecture:** Add a small pure helper `fileStatusGlyph(status diff.FileStatus) (letter string, color lipgloss.Color)` in `ui/render.go`, then use it in `renderTree` to emit a color-styled status letter for file nodes. Folder icons (`📂`/`📁`) and hunk indentation (`"  "`) are untouched. No parser, model, or agent changes.

**Tech Stack:** Go 1.24, Charm's Bubble Tea + Lipgloss, `charmbracelet/x/ansi` (via `delta.Truncate`).

## Global Constraints

- Every case is its own `TestXxx` func — no table-driven tests and no `t.Run` subtests (repo convention, AGENTS.md).
- Commit messages use Conventional Commits: `feat(scope): ...`.
- A compiled `lazydiff` binary is committed at repo root — do **not** commit a rebuild unless intentional.
- Color mapping (verbatim from spec): `Added → "A" / 42`, `Modified → "M" / 214`, `Deleted → "D" / 203`, `Renamed → "R" / 39`, `Binary → "M" / 214`.
- `diff.FileStatus` values: `Modified`, `Added`, `Deleted`, `Renamed`, `Binary` (from `diff/model.go`).

---

### Task 1: Add the `fileStatusGlyph` helper with unit tests

**Files:**
- Create: `ui/render_test.go`
- Modify: `ui/render.go` (add helper + `diff` import)

**Interfaces:**
- Consumes: `diff.FileStatus` (`github.com/alex-irvine/lazydiff/diff`), `lipgloss.Color` (`github.com/charmbracelet/lipgloss`).
- Produces: `func fileStatusGlyph(status diff.FileStatus) (letter string, color lipgloss.Color)` — used by Task 2.

- [ ] **Step 1: Write the failing tests**

Create `ui/render_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./ui -run 'TestFileStatusGlyph' -count=1`
Expected: FAIL with `undefined: fileStatusGlyph`.

- [ ] **Step 3: Add the helper**

In `ui/render.go`, add `diff` to the import block (it currently imports `context`, `fmt`, `path/filepath`, `strings`, `sync`, then the app packages — insert the diff import after the `pr`/`version` app imports, keeping them grouped with the codebase's existing style):

```go
	"github.com/alex-irvine/lazydiff/delta"
	"github.com/alex-irvine/lazydiff/diff"
	"github.com/alex-irvine/lazydiff/pr"
```

Add this function at the end of the file:

```go
// fileStatusGlyph returns the lazygit-style change letter and its color for a
// file's status. Binary files are un-diffable and render as a modification.
func fileStatusGlyph(status diff.FileStatus) (string, lipgloss.Color) {
	switch status {
	case diff.Added:
		return "A", lipgloss.Color("42")
	case diff.Deleted:
		return "D", lipgloss.Color("203")
	case diff.Renamed:
		return "R", lipgloss.Color("39")
	default:
		return "M", lipgloss.Color("214")
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./ui -run 'TestFileStatusGlyph' -count=1`
Expected: PASS for all five `TestFileStatusGlyph*` cases.

- [ ] **Step 5: Commit**

```bash
git add ui/render.go ui/render_test.go
git commit -m "feat(ui): add file status glyph helper"
```

---

### Task 2: Render the colored status letter for file nodes

**Files:**
- Modify: `ui/render.go:197-207` (the `icon` block inside `renderTree`)

**Interfaces:**
- Consumes: `fileStatusGlyph(status diff.FileStatus) (string, lipgloss.Color)` from Task 1; `node.File.Status` where `node.File` is `*diff.File`.
- Produces: task output — a working tree render where file rows show `A`/`M`/`D`/`R` in their status colors.

- [ ] **Step 1: Replace the icon block**

In `ui/render.go`, replace the `icon` construction (currently `ui/render.go:197-206`):

```go
		var icon string
		if node.Hunk != nil {
			icon = "  "
		} else if node.File != nil {
			letter, c := fileStatusGlyph(node.File.Status)
			icon = lipgloss.NewStyle().Foreground(c).Render(letter) + " "
		} else if node.Expanded {
			icon = "📂 "
		} else {
			icon = "📁 "
		}
```

The line after this (`fullLine := prefix + indent + checkbox + icon + node.Label`, then `delta.Truncate(fullLine, maxW)` then the whole line wrapped in a foreground style) is unchanged. `delta.Truncate` calls `ansi.Truncate`, which is ANSI-aware, so the colored letter keeps its color and the byte-width is computed correctly.

- [ ] **Step 2: Build + run the ui unit tests**

Run: `go test ./ui -run TestFileStatusGlyph -count=1`
Expected: PASS.

- [ ] **Step 3: Manual sanity check of the render path**

If a `lazydiff` binary is available or buildable, run the existing render tests once:

Run: `go test ./ui -count=1`
Expected: all existing `ui` tests PASS (no render assertions broke — `renderTree` output still contains `[x]`/`[ ]` in worktree mode per `TestRenderTreeShowsCheckboxesInWorkingTreeMode`).

- [ ] **Step 4: Full verification**

Run (in order, matching CI):

```bash
go test ./... -count=1
go vet ./...
go build ./...
git diff --check
```

Expected: all pass; `go build ./...` leaves `cmd/lazydiff` buildable; `git diff --check` reports no whitespace errors.

- [ ] **Step 5: Commit**

```bash
git add ui/render.go
git commit -m "feat(ui): render colored status letters for file nodes"
```
