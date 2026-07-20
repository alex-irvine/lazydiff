//go:build linux

package integration

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

type ptyFixture struct {
	root   string
	binary string
	config string
	delta  string
	agent  string
}

func newFixture(t *testing.T) ptyFixture {
	t.Helper()
	root := t.TempDir()
	run(t, root, "git", "init", "-b", "main")
	run(t, root, "git", "config", "user.name", "PTY Test")
	run(t, root, "git", "config", "user.email", "pty@example.com")
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, root, "git", "add", "main.go")
	run(t, root, "git", "commit", "-m", "base")
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() { println(\"changed\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tools := t.TempDir()
	delta := filepath.Join(tools, "delta")
	writeExecutable(t, delta, "#!/bin/sh\ncat\nprintf '\\033[32m%s\\033[0m\\n' 'delta-output'\n")
	agent := filepath.Join(tools, "agent")
	writeExecutable(t, agent, "#!/bin/sh\ncat >/dev/null\nprintf 'analysis-output\\n'\n")
	config := filepath.Join(tools, "config.toml")
	if err := os.WriteFile(config, []byte(fmt.Sprintf(`[agent]
provider = "generic"
command = %q
args = []

[agent.prompts]
overall = "Overall {{overall_diff}}"
detail = "Detail {{overall_diff}} {{selection}} {{selected_diff}}"
`, agent)), 0o600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(tools, "lazydiff")
	build := exec.Command("go", "build", "-o", binary, "./cmd/lazydiff")
	build.Dir = filepath.Join("..")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build lazydiff: %v\n%s", err, output)
	}
	return ptyFixture{root: root, binary: binary, config: config, delta: delta, agent: agent}
}

func TestPTYStartupShowsReviewLayoutAndQuit(t *testing.T) {
	fixture := newFixture(t)
	cmd := exec.Command(fixture.binary, "-config", fixture.config)
	cmd.Dir = fixture.root
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(fixture.delta)+":"+os.Getenv("PATH"))
	terminal, err := pty.Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	if err := pty.Setsize(terminal, &pty.Winsize{Cols: 120, Rows: 40}); err != nil {
		t.Fatal(err)
	}
	output := readUntil(t, terminal, "delta-output", 3*time.Second)
	for _, marker := range []string{"delta-output"} {
		if !strings.Contains(output, marker) {
			t.Fatalf("output missing %q:\n%s", marker, output)
		}
	}
	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("lazydiff exit: %v", err)
	}
}

func TestPTYAnalysisStreamsAndNarrowLayout(t *testing.T) {
	fixture := newFixture(t)
	cmd := exec.Command(fixture.binary, "-config", fixture.config)
	cmd.Dir = fixture.root
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(fixture.delta)+":"+os.Getenv("PATH"))
	terminal, err := pty.Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	if err := pty.Setsize(terminal, &pty.Winsize{Cols: 70, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	_ = readUntil(t, terminal, "DIFF", 3*time.Second)
	if _, err := terminal.Write([]byte("A")); err != nil {
		t.Fatal(err)
	}
	output := readUntil(t, terminal, "analysis-output", 3*time.Second)
	if !strings.Contains(output, "analysis-output") {
		t.Fatalf("analysis output missing:\n%s", output)
	}
	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
}

func run(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, output)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func readUntil(t *testing.T, reader *os.File, marker string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var output bytes.Buffer
	buf := make([]byte, 4096)
	for time.Now().Before(deadline) {
		_ = reader.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, err := reader.Read(buf)
		if n > 0 {
			output.Write(buf[:n])
			if strings.Contains(output.String(), marker) {
				return output.String()
			}
		}
		if err != nil {
			continue
		}
	}
	t.Fatalf("timed out waiting for %q:\n%s", marker, output.String())
	return output.String()
}
