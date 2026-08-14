package git

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type CommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
	RunWithStdin(context.Context, io.Reader, string, ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}

func (ExecRunner) RunWithStdin(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = stdin
	return cmd.Output()
}

type Repository struct {
	Root   string
	runner CommandRunner
}

func Open(ctx context.Context, dir string) (Repository, error) {
	return OpenWithRunner(ctx, dir, ExecRunner{})
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
		r.runner = ExecRunner{}
	}
	return r.runner.Run(ctx, "git", append([]string{"-C", r.Root}, args...)...)
}

func (r Repository) runWithStdin(ctx context.Context, stdin io.Reader, args ...string) ([]byte, error) {
	if r.runner == nil {
		r.runner = ExecRunner{}
	}
	return r.runner.RunWithStdin(ctx, stdin, "git", append([]string{"-C", r.Root}, args...)...)
}

func (r Repository) Branches(ctx context.Context) ([]string, error) {
	output, err := r.run(ctx, "branch", "--format=%(refname:short)")
	if err != nil {
		return nil, fmt.Errorf("list branches: %w", err)
	}
	var branches []string
	for _, b := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		b = strings.TrimSpace(b)
		if b != "" {
			branches = append(branches, b)
		}
	}
	return branches, nil
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

func (r Repository) CurrentBranch(ctx context.Context) (string, error) {
	output, err := r.run(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("resolve current branch: %w", err)
	}
	branch := strings.TrimSpace(string(output))
	if branch == "" {
		return "", fmt.Errorf("current branch is empty")
	}
	return branch, nil
}

func (r Repository) RemoteURL(ctx context.Context, remote string) (string, error) {
	output, err := r.run(ctx, "remote", "get-url", remote)
	if err != nil {
		return "", fmt.Errorf("resolve remote %q url: %w", remote, err)
	}
	url := strings.TrimSpace(string(output))
	if url == "" {
		return "", fmt.Errorf("remote %q url is empty", remote)
	}
	return url, nil
}

func (r Repository) Worktrees(ctx context.Context) (map[string]string, error) {
	output, err := r.run(ctx, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("list worktrees: %w", err)
	}
	result := make(map[string]string)
	var currentPath string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "worktree ") {
			currentPath = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "branch ") {
			ref := strings.TrimPrefix(line, "branch ")
			// ref is like "refs/heads/feature" — extract short name
			branch := strings.TrimPrefix(ref, "refs/heads/")
			if branch != "" && currentPath != "" {
				result[branch] = currentPath
			}
		}
	}
	return result, nil
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
