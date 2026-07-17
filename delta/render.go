package delta

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type Result struct {
	Content string
	Styled  bool
	Warning error
}

type Renderer struct {
	Command string
}

func (r Renderer) Render(ctx context.Context, raw string, width int) Result {
	if width < 20 {
		width = 20
	}
	command := r.Command
	if command == "" {
		command = "delta"
	}
	cmd := exec.CommandContext(ctx, command, "--paging=never", "--color-only", "--width="+strconv.Itoa(width))
	cmd.Stdin = strings.NewReader(raw)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return Result{Content: raw, Warning: fmt.Errorf("delta presentation failed: %s", message)}
	}
	return Result{Content: string(output), Styled: true}
}

func VisibleWidth(s string) int { return ansi.StringWidth(s) }

func Truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(s, width, "")
}

func Lines(s string) []string { return strings.Split(s, "\n") }
