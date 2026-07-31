package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestActionDialogConfirmCancelRegenerateKeys(t *testing.T) {
	dialog := NewActionDialog(CommitDialog)
	dialog.SetDraft("subject\n\nbody", nil)
	if action, _ := dialog.Update(tea.KeyMsg{Type: tea.KeyEsc}); action != ActionCancel {
		t.Fatalf("esc action = %v", action)
	}
	if action, _ := dialog.Update(tea.KeyMsg{Type: tea.KeyCtrlS}); action != ActionConfirm {
		t.Fatalf("ctrl+s action = %v", action)
	}
	if action, _ := dialog.Update(tea.KeyMsg{Type: tea.KeyCtrlR}); action != ActionRegenerate {
		t.Fatalf("ctrl+r action = %v", action)
	}
}

func TestActionDialogTypingEditsText(t *testing.T) {
	dialog := NewActionDialog(CommitDialog)
	dialog.SetDraft("initial", nil)
	dialog.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})
	if !strings.Contains(dialog.Text(), "!") {
		t.Fatalf("text = %q, want to contain typed character", dialog.Text())
	}
}

func TestActionDialogNotReadyUntilSetDraft(t *testing.T) {
	dialog := NewActionDialog(PRDialog)
	if dialog.Ready {
		t.Fatal("dialog should not be ready before SetDraft")
	}
	dialog.SetDraft("title\n\nbody", nil)
	if !dialog.Ready {
		t.Fatal("dialog should be ready after SetDraft")
	}
}

func TestConfirmDialogEscCancels(t *testing.T) {
	d := NewConfirmDialog(ApproveDialog, "Approve PR #42 (feat: add login)")
	if action, _ := d.Update(tea.KeyMsg{Type: tea.KeyEsc}); action != ActionCancel {
		t.Fatalf("esc action = %v", action)
	}
}

func TestConfirmDialogCtrlSConfirms(t *testing.T) {
	d := NewConfirmDialog(MergeDialog, "Merge PR #42 (feat: add login)")
	if action, _ := d.Update(tea.KeyMsg{Type: tea.KeyCtrlS}); action != ActionConfirm {
		t.Fatalf("ctrl+s action = %v", action)
	}
}

func TestConfirmDialogStoresTitleAndKind(t *testing.T) {
	d := NewConfirmDialog(ClosePRDialog, "Close PR #42 (feat: add login) + delete branch feat-login")
	if d.Kind != ClosePRDialog || d.Title != "Close PR #42 (feat: add login) + delete branch feat-login" {
		t.Fatalf("dialog = %+v", d)
	}
}

func TestRequestChangesDialogKindReadyImmediately(t *testing.T) {
	d := NewActionDialog(RequestChangesDialog)
	d.SetDraft("", nil)
	if !d.Ready || d.Err != nil {
		t.Fatalf("ready = %v, err = %v", d.Ready, d.Err)
	}
}
