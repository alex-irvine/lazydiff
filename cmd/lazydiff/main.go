package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/alex-irvine/lazydiff/agent"
	"github.com/alex-irvine/lazydiff/config"
	"github.com/alex-irvine/lazydiff/delta"
	"github.com/alex-irvine/lazydiff/diff"
	"github.com/alex-irvine/lazydiff/git"
	"github.com/alex-irvine/lazydiff/pr"
	"github.com/alex-irvine/lazydiff/prompt"
	"github.com/alex-irvine/lazydiff/ui"
	"github.com/alex-irvine/lazydiff/version"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if err := run(context.Background(), os.Args, os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	for _, arg := range args[1:] {
		if arg == "--version" || arg == "-version" {
			_, err := fmt.Fprintf(stdout, "lazydiff %s\n", version.Current)
			return err
		}
	}
	flags := flag.NewFlagSet("lazydiff", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", config.ConfigPath(), "TOML config path")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	return runApp(ctx, *configPath, stdin, stdout, stderr)
}

func loadConfig(path string) (config.Config, error) {
	return config.Load(path)
}

func runApp(ctx context.Context, configPath string, _ io.Reader, _, _ io.Writer) error {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}
	templates, err := prompt.Parse(prompt.Sources{
		Overall:       cfg.Agent.Prompts.Overall,
		Detail:        cfg.Agent.Prompts.Detail,
		CommitMessage: cfg.Agent.Prompts.CommitMessage,
		PRDescription: cfg.Agent.Prompts.PRDescription,
	})
	if err != nil {
		return err
	}
	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current directory: %w", err)
	}
	repo, err := git.Open(ctx, root)
	if err != nil {
		return err
	}
	remoteURL, _ := repo.RemoteURL(ctx, "origin")
	ghReviewer := pr.NewGitHub(remoteURL, git.ExecRunner{})
	loader := repositoryLoader{repo: repo, gh: ghReviewer}
	var runner agent.Runner
	switch cfg.Agent.Provider {
	case "copilot":
		runner = agent.NewCopilot(cfg.Agent.Command, cfg.Agent.Args, cfg.Agent.ReadOnly, cfg.Agent.AllowExternalTools)
	case "opencode":
		runner = agent.NewOpenCode(cfg.Agent.Command, cfg.Agent.Args, cfg.Agent.ReadOnly, cfg.Agent.AllowExternalTools)
	case "generic", "claude":
		runner = agent.NewGeneric(cfg.Agent.Command, cfg.Agent.Args)
	default:
		return fmt.Errorf("unsupported agent provider %q", cfg.Agent.Provider)
	}
	model := ui.NewTeaModel(ui.NewModel(repo, cfg, loader, delta.Renderer{Command: "delta"}, runner, templates, repo, pr.NewOpener(), ghReviewer))
	program := tea.NewProgram(model, tea.WithAltScreen())
	model.SetSend(program.Send)
	_, err = program.Run()
	if errors.Is(ctx.Err(), context.Canceled) {
		return nil
	}
	return err
}

type repositoryLoader struct {
	repo git.Repository
	gh   *pr.GitHub
}

func (l repositoryLoader) Snapshot(ctx context.Context, mode git.Mode) (git.Snapshot, error) {
	return l.repo.Snapshot(ctx, mode)
}

func (l repositoryLoader) SnapshotBranch(ctx context.Context, branch string) (git.Snapshot, error) {
	return l.repo.SnapshotBranch(ctx, branch)
}

func (l repositoryLoader) SnapshotPR(ctx context.Context, number int) (git.Snapshot, error) {
	if l.gh == nil {
		return git.Snapshot{}, fmt.Errorf("gh reviewer not configured")
	}
	p, err := l.gh.PR(ctx, number)
	if err != nil {
		return git.Snapshot{}, err
	}
	raw, err := l.gh.PRDiff(ctx, number)
	if err != nil {
		return git.Snapshot{}, err
	}
	files, err := diff.Parse(raw)
	if err != nil {
		return git.Snapshot{}, err
	}
	base := p.BaseRefName + "..." + p.HeadRefName
	// Content-hashed, matching git.Repository.Snapshot/SnapshotBranch — a
	// PR's CreatedAt never changes as new commits land, so hashing on that
	// (as before) would never mark cached AI analysis stale after an update.
	hash := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s", git.Branch, base, raw)))
	return git.Snapshot{
		ID:      fmt.Sprintf("%x", hash[:]),
		Mode:    git.Branch,
		Base:    base,
		RawDiff: raw,
		Files:   files,
	}, nil
}
