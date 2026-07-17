package git

type Mode int

const (
	WorkingTree Mode = iota
	Staged
	Branch
)

func (m Mode) String() string {
	switch m {
	case WorkingTree:
		return "working tree / HEAD"
	case Staged:
		return "staged / HEAD"
	case Branch:
		return "branch / default"
	default:
		return "unknown"
	}
}

func (m Mode) Next() Mode {
	return Mode((int(m) + 1) % 3)
}
