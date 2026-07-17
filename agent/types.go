package agent

import "context"

type Request struct {
	RepoRoot string
	Prompt   string
}

type EventKind int

const (
	Output EventKind = iota
	Diagnostic
)

type Event struct {
	Kind EventKind
	Text string
}

type Runner interface {
	Run(context.Context, Request, func(Event)) error
}
