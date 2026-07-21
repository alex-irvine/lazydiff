package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/alex-irvine/lazydiff/delta"
	"github.com/alex-irvine/lazydiff/version"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) View() string {
	if m.termW == 0 {
		return "loading..."
	}
	if m.showHelp {
		return lipgloss.Place(m.termW, m.termH, lipgloss.Center, lipgloss.Center, lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Render(m.helpText()))
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

func (m Model) renderTree(r Rect) string {
	title := delta.Truncate(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245")).Render("CHANGED FILES"), max(1, r.W-2))
	lines := []string{title}
	nodes := m.tree.Rows()
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
		fullLine := prefix + indent + icon + node.Label
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

func (m Model) renderDiff(r Rect) string {
	title := "DIFF"
	if file, _, ok := m.tree.Selected(); ok {
		title = "DIFF / " + file.DisplayPath()
	}
	titleRendered := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245")).Render(title)
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
	tabs := "detail   overall   request log"
	active := []string{"detail", "overall", "request log"}[m.activeTab]
	tabRendered := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51")).Render(tabs + "  [" + active + "]")
	displayLines := []string{delta.Truncate(tabRendered, max(1, r.W-2))}
	content := wrapContent(m.analysisLines(), max(1, r.W-4))
	start := min(m.analysisScroll, len(content))
	visible := max(0, r.H-3)
	for i := start; i < len(content) && i < start+visible; i++ {
		displayLines = append(displayLines, content[i])
	}
	return box(r, strings.Join(padLines(displayLines, r.H-2), "\n"), m.focus == FocusAnalysis)
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
	lines = append(lines, strings.Split(ansi.Strip(result.Text), "\n")...)
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
	return fmt.Sprintf("mode: %s  %s  %s  %s  %s", m.mode, deltaState, m.status, "[tab] focus  [a] overall  [A] detail  [m] mode  [?] help  [q] quit", version.Current)
}

func (m Model) helpText() string {
	return "lazydiff keys\n\n[tab] focus  [j/k] navigate  [space] toggle expand  [h] collapse/parent  [l] expand/descend\n[a] overall  [A] detail  [c] cancel  [m] mode  [r] refresh  [g/G] scroll  [?] close help  [q] quit"
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
