// pr/gh.go
package pr

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/alex-irvine/lazydiff/git"
)

// GitHub wraps the gh CLI for PR discovery and review actions. All methods
// require a github.com remote (validated per call); anything else errors.
type GitHub struct {
	Runner    git.CommandRunner
	remoteURL string
}

func NewGitHub(remoteURL string, runner git.CommandRunner) *GitHub {
	return &GitHub{Runner: runner, remoteURL: remoteURL}
}

func (g *GitHub) run(ctx context.Context, args ...string) ([]byte, error) {
	if g.Runner == nil {
		return nil, fmt.Errorf("gh runner unavailable")
	}
	return g.Runner.Run(ctx, "gh", args...)
}

func (g *GitHub) requireGitHub() error {
	host, _, _, err := ownerRepo(g.remoteURL)
	if err != nil {
		return fmt.Errorf("PR review requires a github.com remote: %w", err)
	}
	if host != "github.com" {
		return fmt.Errorf("remote host %q is not github.com; PR review requires github.com", host)
	}
	return nil
}

const prJSONFields = "number,title,author,headRefName,baseRefName,mergeable,url,createdAt"

func (g *GitHub) ListPRs(ctx context.Context, state string) ([]PR, error) {
	if err := g.requireGitHub(); err != nil {
		return nil, err
	}
	out, err := g.run(ctx, "pr", "list", "--state", state, "--json", prJSONFields)
	if err != nil {
		return nil, fmt.Errorf("gh pr list: %w", err)
	}
	var prs []PR
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("parse gh pr list: %w", err)
	}
	return prs, nil
}

func (g *GitHub) PR(ctx context.Context, number int) (PR, error) {
	if err := g.requireGitHub(); err != nil {
		return PR{}, err
	}
	out, err := g.run(ctx, "pr", "view", strconv.Itoa(number), "--json", prJSONFields)
	if err != nil {
		return PR{}, fmt.Errorf("gh pr view #%d: %w", number, err)
	}
	var p PR
	if err := json.Unmarshal(out, &p); err != nil {
		return PR{}, fmt.Errorf("parse gh pr view: %w", err)
	}
	return p, nil
}

func (g *GitHub) PRDiff(ctx context.Context, number int) (string, error) {
	if err := g.requireGitHub(); err != nil {
		return "", err
	}
	out, err := g.run(ctx, "pr", "diff", strconv.Itoa(number), "--patch")
	if err != nil {
		return "", fmt.Errorf("gh pr diff #%d: %w", number, err)
	}
	return string(out), nil
}

func (g *GitHub) Approve(ctx context.Context, number int, comment string) error {
	if err := g.requireGitHub(); err != nil {
		return err
	}
	args := []string{"pr", "review", strconv.Itoa(number), "--approve"}
	if comment != "" {
		args = append(args, "--body", comment)
	}
	if _, err := g.run(ctx, args...); err != nil {
		return fmt.Errorf("approve PR #%d: %w", number, err)
	}
	return nil
}

func (g *GitHub) RequestChanges(ctx context.Context, number int, body string) error {
	if err := g.requireGitHub(); err != nil {
		return err
	}
	if _, err := g.run(ctx, "pr", "review", strconv.Itoa(number), "--request-changes", "--body", body); err != nil {
		return fmt.Errorf("request changes on PR #%d: %w", number, err)
	}
	return nil
}

func (g *GitHub) Merge(ctx context.Context, number int) error {
	if err := g.requireGitHub(); err != nil {
		return err
	}
	if _, err := g.run(ctx, "pr", "merge", strconv.Itoa(number), "--merge"); err != nil {
		return fmt.Errorf("merge PR #%d: %w", number, err)
	}
	return nil
}

func (g *GitHub) Close(ctx context.Context, number int) error {
	if err := g.requireGitHub(); err != nil {
		return err
	}
	if _, err := g.run(ctx, "pr", "close", strconv.Itoa(number)); err != nil {
		return fmt.Errorf("close PR #%d: %w", number, err)
	}
	return nil
}

// DeleteBranch removes the PR's remote branch via `git push origin --delete`.
// Direct exec through the runner (not through git.Repository) per spec.
func (g *GitHub) DeleteBranch(ctx context.Context, branch string) error {
	if err := g.requireGitHub(); err != nil {
		return err
	}
	if _, err := g.Runner.Run(ctx, "git", "push", "origin", "--delete", branch); err != nil {
		return fmt.Errorf("delete remote branch %q: %w", branch, err)
	}
	return nil
}
