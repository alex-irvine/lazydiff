# lazydiff Stage / Commit / PR Design Specification

## Status

Approved. This document defines the design for lazydiff's first repository-mutating feature: reviewing changes, staging a subset of files/hunks, generating a commit message, committing, and opening a GitHub pull request with an AI-generated title and description. It supplements `2026-07-17-lazydiff-design.md` and explicitly revises one of its decisions (see Relationship to Prior Spec).

## Objective

Let a user go from "reviewed diff in lazydiff" to "opened GitHub PR" without leaving the TUI or hand-writing the commit message / PR description. The AI model writes both; the branch name is parsed for a ClickUp ticket ID, which is folded into the PR title (and commit message) in the format ClickUp's GitHub integration recognizes.

## Relationship to Prior Spec

The prior spec's Scope Exclusions state: *"No repository mutation by lazydiff or default analysis agents."* This feature deliberately revises that: lazydiff itself now performs git mutations (`add`, `commit`, `push`) as explicit, keybinding-gated user actions. The second half of that decision is unchanged and load-bearing: **the AI agent subprocess remains exactly as read-only as before.** The agent only ever produces text (a commit message, a PR title+body); lazydiff's own code performs every mutating command. `read_only`/`allow_external_tools` agent config and the Copilot adapter's tool restrictions are untouched.

## Decisions

- Trigger: TUI keybindings only, no new CLI subcommand/flag surface.
- Staging granularity: file-level and hunk-level (partial-file staging), via a new tri-state checkbox tree — not "stage everything" and not pre-staged-only.
- Commit and PR-creation are separate, independently-triggerable actions (repeatable commits before ever opening a PR).
- AI-generated commit message and PR title+body are always shown in an editable dialog before any mutating command runs — never applied blind.
- No config gate on the feature as a whole; it is always available (contrast with per-agent `read_only`, which is unrelated and unaffected).
- Ticket extraction: configurable regex (Go RE2) against the current branch name, default tuned for ClickUp's bare hash-style task IDs (e.g. `869d6rn69`).
- PR title always re-prefixes the extracted ticket as `CU-<id>: <title>`, even though the branch name itself carries it bare — per ClickUp's GitHub-integration docs, bare IDs only auto-link when they are *custom* task IDs; default hash-style IDs require the `#`/`CU-` form to trigger linking.
- Commit messages also carry the ticket as a trailer line, since ClickUp's docs list commit messages (not just PR title/body/branch name) as a valid linking location.
- PR body is a free-form AI summary of the branch-vs-default-branch diff — no fixed section template, no repo-PR-template scaffolding.
- PR creation goes through the browser (GitHub's compare-page quick-PR form, pre-filled via query params), not `gh pr create` / the GitHub API. No new external-service dependency beyond an installed `git` and a way to open a URL.
- GitHub only in v1. No GitLab/Bitbucket equivalent.

## Data Model: Tri-State Checkbox (`ui` package)

`TreeModel` gains `checked map[string]bool`, keyed by **leaf IDs only**:
- Hunk ID, for files that have hunks.
- File ID itself, for files with no hunks (binary, pure rename, mode-only change).

Directory nodes and multi-hunk file nodes have no entry of their own — their displayed state (`[x]` checked / `[ ]` unchecked / `[-]` indeterminate) is derived by aggregating descendant leaf state at render time. Single source of truth; no separate sync step to get wrong.

Toggling (`space`) a node:
- Hunk → flips that one entry.
- File with hunks → not-fully-checked flips all its hunks on; fully-checked flips them all off.
- File with no hunks → flips its own single entry.
- Directory → cascades check/uncheck to every descendant leaf beneath it.

`ctrl+a` checks every leaf in the current snapshot if any are unchecked; unchecks all if everything is already checked (standard "select all" toggle).

Preserved across `SetFiles` rebuilds the same way expand-state already is (snapshot before rebuild, re-apply by ID after — mirrors `collectExpandedIDs`/`applyExpandedIDs`, `ui/tree.go:66-92`). Checkboxes render and respond only when `mode == WorkingTree`; Staged and Branch mode views are visually and behaviorally unchanged from today.

Inherited limitation: hunk IDs embed the hunk header (line ranges), so unrelated edits that shift a hunk's line range change its ID and drop its checked state. This is the same limitation the existing selection-restore mechanism already has — not a regression introduced here.

## Staging Mechanism (`git` + `diff` packages)

`git.CommandRunner` gains `RunWithStdin(ctx, io.Reader, name string, args ...string) ([]byte, error)` — needed because `git apply --cached` and `git commit --file -` both take input on stdin, which today's args-only `Run` cannot express.

New `Repository` methods:
- `CurrentBranch(ctx) (string, error)` — `rev-parse --abbrev-ref HEAD`.
- `StageFile(ctx, oldPath, path string) error` — `git add -A -- <oldPath> <path>` (dedupe empty/equal paths before invoking). Handles Modified/Added/Deleted/Renamed/Binary uniformly; no manual patch construction needed for whole-file staging.
- `StagePatch(ctx, patch string) error` — `git apply --cached`, patch piped via stdin.
- `Commit(ctx, message string) error` — `git commit --file -`, message piped via stdin. Runs real commit hooks exactly as any normal commit would.
- `Push(ctx, remote, branch string) error` — `git push -u <remote> <branch>`. Always passes `-u`; harmless when upstream already exists, so no separate upstream-detection step is needed.
- `RemoteURL(ctx, remote string) (string, error)` — `git remote get-url <remote>`.

`diff` package gains a function that builds a partial patch from a `File` plus a subset of its `Hunks`: the file's existing preamble text (everything in `File.Raw` up to its first hunk — diff/mode/rename headers, `---`/`+++` lines) followed by only the selected hunks' raw text. This is the same technique `git add -p` uses internally; applying a subset of a valid diff's hunks against the unchanged pre-image works correctly as long as the subset doesn't overlap, which per-hunk selection guarantees.

`TreeModel.StagingPlan()` walks all files' checked state and buckets each into: fully checked (including no-hunks files) → `StageFile`; partially checked → `StagePatch` with that subset; unchecked → skip.

## Commit Flow

`c` (guard: `mode == WorkingTree`, at least one checked leaf, no dialog already open):

1. Run `StagingPlan()`; execute each resulting `StageFile`/`StagePatch` action in order.
2. Fetch a fresh `Snapshot(ctx, git.Staged)` — this is the diff that will be committed.
3. Render new `commit_message` prompt template (placeholders: `staged_diff`, `ticket`, `repository`, `mode`).
4. Run it through the existing `agent.Runner` — identical mechanism to today's overall/detail analysis, new template only.
5. Parse output as `subject line \n\n body` (same convention `git commit` messages themselves use). If a ticket was extracted from the branch name, append a blank line plus a `CU-<ticket>` trailer to the body.
6. Open the edit dialog, pre-filled and focused.

Dialog keys: `ctrl+s` confirm → `git commit --file -` with the dialog's current full text (including any manual edits). `esc` cancel → discard the draft; **staged files remain staged** (`git add` is safe and reversible; cancelling the dialog must not silently unstage anything the user might still want). `ctrl+r` regenerate → reruns steps 2–5 from scratch, replacing the current draft (no attempt to preserve or merge manual edits — simplest correct v1 behavor).

## PR Flow

`o` (guard: current branch ≠ resolved default branch, no dialog already open):

1. Fetch a fresh `Snapshot(ctx, git.Branch)` — the existing `branch...HEAD` diff. This step is mode-independent: it doesn't read or need the current UI mode or any checkbox state.
2. Determine current branch name (`CurrentBranch`); extract ticket via the configured `ticket_pattern`.
3. Render new `pr_description` prompt template (placeholders: `branch_diff`, `ticket`, `repository`, `branch`, `base_branch`).
4. Run through `agent.Runner`; parse output with the same subject/body convention as the commit flow.
5. Final title: `"CU-" + ticket + ": " + aiTitle` if a ticket was found, else `aiTitle` unchanged.
6. Open the edit dialog (identical shape to the commit dialog: one textarea, first line is the title).

Unlike the commit flow, nothing mutates just from opening this dialog — no push happens until confirm. Dialog keys: `ctrl+s` confirm →
   1. `Push(ctx, "origin", branch)`.
   2. Parse `owner/repo` from the `origin` remote URL (`RemoteURL`), supporting both `git@github.com:owner/repo.git` and `https://github.com/owner/repo.git` (with or without `.git`). A host other than `github.com` is a clear blocking error, not a silently-broken URL.
   3. Determine base branch via the existing `DefaultBranch()`.
   4. Build `https://github.com/<owner>/<repo>/compare/<base>...<head>?expand=1&title=<url-encoded title>&body=<url-encoded body>`.
   5. If the fully-encoded URL exceeds 6000 characters, truncate the body (title is always short, never truncated) and append `\n\n_(description truncated by lazydiff — full text in the request log tab)_`. The untruncated text remains visible in the existing request-log tab regardless.
   6. Open the URL via a small new per-OS opener (`xdg-open` on linux, `open` on darwin, `rundll32 url.dll,FileProtocolHandler` on windows), invoked directly through `os/exec` with the URL as a literal argument — **not** `cmd /c start`, whose shell would treat the URL's `&` query-param separators as command separators and truncate it.

`esc` cancel → discard draft, no push performed. `ctrl+r` regenerate → reruns steps 1–5, replacing the draft.

No `gh` CLI / GitHub API dependency for this flow. No "does a PR already exist" detection: GitHub's own compare page shows "View pull request" instead of "Create pull request" when one already exists for that branch, so lazydiff doesn't need to know either way. Draft-vs-ready-for-review is GitHub's own create-form choice, made by the human at submit time, not something lazydiff decides.

## Config Additions

```toml
[pr]
ticket_pattern = "(?:^|[-/_])([0-9a-z]{6,10})(?:[-_]|$)"

[agent.prompts]
commit_message = """
...
"""
pr_description = """
...
"""
```

`ticket_pattern` is a Go RE2 regex (no lookaround support, hence the consuming-boundary style above rather than lookaround assertions). Capture group 1, if the pattern has one, is the extracted ticket id; otherwise the whole match is used. The shipped default matches ClickUp's bare hash-style ids (e.g. `869d6rn69`) bounded by `/`, `-`, `_`, or a string edge. Teams using a different scheme (JIRA-style `[A-Z]+-\d+`, GitHub `#\d+`, etc.) override it. `commit_message` requires `{{staged_diff}}`; `pr_description` requires `{{branch_diff}}`; `{{ticket}}` is available to both but not required (empty string when no match).

## TUI Interaction — Keymap Additions

```text
space            toggle check on file/hunk/directory under cursor   (mode == WorkingTree)
ctrl+a           check all / uncheck all                            (mode == WorkingTree)
c                stage checked items, generate commit message,      (mode == WorkingTree,
                 open edit dialog                                    >=1 item checked)
o                generate PR title+body from branch diff,           (branch != default branch)
                 open edit dialog
```

Dialog-open keys (capture all other input as text, both commit and PR dialogs):

```text
ctrl+s           confirm (commit, or push + open browser)
esc              cancel, discard draft
ctrl+r           regenerate from scratch
```

`x` (cancel active analysis) moves from its old binding — `c` is needed for commit. `space` fully replaces its old "expand/collapse directory" behavior — `h` (`CollapseOrParent`) and `l` (`ExpandOrDescend`) already cover that, so no navigation capability is lost. Help text and status line must be updated to reflect both changes.

## Architecture — Package Summary

```text
git/     + CurrentBranch, StageFile, StagePatch, Commit, Push, RemoteURL, RunWithStdin
diff/    + partial-patch construction from a File + subset of its Hunks
pr/      NEW: ticket-pattern extraction, compare-URL construction (remote URL parsing +
         query encoding), per-OS browser opener
prompt/  + commit_message, pr_description templates/placeholders
config/  + [pr] section, 2 new prompt fields
ui/      + TreeModel checked-state (map + toggle/cascade/query), ActionDialog sub-model
         (bubbles/textarea-based, shared by commit and PR dialogs), Model.dialog field,
         new keys, modal key-capture in Update
```

New direct dependency: `github.com/charmbracelet/bubbles` (textarea component only). Same publisher as the existing `bubbletea`/`lipgloss` dependencies; implements `tea.Model` so it composes natively.

`ActionDialog` (new `ui` sub-model) holds `Kind` (commit or PR), lifecycle state (generating / ready / error), the `textarea.Model`, and any generation error. It owns its own `Update`/`View`; `Model.dialog *ActionDialog` is the only new field this adds to the main `Model` struct, rather than the ~10 flat fields a `showCommitDialog`/`commitDraft`/`commitGenerating`/... style would add on top of the existing per-modal-bool pattern already used for the update-check modal.

## Error Handling (Additions)

- Staging failure partway through a plan: abort remaining actions, status-line error, dialog does not open. Whatever staged successfully before the failure stays staged (inspectable/undoable via Staged mode) — no automatic rollback.
- Agent failure generating commit message or PR description: dialog still opens, with an empty draft and a visible error banner; the user can hand-write the message/description and confirm, or cancel.
- `git commit --file -` failure (hooks reject, nothing staged, sign failure, etc.): dialog stays open with the error shown; draft is preserved; user can edit and retry or cancel.
- `git push` failure (auth, network, non-fast-forward): dialog stays open with the error; draft preserved; retry or cancel.
- No `origin` remote, or a remote whose host isn't `github.com`: clear blocking error before any push is attempted.
- Browser-open command missing or failing (e.g. no `xdg-open` in a headless session): clear error; full title+body remain visible in the dialog and the request-log tab so the user can act manually.
- Malformed `ticket_pattern` regex: a config validation error at startup, same pattern as existing prompt-template validation — fails fast with a clear message, never a silent runtime no-match.

## Testing Strategy (Additions)

- Unit: tri-state aggregation and cascade logic across various checked-map states; checked-state preservation across `SetFiles` rebuilds.
- Unit: partial-patch construction produces valid unified diffs across Modified/Added/Deleted/Renamed fixtures.
- Unit: `ticket_pattern` extraction against a table of branch names, including the ClickUp default and at least one overridden JIRA-style pattern; capture-group vs whole-match fallback.
- Unit: PR title formatting, with and without an extracted ticket.
- Unit: compare-URL construction from SSH and HTTPS remote forms, with and without `.git`; non-`github.com` host rejected.
- Unit: subject/body parsing of agent output for both prompts, including empty-output and single-line-only edge cases.
- Subprocess integration: fake `CommandRunner` asserting exact `git add -A --` / `git apply --cached` / `git commit --file -` / `git push -u` invocations and stdin content, extending the existing fake-runner pattern in `git/repository_test.go`.
- Subprocess integration: fake agent asserting `commit_message`/`pr_description` prompts receive the expected placeholders, extending the existing pattern in `ui/model_test.go:182`.
- PTY integration: check a file, `c`, edit dialog appears pre-filled, confirm, verify a commit was created. Check a hunk subset, verify partial staging. `o` on a non-default branch, verify the dialog, confirm, verify push + a captured compare URL (fake opener injected for the test — no real browser launch in CI).

## Scope Exclusions

- No config gate to disable the feature.
- No GitLab/Bitbucket support.
- No `gh` CLI / GitHub API dependency, and consequently no existing-PR detection (GitHub's compare page handles that).
- No draft-vs-ready-for-review decision made by lazydiff.
- No amend/rebase/multi-commit management UI — each `c` press creates exactly one new commit from whatever is currently checked.
- No partial-hunk staging for Binary-status files (no line hunks exist to select; checkbox always operates at whole-file granularity for Binary status).
- No custom PR title/body templating beyond the fixed `CU-<ticket>: <title>` convention — the format is hardcoded, not configurable.
- No "refine based on my edits" regeneration — `ctrl+r` always restarts from the original prompt.

## Open Implementation Decisions

None blocking. Defaults chosen for `ticket_pattern`, URL-truncation threshold (6000 chars), and the per-OS opener commands are stated above; implementation may adjust any of them if testing against real branch names/environments shows a better value, without needing a design change.
