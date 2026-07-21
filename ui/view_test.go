package ui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/alex-irvine/lazydiff/delta"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestBoxWithDeltaContent(t *testing.T) {
	rawDiff := "diff --git a/ui/render.go b/ui/render.go\nindex e11a12e..0809e9a 100644\n--- a/ui/render.go\n+++ b/ui/render.go\n@@ -96,28 +103,45 @@ func (m Model) renderDiff(r Rect) string {\n 	titleRendered := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(\"245\")).Render(title)\n-	lines := []string{delta.Truncate(titleRendered, max(1, r.W-2))}\n-	content := delta.Lines(m.diffText)\n+	displayLines := []string{delta.Truncate(titleRendered, max(1, r.W-2))}\n+	wrapped := wrapContent(delta.Lines(m.diffText), max(1, r.W-4))\n 	visible := max(0, r.H-3)\n-	start := min(m.diffScroll, max(0, len(content)))\n-	for i := start; i < len(content) && i < start+visible; i++ {\n-		lines = append(lines, delta.Truncate(content[i], max(1, r.W-4)))\n+	start := min(m.diffScroll, max(0, len(wrapped)))\n+	for i := start; i < len(wrapped) && i < start+visible; i++ {\n+		displayLines = append(displayLines, wrapped[i])\n 	}\n-	return box(r, strings.Join(padLines(lines, r.H-2), \"\\n\"), m.focus == FocusDiff)\n+	return box(r, strings.Join(padLines(displayLines, r.H-2), \"\\n\"), m.focus == FocusDiff)\n }\n"

	renderer := delta.Renderer{Command: "delta"}
	r := Rect{W: 87, H: 39}
	result := renderer.Render(context.Background(), rawDiff, r.W-2)
	
	wrapped := wrapContent(delta.Lines(result.Content), max(1, r.W-4))
	
	titleRendered := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245")).Render("DIFF / render.go")
	displayLines := []string{delta.Truncate(titleRendered, max(1, r.W-2))}
	visible := max(0, r.H-3)
	for i := 0; i < len(wrapped) && i < visible; i++ {
		displayLines = append(displayLines, wrapped[i])
	}
	padded := padLines(displayLines, r.H-2)
	
	// Test lipgloss padding on title (which is width-truncated to 85)
	titleRendered2 := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245")).Render("DIFF / render.go")
	truncated := delta.Truncate(titleRendered2, 85)
	fmt.Printf("Title truncated: sw=%d rawlen=%d\n", ansi.StringWidth(truncated), len(truncated))
	
	style := lipgloss.NewStyle().Width(85)
	rendered := style.Render(truncated)
	rlines := strings.Split(rendered, "\n")
	fmt.Printf("Title after Width(85): %d lines\n", len(rlines))
	for i, l := range rlines {
		fmt.Printf("  [%d] sw=%d\n", i, ansi.StringWidth(l))
	}
	
	// Test with a short line that shouldn't wrap
	shortLine := "test"
	rendered2 := style.Render(shortLine)
	rlines2 := strings.Split(rendered2, "\n")
	fmt.Printf("Short line after Width(85): %d lines\n", len(rlines2))
	
	// Check padded lines that might wrap with Width(85)
	for i, l := range padded {
		rendered3 := style.Render(l)
		rlines3 := strings.Split(rendered3, "\n")
		if len(rlines3) > 1 {
			fmt.Printf("  padded[%d] wraps to %d lines: sw=%d\n", i, len(rlines3), ansi.StringWidth(l))
			for j, rl := range rlines3 {
				fmt.Printf("    [%d] sw=%d len=%d\n", j, ansi.StringWidth(rl), len(rl))
			}
		}
	}
}
