package ui

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/alex-irvine/lazydiff/delta"
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
	if m.dialog.Kind == PRDialog {
		title = "Pull Request"
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
	body.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render("ctrl+s confirm   esc cancel   ctrl+r regenerate"))

	return lipgloss.Place(m.termW, m.termH, lipgloss.Center, lipgloss.Center, lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Render(body.String()))
}

func (m Model) renderTree(r Rect) string {
	if m.treeMode == TreeModeBranchSelector && m.branchSelector != nil {
		return m.renderBranchSelector(r)
	}
	title := m.renderTabBar()
	titleRendered := delta.Truncate(title, max(1, r.W-2))
	lines := []string{titleRendered}
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

func (m Model) renderTabBar() string {
	green := lipgloss.Color("42")
	dim := lipgloss.Color("245")
	active := lipgloss.NewStyle().Foreground(green).Bold(true).Render
	inactive := lipgloss.NewStyle().Foreground(dim).Render
	switch m.treeMode {
	case TreeModeWorktree, TreeModeStaged:
		return active("Worktree") + "  " + inactive("Branch")
	case TreeModeBranchDiff:
		name := "Branch"
		if m.branchSelector != nil && m.branchSelector.selectedBranch != "" {
			name = m.branchSelector.selectedBranch
		}
		return inactive("Worktree") + "  " + active(name)
	case TreeModeBranchSelector:
		return inactive("Worktree") + "  " + active("Branch")
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
	tabNames := []string{"detail", "overall", "request log"}
	tabRendered := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51")).Render("[3] " + tabNames[m.activeTab])
	displayLines := []string{delta.Truncate(tabRendered, max(1, r.W-2))}
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
	}
	updateHint := ""
	if m.showUpdateModal || m.showUpdating {
		updateHint = ""
	} else if m.updateVersion != "" {
		updateHint = "  [u] update v" + m.updateVersion
	}
	return fmt.Sprintf("mode: %s  %s  %s  %s%s  %s", modeLabel, deltaState, m.status, "[1-3] pane  [space] check  [c] commit  [o] PR  [?] help  [q] quit", updateHint, version.Current)
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
		key("o", "Push and open pull request"),
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
