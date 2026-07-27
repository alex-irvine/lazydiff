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
