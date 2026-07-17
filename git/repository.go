package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type CommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}

type Repository struct {
	Root   string
	runner CommandRunner
}

func Open(ctx context.Context, dir string) (Repository, error) {
	return OpenWithRunner(ctx, dir, execRunner{})
}

func OpenWithRunner(ctx context.Context, dir string, runner CommandRunner) (Repository, error) {
	output, err := runner.Run(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return Repository{}, fmt.Errorf("git repository not found at %s: %w", dir, err)
	}
	root := strings.TrimSpace(string(output))
	if root == "" {
		return Repository{}, fmt.Errorf("git repository at %s returned empty root", dir)
	}
	return Repository{Root: root, runner: runner}, nil
}

func (r Repository) run(ctx context.Context, args ...string) ([]byte, error) {
	if r.runner == nil {
		r.runner = execRunner{}
	}
	return r.runner.Run(ctx, "git", append([]string{"-C", r.Root}, args...)...)
}

func (r Repository) DefaultBranch(ctx context.Context) (string, error) {
	if output, err := r.run(ctx, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil {
		if branch := strings.TrimSpace(string(output)); branch != "" {
			return branch, nil
		}
	}
	for _, candidate := range []string{"origin/main", "main", "master"} {
		if _, err := r.run(ctx, "rev-parse", "--verify", candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not resolve default branch")
}

func runOutput(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil && stderr.Len() > 0 {
		return output, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return output, err
}
