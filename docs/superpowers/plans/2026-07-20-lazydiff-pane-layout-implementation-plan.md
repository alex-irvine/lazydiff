# lazydiff Pane Layout Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Recompose `lazydiff` so Delta/code occupies full height on the right while files and agent analysis split the left rail exactly 50/50.

**Architecture:** Replace the current `Tree/Diff/Analysis` layout rectangles with semantic `Files/Code/Agent` rectangles. Wide terminals use a 28% left rail split by body height and a full-height code rail; terminals below 80 columns stack files, code, and agent vertically. Keep model state, data flow, scroll state, and error behavior unchanged.

**Tech Stack:** Go 1.24.2, Bubble Tea, Lip Gloss, existing UI tests, Linux PTY tests, and static HTML visual reference.

## Global Constraints

- Left rail width is `min(28% of terminal width, width/3)`.
- Wide layout files pane occupies upper-left half.
- Wide layout agent pane occupies lower-left half.
- Wide layout code pane occupies full right-side body height.
- Status bar is full width and excluded from body split.
- Odd body heights assign extra row to lower Agent pane.
- Narrow breakpoint remains below 80 columns.
- Narrow order is Files → Code → Agent → Status.
- Code and Agent retain independent scroll positions.
- Focus order remains files → code → agent.
- Git, Delta, agent, prompt, cache, error, CI, and release behavior remain unchanged.
- Static visual reference must match runtime composition.

---

## File Map

```text
ui/layout.go                         semantic rectangle calculation
ui/render.go                         wide/narrow pane composition and labels
ui/model.go                          layout field and pane-specific scroll references
ui/model_test.go                     rectangle and render behavior tests
integration/pty_linux_test.go        wide/narrow terminal ordering assertions
docs/lazydiff-visual.html            approved static visual reference
```

---

### Task 1: Replace Layout Rectangles

**Files:**
- Modify: `ui/layout.go`
- Modify: `ui/model_test.go:64-76`

**Interfaces:**
- Produces `ui.Layout{Files, Code, Agent, Status Rect}`.
- Preserves `ui.ComputeLayout(width, height int) Layout` signature.

- [ ] **Step 1: Write failing wide-layout tests**

Replace current `Tree/Diff/Analysis` assertions with:

```go
func TestComputeLayoutWideSplit(t *testing.T) {
	l := ComputeLayout(120, 40)
	bodyH := 39
	if l.Files.W != 33 || l.Agent.W != l.Files.W || l.Code.X != l.Files.W {
		t.Fatalf("columns = files=%+v code=%+v agent=%+v", l.Files, l.Code, l.Agent)
	}
	if l.Files.H != bodyH/2 || l.Agent.H != bodyH-bodyH/2 || l.Code.H != bodyH {
		t.Fatalf("heights = files=%+v code=%+v agent=%+v", l.Files, l.Code, l.Agent)
	}
	if l.Files.Y != 0 || l.Agent.Y != l.Files.H || l.Code.Y != 0 {
		t.Fatalf("positions = files=%+v code=%+v agent=%+v", l.Files, l.Code, l.Agent)
	}
	if l.Status.Y != bodyH || l.Status.H != 1 {
		t.Fatalf("status = %+v", l.Status)
	}
}

func TestComputeLayoutOddBodyGivesAgentExtraRow(t *testing.T) {
	l := ComputeLayout(120, 42)
	if l.Files.H != 20 || l.Agent.H != 21 || l.Files.H+l.Agent.H != 41 {
		t.Fatalf("odd body split = files=%+v agent=%+v", l.Files, l.Agent)
	}
}
```

Run `go test ./ui -run 'TestComputeLayout' -v`. Expected: FAIL because `Layout` still exposes old fields and old geometry.

- [ ] **Step 2: Implement semantic wide layout**

Change:

```go
type Layout struct {
	Files, Code, Agent, Status Rect
}
```

Keep existing width clamping. For `width >= 80`, compute:

```go
filesH := bodyH / 2
agentH := bodyH - filesH
return Layout{
	Files:  Rect{X: 0, Y: 0, W: leftW, H: filesH},
	Code:   Rect{X: leftW, Y: 0, W: width - leftW, H: bodyH},
	Agent:  Rect{X: 0, Y: filesH, W: leftW, H: agentH},
	Status: Rect{X: 0, Y: bodyH, W: width, H: 1},
}
```

Use `leftW = min(width*28/100, width/3)` with existing minimum/clamp behavior, but ensure result never exceeds one-third. Run focused tests; expected PASS.

- [ ] **Step 3: Implement narrow files → code → agent layout**

For `width < 80`, allocate full width to all panes. Use non-negative sections that preserve order:

```go
filesH := bodyH / 3
codeH := bodyH / 2
agentH := bodyH - filesH - codeH
return Layout{
	Files:  Rect{X: 0, Y: 0, W: width, H: filesH},
	Code:   Rect{X: 0, Y: filesH, W: width, H: codeH},
	Agent:  Rect{X: 0, Y: filesH + codeH, W: width, H: agentH},
	Status: Rect{X: 0, Y: bodyH, W: width, H: 1},
}
```

Add assertions for 70×24: all pane X values are 0, all widths equal 70, `Files.Y < Code.Y < Agent.Y`, and `Agent.Y+Agent.H == Status.Y`.

- [ ] **Step 4: Run checks and commit**

Run:

```bash
```

Expected: PASS. Commit:

```bash
go test ./ui -v
go test ./... -count=1
go vet ./...
go build ./...
git diff --check
```

---

### Task 2: Recompose Runtime Rendering

**Files:**
- Modify: `ui/render.go:13-87`
- Modify: `ui/model.go:207-272, 300-315`

**Interfaces:**
- Consumes `Layout.Files`, `Layout.Code`, and `Layout.Agent` from Task 1.
- Preserves `FocusTree`, `FocusDiff`, `FocusAnalysis`, `diffScroll`, and `analysisScroll` semantics.
- Produces View composition in wide order Files + Code/Agent, and narrow order Files + Code + Agent.

- [ ] **Step 1: Write failing render tests**

Add model view tests with a fixed 120×40 model and 70×24 model. Assert:

```go
wide := model.View()
if !strings.Contains(wide, "CHANGED FILES") || !strings.Contains(wide, "DIFF") || !strings.Contains(wide, "detail") {
	t.Fatal("wide view missing pane markers")
}
```

Also assert `ComputeLayout` geometry is used by rendered box widths, not hard-coded old right-stack geometry. Run `go test ./ui -run 'Test.*View|TestComputeLayout' -v`. Expected: FAIL or compile failure because old layout fields are still referenced.

- [ ] **Step 2: Implement wide View composition**

Replace:

```go
body := lipgloss.JoinHorizontal(lipgloss.Top, tree, lipgloss.JoinVertical(lipgloss.Left, diffView, analysis))
```

with:

```go
files := m.renderTree(l.Files)
code := m.renderDiff(l.Code)
agent := m.renderAnalysis(l.Agent)
left := lipgloss.JoinVertical(lipgloss.Left, files, agent)
body := lipgloss.JoinHorizontal(lipgloss.Top, left, code)
```

This preserves full code height and exact left-rail split supplied by rectangles.

- [ ] **Step 3: Implement narrow View composition**

Use the layout breakpoint consistently:

```go
var body string
if m.termW < 80 {
	body = lipgloss.JoinVertical(lipgloss.Left, files, code, agent)
} else {
	body = lipgloss.JoinHorizontal(lipgloss.Top, left, code)
}
```

Update pane headers and status wording to identify `FILES`, `CODE`, and `AGENT` where the old labels imply a right-side analysis stack. Keep Delta ANSI untouched and keep analysis ANSI sanitized.

- [ ] **Step 4: Update scroll and focus layout references**

Change code-pane width/height calculations from `m.layout.Diff` to `m.layout.Code`, and analysis scroll calculations from `m.layout.Analysis` to `m.layout.Agent`. Preserve key behavior:

- `Tab` order remains Files → Code → Agent.
- `j/k` scroll Code only when Code focus is active.
- `j/k` scroll Agent only when Agent focus is active.
- `g/G` reset or move to bottom of the focused scroll pane.
- File selection still re-renders Code and does not change Agent tab contents.

- [ ] **Step 5: Run focused and full checks**

Run:

```bash
go test ./ui -v
go test ./... -count=1
go vet ./...
go build ./...
git diff --check
```

Expected: PASS. Commit:

```bash
git add ui/render.go ui/model.go ui/model_test.go
git commit -m "feat: give code full-height review pane"
```

---

### Task 3: Update PTY Layout Assertions

**Files:**
- Modify: `integration/pty_linux_test.go:67-119`

**Interfaces:**
- Consumes runtime layout from Task 2.
- Keeps fake Delta and generic agent commands; no real external tools.

- [ ] **Step 1: Add wide layout ordering assertion**

In the 120×40 test, after reading initial output, assert markers occur in this order:

```go
filesAt := strings.Index(output, "CHANGED FILES")
codeAt := strings.Index(output, "DIFF")
if filesAt < 0 || codeAt < 0 || filesAt > codeAt {
	t.Fatalf("wide pane order invalid: files=%d code=%d", filesAt, codeAt)
}
```

Use a second read or `readUntil` for a marker that appears in the lower-left Agent pane if terminal output includes it. Do not rely on exact ANSI coordinates; layout unit tests own geometry.

- [ ] **Step 2: Update narrow ordering test**

At 70×24, read output after startup and assert `CHANGED FILES` occurs before `DIFF`, and `DIFF` occurs before `detail`. Keep existing `A` analysis streaming assertion.

- [ ] **Step 3: Run PTY and full checks**

Run:

```bash
go test ./integration -v -count=1
go test ./... -count=1
go vet ./...
go build ./...
git diff --check
```

Expected: PASS. Commit:

```bash
git add integration/pty_linux_test.go
git commit -m "test: verify pane ordering in PTY"
```

---

### Task 4: Reconcile Static Visual Reference

**Files:**
- Modify: `docs/lazydiff-visual.html`
- Modify: `docs/superpowers/specs/2026-07-20-lazydiff-pane-layout-design.md` only if implementation wording needs factual correction

**Interfaces:**
- Static HTML must represent exact approved runtime composition.

- [ ] **Step 1: Check visual structural markers**

Run:

```bash
grep -nE 'grid-template-columns|grid-template-rows|tree-pane|diff-section|analysis-section|full-height|exactly half' docs/lazydiff-visual.html
```

Expected: visual uses 28/72 columns, two equal rows, code spanning both rows, files row 1, and agent row 2.

- [ ] **Step 2: Run final consistency checks**

Run:

```bash
go test ./... -count=1
go vet ./...
go build ./...
git diff --check
git status --short
```

Expected: all pass and only intentional layout files remain before commit.

- [ ] **Step 3: Commit visual reconciliation**

```bash
git add docs/lazydiff-visual.html docs/superpowers/specs/2026-07-20-lazydiff-pane-layout-design.md
git commit -m "docs: align pane layout reference"
```

---

## Plan Self-Review

### Spec coverage

- Wide 28/72 layout: Tasks 1 and 2.
- Exact 50/50 left split and odd-row behavior: Task 1.
- Full-height code pane: Tasks 1 and 2.
- Narrow files → code → agent ordering below 80 columns: Tasks 1, 2, and 3.
- Focus and independent scroll behavior: Task 2.
- PTY coverage: Task 3.
- Static visual consistency: Task 4.
- Unchanged data/error behavior: Global Constraints and Task 2.

### Placeholder scan

No `TBD`, `TODO`, `FIXME`, or unspecified implementation files. All commands, paths, rectangle fields, and test expectations are concrete.

### Type and path consistency

- `Layout.Files`, `Layout.Code`, `Layout.Agent`, and `Layout.Status` are defined in Task 1 and consumed in Task 2.
- Existing focus constants map to Files, Code, Agent without changing names or model APIs.
- `diffScroll` uses `Layout.Code`; `analysisScroll` uses `Layout.Agent`.
- PTY tests and static HTML use same Files → Code → Agent ordering.

### Scope check

Four tasks cover one focused UI composition change. No Git, Delta, agent, prompt, cache, release, or runtime error subsystem changes are included.
