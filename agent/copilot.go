package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Copilot struct {
	Command            string
	Args               []string
	ReadOnly           bool
	AllowExternalTools bool
}

func NewCopilot(command string, args []string, readOnly, allowExternalTools bool) Copilot {
	return Copilot{Command: command, Args: append([]string(nil), args...), ReadOnly: readOnly, AllowExternalTools: allowExternalTools}
}

func (c Copilot) Run(ctx context.Context, request Request, emit func(Event)) error {
	tmp, err := os.CreateTemp("", "lazydiff-prompt-*.md")
	if err != nil {
		return fmt.Errorf("create Copilot prompt: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("restrict Copilot prompt: %w", err)
	}
	if _, err := tmp.WriteString(request.Prompt); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write Copilot prompt: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close Copilot prompt: %w", err)
	}

	args := append([]string(nil), c.Args...)
	args = append(args, "--output-format", "text", "--stream", "on", "--silent", "--no-color", "--no-ask-user")
	if !c.AllowExternalTools {
		args = append(args, "--disable-builtin-mcps", "--excluded-tools", "url")
	}
	if c.ReadOnly {
		args = append(args, "--excluded-tools", "write,shell")
	}
	instruction := fmt.Sprintf("Read %s. Inspect repository files only as needed. Do not modify files, run shell commands, access URLs, or use MCP tools. Return only the requested analysis.", filepath.ToSlash(tmpPath))
	args = append(args, "-p", instruction)
	return Generic{Command: c.Command, Args: args}.Run(ctx, Request{RepoRoot: request.RepoRoot}, emitWithSanitizedOutput(emit))
}

func emitWithSanitizedOutput(emit func(Event)) func(Event) {
	return func(event Event) {
		if event.Kind == Output || event.Kind == Diagnostic {
			event.Text = strings.TrimRight(sanitizeOutput(event.Text), "\r")
		}
		if emit != nil {
			emit(event)
		}
	}
}
