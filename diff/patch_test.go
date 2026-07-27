package diff

import (
	"strings"
	"testing"
)

func TestBuildPatchWithSubsetOfHunks(t *testing.T) {
	files, err := Parse(fixture)
	if err != nil {
		t.Fatal(err)
	}
	file := files[0] // agent/runner.go, 2 hunks
	patch := BuildPatch(file, file.Hunks[1:])
	if strings.Contains(patch, `import "fmt"`) {
		t.Fatalf("patch should omit first hunk: %q", patch)
	}
	if !strings.Contains(patch, `fmt.Println("run")`) {
		t.Fatalf("patch should include second hunk: %q", patch)
	}
	if !strings.HasPrefix(patch, "diff --git a/agent/runner.go") {
		t.Fatalf("patch missing preamble: %q", patch)
	}
	if !strings.Contains(patch, "--- a/agent/runner.go") || !strings.Contains(patch, "+++ b/agent/runner.go") {
		t.Fatalf("patch missing file headers: %q", patch)
	}
}

func TestBuildPatchAllHunksReturnsFullRaw(t *testing.T) {
	files, err := Parse(fixture)
	if err != nil {
		t.Fatal(err)
	}
	file := files[0]
	if got := BuildPatch(file, file.Hunks); got != file.Raw {
		t.Fatal("expected full raw diff when all hunks selected")
	}
}

func TestBuildPatchNoHunksReturnsFullRaw(t *testing.T) {
	files, err := Parse(fixture)
	if err != nil {
		t.Fatal(err)
	}
	file := files[0]
	if got := BuildPatch(file, nil); got != file.Raw {
		t.Fatal("expected full raw diff when no hunks given")
	}
}
