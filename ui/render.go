package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/alex-irvine/lazydiff/delta"
	"github.com/alex-irvine/lazydiff/diff"
	"github.com/alex-irvine/lazydiff/pr"
	"github.com/alex-irvine/lazydiff/version"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	glamourRenderer *glamour.TermRenderer
	glamourOnce     sync.Once
	glamourWidth    int
)

func (m Model) View() string {
	if m.termW == 0 {
		return "loading..."
	}
	if m.showHelp {
		return lipgloss.Place(m.termW, m.termH, lipgloss.Center, lipgloss.Center, lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Render(m.helpText()))
	}
	if m.showUpdateModal {
		body := fmt.Sprintf("Update Available\n\nlazydiff %s is available\n(current: %s)\n\n[y] download & install   [n] dismiss", m.updateVersion, version.Current)
		return lipgloss.Place(m.termW, m.termH, lipgloss.Center, lipgloss.Center, lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Render(body))
	}
	if m.showUpdating {
		body := fmt.Sprintf("Updating to lazydiff %s...", m.updateVersion)
		return lipgloss.Place(m.termW, m.termH, lipgloss.Center, lipgloss.Center, lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Render(body))
	}
	if m.confirm != nil {
		return m.renderConfirmDialog()
	}
	if m.dialog != nil {
		return m.renderDialog()
	}
	l := m.layout
	files := m.renderTree(l.Files)
	code := m.renderDiff(l.Code)
	agent := m.renderAnalysis(l.Agent)
	left := lipgloss.JoinVertical(lipgloss.Left, files, agent)
	var body string
	if m.termW < 80 {
		body = lipgloss.JoinVertical(lipgloss.Left, files, code, agent)
	} else {
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, code)
	}
	status := lipgloss.NewStyle().Width(m.termW).Foreground(lipgloss.Color("241")).Render(m.statusLine())
	result := lipgloss.JoinVertical(lipgloss.Left, body, status)
	resultLines := strings.Split(result, "\n")
	if len(resultLines) < m.termH {
		resultLines = append(resultLines, make([]string, m.termH-len(resultLines))...)
	} else if len(resultLines) > m.termH {
		resultLines = resultLines[:m.termH]
	}
	return strings.Join(resultLines, "\n")
}

func (m Model) renderDialog() string {
	title := "Commit Message"
	switch m.dialog.Kind {
	case PRDialog:
		title = "Pull Request"
	case RequestChangesDialog:
		title = "Request Changes"
	}
	width := m.termW - 10
	if width > 100 {
		width = 100
	}
	if width < 20 {
		width = 20
	}
	height := m.termH - 12
	if height > 20 {
		height = 20
	}
	if height < 3 {
		height = 3
	}
	m.dialog.Textarea.SetWidth(width)
	m.dialog.Textarea.SetHeight(height)

	var body strings.Builder
	body.WriteString(lipgloss.NewStyle().Bold(true).Render(title))
	body.WriteString("\n\n")
	if m.dialog.Err != nil {
		body.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render("generation error: " + m.dialog.Err.Error()))
		body.WriteString("\n\n")
	}
	body.WriteString(m.dialog.View())
	body.WriteString("\n\n")
	hint := "ctrl+s confirm   esc cancel"
	if m.dialog.Kind != RequestChangesDialog {
		hint += "   ctrl+r regenerate"
	}
	body.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(hint))

	return lipgloss.Place(m.termW, m.termH, lipgloss.Center, lipgloss.Center, lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Render(body.String()))
}

func (m Model) renderConfirmDialog() string {
	width := m.termW - 10
	if width > 100 {
		width = 100
	}
	if width < 20 {
		width = 20
	}
	height := m.termH - 12
	if height > 20 {
		height = 20
	}
	if height < 3 {
		height = 3
	}
	var body strings.Builder
	body.WriteString(lipgloss.NewStyle().Bold(true).Render(m.confirm.Title))
	body.WriteString("\n\n")
	if m.confirm.Err != nil {
		body.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render("error: " + m.confirm.Err.Error()))
		body.WriteString("\n\n")
	}
	body.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render("ctrl+s confirm   esc cancel"))
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Width(width).Height(height)
	return lipgloss.Place(m.termW, m.termH, lipgloss.Center, lipgloss.Center, style.Render(body.String()))
}

func (m Model) renderTree(r Rect) string {
	if m.treeMode == TreeModePRSelector && m.prSelector != nil {
		return m.renderPRSelector(r)
	}
	if m.treeMode == TreeModeBranchSelector && m.branchSelector != nil {
		return m.renderBranchSelector(r)
	}
	if m.treeMode == TreeModeWorktree && m.worktreeSelector != nil {
		return m.renderWorktreeSelector(r)
	}
	title := m.renderTabBar()
	titleRendered := delta.Truncate(title, max(1, r.W-2))
	lines := []string{titleRendered}
	if m.searchActive {
		searchBar := "/" + m.searchQuery + "_  [n]next [N]prev [esc]cancel"
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("228")).Render(delta.Truncate(searchBar, max(1, r.W-2))))
	}
	nodes := m.visibleNodes()
	if len(nodes) == 0 {
		empty := delta.Truncate(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("(no changes)"), max(1, r.W-2))
		lines = append(lines, empty)
		return box(r, strings.Join(padLines(lines, r.H-2), "\n"), m.focus == FocusTree)
	}
	contentH := r.H - 3
	if contentH < 1 {
		contentH = 1
	}
	m.tree.ClampScroll(contentH)
	scroll := m.tree.scrollOffset
	if scroll < 0 {
		scroll = 0
	}
	if scroll >= len(nodes) {
		scroll = max(0, len(nodes)-1)
	}
	visible := nodes[scroll:]
	maxLines := contentH
	if len(visible) > maxLines {
		visible = visible[:maxLines]
	}
	maxW := max(1, r.W-2)
	for _, node := range visible {
		id := node.ID()
		active := id == m.tree.selectedID
		prefix := "  "
		if active {
			prefix = "▶ "
		}
		indent := strings.Repeat("  ", node.Level)
		checkbox := ""
		if m.treeMode == TreeModeWorktree {
			switch m.tree.CheckState(node) {
			case Checked:
				checkbox = "[x] "
			case Indeterminate:
				checkbox = "[-] "
			default:
				checkbox = "[ ] "
			}
		}
		var icon string
		if node.Hunk != nil {
			icon = "  "
		} else if node.File != nil {
			icon = "📄 "
		} else if node.Expanded {
			icon = "📂 "
		} else {
			icon = "📁 "
		}
		fullLine := prefix + indent + checkbox + icon + node.Label
		truncated := delta.Truncate(fullLine, maxW)
		color := lipgloss.Color("245")
		if active {
			color = lipgloss.Color("51")
		} else if node.Hunk != nil {
			color = lipgloss.Color("179")
		}
		lines = append(lines, lipgloss.NewStyle().Foreground(color).Render(truncated))
	}
	return box(r, strings.Join(padLines(lines, r.H-2), "\n"), m.focus == FocusTree)
}

func (m Model) renderBranchSelector(r Rect) string {
	title := delta.Truncate(m.renderTabBar(), max(1, r.W-2))
	lines := []string{title}
	rows := m.branchSelector.Rows()
	if len(rows) == 0 {
		empty := delta.Truncate(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("(no local branches)"), max(1, r.W-2))
		lines = append(lines, empty)
		return box(r, strings.Join(padLines(lines, r.H-2), "\n"), m.focus == FocusTree)
	}
	maxW := max(1, r.W-2)
	for i, branch := range rows {
		prefix := "  "
		if i == m.branchSelector.Cursor() {
			prefix = "▶ "
		}
		style := lipgloss.Color("245")
		if i == m.branchSelector.Cursor() {
			style = lipgloss.Color("51")
		} else if branch == m.branchSelector.currentBranch {
			style = lipgloss.Color("228")
		}
		line := delta.Truncate(prefix+branch, maxW)
		lines = append(lines, lipgloss.NewStyle().Foreground(style).Render(line))
	}
	return box(r, strings.Join(padLines(lines, r.H-2), "\n"), m.focus == FocusTree)
}

func (m Model) renderWorktreeSelector(r Rect) string {
	title := delta.Truncate(m.renderTabBar(), max(1, r.W-2))
	lines := []string{title}
	rows := m.worktreeSelector.Rows()
	if len(rows) == 0 {
		empty := delta.Truncate(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("(no worktrees)"), max(1, r.W-2))
		lines = append(lines, empty)
		return box(r, strings.Join(padLines(lines, r.H-2), "\n"), m.focus == FocusTree)
	}
	maxW := max(1, r.W-2)
	for i, entry := range rows {
		prefix := "  "
		if i == m.worktreeSelector.Cursor() {
			prefix = "▶ "
		}
		style := lipgloss.Color("245")
		if i == m.worktreeSelector.Cursor() {
			style = lipgloss.Color("51")
		} else if entry.Name == m.worktreeSelector.current {
			style = lipgloss.Color("228")
		}
		line := delta.Truncate(prefix+entry.Name, maxW)
		lines = append(lines, lipgloss.NewStyle().Foreground(style).Render(line))
	}
	return box(r, strings.Join(padLines(lines, r.H-2), "\n"), m.focus == FocusTree)
}

func (m Model) renderPRSelector(r Rect) string {
	title := delta.Truncate(m.renderTabBar(), max(1, r.W-2))
	lines := []string{title}
	rows := m.visiblePRs()
	if len(rows) == 0 {
		msg := "(no open pull requests)"
		if m.prSelector.err != nil {
			msg = "(error: " + m.prSelector.err.Error() + ")"
		}
		empty := delta.Truncate(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(msg), max(1, r.W-2))
		lines = append(lines, empty)
		return box(r, strings.Join(padLines(lines, r.H-2), "\n"), m.focus == FocusTree)
	}
	maxW := max(1, r.W-2)
	for i, p := range rows {
		prefix := "  "
		if i == m.prSelector.Cursor() {
			prefix = "▶ "
		}
		icon := "✓"
		if p.Mergeable == "CONFLICTING" {
			icon = "✗"
		} else if p.Mergeable == "UNKNOWN" {
			icon = "?"
		}
		style := lipgloss.Color("245")
		if i == m.prSelector.Cursor() {
			style = lipgloss.Color("51")
		}
		line := fmt.Sprintf("%s#%d %s  (%s, %s)", prefix, p.Number, p.Title, p.Author, icon)
		lines = append(lines, lipgloss.NewStyle().Foreground(style).Render(delta.Truncate(line, maxW)))
	}
	return box(r, strings.Join(padLines(lines, r.H-2), "\n"), m.focus == FocusTree)
}

func (m Model) visiblePRs() []pr.PR {
	if m.prSelector == nil {
		return nil
	}
	if m.searchFilter == nil {
		return m.prSelector.Rows()
	}
	all := m.prSelector.Rows()
	var result []pr.PR
	for _, p := range all {
		label := fmt.Sprintf("#%d %s", p.Number, p.Title)
		if m.searchFilter.MatchString(label) {
			result = append(result, p)
		}
	}
	return result
}

func (m Model) renderTabBar() string {
	green := lipgloss.Color("42")
	dim := lipgloss.Color("245")
	active := lipgloss.NewStyle().Foreground(green).Bold(true).Render
	inactive := lipgloss.NewStyle().Foreground(dim).Render
	switch m.treeMode {
	case TreeModeWorktree:
		return active("[1] Worktree") + "  " + inactive("Branch") + "  " + inactive("PRs")
	case TreeModeWorktreeDiff:
		name := "Worktree"
		if m.selectedWorktree != "" {
			name = filepath.Base(m.selectedWorktree)
		}
		return active(name) + "  " + inactive("Branch") + "  " + inactive("PRs")
	case TreeModeStaged:
		return active("[1] Worktree") + "  " + inactive("Branch") + "  " + inactive("PRs")
	case TreeModeBranchDiff:
		name := "Branch"
		if m.branchSelector != nil && m.branchSelector.selectedBranch != "" {
			name = m.branchSelector.selectedBranch
		}
		return inactive("[1] Worktree") + "  " + active(name) + "  " + inactive("PRs")
	case TreeModeBranchSelector:
		return inactive("[1] Worktree") + "  " + active("Branch") + "  " + inactive("PRs")
	case TreeModePRSelector:
		return inactive("[1] Worktree") + "  " + inactive("Branch") + "  " + active("PRs")
	case TreeModePRDiff:
		name := "PRs"
		if m.prSelector != nil && m.prSelector.selectedPR != nil {
			name = fmt.Sprintf("#%d", m.prSelector.selectedPR.Number)
		}
		return inactive("[1] Worktree") + "  " + inactive("Branch") + "  " + active(name)
	default:
		return ""
	}
}

func (m Model) renderDiff(r Rect) string {
	title := "DIFF"
	if file, _, ok := m.tree.Selected(); ok {
		title = "DIFF / " + file.DisplayPath()
	}
	titleRendered := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245")).Render("[2] " + title)
	displayLines := []string{delta.Truncate(titleRendered, max(1, r.W-2))}
	wrapped := wrapContent(delta.Lines(m.diffText), max(1, r.W-4))
	visible := max(0, r.H-3)
	start := min(m.diffScroll, max(0, len(wrapped)))
	for i := start; i < len(wrapped) && i < start+visible; i++ {
		displayLines = append(displayLines, wrapped[i])
	}
	return box(r, strings.Join(padLines(displayLines, r.H-2), "\n"), m.focus == FocusDiff)
}

func (m Model) renderAnalysis(r Rect) string {
	green := lipgloss.Color("42")
	dim := lipgloss.Color("245")
	active := lipgloss.NewStyle().Foreground(green).Bold(true).Render
	inactive := lipgloss.NewStyle().Foreground(dim).Render
	tabNames := []string{"Detail", "Overall", "Request Log"}
	title := active("[3] " + tabNames[m.activeTab])
	for i, name := range tabNames {
		if i != int(m.activeTab) {
			title += "  " + inactive(name)
		}
	}
	displayLines := []string{delta.Truncate(title, max(1, r.W-2))}
	content := wrapContent(m.analysisLines(), max(1, r.W-4))
	start := min(m.analysisScroll, len(content))
	visible := max(0, r.H-3)
	for i := start; i < len(content) && i < start+visible; i++ {
		displayLines = append(displayLines, content[i])
	}
	return box(r, strings.Join(padLines(displayLines, r.H-2), "\n"), m.focus == FocusAnalysis)
}

func renderMarkdown(text string, width int) string {
	if text == "" || width < 10 {
		return text
	}
	text = strings.ReplaceAll(text, "\r", "")
	if glamourWidth != width {
		glamourOnce = sync.Once{}
	}
	var onceErr error
	glamourOnce.Do(func() {
		glamourRenderer, onceErr = glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(width),
		)
		glamourWidth = width
	})
	if onceErr != nil {
		return text
	}
	out, err := glamourRenderer.Render(text)
	if err != nil {
		return text
	}
	return out
}

func wrapContent(lines []string, maxW int) []string {
	if maxW < 1 {
		maxW = 1
	}
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.ReplaceAll(line, "\t", "    ")
		if ansi.StringWidth(line) > maxW {
			for _, seg := range strings.Split(ansi.Hardwrap(line, maxW, false), "\n") {
				wrapped = append(wrapped, delta.Truncate(seg, maxW))
			}
		} else {
			wrapped = append(wrapped, delta.Truncate(line, maxW))
		}
	}
	return wrapped
}

func (m Model) analysisLines() []string {
	key := activeResultKey(m)
	result := m.results[key]
	if result == nil {
		return []string{lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("Press a for overall or A for selected detail.")}
	}
	lines := make([]string, 0, 4)
	if result.Active {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Render("AGENT RESPONSE · STREAMING"))
	}
	if result.Stale {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("179")).Render("STALE · refresh and re-analyze for current diff"))
	}
	if result.Error != nil && result.Error != context.Canceled {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render("ERROR: "+result.Error.Error()))
	}
	text := result.Text
	if text != "" && !result.Active && !result.Stale {
		paneW := m.termW - 4
		if m.termW >= 80 {
			paneW = m.layout.Agent.W - 4
		}
		text = renderMarkdown(text, max(paneW, 10))
	}
	lines = append(lines, strings.Split(text, "\n")...)
	return lines
}

func activeResultKey(m Model) string {
	if !m.haveSnap {
		return ""
	}
	file, hunk, ok := m.tree.Selected()
	if !ok {
		return ""
	}
	if m.activeTab == RequestLogTab {
		return requestLogKey(m.snapshot.ID)
	}
	return resultKey(m.snapshot.ID, m.activeTab == DetailTab, file.ID, hunk)
}

func (m Model) statusLine() string {
	deltaState := "delta fallback"
	if m.diffStyled {
		deltaState = "delta active"
	}
	modeLabel := m.treeMode.String()
	if m.treeMode == TreeModeBranchDiff && m.branchSelector != nil {
		modeLabel = "branch diff: " + m.branchSelector.selectedBranch
	} else if m.treeMode == TreeModePRDiff && m.prSelector != nil && m.prSelector.selectedPR != nil {
		p := m.prSelector.selectedPR
		modeLabel = fmt.Sprintf("PR #%d: %s", p.Number, p.Title)
	} else if m.treeMode == TreeModePRSelector {
		modeLabel = "PR selector"
	}
	updateHint := ""
	if m.showUpdateModal || m.showUpdating {
		updateHint = ""
	} else if m.updateVersion != "" {
		updateHint = "  [u] update v" + m.updateVersion
	}
	return fmt.Sprintf("mode: %s  %s  %s  [1-3] pane  [space] check  [c] commit  [o] PR  [?] help  [q] quit%s  %s", modeLabel, deltaState, m.status, updateHint, version.Current)
}

func (m Model) helpText() string {
	section := func(name string) string {
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51")).Render("  " + name)
	}
	key := func(k, desc string) string {
		kStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("228")).Bold(true).Render(" " + k + " ")
		return kStyle + " " + desc
	}
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	lines := []string{
		section("Navigation"),
		key("1 / 2 / 3", "Focus files / diff / analysis pane"),
		key("tab", "Cycle focus forward"),
		key("j / k", "Navigate tree / scroll diff"),
		key("h / l", "Collapse / expand tree node"),
		key("g / G", "Scroll to top / bottom"),
		"",
		section("Staging"),
		key("space", "Toggle check for staging (working tree mode)"),
		key("ctrl+a", "Check / uncheck all"),
		key("c", "Stage checked items and commit"),
		key("e", "Open selected file in editor"),
		key("o", "Push and open pull request"),
		"",
		section("PR Review"),
		key("[ga]", "Approve PR (PR diff view)"),
		key("[gr]", "Request changes (PR diff view)"),
		key("[gm]", "Merge PR (PR diff view)"),
		key("[gd]", "Close PR + delete branch (PR diff view)"),
		key("o", "Open selected PR in browser"),
		key("r", "Refresh PR list / PR diff"),
		"",
		section("Analysis"),
		key("a / A", "Overall / detail review"),
		key("[/]", "Switch analysis tab"),
		key("x", "Cancel running analysis"),
		"",
		section("General"),
		key("r", "Refresh snapshot"),
		key("[/]", "Cycle left pane / analysis tab"),
		key("u", "Check for update"),
		key("? / esc", "Close this help"),
		key("q", "Quit"),
		"",
		dim.Render("  lazydiff " + version.Current),
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func box(r Rect, content string, focused bool) string {
	border := lipgloss.NormalBorder()
	color := lipgloss.Color("238")
	if focused {
		border = lipgloss.RoundedBorder()
		color = lipgloss.Color("63")
	}
	return lipgloss.NewStyle().Border(border).BorderForeground(color).Width(max(1, r.W-2)).Height(max(1, r.H-2)).Render(content)
}

func padLines(lines []string, height int) []string {
	if height < 1 {
		return []string{""}
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return lines
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// fileStatusGlyph returns the lazygit-style change letter and its color for a
// file's status. Binary files are un-diffable and render as a modification.
func fileStatusGlyph(status diff.FileStatus) (string, lipgloss.Color) {
	switch status {
	case diff.Added:
		return "A", lipgloss.Color("42")
	case diff.Deleted:
		return "D", lipgloss.Color("203")
	case diff.Renamed:
		return "R", lipgloss.Color("39")
	default:
		return "M", lipgloss.Color("214")
	}
}
