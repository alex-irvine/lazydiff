package diff

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	diffHeaderPattern = regexp.MustCompile(`^diff --git (.+) (.+)$`)
	hunkPattern       = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(?:.*)$`)
)

func Parse(raw string) ([]File, error) {
	if raw == "" {
		return nil, nil
	}
	lines := strings.SplitAfter(raw, "\n")
	var files []File
	for i := 0; i < len(lines); {
		line := strings.TrimSuffix(lines[i], "\n")
		if line == "" {
			i++
			continue
		}
		if !strings.HasPrefix(line, "diff --git ") {
			return nil, fmt.Errorf("diff file header expected at line %d", i+1)
		}
		start := i
		end := i + 1
		for end < len(lines) && !strings.HasPrefix(strings.TrimSuffix(lines[end], "\n"), "diff --git ") {
			end++
		}
		file, err := parseFile(lines[start:end])
		if err != nil {
			return nil, err
		}
		files = append(files, file)
		i = end
	}
	return files, nil
}

func parseFile(lines []string) (File, error) {
	first := strings.TrimSuffix(lines[0], "\n")
	matches := diffHeaderPattern.FindStringSubmatch(first)
	if len(matches) != 3 {
		return File{}, fmt.Errorf("invalid diff file header %q", first)
	}
	oldPath := cleanHeaderPath(matches[1])
	path := cleanHeaderPath(matches[2])
	file := File{Path: path, OldPath: oldPath, Status: Modified, Raw: strings.Join(lines, "")}
	var hunkStart = -1
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSuffix(lines[i], "\n")
		switch {
		case strings.HasPrefix(line, "new file mode"):
			file.Status = Added
		case strings.HasPrefix(line, "deleted file mode"):
			file.Status = Deleted
		case strings.HasPrefix(line, "rename from "):
			file.OldPath = strings.TrimPrefix(line, "rename from ")
			file.Status = Renamed
		case strings.HasPrefix(line, "rename to "):
			file.Path = strings.TrimPrefix(line, "rename to ")
			file.Status = Renamed
		case strings.HasPrefix(line, "Binary files "):
			file.Status = Binary
		case strings.HasPrefix(line, "@@ "):
			if hunkStart >= 0 {
				file.Hunks = append(file.Hunks, makeHunk(file, lines[hunkStart:i], len(file.Hunks)))
			}
			if !hunkPattern.MatchString(line) {
				return File{}, fmt.Errorf("invalid hunk header %q", line)
			}
			hunkStart = i
		}
	}
	if hunkStart >= 0 {
		file.Hunks = append(file.Hunks, makeHunk(file, lines[hunkStart:], len(file.Hunks)))
	}
	if file.Status == Modified && file.OldPath != file.Path && file.OldPath != "" {
		file.Status = Renamed
	}
	file.ID = fmt.Sprintf("%s:%s:%s", file.Status, file.Path, file.OldPath)
	for i := range file.Hunks {
		file.Hunks[i].ID = fmt.Sprintf("%s:%d:%s", file.ID, i, file.Hunks[i].Header)
	}
	return file, nil
}

func makeHunk(file File, lines []string, ordinal int) Hunk {
	header := strings.TrimSuffix(lines[0], "\n")
	parts := hunkPattern.FindStringSubmatch(header)
	oldStart := mustInt(parts[1])
	oldCount := 1
	if parts[2] != "" {
		oldCount = mustInt(parts[2])
	}
	newStart := mustInt(parts[3])
	newCount := 1
	if parts[4] != "" {
		newCount = mustInt(parts[4])
	}
	return Hunk{
		ID:       fmt.Sprintf("%s:%d:%s", file.ID, ordinal, header),
		Header:   header,
		OldStart: oldStart,
		OldCount: oldCount,
		NewStart: newStart,
		NewCount: newCount,
		Raw:      strings.Join(lines, ""),
	}
}

func mustInt(value string) int {
	n, _ := strconv.Atoi(value)
	return n
}

func cleanHeaderPath(path string) string {
	path = strings.TrimSpace(path)
	if len(path) >= 2 && path[0] == '"' && path[len(path)-1] == '"' {
		path = path[1 : len(path)-1]
	}
	if strings.HasPrefix(path, "a/") || strings.HasPrefix(path, "b/") {
		return path[2:]
	}
	if path == "/dev/null" {
		return ""
	}
	return path
}
