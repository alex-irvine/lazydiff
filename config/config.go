package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	Agent AgentConfig
	PR    PRConfig
}

type AgentConfig struct {
	Provider           string
	Command            string
	Args               []string
	Model              string
	ReadOnly           bool
	AllowExternalTools bool
	Prompts            PromptConfig
}

type PromptConfig struct {
	Overall       string
	Detail        string
	CommitMessage string
	PRDescription string
}

type PRConfig struct {
	TicketPattern string
}

type fileConfig struct {
	Agent fileAgentConfig `toml:"agent"`
	PR    filePRConfig    `toml:"pr"`
}

type fileAgentConfig struct {
	Provider           *string          `toml:"provider"`
	Command            *string          `toml:"command"`
	Args               *[]string        `toml:"args"`
	Model              *string          `toml:"model"`
	ReadOnly           *bool            `toml:"read_only"`
	AllowExternalTools *bool            `toml:"allow_external_tools"`
	Prompts            filePromptConfig `toml:"prompts"`
}

type filePromptConfig struct {
	Overall       *string `toml:"overall"`
	Detail        *string `toml:"detail"`
	CommitMessage *string `toml:"commit_message"`
	PRDescription *string `toml:"pr_description"`
}

type filePRConfig struct {
	TicketPattern *string `toml:"ticket_pattern"`
}

var placeholderPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}`)

var allowedPlaceholders = map[string]struct{}{
	"repository":     {},
	"mode":           {},
	"overall_diff":   {},
	"change_context": {},
	"selection":      {},
	"selected_diff":  {},
	"staged_diff":    {},
	"branch_diff":    {},
	"ticket":         {},
	"branch":         {},
	"base_branch":    {},
}

func Default() Config {
	return Config{
		Agent: AgentConfig{
			Provider:           "opencode",
			Command:            "opencode",
			Args:               []string{"run"},
			ReadOnly:           true,
			AllowExternalTools: false,
			Prompts: PromptConfig{
				Overall:       defaultOverallPrompt,
				Detail:        defaultDetailPrompt,
				CommitMessage: defaultCommitMessagePrompt,
				PRDescription: defaultPRDescriptionPrompt,
			},
		},
		PR: PRConfig{TicketPattern: defaultTicketPattern},
	}
}

func ConfigPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(".config", "lazydiff", "config.toml")
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "lazydiff", "config.toml")
}

func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}

	var file fileConfig
	if err := toml.Unmarshal(data, &file); err != nil {
		return Config{}, fmt.Errorf("decode config %s: %w", path, err)
	}
	overlay := file.Agent
	if overlay.Provider != nil {
		cfg.Agent.Provider = *overlay.Provider
	}
	if overlay.Command != nil {
		cfg.Agent.Command = *overlay.Command
	}
	if overlay.Args != nil {
		cfg.Agent.Args = append([]string(nil), (*overlay.Args)...)
	}
	if overlay.Model != nil {
		cfg.Agent.Model = *overlay.Model
	}
	if overlay.ReadOnly != nil {
		cfg.Agent.ReadOnly = *overlay.ReadOnly
	}
	if overlay.AllowExternalTools != nil {
		cfg.Agent.AllowExternalTools = *overlay.AllowExternalTools
	}
	if overlay.Prompts.Overall != nil {
		cfg.Agent.Prompts.Overall = *overlay.Prompts.Overall
	}
	if overlay.Prompts.Detail != nil {
		cfg.Agent.Prompts.Detail = *overlay.Prompts.Detail
	}
	if overlay.Prompts.CommitMessage != nil {
		cfg.Agent.Prompts.CommitMessage = *overlay.Prompts.CommitMessage
	}
	if overlay.Prompts.PRDescription != nil {
		cfg.Agent.Prompts.PRDescription = *overlay.Prompts.PRDescription
	}
	if file.PR.TicketPattern != nil {
		cfg.PR.TicketPattern = *file.PR.TicketPattern
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.Agent.Provider != "copilot" && c.Agent.Provider != "generic" && c.Agent.Provider != "claude" && c.Agent.Provider != "opencode" {
		return fmt.Errorf("agent provider %q is invalid; use copilot, generic, claude, or opencode", c.Agent.Provider)
	}
	if strings.TrimSpace(c.Agent.Command) == "" {
		return errors.New("agent command must not be empty")
	}
	if err := validateTemplate("overall", c.Agent.Prompts.Overall, "overall_diff"); err != nil {
		return err
	}
	if err := validateTemplate("detail", c.Agent.Prompts.Detail, "selection", "selected_diff"); err != nil {
		return err
	}
	if err := validateTemplate("commit_message", c.Agent.Prompts.CommitMessage, "staged_diff"); err != nil {
		return err
	}
	if err := validateTemplate("pr_description", c.Agent.Prompts.PRDescription, "branch_diff"); err != nil {
		return err
	}
	if _, err := regexp.Compile(c.PR.TicketPattern); err != nil {
		return fmt.Errorf("pr.ticket_pattern is invalid: %w", err)
	}
	return nil
}

func validateTemplate(name, source string, required ...string) error {
	if strings.TrimSpace(source) == "" {
		return fmt.Errorf("agent prompts.%s must not be empty", name)
	}
	for _, match := range placeholderPattern.FindAllStringSubmatch(source, -1) {
		if _, ok := allowedPlaceholders[match[1]]; !ok {
			return fmt.Errorf("agent prompts.%s contains unknown placeholder %q", name, match[1])
		}
	}
	if strings.Contains(source, "{{") || strings.Contains(source, "}}") {
		funcs := template.FuncMap{}
		for placeholder := range allowedPlaceholders {
			funcs[placeholder] = func() string { return "" }
		}
		if _, err := template.New(name).Funcs(funcs).Parse(source); err != nil {
			return fmt.Errorf("agent prompts.%s is malformed: %w", name, err)
		}
	}
	for _, placeholder := range required {
		found := false
		for _, match := range placeholderPattern.FindAllStringSubmatch(source, -1) {
			if match[1] == placeholder {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("agent prompts.%s must include {{%s}}", name, placeholder)
		}
	}
	return nil
}

const defaultOverallPrompt = `You are reviewing a Git change in read-only mode.

Repository: {{repository}}
Diff mode: {{mode}}

Overall diff:
{{overall_diff}}

Explain the purpose of this change, its architecture impact, risks, and likely testing gaps. Return concise Markdown. Do not modify files, run mutating commands, use network access, or use MCP tools.`

const defaultDetailPrompt = `You are explaining one file of a larger Git change, in read-only mode.

Repository: {{repository}}
Diff mode: {{mode}}
File in view: {{selection}}

The wider change this file belongs to, as background — read it to understand the intent, but do not review or explain any file other than {{selection}}:
{{change_context}}

Diff for {{selection}}:
{{selected_diff}}

Explain the change to {{selection}} and nothing else. Use exactly these Markdown headings, in this order:

## What changed
Two or three sentences on what this file now does differently.

## Why
The reason for this change, inferred from the code itself. Prefer the concrete motivation over a restatement of the diff.

## How it fits
How this file connects to the rest of the wider change above: what it enables, what depends on it, what would be incomplete without it. Name the specific other files or functions it works with.

Only if something in this file contradicts the wider change, add a final "## Watch out" section of at most two sentences. Otherwise omit it entirely.

Do not review the other files, do not produce a code review, and do not list risks, test gaps, style notes, or suggestions. Keep the whole answer under 250 words. Return Markdown. Do not modify files, run mutating commands, use network access, or use MCP tools.`

const defaultTicketPattern = `(?:^|[-/_])([0-9a-z]{6,10})(?:[-_]|$)`

const defaultCommitMessagePrompt = `You are writing a Git commit message in read-only mode; you do not run any commands.

Repository: {{repository}}

Staged diff:
{{staged_diff}}

Write a concise commit message: a short subject line (50 characters or fewer, no trailing period), a blank line, then a body explaining what changed and why. Plain text only — no markdown formatting, no code fences, no bullet symbols, no headers. Return only the commit message text, nothing else.`

const defaultPRDescriptionPrompt = `You are writing a GitHub pull request title and description in read-only mode; you do not run any commands.

Repository: {{repository}}
Branch: {{branch}}
Base branch: {{base_branch}}

Branch diff:
{{branch_diff}}

Write a concise PR title as the first line (no prefix, no trailing period), then a blank line, then a free-form Markdown description of what changed and why. Return only the title and description text, nothing else.`
