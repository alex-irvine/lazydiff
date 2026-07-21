package config

import (
	"os"
	"path/filepath"
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
	if cfg.Agent.Provider != "generic" || cfg.Agent.Command != "claude" || len(cfg.Agent.Args) != 2 || cfg.Agent.Args[0] != "--model" || cfg.Agent.Args[1] != "haiku-latest" || !cfg.Agent.ReadOnly {
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

func TestConfigPathUsesXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/lazydiff-xdg")
	if got, want := ConfigPath(), "/tmp/lazydiff-xdg/lazydiff/config.toml"; got != want {
		t.Fatalf("ConfigPath() = %q, want %q", got, want)
	}
}
