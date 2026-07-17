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
}

type AgentConfig struct {
	Provider           string
	Command            string
	Args               []string
	ReadOnly           bool
	AllowExternalTools bool
	Prompts            PromptConfig
}

type PromptConfig struct {
	Overall string
	Detail  string
}

type fileConfig struct {
	Agent fileAgentConfig `toml:"agent"`
}

type fileAgentConfig struct {
	Provider           *string          `toml:"provider"`
	Command            *string          `toml:"command"`
	Args               *[]string        `toml:"args"`
	ReadOnly           *bool            `toml:"read_only"`
	AllowExternalTools *bool            `toml:"allow_external_tools"`
	Prompts            filePromptConfig `toml:"prompts"`
}

type filePromptConfig struct {
	Overall *string `toml:"overall"`
	Detail  *string `toml:"detail"`
}

var placeholderPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}`)

var allowedPlaceholders = map[string]struct{}{
	"repository":    {},
	"mode":          {},
	"overall_diff":  {},
	"selection":     {},
	"selected_diff": {},
}

func Default() Config {
	return Config{Agent: AgentConfig{
		Provider:           "copilot",
		Command:            "copilot",
		ReadOnly:           true,
		AllowExternalTools: false,
		Prompts: PromptConfig{
			Overall: defaultOverallPrompt,
			Detail:  defaultDetailPrompt,
		},
	}}
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
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.Agent.Provider != "copilot" && c.Agent.Provider != "generic" {
		return fmt.Errorf("agent provider %q is invalid; use copilot or generic", c.Agent.Provider)
	}
	if strings.TrimSpace(c.Agent.Command) == "" {
		return errors.New("agent command must not be empty")
	}
	if err := validateTemplate("overall", c.Agent.Prompts.Overall, "overall_diff"); err != nil {
		return err
	}
	if err := validateTemplate("detail", c.Agent.Prompts.Detail, "overall_diff", "selection", "selected_diff"); err != nil {
		return err
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

const defaultDetailPrompt = `You are explaining one Git change in read-only mode.

Repository: {{repository}}
Diff mode: {{mode}}
Selected target: {{selection}}

Overall diff:
{{overall_diff}}

Selected diff:
{{selected_diff}}

Explain why this file or hunk exists, how it relates to the wider change, and any risks or inconsistencies. Return concise Markdown. Do not modify files, run mutating commands, use network access, or use MCP tools.`
