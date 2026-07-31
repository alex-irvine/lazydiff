package ui

import (
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

type DialogKind int

const (
	CommitDialog DialogKind = iota
	PRDialog
	ApproveDialog
	RequestChangesDialog
	MergeDialog
	ClosePRDialog
)

// ActionDialog shows AI-generated text (a commit message, or a PR
// title+body) for the user to review and edit before anything mutates.
// Ready is false while the AI request is still in flight; Err holds a
// generation error, if any (the dialog still opens and lets the user
// hand-write the text in that case).
type ActionDialog struct {
	Kind     DialogKind
	Textarea textarea.Model
	Ready    bool
	Err      error
}

func NewActionDialog(kind DialogKind) *ActionDialog {
	ta := textarea.New()
	ta.Placeholder = "generating..."
	ta.Focus()
	return &ActionDialog{Kind: kind, Textarea: ta}
}

// SetDraft populates the dialog with generated text (or an error) once the
// AI request finishes.
func (d *ActionDialog) SetDraft(text string, err error) {
	d.Ready = true
	d.Err = err
	d.Textarea.SetValue(text)
}

type DialogAction int

const (
	ActionNone DialogAction = iota
	ActionConfirm
	ActionCancel
	ActionRegenerate
)

// Update intercepts esc/ctrl+s/ctrl+r; every other message is forwarded to
// the inner textarea for normal text editing.
func (d *ActionDialog) Update(msg tea.Msg) (DialogAction, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "esc":
			return ActionCancel, nil
		case "ctrl+s":
			return ActionConfirm, nil
		case "ctrl+r":
			return ActionRegenerate, nil
		}
	}
	var cmd tea.Cmd
	d.Textarea, cmd = d.Textarea.Update(msg)
	return ActionNone, cmd
}

func (d *ActionDialog) View() string {
	return d.Textarea.View()
}

func (d *ActionDialog) Text() string {
	return d.Textarea.Value()
}

// ConfirmDialog is a simple title + hint modal with no editable text,
// used for approve/merge/close+delete confirmations. Err holds a
// mutation failure, if any (the dialog stays open so the user can retry
// or cancel).
type ConfirmDialog struct {
	Kind  DialogKind
	Title string
	Err   error
}

func NewConfirmDialog(kind DialogKind, title string) *ConfirmDialog {
	return &ConfirmDialog{Kind: kind, Title: title}
}

// Update maps esc/ctrl+s onto DialogAction; every other key is ignored.
func (d *ConfirmDialog) Update(msg tea.Msg) (DialogAction, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "esc":
			return ActionCancel, nil
		case "ctrl+s":
			return ActionConfirm, nil
		}
	}
	return ActionNone, nil
}
