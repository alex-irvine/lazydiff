# lazydiff

`lazydiff` is a terminal Git diff reviewer that uses an AI agent to explain an individual file or hunk in context of the wider change.

## Features

- Go + Bubble Tea terminal UI.
- Changed-file tree with expandable hunks.
- Delta-rendered diff output with raw-diff fallback.
- Working-tree, staged, and branch-vs-default diff modes.
- Separate overall and selected-detail analysis requests.
- Default GitHub Copilot CLI adapter with read-only repository inspection.
- Generic agent command support through stdin.
- Session-only results with stale marking after Git changes.
- Responsive 28/72 tree-to-review composition for narrow terminals.

## Requirements

- Go 1.24.2 or newer.
- Git repository.
- Delta optional. Without Delta, raw Git diff remains available.
- Copilot CLI optional when using `provider = "generic"`; required for default configuration.

## Build and Run

```bash
go build -o lazydiff ./cmd/lazydiff
./lazydiff
```

Run from repository root. Use custom TOML configuration with:

```bash
./lazydiff -config /path/to/config.toml
```

## Configuration

Default path is `$XDG_CONFIG_HOME/lazydiff/config.toml`, or `$HOME/.config/lazydiff/config.toml` when `XDG_CONFIG_HOME` is unset.

Default configuration uses Copilot:

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
"""

detail = """
Explain selected Git change in context of wider diff.

Repository: {{repository}}
Diff mode: {{mode}}
Target: {{selection}}

Overall diff:
{{overall_diff}}

Selected diff:
{{selected_diff}}
"""
```

Prompt templates support `{{repository}}`, `{{mode}}`, `{{overall_diff}}`, `{{selection}}`, and `{{selected_diff}}`. Overall templates require `{{overall_diff}}`; detail templates require overall, selection, and selected-diff placeholders.

Generic commands receive the rendered prompt through stdin:

```toml
[agent]
provider = "generic"
command = "/home/user/bin/review-agent"
args = ["--markdown"]
```

Commands execute directly without shell parsing. Copilot requests use a restrictive temporary prompt file to avoid command-line length limits. Read-only mode excludes file writes and shell tools; external tools and MCP servers are disabled by default.

## Controls

```text
Tab / Shift+Tab  move focus
Up / Down        navigate focused pane
Space            expand/collapse file
a                analyze overall diff
A                analyze selected file or hunk
1 / 2 / 3        detail / overall / request log tab
c                cancel active analysis
m                cycle diff mode
r                refresh Git snapshot
g / G            top / bottom
?                help
q                quit
```

Selecting a file analyzes its complete file diff. Selecting a hunk analyzes only that hunk while always sending the complete raw overall diff as context. Analysis results are kept in memory for the current process. Git changes mark existing results stale.

## Delta Presentation

Raw Git output is canonical for parsing and agent prompts. Display output is piped directly to:

```text
delta --paging=never --color-only --width=<available width>
```

Delta inherits Git configuration, including syntax themes, line numbers, side-by-side settings, and colors. If Delta is unavailable or fails, `lazydiff` displays raw diff and reports fallback status.

## Verification

```bash
go test ./... -count=1
go vet ./...
go build ./...
```

Linux PTY integration tests use temporary repositories and fake Delta/agent commands:

```bash
go test ./integration -v -count=1
```

Visual reference: [`docs/lazydiff-visual.html`](docs/lazydiff-visual.html). Design specification: [`docs/superpowers/specs/2026-07-17-lazydiff-design.md`](docs/superpowers/specs/2026-07-17-lazydiff-design.md).
