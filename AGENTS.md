# lazydiff

Terminal Git diff reviewer (Go 1.24 + Bubble Tea) that shells out to an AI CLI (default provider: `generic`/`claude`, see drift note below) to explain diffs, and can stage/commit/push and open a GitHub PR. The AI agent is always read-only; **all git mutations happen in Go code**, never in the agent process.

## Build, test, verify

Run in this order (matches `.github/workflows/ci.yml`):

```bash
go test ./... -count=1
go vet ./...
go build ./...
git diff --check
```

- No `golangci-lint`, `Makefile`, or `gofmt`/`goimports` check exists anywhere in the repo or CI — don't assume one.
- No table-driven tests / no `t.Run` subtests anywhere — every case is its own `TestXxx` func, so `go test ./pkg -run TestName -v -count=1` targets one case directly.
- `go test ./integration -v -count=1` is Linux-only (`//go:build linux` + `_linux.go` suffix) and **compiles a fresh `lazydiff` binary per test** inside `newFixture` (`integration/pty_linux_test.go:58-64`) — slower than unit tests. It drives the real binary through a PTY (`creack/pty`) against a scratch git repo, faking `delta` and the agent as throwaway shell scripts.
- A compiled binary `lazydiff` is committed at repo root (tracked, not gitignored). Rebuilding it locally (`go build -o lazydiff ./cmd/lazydiff`, per README) dirties the working tree — don't commit that rebuild unless intentional.
- `scripts/build-all.sh <version>` cross-compiles linux/darwin(amd64+arm64)/windows with `-ldflags -X .../version.Current=<version>`; only called from `.github/workflows/release.yml` on `v*` tags. `scripts/build-all_test.sh` smoke-tests it but isn't wired into any CI workflow — run manually if you touch `build-all.sh`.
- Commit messages follow Conventional Commits (`feat(scope): ...`, `fix(scope): ...`, `test(scope): ...`, `docs(scope): ...`) — see `git log`.

## Architecture

Dependency direction (leaf → root): `diff` → `git` → `ui`; `agent`, `config`, `delta`, `pr`, `prompt`, `version` are independent leaves also consumed by `ui`; `cmd/lazydiff` wires everything and is the only `main` package.

| Package | Responsibility |
|---|---|
| `diff` | Pure unified-diff parser/model (`File`, `Hunk`) plus `BuildPatch` (slices a sub-patch from selected hunks for partial staging, git-add-p style). No git CLI calls. |
| `git` | The **only** package that shells out to `git`. `Repository` (via `git.Open`) exposes reads (`Snapshot`, `CurrentBranch`, `DefaultBranch`, `RemoteURL`) and mutations (`StageFile`, `StagePatch`, `Commit`, `Push`), all routed through the `CommandRunner` interface — the seam faked in tests. |
| `agent` | Runs the external AI CLI via the `Runner` interface. `Generic` pipes the prompt over stdin, streams stdout/stderr line-by-line as `Event`s. `Copilot` wraps `Generic`, writing the prompt to a temp file and adding read-only/no-external-tool CLI flags. |
| `config` | Loads/validates `$XDG_CONFIG_HOME/lazydiff/config.toml` (TOML), overlaid onto `Default()`. |
| `prompt` | Compiles the 4 prompt templates (overall/detail/commit_message/pr_description) from `config` into `text/template`s, renders against a `Context`. |
| `delta` | Shells out to `delta` for ANSI-colored diff display; falls back to raw diff text on any failure (missing binary, non-zero exit). Display-only — raw diff is always what's parsed and sent to the agent. |
| `pr` | Ticket-ID extraction, GitHub-only compare-URL construction, opening it in the browser. No `gh` CLI, no GitHub API — "PR creation" is push + a pre-filled `github.com/.../compare/...` URL (lazygit-style); non-`github.com` remotes error out by design. Default ticket pattern extracts a bare 6-10 char lowercase-alphanumeric ID from the branch name (ClickUp-style) and prefixes it `CU-` only in generated titles/trailers. |
| `version` | `Current` var (default `"dev"`, overridden via `-ldflags` on tagged builds) plus self-update (`CheckForUpdate`/`PerformUpdate`) — the one place that shells out to `gh` (distinct from `pr`, which never does). |
| `ui` | The Bubble Tea app: `Model` (state), `TreeModel` (file/hunk tree + staging checkboxes), `ActionDialog` (shared commit/PR modal), layout/rendering. |
| `cmd/lazydiff` | `main` — loads config, opens the repo, picks the agent adapter from `cfg.Agent.Provider`, wires everything into `ui.Model`, runs the Bubble Tea program. |

### UI async pattern

Every git/agent operation is a `tea.Cmd` (closure over value-copied deps, never `*Model`) returning a typed `*Msg` handled in `Model.Update` (`ui/model.go`). Streamed agent output is pushed back into the loop via a captured `send func(tea.Msg)` (wired post-construction: `TeaModel.SetSend` ← `cmd/lazydiff/main.go:89`), not through the `tea.Cmd` return value.

`SnapshotLoader` (reads) and `Mutator` (stage/commit/push/branch/remote) are the interfaces `ui.Model` depends on instead of `git.Repository` directly — `Repository` satisfies both implicitly (structural typing, no `var _ Mutator = ...`). This is the seam that lets `ui` tests run with zero real git process.

### Commit/PR mutation flow (confirm-before-mutate)

`c` (commit — only when `mode == git.WorkingTree` and the staging plan is non-empty) and `o` (PR — always available) open `ActionDialog` immediately in a "generating" state, stage/diff/render a prompt, and stream the agent's draft into an editable `bubbles/textarea`. **Nothing mutates the repo (stage, commit, push, or open a browser URL) until the user presses `ctrl+s` inside the open dialog**; `esc` cancels, `ctrl+r` restarts generation from scratch. Confirm handlers additionally no-op unless `dialog.Ready && dialog.Err == nil`.

### Staging → git mapping

`TreeModel` checkboxes are tri-state (`Unchecked`/`Checked`/`Indeterminate`), keyed by stable `diff.File.ID`/`diff.Hunk.ID` (survives tree rebuilds on refresh). `TreeModel.StagingPlan()` turns checked state into one `StageAction` per touched file: fully-checked file → `Mutator.StageFile`; partially-checked (some hunks) → `diff.BuildPatch` + `Mutator.StagePatch`. Checkboxes only render/apply in `git.WorkingTree` mode.

## Testing conventions

- **Fakes, not mocks** — hand-written structs implementing the real interfaces (`fakeRunner` for `git.CommandRunner`; `fakeLoader`/`fakeRenderer`/`fakeMutator`/`fakeOpener` for `ui`'s interfaces). `agent.Runner` and `delta.Renderer` are tested against literal throwaway shell-script "binaries" written to `t.TempDir()` instead — no real `copilot`/`delta`/network needed outside `integration/`.
- No golden files, no `testdata/` directories.
- **`fakeLoader` gotcha** (`ui/model_test.go:215-226`): `Snapshot()` returns `snapshots[index]` and only advances `index` if not already at the last element (repeats the final snapshot forever instead of panicking on overrun). When a test calls a `tea.Cmd` directly (e.g. `model.startCommitCmd()()`, bypassing `Init`), seed **exactly** the one snapshot that call will consume, at index 0 — extra "just in case" entries silently change which snapshot content the assertion sees. Documented inline at `ui/model_test.go:388-392`, `:549-552`, `:636-638`.
- `ui/view_test.go`'s `TestBoxWithDeltaContent` is a `Printf`-based debug harness with no real assertions — it always passes; don't copy its style.

## Documentation drift (trust code over README here)

- README's example config shows `provider = "copilot"` as the default; the actual compiled-in default (`config.Default()`, `config/config.go:80-97`) is `provider = "generic"`, `command = "claude"`, `args = ["--model", "haiku-latest"]`.
- README's "Controls" table is stale versus `ui/render.go`'s `helpText()`/`statusLine()` and the real `updateKey` switch in `ui/model.go`: `space` toggles a staging checkbox (expand/collapse is `h`/`l`), `c` starts the commit flow (cancel is `x`), and `ctrl+a` (check-all) / `o` (PR) are missing from the README table entirely.

## Design docs

`docs/superpowers/specs/*.md` and `docs/superpowers/plans/*.md` hold the original feature specs and implementation plans (UI design, CI/release design, pane layout, stage-commit-PR design) — check these for the "why" behind a package boundary or UI decision before re-deriving it.
