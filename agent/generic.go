package agent

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
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
	stdinErr := func() error {
		_, err := io.Copy(stdin, strings.NewReader(request.Prompt))
		if err != nil {
			_ = stdin.Close()
			_ = cmd.Process.Kill()
			return err
		}
		return stdin.Close()
	}()

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
	// EPIPE on stdin write is expected when the process exits before
	// consuming input (e.g. a script that ignores stdin). All output
	// was already captured above, so it's safe to ignore.
	if stdinErr != nil && waitErr == nil && errors.Is(stdinErr, syscall.EPIPE) {
		stdinErr = nil
	}
	if stdinErr != nil {
		return fmt.Errorf("write agent prompt: %w", stdinErr)
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
