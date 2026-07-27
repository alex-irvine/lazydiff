package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alex-irvine/lazydiff/diff"
)

func TestStageFileStagesModifiedFile(t *testing.T) {
	dir := testRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "base.txt"), []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.StageFile(context.Background(), "", "base.txt"); err != nil {
		t.Fatal(err)
	}
	staged, err := r.Snapshot(context.Background(), Staged)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(staged.RawDiff, "three") {
		t.Fatalf("staged diff = %s", staged.RawDiff)
	}
}

func TestStageFileStagesDeletion(t *testing.T) {
	dir := testRepo(t)
	if err := os.Remove(filepath.Join(dir, "base.txt")); err != nil {
		t.Fatal(err)
	}
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.StageFile(context.Background(), "", "base.txt"); err != nil {
		t.Fatal(err)
	}
	staged, err := r.Snapshot(context.Background(), Staged)
	if err != nil {
		t.Fatal(err)
	}
	if len(staged.Files) != 1 || staged.Files[0].Status != diff.Deleted {
		t.Fatalf("staged files = %+v", staged.Files)
	}
}

func TestStageFileStagesRenameAcrossBothPaths(t *testing.T) {
	dir := testRepo(t)
	if err := os.Rename(filepath.Join(dir, "base.txt"), filepath.Join(dir, "renamed.txt")); err != nil {
		t.Fatal(err)
	}
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.StageFile(context.Background(), "base.txt", "renamed.txt"); err != nil {
		t.Fatal(err)
	}
	staged, err := r.Snapshot(context.Background(), Staged)
	if err != nil {
		t.Fatal(err)
	}
	if len(staged.Files) != 1 || staged.Files[0].Status != diff.Renamed {
		t.Fatalf("staged files = %+v", staged.Files)
	}
}

func TestStageFileDedupesEmptyAndEqualPaths(t *testing.T) {
	dir := testRepo(t)
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.StageFile(context.Background(), "", ""); err == nil {
		t.Fatal("expected error when both paths are empty")
	}
}

func TestStagePatchAppliesToIndexOnly(t *testing.T) {
	dir := testRepo(t)
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	patch := "diff --git a/base.txt b/base.txt\n" +
		"--- a/base.txt\n" +
		"+++ b/base.txt\n" +
		"@@ -1,2 +1,3 @@\n" +
		" one\n" +
		" two\n" +
		"+three\n"
	if err := r.StagePatch(context.Background(), patch); err != nil {
		t.Fatal(err)
	}
	staged, err := r.Snapshot(context.Background(), Staged)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(staged.RawDiff, "+three") {
		t.Fatalf("staged diff = %s", staged.RawDiff)
	}
}

func TestStagePatchRejectsInvalidPatch(t *testing.T) {
	dir := testRepo(t)
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.StagePatch(context.Background(), "not a patch"); err == nil {
		t.Fatal("expected error for invalid patch")
	}
}
