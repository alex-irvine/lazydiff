package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agent.Provider != "opencode" || cfg.Agent.Command != "opencode" || len(cfg.Agent.Args) != 1 || cfg.Agent.Args[0] != "run" || !cfg.Agent.ReadOnly {
		t.Fatalf("unexpected defaults: %+v", cfg.Agent)
	}
	if cfg.Agent.AllowExternalTools {
		t.Fatal("external tools enabled by default")
	}
	if !strings.Contains(cfg.Agent.Prompts.Overall, "{{overall_diff}}") {
		t.Fatal("default overall prompt missing diff placeholder")
	}
}

func TestLoadDecodesAndMergesConfig(t *testing.T) {
	path := writeConfig(t, `[agent]
provider = "generic"
command = "review-agent"
args = ["--plain"]
read_only = false

[agent.prompts]
overall = "Repo {{repository}}\n{{overall_diff}}"
detail = "Target {{selection}}\n{{overall_diff}}\n{{selected_diff}}"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agent.Provider != "generic" || cfg.Agent.Command != "review-agent" || len(cfg.Agent.Args) != 1 {
		t.Fatalf("unexpected agent config: %+v", cfg.Agent)
	}
	if cfg.Agent.ReadOnly {
		t.Fatal("explicit read_only=false was not preserved")
	}
}

func TestLoadRejectsUnknownProvider(t *testing.T) {
	path := writeConfig(t, `[agent]
provider = "unknown"
command = "agent"
`)
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "provider") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRejectsMissingCommand(t *testing.T) {
	path := writeConfig(t, `[agent]
provider = "generic"
command = ""
`)
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "command") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRejectsUnknownPlaceholder(t *testing.T) {
	path := writeConfig(t, `[agent.prompts]
overall = "{{unknown}}"
detail = "{{overall_diff}} {{selection}} {{selected_diff}}"
`)
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "placeholder") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadDefaultsIncludePRTicketPattern(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatal(err)
	}
	re, err := regexp.Compile(cfg.PR.TicketPattern)
	if err != nil {
		t.Fatalf("default ticket_pattern does not compile: %v", err)
	}
	match := re.FindStringSubmatch("feature/869d6rn69-add-login")
	if len(match) != 2 || match[1] != "869d6rn69" {
		t.Fatalf("default ticket_pattern match = %v", match)
	}
	if !strings.Contains(cfg.Agent.Prompts.CommitMessage, "{{staged_diff}}") {
		t.Fatal("default commit_message prompt missing staged_diff placeholder")
	}
	if !strings.Contains(cfg.Agent.Prompts.PRDescription, "{{branch_diff}}") {
		t.Fatal("default pr_description prompt missing branch_diff placeholder")
	}
}

func TestLoadOverlaysPRSection(t *testing.T) {
	path := writeConfig(t, `[pr]
ticket_pattern = "[A-Z]+-\\d+"

[agent.prompts]
commit_message = "{{staged_diff}}"
pr_description = "{{branch_diff}}"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PR.TicketPattern != `[A-Z]+-\d+` {
		t.Fatalf("ticket_pattern = %q", cfg.PR.TicketPattern)
	}
	if cfg.Agent.Prompts.CommitMessage != "{{staged_diff}}" || cfg.Agent.Prompts.PRDescription != "{{branch_diff}}" {
		t.Fatalf("prompts = %+v", cfg.Agent.Prompts)
	}
}

func TestLoadRejectsInvalidTicketPattern(t *testing.T) {
	path := writeConfig(t, `[pr]
ticket_pattern = "("
`)
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "ticket_pattern") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRejectsCommitMessageMissingRequiredPlaceholder(t *testing.T) {
	path := writeConfig(t, `[agent.prompts]
commit_message = "no placeholder here"
`)
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "staged_diff") {
		t.Fatalf("error = %v", err)
	}
}

func TestConfigPathUsesXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/lazydiff-xdg")
	if got, want := ConfigPath(), "/tmp/lazydiff-xdg/lazydiff/config.toml"; got != want {
		t.Fatalf("ConfigPath() = %q, want %q", got, want)
	}
}

func TestLoadDecodesModelField(t *testing.T) {
	path := writeConfig(t, `[agent]
provider = "opencode"
model = "anthropic/claude-sonnet-4-20250514"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agent.Model != "anthropic/claude-sonnet-4-20250514" {
		t.Fatalf("model = %q, want %q", cfg.Agent.Model, "anthropic/claude-sonnet-4-20250514")
	}
}

func TestLoadDefaultModelIsEmpty(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agent.Model != "" {
		t.Fatalf("default model = %q, want empty", cfg.Agent.Model)
	}
}
