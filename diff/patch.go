package diff

import "strings"

// BuildPatch reconstructs a minimal valid unified diff patch for file,
// containing only the given subset of its hunks. Passing all of file's
// hunks (or none) returns the file's full raw diff unchanged. This is the
// same technique `git add -p` uses internally: the file's existing preamble
// (diff/mode/rename headers, ---/+++ lines) plus only the selected hunks'
// raw text.
func BuildPatch(file File, hunks []Hunk) string {
	if len(hunks) == 0 || len(hunks) == len(file.Hunks) {
		return file.Raw
	}
	preamble := file.Raw
	if len(file.Hunks) > 0 {
		if idx := strings.Index(file.Raw, file.Hunks[0].Raw); idx >= 0 {
			preamble = file.Raw[:idx]
		}
	}
	var patch strings.Builder
	patch.WriteString(preamble)
	for _, hunk := range hunks {
		patch.WriteString(hunk.Raw)
	}
	return patch.String()
}
