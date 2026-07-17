package agent

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

type Generic struct {
	Command string
	Args    []string
}

func NewGeneric(command string, args []string) Generic {
	return Generic{Command: command, Args: append([]string(nil), args...)}
}

func (g Generic) Run(ctx context.Context, request Request, emit func(Event)) error {
	cmd := exec.CommandContext(ctx, g.Command, g.Args...)
	cmd.Dir = request.RepoRoot
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("open agent stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open agent stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("open agent stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start agent: %w", err)
	}
	if _, err := io.Copy(stdin, strings.NewReader(request.Prompt)); err != nil {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		return fmt.Errorf("write agent prompt: %w", err)
	}
	if err := stdin.Close(); err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("close agent stdin: %w", err)
	}

	var wg sync.WaitGroup
	var stdoutErr, stderrErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		stdoutErr = scanLines(stdout, Output, emit)
	}()
	go func() {
		defer wg.Done()
		stderrErr = scanLines(stderr, Diagnostic, emit)
	}()
	wg.Wait()
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if stdoutErr != nil {
		return fmt.Errorf("read agent output: %w", stdoutErr)
	}
	if stderrErr != nil {
		return fmt.Errorf("read agent diagnostics: %w", stderrErr)
	}
	if waitErr != nil {
		return fmt.Errorf("agent exited: %w", waitErr)
	}
	return nil
}

func scanLines(reader io.Reader, kind EventKind, emit func(Event)) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		if emit != nil {
			emit(Event{Kind: kind, Text: scanner.Text()})
		}
	}
	return scanner.Err()
}

func sanitizeOutput(text string) string {
	return string(bytes.Map(func(r rune) rune {
		if r == '\x1b' {
			return -1
		}
		return r
	}, []byte(text)))
}
