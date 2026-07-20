# lazydiff Pane Layout Revision Design

## Status

Approved design. This revision changes only TUI composition and its visual reference. Git, Delta, agent, prompt, cache, error, and release behavior remain unchanged.

## Objective

Give code/Delta full terminal height while placing agent analysis in the lower half of the existing left rail. Keep changed-file navigation above agent analysis, cap the left rail at 28% of terminal width, and preserve usable code width.

## Decisions

- Wide layout uses two horizontal columns: left rail and full-height code rail.
- Left rail width is `min(28% of terminal width, width/3)`.
- Left rail is split exactly 50/50 by body height, with any odd row assigned to lower agent pane.
- Files pane occupies upper-left half.
- Agent pane occupies lower-left half.
- Code/Delta pane occupies full right-side body height.
- Status bar is full width and excluded from body split.
- Focus order remains files → code → agent.
- Narrow breakpoint remains below 80 columns.
- Narrow order is files → code → agent → status.
- Narrow panes use full terminal width; wide 50/50 left split does not apply.
- Code and agent keep independent scroll positions.
- Analysis tabs remain `detail`, `overall`, and `request log`.
- Existing data flow and error behavior are unchanged.

## Wide Composition

```text
┌─ Files / Hunks ───────┬─ Full-height Delta code ─────────────────┐
│ upper half            │                                          │
│                       │                                          │
├───────────────────────┤                                          │
│ Agent analysis        │                                          │
│ lower half            │                                          │
└───────────────────────┴──────────────────────────────────────────┘
```

Layout calculation:

```text
statusHeight = 1
bodyHeight = max(0, terminalHeight - statusHeight)
leftWidth = min(28% of terminalWidth, terminalWidth / 3)
rightWidth = terminalWidth - leftWidth
filesHeight = bodyHeight / 2
agentHeight = bodyHeight - filesHeight
```

Rectangles:

```go
type Layout struct {
    Files  Rect
    Code   Rect
    Agent  Rect
    Status Rect
}
```

- `Files`: `X=0, Y=0, W=leftWidth, H=filesHeight`.
- `Agent`: `X=0, Y=filesHeight, W=leftWidth, H=agentHeight`.
- `Code`: `X=leftWidth, Y=0, W=rightWidth, H=bodyHeight`.
- `Status`: `X=0, Y=bodyHeight, W=terminalWidth, H=1`.

For odd `bodyHeight`, integer floor gives extra row to `Agent`. All rectangle dimensions remain non-negative for small terminals.

## Narrow Composition

Below 80 columns:

```text
┌─ Files / Hunks ───────┐
│                       │
├─ Full-width Code ─────┤
│                       │
├─ Agent analysis ──────┤
│                       │
└─ Status ──────────────┘
```

- Files, Code, and Agent each use full terminal width.
- Code appears before Agent so implementation context remains primary.
- Status remains final row.
- Sections receive usable vertical heights without negative dimensions.

## Data Flow

```text
Git raw snapshot
  ├─ parsed file/hunk tree → Files pane
  ├─ Delta presentation → full-height Code pane
  └─ raw prompt context → Agent pane
```

Selection continues to originate in Files. Selected file/hunk controls Code content and detail prompt context. Agent output remains session-only and stale marking remains snapshot-based.

## Rendering and Focus

- Rename semantic rendering references from tree/diff/analysis layout fields to files/code/agent fields.
- Files pane renders changed-file tree and hunk selection.
- Code pane renders Delta ANSI output and owns code scroll position.
- Agent pane renders tabs, streamed response, errors, stale state, and owns analysis scroll position.
- Focus constants may remain `FocusTree`, `FocusDiff`, and `FocusAnalysis` for compatibility with current model semantics, but rendered pane mapping must be Files, Code, Agent in that order.
- Focused borders and status hints must identify current pane correctly.

## Testing

Update layout tests:

- At 120×40, Files and Agent share left width; Files plus Agent equals body height; heights differ by at most one; Code height equals body height; Code starts at Files width.
- At odd body height, Files receives floor half and Agent receives remaining row.
- At 80 columns, wide layout remains active and preserves horizontal columns.
- At 70×24, Files → Code → Agent stack vertically and each uses full width.
- Status occupies final row in every layout.
- Tree width never exceeds one-third.

Keep existing tests for selection, Delta rendering, agent prompt context, tabs, scroll, stale results, Git failures, Delta fallback, and agent errors.

PTY tests must cover wide ordering and narrow ordering. They must not require real Copilot or Delta.

## Error Behavior

No changes:

- Git refresh failure retains last good snapshot and reports status.
- Delta failure falls back to raw diff and reports status.
- Agent errors appear in Agent pane/request log.
- Git refresh marks previous analysis stale.
- Narrow layout retains all functional panes.

## Visual Reference

Update `docs/lazydiff-visual.html` to show exact 50/50 left rail and full-height code rail. Open Design remains visual design source only; runtime remains Go, Bubble Tea, and Lip Gloss.

## Scope Exclusions

- No changes to Git snapshot collection.
- No changes to Delta invocation.
- No changes to agent transport or prompts.
- No changes to release/CI behavior.
- No dynamic focus-driven resizing.
