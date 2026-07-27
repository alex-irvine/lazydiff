package pr

import (
	"fmt"
	"regexp"
)

// ExtractTicket applies pattern (a Go regexp) against branch and returns the
// extracted ticket id. If the pattern has a capture group, group 1 is
// returned; otherwise the whole match is returned. Returns ("", nil) when
// the pattern does not match anywhere in branch.
func ExtractTicket(pattern, branch string) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("compile ticket pattern %q: %w", pattern, err)
	}
	match := re.FindStringSubmatch(branch)
	if match == nil {
		return "", nil
	}
	if len(match) > 1 {
		return match[1], nil
	}
	return match[0], nil
}

// FormatTitle builds the final PR title: the ticket re-prefixed as CU-<id>
// when present (ClickUp's GitHub integration only auto-links default
// hash-style ids in this form, not bare), otherwise the AI title unchanged.
func FormatTitle(ticket, title string) string {
	if ticket == "" {
		return title
	}
	return "CU-" + ticket + ": " + title
}
