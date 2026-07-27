package git

import (
	"context"
	"fmt"
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
