package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRejectsInvalidRepository(t *testing.T) {
	t.Chdir(t.TempDir())
	err := run(context.Background(), []string{"lazydiff", "-config", filepath.Join(t.TempDir(), "missing.toml")}, strings.NewReader(""), os.Stdout, os.Stderr)
	if err == nil || !strings.Contains(err.Error(), "git repository") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunConfigPathIsAccepted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`[agent]
provider = "generic"
command = "cat"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path); err != nil {
		t.Fatal(err)
	}
}
