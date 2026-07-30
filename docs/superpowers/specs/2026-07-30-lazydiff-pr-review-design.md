# lazydiff PR Review Design Specification

## Status

Draft. Adds a read-only PR diff review mode with actionable review commands (approve, request changes, merge, close+delete-branch) to lazydiff's left pane, using the GitHub CLI (`gh`) for PR discovery and mutation.

## Objective

Let the user browse open pull requests for the current repository, select one to review its diff, and perform code review actions (approve, request changes, merge, close and delete branch) — all from within lazydiff's TUI. The existing analysis panes (overall/detail AI review, request log) work exactly as they do for local branches.

## Relationship to Prior Specs

This spec extends `2026-07-29-branch-diff-review-design.md` and `2026-07-27-lazydiff-stage-commit-pr-design.md`.

Changes to prior behavior:

- The `[`/`]` left-pane cycle expands from `worktree → staged → branch selector` to `worktree → staged → branch selector → PR selector`.
- `SnapshotLoader` gains a `SnapshotPR` method.
- A new `PRReviewer` interface is added for PR mutation actions, alongside the existing `Mutator`.
- `cmd/lazydiff/main.go` wires a new `pr.GitHub` dependency.

## Design

### New types: `pr.PR` and `pr.GitHub`

```go
// pr/types.go
package pr

type PR struct {
    Number      int
    Title       string
    Author      string
    HeadRefName string
    BaseRefName string
    URL         string
    Mergeable   string // "MERGEABLE" | "CONFLICTING" | "UNKNOWN"
    CreatedAt   string
}
```

```go
// pr/gh.go
package pr

type GitHub struct {
    Runner CommandRunner // execRunner in production, fake in tests
}
```

**`pr.GitHub` methods** (all accept `context.Context`):

| Method | `gh` command |
|--------|-------------|
| `ListPRs(state string) ([]PR, error)` | `gh pr list --state <state> --json number,title,author,headRefName,baseRefName,mergeable,url,createdAt` |
| `PRDiff(number int) (string, error)` | `gh pr diff <number> --patch` |
| `Approve(number int, comment string) error` | `gh pr review <number> --approve` (`--body` when comment non-empty) |
| `RequestChanges(number int, body string) error` | `gh pr review <number> --request-changes --body <body>` |
| `Merge(number int) error` | `gh pr merge <number> --merge` |
| `Close(number int) error` | `gh pr close <number>` |
| `DeleteBranch(branch string) error` | `git push origin --delete <branch>` (via direct exec, not through `Repository`) |

`CommandRunner` is the same interface used by `git.Repository` — the `execRunner` struct already exists. Non-`github.com` remotes produce clear blocking errors for all `gh` operations.

### `SnapshotLoader` interface addition

```go
type SnapshotLoader interface {
    Snapshot(context.Context, git.Mode) (git.Snapshot, error)
    SnapshotBranch(context.Context, string) (git.Snapshot, error)
    SnapshotPR(context.Context, int) (git.Snapshot, error)  // NEW
}
```

`SnapshotPR` calls `gh pr diff <number>`, parses the raw diff through `diff.Parse`, and wraps the result in a `git.Snapshot` with `Mode: git.Branch` and `Base: "<baseRefName>...<headRefName>"`.

### New `PRReviewer` interface

```go
type PRReviewer interface {
    Approve(ctx context.Context, number int, comment string) error
    RequestChanges(ctx context.Context, number int, body string) error
    Merge(ctx context.Context, number int) error
    Close(ctx context.Context, number int) error
    DeleteBranch(ctx context.Context, branch string) error
}
```

`pr.GitHub` satisfies this interface. It is wired separately from `Mutator` — PR actions are conceptually distinct from git mutations.

### `ui.TreeMode` additions

```go
const (
    TreeModeWorktree TreeMode = iota
    TreeModeStaged
    TreeModeBranchSelector
    TreeModeBranchDiff
    TreeModePRSelector         // NEW
    TreeModePRDiff             // NEW
)
```

### `ui.PRSelector` model

A small model for the inline PR list, mirroring `BranchSelector`:

```go
type PRSelector struct {
    prs        []pr.PR
    cursor     int
    selectedPR *pr.PR  // nil while browsing; set when user opens a PR diff
    diffCache  map[int]git.Snapshot  // cached diffs, cleared on refresh
}
```

Methods: `Move(delta)`, `Select(number)`, `Selected()`, `Rows()` — same contract as `BranchSelector`.

Row display: `#<number> <title>  (<author>, <state-icon>)` — e.g. `#42 feat: add login  (alex, ✓)`.
State icons: no conflicts → `✓`, merge conflicts → `✗`, unknown → `?`.

### `ui.Model` state additions

```go
type Model struct {
    // ... existing fields ...

    prSelector  *PRSelector     // present when treeMode is PRSelector or PRDiff
    prReviewer  PRReviewer      // wired from cmd/main.go
}
```

New message types:
- `prsLoadedMsg{PRs: []pr.PR}`
- `prsErrorMsg{Err: error}`
- `prDiffMsg{Snapshot: git.Snapshot}` (reuses `snapshotMsg` pattern)

### Left-pane behavior

The left pane can show one of six views. The cycle from `[`/`]` when `focus == FocusTree`:

```
worktree → staged → branch selector → PR selector → (wrap to worktree)
```

Backwards: `worktree ← staged ← branch selector ← PR selector ← (wrap)`

#### PR selector view (`TreeModePRSelector`)

A flat list of open PRs fetched from `gh pr list`. Each row shows the PR number, title, author, and mergeability icon. `j`/`k` move the cursor. `l`/`enter` selects the PR, triggers `SnapshotPR`, sets `treeMode = TreeModePRDiff`. `r` refreshes the PR list. `h` returns to `TreeModeBranchSelector`.

#### PR diff view (`TreeModePRDiff`)

A read-only file tree for the selected PR's diff (the PR branch vs its base branch). Checkboxes are not rendered. `h` returns to `TreeModePRSelector`. File/hunk navigation, analysis panes, and diff scrolling work exactly as in branch-diff mode.

### Key bindings

#### When `focus == FocusTree`

| Key | Action |
|-----|--------|
| `j` / `k` | Move cursor in the active left-pane view |
| `h` | Collapse directory, go to parent, return from diff to selector |
| `l` / `enter` | Expand directory, descend into hunk, or open selected PR diff |
| `[` / `]` | Cycle left-pane view: `worktree → staged → branch selector → PR selector` |
| `/` | Open regex search |
| `n` / `N` | Next / previous search match |
| `esc` | Clear search |

#### In `TreeModePRSelector` only

| Key | Action |
|-----|--------|
| `r` | Refresh PR list (re-fetch from `gh`) |

#### In `TreeModePRDiff` only

| Key | Action |
|-----|--------|
| `o` | Open selected PR in browser |
| `r` | Refresh PR diff |
| `g` `a` | Approve PR — opens confirm dialog |
| `g` `r` | Request changes — opens dialog with textarea for comment |
| `g` `m` | Merge PR — opens confirm dialog |
| `g` `d` | Close PR + delete branch — opens confirm dialog |

#### When `focus == FocusAnalysis`

`[` and `]` continue to cycle analysis tabs (`detail → overall → request log`), unchanged.

#### When `focus == FocusDiff`

Unchanged from existing behavior.

### Dialog behavior for PR actions

PR actions use the `g` prefix to avoid conflicts with analysis keys (`a`/`A`). Two dialog types:

**Simple confirmation** (`ga`, `gm`, `gd`):
- Opens a `ConfirmDialog` (new lightweight type or a read-only `ActionDialog` variant) showing:
  - The action being performed (e.g. "Approve PR #42 (feat: add login)")
  - Hint: `ctrl+s confirm   esc cancel`
- `ctrl+s` → executes the action → status-line result.
- `esc` → cancels, no mutation.

**Textarea dialog** (`gr` — request changes):
- Opens the existing `ActionDialog` with a textarea for the review comment.
- `ctrl+s` → runs `gh pr review <number> --request-changes --body <comment>`.
- `esc` → cancels.
- `ctrl+r` → re-generates AI comment (optional — can hand-write).

All action dialogs:
- Read the PR number and title from `prSelector.selectedPR`.
- Status-line feedback on success/failure.
- No automatic refresh after mutation (user can `r` to refresh the PR list, which re-fetches from `gh`).

### Tab bar and status line

**Tab bar** gets a fourth mode:
```
Worktree  |  Branch  |  PRs
```
Active tab highlighted green. In PR mode the active tab shows the PR number (e.g. `#42`).

**Status line** updates:
- `PR selector` when viewing PR list.
- `PR #42: feat: add login` in PR diff view (truncated to fit).

### Help text

New section under "General" (PR mode):
```
PR Review (PR mode):
[ga]  approve     [gr]  request changes
[gm]  merge       [gd]  close + delete branch
[o]   open in browser
[r]   refresh
```

### Error handling

| Scenario | Behavior |
|----------|----------|
| `gh` not installed | Status-line error on PR list load |
| `gh` not authenticated | Status-line error |
| PR list fetch fails | Status-line error, PR selector shows `(error: <msg>)` |
| `gh pr diff` fails | Status-line error, diff view empty |
| PR action fails | Dialog stays open with error, draft preserved |
| Non-`github.com` remote | Status-line note: "PR review requires github.com" |
| No open PRs | PR selector shows `(no open pull requests)` |
| PR already merged/closed before action | `gh` returns error shown in dialog |

### Caching

PR diffs are cached in `PRSelector.diffCache` so returning from `h` and re-entering doesn't re-fetch. Cache is invalidated on `r` (refresh in PR diff view) or when the PR list is refreshed.

### Scope exclusions

- No PR creation from lazydiff (the existing browser-based `o` flow handles that).
- No draft PR support — only listed as `open` PRs; the user creates drafts in the browser.
- No per-PR review thread viewing or inline comments (too complex for a terminal UI).
- No `gh` CLI path configuration — it must be on `PATH`.
- No GitLab/Bitbucket PR support — `github.com` only.
- No multi-account/repo PR support — operates on the `origin` remote's repo.

## Architecture / Package Changes

```text
pr/
  gh.go     NEW: PR type, GitHub CLI wrapper (ListPRs, PRDiff, Approve, RequestChanges, Merge, Close, DeleteBranch)
git/
  snapshot.go  + SnapshotPR (loader calls pr.GitHub.PRDiff internally)
ui/
  pr_selector.go      NEW: PRSelector model (mirrors BranchSelector)
  pr_selector_test.go NEW
  model.go            + TreeModePRSelector/PRDiff, prSelector, prReviewer; SnapshotPR on loader; g-prefix key routing
  model_test.go       + PR mode transition tests, PR diff loading, action dialogs
  dialog.go           + ConfirmDialog type for simple y/n confirmations
  render.go           + renderPRSelector, tab bar with PRs, status line, help text
  view_test.go        + render tests for PR selector and PR diff
cmd/lazydiff/
  main.go             + wire pr.GitHub as PRReviewer, add SnapshotPR to repositoryLoader
```

No new external dependencies. `gh` CLI is already called for version updates.

## Testing Strategy

- Unit: `pr.GitHub` methods through a fake `CommandRunner` — assert exact `gh` invocations and JSON/error parsing.
- Unit: `PRSelector` cursor movement, selection, row display, diff cache hit/miss.
- Unit: `SnapshotPR` via fake `CommandRunner` returning `gh pr diff` output.
- Unit: confirm dialog rendering and key routing (`y`/`n`/`esc`).
- Unit: `g`-prefix key routing in `ui.Model.Update` — `ga`/`gr`/`gm`/`gd` routed correctly only in `TreeModePRDiff`.
- Unit: tab bar and status line rendering in PR modes.
- Subprocess integration: fake `pr.GitHub` asserting correct `gh` invocations for approve/merge/close.
- PTY integration: open PR selector, verify PRs listed, select one, verify diff pane updates; `h` returns to selector; `ga` opens confirm dialog.

## Open Implementation Decisions

- Whether `ConfirmDialog` is a new struct or a wrapper around `ActionDialog` with the textarea hidden. (Default: new `ConfirmDialog` struct for clarity, since textarea has no purpose in approve/merge/close flows.)
- `SnapshotPR` lives in `git/snapshot.go` (takes a `pr.GitHub`-like interface) vs `pr/gh.go` (returns raw string parsed by caller). (Default: raw string returned from `pr.GitHub.PRDiff`; construction of `git.Snapshot` happens in the caller, keeping the `git` package free of `pr` imports.)
