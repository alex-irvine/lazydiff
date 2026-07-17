package git

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alex-irvine/lazydiff/diff"
)

type Snapshot struct {
	ID          string
	Mode        Mode
	Base        string
	RawDiff     string
	Files       []diff.File
	StatusError error
}

func (s Snapshot) Target(fileID string) (*diff.File, bool) {
	for i := range s.Files {
		if s.Files[i].ID == fileID {
			return &s.Files[i], true
		}
	}
	return nil, false
}

func (r Repository) Snapshot(ctx context.Context, mode Mode) (Snapshot, error) {
	args := []string{"diff", "--no-color", "--binary"}
	base := "HEAD"
	switch mode {
	case WorkingTree:
		args = append(args, "HEAD")
	case Staged:
		args = append(args, "--cached")
	case Branch:
		branch, err := r.DefaultBranch(ctx)
		if err != nil {
			return Snapshot{}, err
		}
		base = branch + "...HEAD"
		args = append(args, base)
	default:
		return Snapshot{}, fmt.Errorf("unsupported diff mode %d", mode)
	}
	raw, err := r.run(ctx, args...)
	if err != nil && len(raw) == 0 {
		return Snapshot{}, fmt.Errorf("collect %s diff: %w", mode, err)
	}
	rawText := string(raw)
	if mode == WorkingTree {
		untracked, untrackedErr := r.untrackedDiffs(ctx)
		if untrackedErr != nil {
			return Snapshot{}, untrackedErr
		}
		rawText += untracked
	}
	files, parseErr := diff.Parse(rawText)
	if parseErr != nil {
		return Snapshot{}, fmt.Errorf("parse %s diff: %w", mode, parseErr)
	}
	hash := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s", mode, base, rawText)))
	return Snapshot{ID: fmt.Sprintf("%x", hash[:]), Mode: mode, Base: base, RawDiff: rawText, Files: files}, nil
}

func (r Repository) untrackedDiffs(ctx context.Context) (string, error) {
	output, err := r.run(ctx, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return "", fmt.Errorf("list untracked files: %w", err)
	}
	var builder strings.Builder
	for _, name := range strings.Split(string(output), "\x00") {
		if name == "" {
			continue
		}
		path := filepath.Join(r.Root, filepath.FromSlash(name))
		info, statErr := os.Stat(path)
		if statErr != nil || info.IsDir() || info.Size() > 1024*1024 {
			continue
		}
		content, diffErr := r.run(ctx, "diff", "--no-index", "--no-color", "--binary", "/dev/null", name)
		if diffErr != nil && len(content) == 0 {
			return "", fmt.Errorf("diff untracked file %s: %w", name, diffErr)
		}
		builder.Write(content)
	}
	return builder.String(), nil
}
