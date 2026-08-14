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

func TestPTYCheckFileCommitFlow(t *testing.T) {
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
	_ = readUntil(t, terminal, "delta-output", 3*time.Second)
	if _, err := terminal.Write([]byte(" ")); err != nil { // check main.go (cursor starts there)
		t.Fatal(err)
	}
	if _, err := terminal.Write([]byte("c")); err != nil { // stage + generate commit message
		t.Fatal(err)
	}
	output := readUntil(t, terminal, "analysis-output", 5*time.Second)
	if !strings.Contains(output, "analysis-output") {
		t.Fatalf("commit dialog did not show the generated draft:\n%s", output)
	}
	if _, err := terminal.Write([]byte{19}); err != nil { // ctrl+s
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond) // let the commit actually land before quitting
	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	log := runOutput(t, fixture.root, "log", "-1", "--pretty=%B")
	if !strings.Contains(log, "analysis-output") {
		t.Fatalf("expected a new commit with the fake agent's output, git log = %q", log)
	}
}

func runOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func TestPTYOpenPRDialogAndCancel(t *testing.T) {
	fixture := newFixture(t)
	run(t, fixture.root, "git", "checkout", "-b", "feature/869d6rn69-thing")
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
	_ = readUntil(t, terminal, "delta-output", 3*time.Second)
	if _, err := terminal.Write([]byte("o")); err != nil {
		t.Fatal(err)
	}
	output := readUntil(t, terminal, "analysis-output", 5*time.Second)
	if !strings.Contains(output, "analysis-output") {
		t.Fatalf("pr dialog did not show the generated draft:\n%s", output)
	}
	if _, err := terminal.Write([]byte{27}); err != nil { // esc
		t.Fatal(err)
	}
	// A bare ESC immediately followed by another byte is ambiguous input
	// (standalone Escape vs. the start of an escape sequence / alt+key
	// combo); give the terminal input parser a moment to resolve it as a
	// standalone Escape before sending the next key.
	time.Sleep(100 * time.Millisecond)
	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("lazydiff exit: %v", err)
	}
}

func TestPTYBranchDiffModeSelectsBranchAndShowsDiff(t *testing.T) {
	fixture := newFixture(t)
	run(t, fixture.root, "git", "checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(fixture.root, "feat.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, fixture.root, "git", "add", "feat.txt")
	run(t, fixture.root, "git", "commit", "-m", "feature commit")
	run(t, fixture.root, "git", "checkout", "main")

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
	_ = readUntil(t, terminal, "delta-output", 3*time.Second)

	// worktree → branch selector
	if _, err := terminal.Write([]byte("]")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	// "feature" only appears in output once branch selector loads the branch list
	output := readUntil(t, terminal, "feature", 3*time.Second)
	if !strings.Contains(output, "main") {
		t.Fatalf("expected branch list:\n%s", output)
	}

	// move cursor to feature and select it
	if _, err := terminal.Write([]byte("j")); err != nil {
		t.Fatal(err)
	}
	if _, err := terminal.Write([]byte{13}); err != nil { // enter
		t.Fatal(err)
	}
	output = readUntil(t, terminal, "feat.txt", 3*time.Second)
	if !strings.Contains(output, "feat.txt") {
		t.Fatalf("expected diff of feat.txt:\n%s", output)
	}

	// esc returns to branch selector
	if _, err := terminal.Write([]byte{27}); err != nil { // esc
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	output = readUntil(t, terminal, "Branch", 3*time.Second)
	if !strings.Contains(output, "Branch") {
		t.Fatalf("expected Branch tab after esc:\n%s", output)
	}

	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("lazydiff exit: %v", err)
	}
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
	result := make(chan string)
	go func() {
		var output bytes.Buffer
		buf := make([]byte, 4096)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				output.Write(buf[:n])
				if strings.Contains(output.String(), marker) {
					result <- output.String()
					return
				}
			}
			if err != nil {
				result <- output.String()
				return
			}
		}
	}()
	select {
	case res := <-result:
		return res
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for %q", marker)
		return ""
	}
}

func TestPTYPRReviewApproveFlow(t *testing.T) {
	fixture := newFixture(t)
	run(t, fixture.root, "git", "remote", "add", "origin", "git@github.com:alex-irvine/lazydiff.git")
	ghLog := filepath.Join(fixture.root, "gh.log")
	gh := filepath.Join(filepath.Dir(fixture.binary), "gh")
	writeExecutable(t, gh, `#!/bin/sh
case "$1 $2" in
"pr list") echo '[{"number":42,"title":"feat: add login","author":{"login":"alex"},"headRefName":"feat-login","baseRefName":"main","mergeable":"MERGEABLE","url":"https://github.com/alex-irvine/lazydiff/pull/42","createdAt":"2026-07-01T00:00:00Z"}]';;
"pr view") echo '{"number":42,"title":"feat: add login","author":{"login":"alex"},"headRefName":"feat-login","baseRefName":"main","mergeable":"MERGEABLE","url":"https://github.com/alex-irvine/lazydiff/pull/42","createdAt":"2026-07-01T00:00:00Z"}';;
"pr diff") echo "pr diff $3" >> "$GH_LOG"; printf 'diff --git a/login.go b/login.go\nnew file mode 100644\n--- /dev/null\n+++ b/login.go\n@@ -0,0 +1 @@\n+func login() {}\n';;
"pr review") echo "$@" >> "$GH_LOG";;
esac
`)
	defer os.Remove(gh)

	cmd := exec.Command(fixture.binary, "-config", fixture.config)
	cmd.Dir = fixture.root
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(fixture.delta)+":"+os.Getenv("PATH"), "GH_LOG="+ghLog)
	terminal, err := pty.Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	if err := pty.Setsize(terminal, &pty.Winsize{Cols: 120, Rows: 40}); err != nil {
		t.Fatal(err)
	}
	_ = readUntil(t, terminal, "delta-output", 3*time.Second)

	// worktree → branch selector → PR selector
	if _, err := terminal.Write([]byte("]")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := terminal.Write([]byte("]")); err != nil {
		t.Fatal(err)
	}
	output := readUntil(t, terminal, "#42", 3*time.Second)
	if !strings.Contains(output, "feat: add login") {
		t.Fatalf("expected PR row:\n%s", output)
	}

	// select PR #42 → PR diff
	if _, err := terminal.Write([]byte{13}); err != nil { // enter
		t.Fatal(err)
	}
	output = readUntil(t, terminal, "login.go", 3*time.Second)
	if !strings.Contains(output, "login.go") {
		t.Fatalf("expected PR diff:\n%s", output)
	}

	// esc returns to PR selector
	if _, err := terminal.Write([]byte{27}); err != nil { // esc
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	output = readUntil(t, terminal, "#42", 3*time.Second)
	if !strings.Contains(output, "#42") {
		t.Fatalf("expected PR list after esc:\n%s", output)
	}
	// re-enter PR diff from cache
	if _, err := terminal.Write([]byte{13}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)

	// ga → confirm dialog → ctrl+s approve
	if _, err := terminal.Write([]byte("g")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if _, err := terminal.Write([]byte("a")); err != nil {
		t.Fatal(err)
	}
	output = readUntil(t, terminal, "Approve PR #42", 5*time.Second)
	if !strings.Contains(output, "ctrl+s confirm") {
		t.Fatalf("expected confirm hint:\n%s", output)
	}
	time.Sleep(200 * time.Millisecond)
	if _, err := terminal.Write([]byte{19}); err != nil { // ctrl+s
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)
	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("lazydiff exit: %v", err)
	}
	logData, err := os.ReadFile(ghLog)
	if err != nil {
		t.Fatalf("read gh log: %v", err)
	}
	if !strings.Contains(string(logData), "pr review 42 --approve") {
		t.Fatalf("expected approve invocation, gh log = %q", logData)
	}
	if got := strings.Count(string(logData), "pr diff 42"); got < 1 {
		t.Fatalf("gh pr diff not invoked, gh log = %q", logData)
	}
}
