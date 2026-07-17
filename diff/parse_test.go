package diff

import (
	"strings"
	"testing"
)

const fixture = `diff --git a/agent/runner.go b/agent/runner.go
index 1111111..2222222 100644
--- a/agent/runner.go
+++ b/agent/runner.go
@@ -1,3 +1,4 @@
 package agent
+import "fmt"
 
 type Agent struct{}
@@ -10,2 +11,3 @@ func run()
 func run() {
-	return
+	fmt.Println("run")
+}
\ No newline at end of file
diff --git a/config/new.go b/config/new.go
new file mode 100644
index 0000000..3333333
--- /dev/null
+++ b/config/new.go
@@ -0,0 +1,2 @@
+package config
+
diff --git a/old.go b/old.go
deleted file mode 100644
index 4444444..0000000
--- a/old.go
+++ /dev/null
@@ -1 +0,0 @@
-old
diff --git a/oldname.go b/newname.go
similarity index 95%
rename from oldname.go
rename to newname.go
diff --git a/image.png b/image.png
index 5555555..6666666 binary
Binary files a/image.png and b/image.png differ
`

func TestParseFilesAndHunks(t *testing.T) {
	files, err := Parse(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 5 {
		t.Fatalf("got %d files, want 5", len(files))
	}
	if files[0].Path != "agent/runner.go" || files[0].Status != Modified {
		t.Fatalf("first file = %+v", files[0])
	}
	if len(files[0].Hunks) != 2 {
		t.Fatalf("got %d hunks, want 2", len(files[0].Hunks))
	}
	if files[0].Hunks[0].OldStart != 1 || files[0].Hunks[0].OldCount != 3 || files[0].Hunks[0].NewStart != 1 || files[0].Hunks[0].NewCount != 4 {
		t.Fatalf("first hunk = %+v", files[0].Hunks[0])
	}
	if !strings.Contains(files[0].Hunks[1].Raw, `\ No newline at end of file`) {
		t.Fatal("second hunk omitted no-newline marker")
	}
	if files[1].Status != Added || files[1].Path != "config/new.go" || len(files[1].Hunks) != 1 {
		t.Fatalf("added file = %+v", files[1])
	}
	if files[2].Status != Deleted || files[2].Path != "old.go" {
		t.Fatalf("deleted file = %+v", files[2])
	}
	if files[3].Status != Renamed || files[3].OldPath != "oldname.go" || files[3].Path != "newname.go" {
		t.Fatalf("renamed file = %+v", files[3])
	}
	if files[4].Status != Binary || len(files[4].Hunks) != 0 {
		t.Fatalf("binary file = %+v", files[4])
	}
}

func TestParsePreservesRawSlices(t *testing.T) {
	files, err := Parse(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(files[0].Raw, "diff --git a/agent/runner.go") {
		t.Fatalf("raw file = %q", files[0].Raw)
	}
	if !strings.HasPrefix(files[0].Hunks[0].Raw, "@@ -1,3 +1,4 @@") {
		t.Fatalf("raw hunk = %q", files[0].Hunks[0].Raw)
	}
	if !strings.Contains(files[0].Raw, files[0].Hunks[1].Raw) {
		t.Fatal("file raw does not contain hunk raw")
	}
}

func TestParseStableIDs(t *testing.T) {
	first, err := Parse(fixture)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Parse(fixture)
	if err != nil {
		t.Fatal(err)
	}
	for i := range first {
		if first[i].ID == "" || first[i].ID != second[i].ID {
			t.Fatalf("file ID mismatch at %d: %q / %q", i, first[i].ID, second[i].ID)
		}
		for j := range first[i].Hunks {
			if first[i].Hunks[j].ID == "" || first[i].Hunks[j].ID != second[i].Hunks[j].ID {
				t.Fatalf("hunk ID mismatch at %d/%d", i, j)
			}
		}
	}
}

func TestParseRejectsMalformedHunk(t *testing.T) {
	_, err := Parse("diff --git a/a.go b/a.go\n@@ malformed\n")
	if err == nil || !strings.Contains(err.Error(), "hunk") {
		t.Fatalf("error = %v", err)
	}
}
