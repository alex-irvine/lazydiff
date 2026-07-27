package pr

import "testing"

const testDefaultPattern = `(?:^|[-/_])([0-9a-z]{6,10})(?:[-_]|$)`

func TestExtractTicketDefaultPatternMatchesClickUpID(t *testing.T) {
	ticket, err := ExtractTicket(testDefaultPattern, "feature/869d6rn69-add-login")
	if err != nil {
		t.Fatal(err)
	}
	if ticket != "869d6rn69" {
		t.Fatalf("ticket = %q", ticket)
	}
}

func TestExtractTicketAtStartOfBranchNoPrefix(t *testing.T) {
	ticket, err := ExtractTicket(testDefaultPattern, "869d6rn69-fix-bug")
	if err != nil {
		t.Fatal(err)
	}
	if ticket != "869d6rn69" {
		t.Fatalf("ticket = %q", ticket)
	}
}

func TestExtractTicketNoMatchReturnsEmpty(t *testing.T) {
	ticket, err := ExtractTicket(testDefaultPattern, "quick-fix-typo")
	if err != nil {
		t.Fatal(err)
	}
	if ticket != "" {
		t.Fatalf("expected no match, got %q", ticket)
	}
}

func TestExtractTicketJIRAStyleOverridePattern(t *testing.T) {
	ticket, err := ExtractTicket(`[A-Z]+-\d+`, "feature/ENG-1234-add-login")
	if err != nil {
		t.Fatal(err)
	}
	if ticket != "ENG-1234" {
		t.Fatalf("ticket = %q", ticket)
	}
}

func TestExtractTicketInvalidPattern(t *testing.T) {
	if _, err := ExtractTicket("(", "branch"); err == nil {
		t.Fatal("expected error for invalid pattern")
	}
}

func TestFormatTitleWithTicket(t *testing.T) {
	if got, want := FormatTitle("869d6rn69", "Add OAuth login"), "CU-869d6rn69: Add OAuth login"; got != want {
		t.Fatalf("title = %q, want %q", got, want)
	}
}

func TestFormatTitleWithoutTicket(t *testing.T) {
	if got, want := FormatTitle("", "Add OAuth login"), "Add OAuth login"; got != want {
		t.Fatalf("title = %q, want %q", got, want)
	}
}
