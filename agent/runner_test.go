package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeAgent(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent.sh")
	script := "#!/bin/sh\nprintf 'out-one\\nout-two\\n'\nprintf 'diagnostic\\n' >&2\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGenericStreamsPromptOutputAndDiagnostics(t *testing.T) {
	command := fakeAgent(t, "exit 0")
	runner := NewGeneric(command, nil)
	var events []Event
	err := runner.Run(context.Background(), Request{RepoRoot: t.TempDir(), Prompt: "prompt-body"}, func(event Event) { events = append(events, event) })
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[0].Kind != Output || events[0].Text != "out-one" || events[2].Kind != Diagnostic {
		t.Fatalf("events = %+v", events)
	}
}

func TestGenericReturnsNonZeroAfterPreservingEvents(t *testing.T) {
	command := fakeAgent(t, "exit 7")
	runner := NewGeneric(command, nil)
	var events []Event
	err := runner.Run(context.Background(), Request{RepoRoot: t.TempDir(), Prompt: "prompt"}, func(event Event) { events = append(events, event) })
	if err == nil || !strings.Contains(err.Error(), "agent exited") || len(events) != 3 {
		t.Fatalf("err = %v, events = %+v", err, events)
	}
}

func TestGenericCancellation(t *testing.T) {
	command := fakeAgent(t, "while :; do :; done")
	runner := NewGeneric(command, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := runner.Run(ctx, Request{RepoRoot: t.TempDir(), Prompt: "prompt"}, nil)
	if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("err = %v", err)
	}
}

func TestCopilotCommandUsesTempPromptAndRestrictiveFlags(t *testing.T) {
	recordDir := t.TempDir()
	argvPath := filepath.Join(recordDir, "argv")
	command := filepath.Join(recordDir, "copilot.sh")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >\"$ARGV_FILE\"\nprintf 'ok\\n'"
	if err := os.WriteFile(command, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ARGV_FILE", argvPath)
	runner := NewCopilot(command, nil, true, false)
	err := runner.Run(context.Background(), Request{RepoRoot: recordDir, Prompt: "secret prompt"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(argvPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(args)
	for _, wanted := range []string{"--output-format", "text", "--stream", "on", "--silent", "--no-ask-user", "--disable-builtin-mcps", "write,shell", "-p"} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("argv missing %q: %s", wanted, text)
		}
	}
}
