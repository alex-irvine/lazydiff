# lazydiff Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build standalone `lazydiff`, a Go Bubble Tea TUI that renders Git changes through Delta and sends explicit overall or file/hunk explanations to a read-only Copilot or generic agent command.

**Architecture:** Keep raw Git diff as canonical snapshot data. Parse raw diffs into stable file/hunk targets, render a separate Delta ANSI presentation, and use one Bubble Tea model with a 28/72 tree-to-review layout. Agent requests run behind a small runner interface so generic stdin commands and the Copilot temp-file adapter share lifecycle, streaming, cancellation, and error handling.

**Tech Stack:** Go 1.24.2 module `github.com/alex-irvine/lazydiff`; Bubble Tea, Lip Gloss, Charm ANSI utilities, `github.com/pelletier/go-toml/v2`, and `github.com/creack/pty` for Linux PTY integration tests. No Markdown parser in v1; streamed Markdown is displayed as styled plain text.

## Global Constraints

- Work only in `/home/alex/Proj/lazydiff`; do not modify `lazyorc`.
- Current working directory is the default repository; support `-config` only as an explicit startup override.
- Raw Git output must use `--no-color` and remain the source for parsing, prompts, cache identity, and tests.
- Delta is presentation-only: invoke directly with `--paging=never --color-only --width=<width>` and fall back to raw diff on failure.
- Tree width must never exceed one-third of terminal width; target 28%.
- Right side must stack Delta diff above AI analysis; do not use a triple vertical split.
- Default agent is Copilot; default agent access is read-only repository inspection with network, shell, write, URL, and MCP access disabled.
- Generic agent commands use separate executable and argument fields and receive rendered prompts through stdin.
- Copilot prompts use a restrictive temporary file because large prompts must not be placed in argv.
- Detail prompts contain raw overall diff and selected raw file/hunk diff, never the cached overall answer.
- Analysis results remain in memory only and become stale when snapshot identity changes.
- Run focused tests after every implementation step and `go test ./...`, `go vet ./...`, and `go build ./...` before each task commit.
- Do not add repository mutation, API-provider backends, shell command parsing, persistent analysis files, or browser runtime code.

---

## File Map

Create these files in task order:

```text
go.mod
go.sum
cmd/lazydiff/main.go
config/config.go
config/config_test.go
diff/model.go
diff/parse.go
diff/parse_test.go
git/mode.go
git/repository.go
git/snapshot.go
git/repository_test.go
delta/render.go
delta/render_test.go
prompt/template.go
prompt/template_test.go
agent/types.go
agent/generic.go
agent/copilot.go
agent/runner_test.go
ui/tree.go
ui/layout.go
ui/model.go
ui/render.go
ui/model_test.go
integration/pty_linux_test.go
README.md
```

Responsibilities:

- `config`: XDG TOML path, defaults, template values, validation.
- `diff`: ANSI-free unified diff parser and stable file/hunk model.
- `git/mode.go`: supported modes and human-readable labels.
- `git/repository.go`: repository discovery, command execution, default branch resolution.
- `git/snapshot.go`: raw diff collection, untracked-file synthesis, snapshot identity.
- `delta`: width-aware presentation subprocess and raw fallback.
- `prompt`: strict template parsing and rendering.
- `agent`: generic command lifecycle and Copilot adapter.
- `ui/tree.go`: hierarchical file/hunk cursor state.
- `ui/layout.go`: 28/72 stacked layout rectangles.
- `ui/model.go`: Bubble Tea state, commands, messages, cache, refresh, requests.
- `ui/render.go`: terminal rendering and ANSI-safe display handling.
- `integration/pty_linux_test.go`: compiled-binary pseudo-terminal coverage.

---

### Task 1: Scaffold Module and Configuration

**Files:**
- Create: `go.mod`
- Create: `config/config.go`
- Create: `config/config_test.go`

**Interfaces:**
- Produces `config.Config`, `config.AgentConfig`, `config.PromptConfig`, `config.Default()`, `config.ConfigPath()`, and `config.Load(path string) (config.Config, error)` for all later tasks.

- [ ] **Step 1: Create module and dependency declarations**

Create `go.mod` with:

```go
module github.com/alex-irvine/lazydiff

go 1.24.2

require (
	github.com/charmbracelet/bubbles v1.0.0
	github.com/charmbracelet/bubbletea v1.3.10
	github.com/charmbracelet/lipgloss v1.1.0
	github.com/charmbracelet/x/ansi v0.11.6
	github.com/creack/pty v1.1.18
	github.com/mattn/go-runewidth v0.0.19
	github.com/pelletier/go-toml/v2 v2.2.4
)
```

Run `go mod tidy`. Expected: `go.sum` is created and no package-resolution error occurs.

- [ ] **Step 2: Write failing configuration tests**

Add tests for default values, missing-file behavior, TOML decoding, unknown provider rejection, missing command rejection, unknown prompt placeholder rejection, and XDG path resolution. Test expectations include:

```go
func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil { t.Fatal(err) }
	if cfg.Agent.Provider != "copilot" || cfg.Agent.Command != "copilot" || !cfg.Agent.ReadOnly {
		t.Fatalf("unexpected defaults: %+v", cfg.Agent)
	}
}

func TestLoadRejectsUnknownProvider(t *testing.T) {
	path := writeConfig(t, `[agent]
provider = "unknown"
command = "agent"
`)
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "provider") { t.Fatalf("error = %v", err) }
}
```

Run `go test ./config`. Expected: FAIL because config types and functions do not exist.

- [ ] **Step 3: Implement configuration types and defaults**

Implement:

```go
type Config struct { Agent AgentConfig }
type AgentConfig struct {
	Provider string `toml:"provider"`
	Command string `toml:"command"`
	Args []string `toml:"args"`
	ReadOnly bool `toml:"read_only"`
	AllowExternalTools bool `toml:"allow_external_tools"`
	Prompts PromptConfig `toml:"prompts"`
}
type PromptConfig struct { Overall string `toml:"overall"`; Detail string `toml:"detail"` }
```

Use default provider/command `copilot`, `ReadOnly=true`, `AllowExternalTools=false`, empty args, and templates containing all required placeholders. `Load` must return defaults when file does not exist, decode TOML when present, merge omitted fields with defaults, and call `Validate`.

`ConfigPath` must use `$XDG_CONFIG_HOME/lazydiff/config.toml` when set and `$HOME/.config/lazydiff/config.toml` otherwise.

- [ ] **Step 4: Implement validation and rerun focused tests**

Reject providers other than `copilot` and `generic`, reject empty command, reject empty overall/detail templates, reject unknown placeholders, and require `{{overall_diff}}` in overall plus `{{overall_diff}}`, `{{selection}}`, and `{{selected_diff}}` in detail. Return errors naming the invalid field.

Run `go test ./config -run 'TestLoad|TestConfigPath' -v`. Expected: PASS.

- [ ] **Step 5: Run repository checks and commit**

Run:

```bash
go test ./...
go vet ./...
go build ./...
```

Expected: all commands exit 0. Commit:

```bash
git add go.mod go.sum config
git commit -m "feat: add lazydiff configuration"
```

---

### Task 2: Parse Raw Diffs into Stable Targets

**Files:**
- Create: `diff/model.go`
- Create: `diff/parse.go`
- Create: `diff/parse_test.go`

**Interfaces:**
- Produces `diff.File`, `diff.Hunk`, `diff.FileStatus`, `diff.Parse(raw string) ([]diff.File, error)`, `File.RawDiff() string`, and `Hunk.RawDiff() string` for Git and UI tasks.

- [ ] **Step 1: Write parser fixture and failing tests**

Test a fixture containing modified, added, deleted, renamed, binary, and two-hunk files. Assert file paths/statuses, hunk headers and line ranges, exact raw file slices, exact raw hunk slices, and stable IDs. Add malformed-header test returning an error rather than silently dropping content.

Required model shape:

```go
type FileStatus string
const ( Modified FileStatus = "modified"; Added FileStatus = "added"; Deleted FileStatus = "deleted"; Renamed FileStatus = "renamed"; Binary FileStatus = "binary" )
type File struct { ID, Path, OldPath string; Status FileStatus; Hunks []Hunk; Raw string }
type Hunk struct { ID, Header string; OldStart, OldCount, NewStart, NewCount int; Raw string }
```

Run `go test ./diff`. Expected: FAIL because parser is absent.

- [ ] **Step 2: Implement file/header parsing**

Parse `diff --git a/... b/...` sections, `similarity index`, `rename from/to`, `new file mode`, `deleted file mode`, `Binary files`, `---`, and `+++` headers. Preserve raw section text exactly. Normalize quoted Git paths only as needed for IDs; retain display path.

- [ ] **Step 3: Implement hunk parsing and IDs**

Parse `@@ -old[,count] +new[,count] @@ optional function` with count default 1. A file ID is `status:path:oldpath`; a hunk ID is `fileID:ordinal:header`. Include `\ No newline at end of file` lines in hunk raw text. Return a file with zero hunks for binary changes.

- [ ] **Step 4: Run focused and full checks**

Run `go test ./diff -v`, then `go test ./...`, `go vet ./...`, and `go build ./...`. Expected: PASS. Commit:

```bash
git add diff
git commit -m "feat: parse diff files and hunks"
```

---

### Task 3: Collect Git Snapshots

**Files:**
- Create: `git/mode.go`
- Create: `git/repository.go`
- Create: `git/snapshot.go`
- Create: `git/repository_test.go`

**Interfaces:**
- Produces `git.Mode`, `git.Repository`, `git.Snapshot`, `git.Open(ctx, dir)`, `Repository.Snapshot(ctx, mode)`, `Repository.DefaultBranch(ctx)`, and `Snapshot.Target(fileID string) (*diff.File, bool)`.

- [ ] **Step 1: Write temporary-repository tests**

Create a helper that runs `git init`, configures author identity, commits a base file, then creates modifications. Test:

- working-tree mode includes staged and unstaged tracked changes;
- staged mode includes only staged changes;
- branch mode compares current `HEAD` against default branch using `default...HEAD`;
- untracked text files are included in working-tree mode;
- ignored files are excluded;
- default branch is resolved from `refs/remotes/origin/HEAD`, then `origin/main`, `main`, `master`;
- non-repository `Open` returns an error containing `git repository`;
- snapshot ID changes when raw diff or mode changes.

Run `go test ./git`. Expected: FAIL because Git package is absent.

- [ ] **Step 2: Implement safe Git command runner and repository discovery**

Implement `Repository{Root string}` and direct `exec.CommandContext` execution. `Open` runs `git rev-parse --show-toplevel` in the supplied directory, trims output, and rejects non-zero exit. Never invoke a shell. Add a package-level runner interface for tests:

```go
type CommandRunner interface { Run(context.Context, string, ...string) ([]byte, error) }
```

- [ ] **Step 3: Implement tracked diff modes**

Use these exact commands with `--no-color --binary`:

```text
working: git -C <root> diff --no-color --binary HEAD
staged:  git -C <root> diff --no-color --binary --cached
branch:  git -C <root> diff --no-color --binary <default>...HEAD
```

Accept exit code 1 only for diff commands when output is available. Parse raw output with `diff.Parse`.

- [ ] **Step 4: Add bounded untracked-file synthesis**

For working-tree mode, run `git ls-files --others --exclude-standard -z`. For each regular file at or below 1 MiB, run direct `git diff --no-index --no-color --binary /dev/null <relative-path>` and accept its expected exit code 1. Append the resulting raw section before parsing. For files over 1 MiB, directories, unreadable files, or binary-only output, add a file entry with status `Binary` or an explicit `Unanalyzable` status and an empty hunk list; do not include unbounded content in prompts.

- [ ] **Step 5: Implement snapshot identity and default-branch fallback**

Hash mode, base ref, and raw diff with SHA-256. Resolve default branch by trying symbolic remote HEAD, then `origin/main`, `main`, and `master`; return a clear error if no candidate exists. Store status/error metadata without losing last successful snapshot at UI level.

- [ ] **Step 6: Run checks and commit**

Run:

```bash
go test ./git -v
go test ./...
go vet ./...
go build ./...
```

Expected: PASS. Commit:

```bash
git add git
git commit -m "feat: collect Git diff snapshots"
```

---

### Task 4: Add Delta Presentation with ANSI Fallback

**Files:**
- Create: `delta/render.go`
- Create: `delta/render_test.go`

**Interfaces:**
- Produces `delta.Renderer`, `delta.Result`, and `Renderer.Render(ctx context.Context, raw string, width int) Result`.

- [ ] **Step 1: Write failing renderer tests**

Use a temporary executable named `delta` earlier in `PATH` that records stdin/args and emits `\x1b[32m+added\x1b[0m`. Assert input is raw, args contain `--paging=never`, `--color-only`, and `--width=80`, and ANSI output is preserved. Add missing-executable and non-zero-exit tests asserting raw fallback and warning state.

Run `go test ./delta`. Expected: FAIL because renderer is absent.

- [ ] **Step 2: Implement direct Delta invocation**

Implement:

```go
type Result struct { Content string; Styled bool; Warning error }
type Renderer struct { Command string }
func (r Renderer) Render(ctx context.Context, raw string, width int) Result
```

Default command is `delta`. Clamp width to at least 20. Invoke with `--paging=never --color-only --width=<width>`, write raw to stdin, and capture stdout/stderr. Return raw content with `Styled=false` and a wrapped warning when command lookup, write, or exit fails.

- [ ] **Step 3: Add ANSI-safe utilities**

Use `github.com/charmbracelet/x/ansi` for visible width and truncation. Add internal helpers that split rendered output on newlines without stripping escape sequences and truncate by display cells without byte slicing. Test that an escape sequence remains intact after truncation.

- [ ] **Step 4: Run checks and commit**

Run `go test ./delta -v`, `go test ./...`, `go vet ./...`, and `go build ./...`. Expected: PASS. Commit:

```bash
git add delta
git commit -m "feat: render diffs through Delta"
```

---

### Task 5: Render Strict Analysis Prompts

**Files:**
- Create: `prompt/template.go`
- Create: `prompt/template_test.go`

**Interfaces:**
- Produces `prompt.Context`, `prompt.Templates`, `prompt.Parse(overall, detail string)`, `Templates.RenderOverall(Context)`, and `Templates.RenderDetail(Context)`.

- [ ] **Step 1: Write failing template tests**

Test exact substitution of repository, mode, overall diff, selection, and selected diff. Test unknown placeholder rejection, malformed template rejection, missing required placeholders, and preservation of diff text containing braces, Markdown, ANSI-like text, and trailing newlines.

Run `go test ./prompt`. Expected: FAIL because prompt package is absent.

- [ ] **Step 2: Implement strict parsing**

Use `text/template` with `Option("missingkey=error")`. Parse allowed placeholders into a map of strings, reject identifiers outside the five supported names before execution, and validate overall/detail required fields. Do not HTML-escape or trim rendered content.

- [ ] **Step 3: Implement rendering and tests**

Define:

```go
type Context struct { Repository, Mode, OverallDiff, Selection, SelectedDiff string }
type Templates struct { overall, detail *template.Template }
```

Render with `template.Execute` into `strings.Builder`. Ensure output is deterministic for identical context. Run `go test ./prompt -v`, then full checks. Commit:

```bash
git add prompt
git commit -m "feat: render strict analysis prompts"
```

---

### Task 6: Implement Generic and Copilot Agent Runners

**Files:**
- Create: `agent/types.go`
- Create: `agent/generic.go`
- Create: `agent/copilot.go`
- Create: `agent/runner_test.go`

**Interfaces:**
- Produces `agent.Request`, `agent.Event`, `agent.EventKind`, `agent.Runner`, `agent.NewGeneric`, `agent.NewCopilot`, and `Runner.Run(ctx, Request, func(Event)) error`.

- [ ] **Step 1: Verify Copilot flags before coding**

Run:

```bash
copilot --help
copilot help permissions
```

Confirm current names for `--output-format text`, `--stream on`, `--silent`, `--no-ask-user`, `--disable-builtin-mcps`, `--excluded-tools`, and `-p`. Record exact verified names in a test comment and use only flags present in this installed CLI. If a flag differs, update the command builder and its test rather than assuming compatibility.

- [ ] **Step 2: Write fake-command tests first**

Create a temporary executable that reads stdin into a file, writes two stdout lines and one stderr line, then exits with a configurable code. Test generic runner working directory, exact argv, prompt bytes, output events, diagnostic event, non-zero error, and cancellation. Test stdout/stderr concurrently so a large stderr stream cannot deadlock the runner.

Run `go test ./agent`. Expected: FAIL because runners are absent.

- [ ] **Step 3: Implement shared request/event types**

Use:

```go
type Request struct { RepoRoot, Prompt string }
type EventKind int
const ( Output EventKind = iota; Diagnostic )
type Event struct { Kind EventKind; Text string }
type Runner interface { Run(context.Context, Request, func(Event)) error }
```

The callback must be invoked from stream goroutines only with complete lines; the runner waits for both stdout/stderr readers before returning. A context cancellation returns `context.Canceled` and kills the child process.

- [ ] **Step 4: Implement generic direct command runner**

`Generic` stores command and args, invokes `exec.CommandContext` directly with `cmd.Dir = RepoRoot`, writes prompt to stdin, closes stdin, scans stdout/stderr concurrently with 1 MiB scanner buffers, emits output/diagnostic lines, waits, and wraps non-zero exit errors. No shell is used.

- [ ] **Step 5: Implement Copilot temp-file adapter**

Create a 0600 temp file using `os.CreateTemp`, write the prompt, close it, and remove it with `defer`. Build args from configured args plus verified defaults: `--output-format text`, `--stream on`, `--silent`, `--no-color`, `--no-ask-user`, and `-p <instruction>`. When `ReadOnly=true`, add verified exclusions for write, shell, and URL tools. When `AllowExternalTools=false`, add `--disable-builtin-mcps` and URL exclusion. The instruction must tell Copilot to read the temp file, inspect repository files read-only, make no changes, use no shell/network/MCP, and return only the analysis.

Run Copilot from `RepoRoot`. Do not use `--allow-all`, `--yolo`, `--allow-all-tools`, or `--allow-all-urls`.

- [ ] **Step 6: Test Copilot command construction without real Copilot**

Inject a command path in the Copilot runner test. The fake executable records argv, reads the temp-file path from the prompt argument, verifies prompt contents and 0600 mode, emits output, and exits 0. Assert the temp file is gone after return and restrictive flags are present. Add opt-out test for explicitly disabled `ReadOnly`/enabled external tools.

- [ ] **Step 7: Run checks and commit**

Run `go test ./agent -v`, `go test ./...`, `go vet ./...`, and `go build ./...`. Expected: PASS. Commit:

```bash
git add agent
git commit -m "feat: stream read-only agent analysis"
```

---

### Task 7: Build Tree and Layout Primitives

**Files:**
- Create: `ui/tree.go`
- Create: `ui/layout.go`
- Create: `ui/model_test.go`

**Interfaces:**
- Produces `ui.Focus`, `ui.AnalysisTab`, `ui.TreeModel`, `ui.NewTree(files []diff.File) *TreeModel`, `TreeModel.SetFiles([]diff.File)`, `TreeModel.Move(delta int)`, `TreeModel.Toggle()`, `TreeModel.Selected() (diff.File, *diff.Hunk, bool)`, `ui.Layout`, and `ui.ComputeLayout(width, height int) Layout`.

- [ ] **Step 1: Write tree and layout tests**

Test hierarchical rows for files/hunks, cursor movement, expansion/collapse, file target selection, hunk target selection, stable selection by ID after replacing files, and empty state. Test `ComputeLayout` at 120x40, 80x24, and narrow widths:

- tree width is `min(max(28% of width, 20), width/3)`;
- right side width is the remainder;
- right side splits into diff above analysis;
- narrow dimensions never produce negative widths/heights.

Run `go test ./ui`. Expected: FAIL because UI primitives are absent.

- [ ] **Step 2: Implement tree model**

Represent rows as file nodes with hunk children and maintain `cursor`, `expanded map[string]bool`, and `selectedID`. `SetFiles` preserves selected ID when available, otherwise selects first visible row. `Selected` returns a `diff.File` plus optional `diff.Hunk`.

- [ ] **Step 3: Implement approved 28/72 layout**

Define:

```go
type Layout struct { Tree, Diff, Analysis, Status Rect }
type Rect struct { X, Y, W, H int }
```

Compute tree width no greater than `width/3`, target 28%, allocate the remainder to a right stack with diff height 57% and analysis height 43%, reserve one status row, and collapse to vertical sections below 80 columns. Clamp every rectangle dimension to non-negative values.

- [ ] **Step 4: Run focused tests and commit**

Run `go test ./ui -run 'TestTree|TestComputeLayout' -v`, then full checks. Expected: PASS. Commit:

```bash
git add ui/tree.go ui/layout.go ui/model_test.go
git commit -m "feat: add diff tree and review layout"
```

---

### Task 8: Implement Bubble Tea Model and Rendering

**Files:**
- Create: `ui/model.go`
- Create: `ui/render.go`
- Modify: `ui/model_test.go`

**Interfaces:**
- Produces `ui.SnapshotLoader`, `ui.NewModel(repo git.Repository, cfg config.Config, loader SnapshotLoader, renderer delta.Renderer, runner agent.Runner) Model`, `Model.Init`, `Model.Update`, and `Model.View`.

- [ ] **Step 1: Write model message and behavior tests**

Use fake snapshot loader, fake Delta renderer, and fake agent runner. Test initial refresh, mode cycling, manual refresh, file/hunk selection, overall request prompt context, detail request prompt context, separate tab results, streamed output, cancellation, stale result after snapshot ID change, failed Git refresh retaining last snapshot, and failed agent output appearing in analysis/request log.

Test key mapping:

```go
for _, key := range []string{"a", "A", "c", "m", "r", "tab", "?", "q"} {
	model, _ = model.Update(tea.KeyMsg{...})
}
```

Run `go test ./ui`. Expected: FAIL until model exists.

- [ ] **Step 2: Implement model state and messages**

Define the loader boundary before the model:

```go
type SnapshotLoader interface {
	Snapshot(context.Context, git.Mode) (git.Snapshot, error)
}
```

Include current mode, snapshot, layout dimensions, focus, tree, selected display content, analysis tabs, request IDs, request contexts/cancel functions, stale flags, warning/error strings, and an in-memory result map keyed by `snapshotID + tab + targetID`. Define messages for snapshot, Delta render, analysis output, analysis completion, and refresh tick.

- [ ] **Step 3: Implement refresh and Delta commands**

`refreshCmd` calls the injected snapshot loader in a goroutine. On success, update tree and request Delta rendering for selected display target at current right-pane width. On error, preserve prior snapshot and set status error. `deltaCmd` passes raw selected diff and width to renderer. Ignore messages with an old snapshot/request ID.

- [ ] **Step 4: Implement analysis commands and cancellation**

Overall `a` renders overall template with raw snapshot diff and starts `agent.Run`. Detail `A` renders selected file/hunk context and starts its request. Store cancellers by tab/target. Stream output into the matching result buffer. On completion classify success, cancellation, or error; append stderr/exit diagnostics to request log; mark result stale when current snapshot differs.

- [ ] **Step 5: Implement keyboard and focus behavior**

Implement Tab/Shift+Tab focus cycling across tree, diff, and analysis; Up/Down according to focused pane; Space expansion; `m`, `r`, `a`, `A`, `c`, `g`, `G`, `?`, and `q`. Disable analysis when no analyzable target exists. Add periodic refresh with a two-second `tea.Tick` and reschedule only while the model is active.

- [ ] **Step 6: Implement terminal rendering**

Render the approved composition:

- file tree capped at one-third width;
- Delta ANSI content in upper-right without generic Lip Gloss recoloring;
- analysis tabs `detail`, `overall`, `request log` below;
- cyan selection/live state, amber mode/hunk metadata, Delta-owned green/red changes;
- status line for Git state, Delta active/fallback, agent state, stale/error state, and key hints;
- independent scrolling and ANSI-safe truncation using Charm ANSI utilities;
- narrow layout as vertical tree/diff/analysis sections.

Sanitize agent ANSI/control sequences before rendering analysis text, but preserve ordinary Markdown characters. Keep browser-only gradients/shadows out of terminal output.

- [ ] **Step 7: Run checks and commit**

Run `go test ./ui -v`, `go test ./...`, `go vet ./...`, and `go build ./...`. Expected: PASS. Commit:

```bash
git add ui
git commit -m "feat: add interactive review TUI"
```

---

### Task 9: Wire Startup and Application Lifecycle

**Files:**
- Create: `cmd/lazydiff/main.go`

**Interfaces:**
- Consumes all package interfaces from Tasks 1-8.
- Produces executable `lazydiff` and `-config <path>` startup option.

- [ ] **Step 1: Write startup smoke test**

Add a testable `run(ctx, args, stdin, stdout, stderr) error` function. Test invalid current directory returns a message containing `git repository`, missing config uses Copilot defaults, and `-config` loads a generic fake command without starting it until analysis is requested.

- [ ] **Step 2: Implement startup wiring**

Parse `-config` with the standard library `flag` package. Resolve current directory, open Git repository, load config, parse prompt templates, construct Delta renderer and either `agent.NewCopilot` or `agent.NewGeneric`, construct UI model, and run `tea.NewProgram(model, tea.WithAltScreen()).Run()`.

Use `log.SetOutput(stderr)` only before Bubble Tea starts. Return errors to `main` for one concise stderr message and non-zero exit. Do not add subcommands or shell execution.

- [ ] **Step 3: Run application checks**

Run:

```bash
go run ./cmd/lazydiff --help
go build -o /tmp/lazydiff ./cmd/lazydiff
go vet ./...
go test ./...
```

Expected: help exits 0; build, vet, and tests exit 0. Commit:

```bash
git add cmd/lazydiff
git commit -m "feat: add lazydiff executable"
```

---

### Task 10: Add Full Linux PTY Integration Coverage

**Files:**
- Create: `integration/pty_linux_test.go`

**Interfaces:**
- Consumes compiled `cmd/lazydiff`, temporary Git repositories, temporary generic-agent and Delta executables, and `github.com/creack/pty`.

- [ ] **Step 1: Build PTY test helpers**

Implement helpers that:

- create a temporary repository with base commit and controlled changes;
- write a TOML config selecting `provider = "generic"`, a fake agent path, and fake Delta path;
- build `./cmd/lazydiff` into `t.TempDir()`;
- start it with `pty.Start`, set terminal size with `pty.Setsize`, and collect output until a required marker or timeout;
- send key bytes and close/kill processes with cleanup.

Use a 120x40 PTY for normal tests and 70x24 for narrow-layout tests. Do not call the real Copilot or real Delta.

- [ ] **Step 2: Add startup and navigation scenarios**

Cover valid startup, invalid repository, empty state, file/hunk tree navigation, Space expansion, mode cycling, refresh, help, quit, and narrow layout. Assert output contains `changed files`, `delta`, `detail`, and no third vertical analysis column. Assert tree width is at most one-third by checking layout markers emitted by a test-only debug-free rendering assertion or by model-level layout tests; do not parse terminal pixel positions.

- [ ] **Step 3: Add Delta and agent scenarios**

Fake Delta emits a known ANSI green line and records width. Assert terminal output retains the ANSI sequence and status says Delta active. Remove fake Delta and assert raw fallback status. Fake agent records prompt stdin, emits delayed stdout lines and stderr, and exits 0; assert overall `a` prompt contains raw overall diff and detail `A` prompt contains both raw overall and selected diff. Assert result streams into the analysis pane.

- [ ] **Step 4: Add error, cancel, stale, and resize scenarios**

Fake agent exits non-zero with stderr; assert streamed output remains and request log includes stderr. Fake agent blocks until killed; press `c` and assert cancelled status. Modify repository during request, refresh, and assert prior answer is marked stale. Resize PTY and assert process remains alive and Delta receives new width.

- [ ] **Step 5: Run complete verification and commit**

Run:

```bash
go test ./integration -v -count=1
go test ./... -count=1
go vet ./...
go build ./...
git diff --check
```

Expected: all commands exit 0. Commit:

```bash
git add integration
git commit -m "test: cover lazydiff through PTY"
```

---

### Task 11: Final Review and Documentation Consistency

**Files:**
- Create: `README.md`

**Interfaces:**
- Documents the executable, config path, supported modes, Delta fallback, Copilot safety defaults, key map, and exact test commands.

- [ ] **Step 1: Write README acceptance checklist**

Document:

- prerequisites: Go, Git, Delta optional, Copilot optional when using generic provider;
- `go build -o lazydiff ./cmd/lazydiff`;
- launch from a Git repository with `./lazydiff`;
- `$XDG_CONFIG_HOME/lazydiff/config.toml` or `$HOME/.config/lazydiff/config.toml`;
- default Copilot behavior and read-only/external-access restrictions;
- generic stdin agent example;
- working, staged, and branch modes;
- file/hunk detail analysis and stale result behavior;
- Delta presentation and raw fallback;
- `go test ./...`, `go vet ./...`, `go build ./...`;
- link to `docs/lazydiff-visual.html` and design spec.

- [ ] **Step 2: Reconcile README with implementation**

Run `grep -R "TODO\|TBD\|FIXME" --exclude-dir=.git --exclude-dir=.superpowers --exclude='*implementation-plan.md' .`. Expected: no matches. Correct any README statement that disagrees with config defaults, key map, or actual command flags.

- [ ] **Step 3: Run final verification and commit**

Run:

```bash
go test ./... -count=1
go vet ./...
go build ./...
git diff --check
git status --short
```

Expected: tests, vet, and build exit 0; diff check emits no output; status contains only intentional committed-state changes before commit. Commit:

```bash
git add README.md
git commit -m "docs: document lazydiff usage"
```

---

## Plan Self-Review

### Spec coverage

- Go/Bubble Tea/Lip Gloss: Tasks 1, 7, 8, and 9.
- Git CLI and three diff modes: Task 3.
- Raw canonical snapshots and stable IDs: Tasks 2 and 3.
- Untracked/binary/rename handling: Tasks 2, 3, and 10.
- Delta ANSI presentation and fallback: Task 4 and 10.
- 28/72 stacked visual composition: Tasks 7, 8, and 10.
- TOML defaults and strict prompts: Tasks 1 and 5.
- Generic stdin runner: Task 6.
- Copilot temp-file adapter and safety flags: Task 6.
- Read-only repository inspection: Task 6.
- Separate overall/detail context: Tasks 5, 6, and 8.
- Session-only cache and stale results: Task 8 and 10.
- Error handling and cancellation: Tasks 6, 8, and 10.
- Unit, subprocess, and PTY verification: Tasks 1-10.
- Static visual export: already committed at `docs/lazydiff-visual.html`; Task 11 links it.

### Placeholder scan

No implementation step uses `TBD`, `TODO`, `FIXME`, “implement later”, or an unspecified file. Copilot flag names are verified in Task 6 before coding and are covered by a fake-command test.

### Type consistency

- `config.Config` feeds `prompt.Parse` and agent selection in `cmd/lazydiff/main.go`.
- `diff.Parse` returns `[]diff.File`, consumed by `git.Snapshot` and `ui.TreeModel.SetFiles`.
- `git.Repository.Snapshot` returns `git.Snapshot`, consumed by `ui.SnapshotLoader`.
- `delta.Renderer.Render` returns `delta.Result`, consumed by UI Delta messages.
- `prompt.Templates.RenderOverall/RenderDetail` return strings consumed by `agent.Request`.
- `agent.Runner.Run` emits `agent.Event` values consumed by UI analysis messages.
- `ui.NewModel` receives repository, config, loader, renderer, and runner dependencies used by startup and tests.

### Scope check

Tasks produce a usable vertical slice in order: config, parser, Git snapshot, Delta, prompts, agent, UI, executable, PTY tests, documentation. No independent subsystem is left without a testable integration point.
