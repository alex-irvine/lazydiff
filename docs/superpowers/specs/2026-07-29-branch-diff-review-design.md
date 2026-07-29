# lazydiff Branch Diff Review Design Specification

## Status

Approved. Adds a read-only branch-diff review mode to lazydiff's left file tree pane.

## Objective

Let the user pick any local branch and review its diff against the repository default branch (e.g. `main`), using the same diff and analysis panes as the existing working-tree review flow. The left pane becomes read-only in this mode: checkboxes are hidden, staging is disabled, and commit is unavailable.

## Relationship to Prior Specs

This spec extends `2026-07-17-lazydiff-design.md` and `2026-07-27-lazydiff-stage-commit-pr-design.md`.

Changes to prior behavior:

- The `m` key, which cycled `working tree → staged → branch`, is retired.
- `[` and `]` become context-sensitive:
  - When focus is on the file tree pane, they cycle `worktree → staged → branch selector`.
  - When focus is on the analysis pane, they still cycle `detail → overall → request log`.
- The existing `git.Branch` mode is re-implemented as an explicit branch selection rather than implicitly comparing `HEAD` against the default branch.

## Design

### New component: `ui.BranchSelector`

A small model responsible for the inline branch list shown in the left pane.

State:

- `branches []string` — local branch names, default branch first, then alphabetical.
- `currentBranch string` — the currently checked-out branch, highlighted.
- `cursor int` — selected index in the list.
- `selectedBranch string` — empty while showing the selector; set when the user opens a branch diff.

Behavior:

- `Move(delta)` moves the cursor, clamped to the list.
- `Select(branch)` sets `selectedBranch`.
- `Rows()` returns visible branch names for rendering.

### `git` package additions

- `Repository.Branches(ctx context.Context) ([]string, error)`  
  Runs `git branch --format='%(refname:short)'` and returns local branch names.

- `Repository.SnapshotBranch(ctx context.Context, branch string) (Snapshot, error)`  
  Produces a `git diff <default>...<branch>` snapshot, reusing the same three-dot semantics as the previous implicit branch mode. The snapshot's `Mode` is `git.Branch` and its `Base` records `default...branch`.

### `ui.Model` state changes

Replace the single `mode git.Mode` with:

- `treeMode TreeMode` — one of:
  - `TreeModeWorktree`
  - `TreeModeStaged`
  - `TreeModeBranchSelector`
  - `TreeModeBranchDiff`
- `branchSelector *BranchSelector` — present whenever `treeMode` is `BranchSelector` or `BranchDiff`.
- `searchQuery string`, `searchRegex *regexp.Regexp`, `searchActive bool` — shared search state for the left pane.

The existing `snapshot git.Snapshot` remains the diff currently shown in the right panes.

### Left-pane behavior

The left pane can show one of four views. Cursor movement (`j`/`k`), expansion (`l`/`enter`), collapse/parent (`h`), and regex search (`/`) operate on whichever view is active.

#### Worktree view (`TreeModeWorktree`)

Unchanged from today: hierarchical file tree, tri-state checkboxes, hunks expandable.

#### Staged view (`TreeModeStaged`)

Same file tree, but checkboxes are hidden and staging actions are disabled. This preserves the ability to inspect staged changes after the user has staged files in working-tree mode.

#### Branch selector view (`TreeModeBranchSelector`)

A flat inline list of local branches. The current branch is visually highlighted. `j`/`k` move the cursor. `l` or `enter` selects the branch, sets `treeMode = TreeModeBranchDiff`, and triggers a `SnapshotBranch` load for that branch.

#### Branch diff view (`TreeModeBranchDiff`)

A read-only file tree for the selected branch's diff against the default branch. Checkboxes are not rendered. `h` at the top level returns to `TreeModeBranchSelector`. File/hunk navigation and analysis work exactly as in working-tree mode. For `[`/`]` cycling, branch diff is treated as part of the branch-selector group, so pressing `[` moves to `TreeModeWorktree` and `]` moves to `TreeModeStaged`.

### Key bindings

#### When `focus == FocusTree`

| Key | Action |
|-----|--------|
| `j` / `k` | Move cursor in the active left-pane view. |
| `h` | Collapse directory, go to parent, or return from branch diff to branch selector. |
| `l` / `enter` | Expand directory, descend into hunk, or open the selected branch diff. |
| `[` / `]` | Cycle left-pane view: `worktree → staged → branch selector`. |
| `/` | Open a case-insensitive regex search prompt for the left pane. |
| `n` / `N` | Move to next / previous visible search match. |
| `esc` | Clear active search. |

#### When `focus == FocusAnalysis`

`[` and `]` continue to cycle analysis tabs (`detail → overall → request log`), unchanged.

#### Retired bindings

- `m` is no longer bound to mode switching.

### Regex search

- `/` opens a one-line search input rendered at the bottom of the left pane.
- Search is case-insensitive.
- In `Worktree`, `Staged`, and `BranchDiff` views, search matches file paths only. Hunk headers are not searched.
- In `BranchSelector` view, search matches branch names.
- Non-matching items are hidden. `n`/`N` move the cursor through the visible matches. `esc` clears the filter.
- An empty or invalid regex shows all items. An invalid regex is shown as a status-line error.

### Staging / commit / PR

- Checkboxes render only when `treeMode == TreeModeWorktree`.
- `c` (commit) is disabled unless `treeMode == TreeModeWorktree` and `len(tree.StagingPlan()) > 0`.
- `o` (open PR) behavior depends on the active view:
  - `Worktree` or `Staged`: opens a PR for the currently checked-out branch.
  - `BranchDiff`: opens a PR for the reviewed branch (`branchSelector.selectedBranch`).

### Status line

Shows the active left-pane view:

- `worktree`
- `staged`
- `branch selector`
- `branch diff: <branch> vs <base>`

### Error handling

- Branch list load failure: status-line error; left pane falls back to an empty state with the error message.
- `SnapshotBranch` failure: status-line error; branch diff view remains empty.
- Empty branch list: show `(no local branches)` in the selector.
- Selecting the default branch itself: valid, produces an empty diff.

## Architecture / Package Changes

```text
git/     + Branches, SnapshotBranch
ui/      + BranchSelector model
         + TreeMode enum and left-pane view routing
         + regex search state and input handling
         - retire m-key mode cycle
```

No new external dependencies. `bubbles` is already used for dialogs; the search input can reuse `bubbles/textinput` or a lightweight custom input.

## Testing Strategy

- Unit: `BranchSelector` movement, selection, sorting, and current-branch highlight.
- Unit: `git.Branches` and `git.SnapshotBranch` construction through the fake `CommandRunner`.
- Unit: regex search filtering for file paths and branch names, including invalid-regex handling.
- Unit: `TreeMode` key routing in `ui.Model.Update` (which keys go to tree vs analysis vs search).
- Subprocess integration: selecting a branch loads the correct `git diff <default>...<branch>` snapshot.
- PTY integration: open branch selector, pick a branch, verify diff pane updates; `h` returns to selector; `/` filters branches and files.

## Scope Exclusions

- No remote branch selection in v1 (local branches only).
- No checkout from inside lazydiff; the reviewed branch stays un-checked-out.
- No additional diff modes beyond worktree, staged, and branch-vs-default.
- No persistent recently-reviewed branches list.

## Open Implementation Decisions

None blocking. Exact search input component (`bubbles/textinput` vs custom) and branch sort order can be chosen during implementation based on simplicity and testability.
