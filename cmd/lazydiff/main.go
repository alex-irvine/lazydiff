package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/alex-irvine/lazydiff/agent"
	"github.com/alex-irvine/lazydiff/config"
	"github.com/alex-irvine/lazydiff/delta"
	"github.com/alex-irvine/lazydiff/git"
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
	templates, err := prompt.Parse(cfg.Agent.Prompts.Overall, cfg.Agent.Prompts.Detail)
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
	loader := repositoryLoader{repo: repo}
	var runner agent.Runner
	switch cfg.Agent.Provider {
	case "copilot":
		runner = agent.NewCopilot(cfg.Agent.Command, cfg.Agent.Args, cfg.Agent.ReadOnly, cfg.Agent.AllowExternalTools)
	case "generic", "claude":
		runner = agent.NewGeneric(cfg.Agent.Command, cfg.Agent.Args)
	default:
		return fmt.Errorf("unsupported agent provider %q", cfg.Agent.Provider)
	}
	model := ui.NewTeaModel(ui.NewModel(repo, cfg, loader, delta.Renderer{Command: "delta"}, runner, templates))
	program := tea.NewProgram(model, tea.WithAltScreen())
	model.SetSend(program.Send)
	_, err = program.Run()
	if errors.Is(ctx.Err(), context.Canceled) {
		return nil
	}
	return err
}

type repositoryLoader struct{ repo git.Repository }

func (l repositoryLoader) Snapshot(ctx context.Context, mode git.Mode) (git.Snapshot, error) {
	return l.repo.Snapshot(ctx, mode)
}
