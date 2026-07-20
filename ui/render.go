package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/alex-irvine/lazydiff/delta"
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
	return lipgloss.JoinVertical(lipgloss.Left, body, status)
}

func (m Model) renderTree(r Rect) string {
	lines := []string{lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245")).Render("CHANGED FILES")}
	rows := m.tree.Rows()
	if len(rows) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("(no changes)"))
	}
	for _, row := range rows {
		id := row.ID()
		active := id == m.tree.selectedID
		prefix := "  "
		if active {
			prefix = "▶ "
		}
		name := ""
		if row.Hunk != nil {
			name = row.Hunk.Header
		} else if row.File != nil {
			name = row.File.DisplayPath()
		}
		if row.Depth > 0 {
			name = "  " + name
		}
		color := lipgloss.Color("245")
		if active {
			color = lipgloss.Color("51")
		} else if row.Hunk != nil {
			color = lipgloss.Color("179")
		}
		lines = append(lines, lipgloss.NewStyle().Foreground(color).Render(prefix+delta.Truncate(name, max(1, r.W-5))))
	}
	return box(r, strings.Join(padLines(lines, r.H-2), "\n"), m.focus == FocusTree)
}

func (m Model) renderDiff(r Rect) string {
	title := "DIFF"
	if file, _, ok := m.tree.Selected(); ok {
		title = "DIFF / " + file.DisplayPath()
	}
	lines := []string{lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245")).Render(truncate(title, max(1, r.W-4)))}
	content := delta.Lines(m.diffText)
	visible := max(0, r.H-3)
	start := min(m.diffScroll, max(0, len(content)))
	for i := start; i < len(content) && i < start+visible; i++ {
		lines = append(lines, delta.Truncate(content[i], max(1, r.W-4)))
	}
	return box(r, strings.Join(padLines(lines, r.H-2), "\n"), m.focus == FocusDiff)
}

func (m Model) renderAnalysis(r Rect) string {
	tabs := "detail   overall   request log"
	active := []string{"detail", "overall", "request log"}[m.activeTab]
	lines := []string{lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51")).Render(tabs + "  [" + active + "]")}
	content := m.analysisLines()
	start := min(m.analysisScroll, len(content))
	visible := max(0, r.H-3)
	for i := start; i < len(content) && i < start+visible; i++ {
		lines = append(lines, delta.Truncate(content[i], max(1, r.W-4)))
	}
	return box(r, strings.Join(padLines(lines, r.H-2), "\n"), m.focus == FocusAnalysis)
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
	return fmt.Sprintf("mode: %s  %s  %s  [tab] focus  [a] overall  [A] detail  [m] mode  [?] help  [q] quit", m.mode, deltaState, m.status)
}

func (m Model) helpText() string {
	return "lazydiff keys\n\n[tab] focus  [j/k] navigate  [space] expand  [a] overall  [A] detail\n[c] cancel  [m] mode  [r] refresh  [g/G] scroll  [?] close help  [q] quit"
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

func truncate(s string, width int) string {
	if len(s) <= width {
		return s
	}
	return s[:width]
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
