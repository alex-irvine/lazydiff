# File Status Icon Design Specification

## Status

Draft. Changes the left-pane tree's per-file icon from a single `📄` glyph to a colored status letter (lazygit-style): red `D` for deleted, amber `M` for modified, green `A` for added, plus `R` for renamed and `M` for binary.

## Objective

Give the file tree a quick visual read of each file's change type, matching the mental model lazygit users already have. Today every file node renders the same `📄` emoji regardless of whether it was added, modified, deleted, or renamed.

## Relationship to Prior Specs

This is a self-contained rendering change; it does not build on any prior feature spec. It touches only the tree renderer in `ui/render.go`. No new types, interfaces, or dependencies.

## Design

### Mapping

A file node's icon is a single letter derived from `diff.File.Status`, rendered in its own foreground color so the filename keeps its usual gray / active-cyan row color:

| `diff.FileStatus` | Letter | Color | Lipgloss |
|---|---|---|---|
| `Added` | `A` | green | `42` |
| `Modified` | `M` | amber | `214` |
| `Deleted` | `D` | red | `203` |
| `Renamed` | `R` | blue | `39` |
| `Binary` | `M` | amber | `214` |

Colors reuse the codebase's existing palette where possible (`42` green and `203` red already appear in `ui/render.go`); `214` (amber) and `39` (blue) are new to that file.

`Binary` files are un-diffable, so the parser cannot tell whether a given binary file was added, modified, or deleted — it renders as a modification (`M`, amber), consistent with how a modified file's change is surfaced. This requires **no** change to `diff/parse.go`; the parser already labels binary files with the `Binary` status.

### Implementation in `ui/render.go`

In `renderTree` (currently `ui/render.go:197-206`), replace the file-icon branch:

- Keep the hunk branch (`"  "`) and the folder branches (`📂` expanded / `📁` collapsed) unchanged.
- For a file node, render the status letter as a separately styled segment, e.g. `lipgloss.NewStyle().Foreground(iconColor).Render(letter) + " "` (letter + trailing space = 2 display cells).
- The colored segment is embedded in the row string before `node.Label`. `delta.Truncate` delegates to `ansi.Truncate`, which is ANSI-aware, so the icon keeps its color and the width is computed correctly when the row is truncated.

Add a small helper that maps `diff.FileStatus` → `(letter string, color lipgloss.Color)` so the mapping is defined in one place and easy to test/extend.

Visual result (worktree mode, file rows):

```
▶ [x] M  src/foo.go
  [x] A  src/new.go
  [ ] D  old.go
```

### Layout note

The letter glyph row is 2 cells (`X `) versus the previous 3-cell `📄 `. File rows therefore now align with hunk rows (also 2 cells); folder rows keep their 3-cell `📂`/`📁` icons, so a folder's icon is 1 cell wider than its child files' icons. This is acceptable and matches the icon being a status column rather than a fixed-width decoration.

## Out of scope

- Folder icons (`📂`/`📁`) and hunk indentation are unchanged.
- No changes to `diff/parse.go`, `diff/model.go`, or the git/agent layers.
- No new tests are required beyond the existing render path; the change is purely presentational. (See Testing below for the existing caveat.)

## Testing

The render path is exercised manually via the TUI. `ui/view_test.go` contains only a `Printf`-debug harness (`TestBoxWithDeltaContent`) with no real assertions (see AGENTS.md) and is not a suitable place to assert icon output. A focused unit test for the new status→letter/color helper is reasonable and cheap; it asserts each `diff.FileStatus` maps to the expected letter and color class.

## Files changed

- `ui/render.go` — tree icon branch + `diff.FileStatus` → (letter, color) helper.
- `ui/render_test.go` (new) — helper mapping test.

## Verification

```bash
go test ./... -count=1
go vet ./...
go build ./...
git diff --check
```
