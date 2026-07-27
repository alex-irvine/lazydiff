package git

import (
	"context"
	"fmt"
	"strings"
)

// StageFile stages a whole file via `git add -A`, which handles
// modifications, additions, deletions, and renames uniformly. Pass both
// oldPath and path for a rename (empty/duplicate paths are deduped).
func (r Repository) StageFile(ctx context.Context, oldPath, path string) error {
	paths := uniqueNonEmpty(oldPath, path)
	if len(paths) == 0 {
		return fmt.Errorf("stage file: no path given")
	}
	args := append([]string{"add", "-A", "--"}, paths...)
	if _, err := r.run(ctx, args...); err != nil {
		return fmt.Errorf("stage %v: %w", paths, err)
	}
	return nil
}

// StagePatch applies patch to the index only (git apply --cached), leaving
// the working tree untouched. patch must be a valid unified diff for
// exactly one file, e.g. built by diff.BuildPatch.
func (r Repository) StagePatch(ctx context.Context, patch string) error {
	if _, err := r.runWithStdin(ctx, strings.NewReader(patch), "apply", "--cached"); err != nil {
		return fmt.Errorf("apply patch: %w", err)
	}
	return nil
}

// Commit creates a commit from whatever is currently staged, with message
// piped to `git commit --file -` (avoids shell-escaping issues with
// multi-line messages).
func (r Repository) Commit(ctx context.Context, message string) error {
	if strings.TrimSpace(message) == "" {
		return fmt.Errorf("commit message must not be empty")
	}
	if _, err := r.runWithStdin(ctx, strings.NewReader(message), "commit", "--file", "-"); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func uniqueNonEmpty(values ...string) []string {
	seen := make(map[string]bool, len(values))
	var result []string
	for _, v := range values {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		result = append(result, v)
	}
	return result
}
