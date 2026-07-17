# lazydiff Design Specification

## Status

Approved visual direction. This document defines v1 product and technical design for a standalone AI-connected Git diff TUI.

## Objective

Build `lazydiff`, a standalone terminal application that helps users understand why an individual Git file or hunk exists in the context of the wider change. The application reads repository state through Git, renders diffs through the user's configured Delta presentation, and sends explicit overall or selected-target analysis requests to a local agent CLI.

The first implementation lives in `~/Proj/lazydiff` and does not depend on `lazyorc`.

## Decisions

- Runtime: Go.
- TUI framework: Bubble Tea.
- Terminal styling/layout: Lip Gloss, with ANSI-aware rendering where needed.
- Git access: installed Git CLI subprocesses, not a Go Git library.
- Agent default: GitHub Copilot CLI.
- Agent configuration: TOML file with structured executable and argument fields.
- Generic agent transport: rendered prompt through stdin, Markdown stdout streamed to UI.
- Copilot transport: built-in adapter writes prompt to a restrictive temporary file and invokes documented non-interactive Copilot mode with a short file-reading instruction.
- Agent access: read-only repository inspection allowed; mutations, network access, and MCP tools disabled by default.
- Repository: current working directory at launch.
- Default diff mode: working tree versus `HEAD`.
- Supported modes: working tree versus `HEAD`, staged versus `HEAD`, current branch versus default branch.
- Analysis target: file or hunk.
- Overall and detail analysis: separate requests.
- Detail context: raw overall diff plus selected raw file/hunk diff; cached overall answer is not sent.
- Prompt configuration: separate overall and detail templates with explicit placeholders.
- Analysis persistence: current process only.
- Delta behavior: use Delta for display; fall back to raw diff if unavailable or failing.
- Visual composition: file tree at 28% maximum width; right side vertically stacks Delta diff above AI analysis.
- Testing: unit tests, subprocess integration tests, and full pseudo-terminal integration suite.

## Visual Design

The approved visual reference is `docs/lazydiff-visual.html`. It is a standalone static HTML export of the approved Open Design mockup and is not runtime UI code.

### Composition

```text
┌─ Changed Files / Hunks ─────┬─ Delta Diff ────────────────────────────────┐
│                             │                                             │
│  file tree                  │  full-width selected diff                   │
│  files and hunks            │                                             │
│                             ├─────────────────────────────────────────────┤
│                             │  Overall / Detail / Request Log analysis    │
│  max 28% width              │  full-width streamed Markdown              │
└─────────────────────────────┴─────────────────────────────────────────────┘
```

- The tree must never exceed one-third of terminal width; target 28%.
- The right side is one vertical review stack, not two narrow right columns.
- Delta diff occupies upper right and receives available horizontal width.
- AI analysis occupies lower right and receives available horizontal width.
- Analysis tabs are `detail`, `overall`, and `request log`.
- Narrow terminals stack tree, diff, and analysis vertically while preserving navigation.
- Teal/cyan marks selection, active analysis, and live streaming state.
- Green/red are reserved for Delta added/removed semantics.
- Amber marks diff mode and hunk metadata.
- Lip Gloss must not overwrite Delta's ANSI styling with generic add/remove colors.

### Density and behavior

- Use a dense monospace workbench suitable for repeated code review.
- Preserve visible line numbers and long code context where terminal width allows.
- Keep headers and status information compact.
- Make active file, active hunk, active tab, running request, stale result, and errors visually distinct.
- Do not rely on decorative browser-only effects in the TUI; gradients and shadows from the visual reference are translated to terminal-compatible borders, colors, and emphasis.

## Architecture

Proposed package boundaries:

```text
lazydiff/
├── cmd/lazydiff/       startup, flags, process wiring
├── config/             TOML loading, defaults, validation
├── git/                repository and Git CLI operations
├── diff/               raw diff parser and file/hunk model
├── delta/              Delta presentation subprocess and ANSI output
├── agent/              generic runner and Copilot adapter
├── prompt/             template parsing and context rendering
└── ui/                 Bubble Tea model, panes, keys, rendering
```

Keep boundaries independently testable:

- `git` returns raw ANSI-free snapshots and metadata.
- `diff` parses raw snapshots into stable selectable targets.
- `delta` turns raw diff text into terminal-formatted display text for a known width.
- `prompt` renders validated templates from a snapshot and target.
- `agent` manages one request lifecycle and emits stream/error/completion events.
- `ui` owns focus, selection, caching, stale state, and terminal rendering.

## Git Snapshot Model

Every refresh creates or reuses a snapshot containing:

- repository root
- active diff mode
- base reference, where applicable
- raw overall diff with colors disabled
- changed file metadata
- parsed file/hunk targets
- stable snapshot identity derived from mode, refs, and raw diff content
- Git status/error information

Raw diff is canonical. It is used for parsing, prompt construction, cache identity, and tests. No ANSI escape sequences enter agent prompts.

### Diff modes

1. Working tree versus `HEAD`.
2. Staged index versus `HEAD`.
3. Current branch versus repository default branch.

The Git layer must handle ordinary modifications, additions, deletions, renames, binary files, and untracked files according to explicit v1 behavior. If untracked files are included in working-tree mode, their content must be represented in a bounded, deterministic way; otherwise they are listed with a clear non-analyzable state.

### Parsing

Parse Git unified diff headers and hunk headers before any display formatting. File targets include path and file status. Hunk targets include parent file identity, hunk ordinal, header, and raw line range/content. Stable identities preserve selection across refreshes when the same file/hunk remains present.

## Delta Presentation

The display pipeline is:

```text
Git CLI --no-color
  → raw diff snapshot
  → Delta --paging=never --color-only --width=<available width>
  → ANSI-aware terminal renderer
```

- Delta inherits repository and global Git configuration.
- `--paging=never` prevents pager input from competing with Bubble Tea.
- `--color-only` preserves diff structure while applying configured Delta styling.
- Width is recomputed on terminal resize.
- Delta output is not parsed for file/hunk selection.
- If Delta is absent or exits unsuccessfully, render raw diff with a visible degraded status.
- Delta must be invoked directly without shell interpolation.
- ANSI-aware slicing/wrapping is required; byte slicing must not split escape sequences or miscalculate visible widths.

## Agent Contract

### Configuration

Example shape:

```toml
[agent]
provider = "copilot"
command = "copilot"
args = []
read_only = true
allow_external_tools = false

[agent.prompts]
overall = """
Review overall Git change.

Repository: {{repository}}
Diff mode: {{mode}}

Overall diff:
{{overall_diff}}

Explain purpose, architecture impact, risks, and testing gaps.
"""

detail = """
Explain this selected Git change in context of the wider diff.

Repository: {{repository}}
Diff mode: {{mode}}
Target: {{selection}}

Overall diff:
{{overall_diff}}

Selected diff:
{{selected_diff}}
"""
```

Executable and arguments are separate fields. No shell parsing or command-string interpolation is used.

### Placeholders

Templates fully control context placement. Unknown placeholders and malformed templates are configuration errors. Supported placeholders are:

- `{{repository}}`
- `{{mode}}`
- `{{overall_diff}}`
- `{{selection}}`
- `{{selected_diff}}`

Overall templates require the fields needed for overall analysis. Detail templates require selection and selected diff fields. Validation occurs before request execution.

### Generic command runner

- Execute configured command directly with `os/exec`.
- Set working directory to repository root.
- Write rendered prompt to stdin.
- Read stdout and stderr concurrently.
- Stream stdout as plain Markdown to the UI.
- Preserve stderr for diagnostics.
- Classify non-zero exit as failure while retaining streamed output.
- Support cancellation through context/process signals.
- Never pass mutation/approval flags automatically.

### Copilot adapter

Copilot's documented non-interactive interface accepts prompts through `-p/--prompt`, so large rendered diffs must not be passed as a giant argument. The adapter:

1. Writes rendered prompt to a restrictive temporary file.
2. Invokes Copilot with a short `-p` instruction to read that file.
3. Requests text output and streaming.
4. Disables external/network/MCP access by default.
5. Applies provider-specific read-only tool restrictions.
6. Runs from repository root so Copilot can inspect repository files.
7. Removes temporary file on completion, failure, or cancellation.

Repository reads are allowed as supplementary context. Supplied overall and selected diffs remain mandatory prompt context.

## Prompt and Analysis Flow

### Overall analysis

Triggered explicitly by `a`. It sends repository path, active mode, and complete raw overall diff to the overall template. Result is stored under the current snapshot identity.

### Detail analysis

Triggered explicitly by `A`. It sends repository path, active mode, complete raw overall diff, selected file/hunk identity, and selected raw diff to the detail template. Selecting a file sends its complete file diff. Selecting a hunk sends only that hunk. Cached overall output is not included.

### Request state

- At most one active overall request and one active detail request.
- A new request for the same target cancels/replaces the previous request.
- `c` cancels active analysis.
- Stream output appears immediately in the selected analysis tab.
- Results are session-only.
- Results are keyed by snapshot identity and target identity.
- When Git changes, existing responses remain visible but become stale.
- Active requests finish unless explicitly cancelled; completed output is marked stale if its snapshot no longer matches current state.

## TUI Interaction

Key map baseline:

```text
Tab / Shift+Tab  move focus
Up / Down        navigate tree, diff, or analysis based on focus
Space            expand/collapse file node
a                analyze overall diff
A                analyze selected file or hunk
c                cancel active analysis
m                cycle diff mode
r                refresh Git snapshot
g / G            top / bottom in scrollable pane
?                help
q                quit
```

The final key map may change during implementation if conflicts appear, but all actions must remain discoverable in help/status text.

### Tree

- Hierarchical file nodes with child hunk nodes.
- File selection targets complete file diff.
- Hunk selection targets individual hunk.
- Stable selection restoration after refresh.
- Explicit expansion/collapse.
- Empty state explains active mode and disables analysis actions.

### Diff

- Shows Delta output for selected file or the overall snapshot, depending on selected navigation state.
- Scrolls independently.
- Keeps current target visible when selection changes.
- Uses raw-to-rendered line mapping only where needed; target identity comes from parser.

### Analysis

- Switchable `detail`, `overall`, and `request log` tabs.
- Scrollable Markdown-like output.
- Streaming indicator while request runs.
- Stale badge when snapshot changed.
- Error output includes agent stderr and exit reason.

## Error Handling

- Invalid repository at startup: clear error and exit.
- Git refresh failure: retain last good snapshot and show error status.
- No changes: show empty state; no analysis request.
- Delta missing/failing: raw diff fallback plus visible warning.
- Agent missing or invalid config: disable analysis with actionable status.
- Agent non-zero exit: retain output, append stderr and exit status.
- Agent cancellation: classify as cancelled, not failed.
- Resize: reload Delta presentation at new width without changing raw snapshot identity.
- Prompt validation failure: show config error before launching process.

## Testing Strategy

### Unit tests

- TOML defaults and validation.
- Prompt placeholder validation and rendering.
- Git mode command construction.
- Unified diff file/hunk parsing.
- Stable target identity.
- Snapshot identity and cache invalidation.
- ANSI-aware visible width and line handling.
- Agent event classification.

### Subprocess integration tests

Use temporary repositories and fake executables. Do not depend on user's repository, global Git config, installed Delta, or installed Copilot.

- Git modes produce expected raw snapshots.
- Fake Delta receives raw input and width, returns controlled ANSI output.
- Missing/failing Delta falls back correctly.
- Fake agent receives expected stdin/template context.
- Copilot adapter creates/removes prompt file and passes read-only/external-access flags.
- stdout/stderr interleave without deadlock.
- Non-zero exit and cancellation classify correctly.

### Full PTY integration suite

Drive compiled `lazydiff` against temporary repositories through a pseudo-terminal. Cover:

- startup in valid and invalid repositories
- empty repository state
- modified, staged, added, deleted, renamed, binary, and untracked files
- all diff modes
- file and hunk navigation
- expansion/collapse and selection restoration
- Delta-rendered ANSI output
- raw Delta fallback
- overall analysis streaming
- detail analysis with wider context
- stderr and non-zero agent exit
- cancellation
- Git refresh and stale result state
- terminal resize and narrow layout
- quit during active request

## Scope Exclusions

- No persistent analysis files.
- No API-provider backend in v1.
- No shell command configuration.
- No generic full CLI/subcommand surface beyond app startup.
- No repository mutation by `lazydiff` or default analysis agents.
- No browser UI in the product; `docs/lazydiff-visual.html` is a static design reference only.

## Open Implementation Decisions

None blocking design approval. Implementation plan must choose exact Go module name, TOML parser dependency, Markdown rendering approach, PTY test library, and Copilot flag names based on current CLI behavior and tests.
