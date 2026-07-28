package agent

import (
	"context"
)

type OpenCode struct {
	Command            string
	Args               []string
	ReadOnly           bool
	AllowExternalTools bool
}

func NewOpenCode(command string, args []string, readOnly, allowExternalTools bool) OpenCode {
	return OpenCode{Command: command, Args: append([]string(nil), args...), ReadOnly: readOnly, AllowExternalTools: allowExternalTools}
}

func (o OpenCode) Run(ctx context.Context, request Request, emit func(Event)) error {
	args := make([]string, 0, len(o.Args)+4)
	if !o.AllowExternalTools {
		args = append(args, "--pure")
	}
	args = append(args, o.Args...)
	args = append(args, "--auto")
	return Generic{Command: o.Command, Args: args}.Run(ctx, request, emit)
}
