package diff

import "strings"

type FileStatus string

const (
	Modified FileStatus = "modified"
	Added    FileStatus = "added"
	Deleted  FileStatus = "deleted"
	Renamed  FileStatus = "renamed"
	Binary   FileStatus = "binary"
)

type File struct {
	ID      string
	Path    string
	OldPath string
	Status  FileStatus
	Hunks   []Hunk
	Raw     string
}

type Hunk struct {
	ID       string
	Header   string
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Raw      string
}

func (f File) RawDiff() string { return f.Raw }

func (h Hunk) RawDiff() string { return h.Raw }

func (f File) SelectedHunk(id string) *Hunk {
	for i := range f.Hunks {
		if f.Hunks[i].ID == id {
			return &f.Hunks[i]
		}
	}
	return nil
}

func (f File) DisplayPath() string {
	if f.Path != "" {
		return f.Path
	}
	return strings.TrimSpace(f.OldPath)
}
