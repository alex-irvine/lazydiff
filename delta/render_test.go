package delta

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func fakeDelta(t *testing.T, output string, exitCode int) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "delta")
	script := "#!/bin/sh\ncat >/dev/null\nprintf '" + output + "'\nexit " + strconv.Itoa(exitCode) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRenderUsesDeltaAndPreservesANSI(t *testing.T) {
	path := fakeDelta(t, `\033[32m+added\033[0m`, 0)
	renderer := Renderer{Command: path}
	result := renderer.Render(context.Background(), "raw diff", 80)
	if !result.Styled || result.Warning != nil {
		t.Fatalf("result = %+v", result)
	}
	if !strings.Contains(result.Content, "\x1b[32m") {
		t.Fatalf("ANSI output lost: %q", result.Content)
	}
}

func TestRenderFallsBackWhenDeltaFails(t *testing.T) {
	path := fakeDelta(t, "broken", 1)
	renderer := Renderer{Command: path}
	result := renderer.Render(context.Background(), "raw diff", 80)
	if result.Styled || result.Content != "raw diff" || result.Warning == nil {
		t.Fatalf("result = %+v", result)
	}
}

func TestRenderFallsBackWhenDeltaMissing(t *testing.T) {
	renderer := Renderer{Command: filepath.Join(t.TempDir(), "missing-delta")}
	result := renderer.Render(context.Background(), "raw diff", 80)
	if result.Styled || result.Content != "raw diff" || result.Warning == nil {
		t.Fatalf("result = %+v", result)
	}
}
