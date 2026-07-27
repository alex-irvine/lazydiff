package pr

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var remoteURLPattern = regexp.MustCompile(`(?:^git@([^:]+):|^https?://([^/]+)/)([^/]+)/(.+?)(?:\.git)?/?$`)

// ownerRepo extracts the host, owner, and repo name from a git remote URL,
// supporting both SSH (git@host:owner/repo.git) and HTTPS
// (https://host/owner/repo[.git]) forms.
func ownerRepo(remoteURL string) (host, owner, repo string, err error) {
	match := remoteURLPattern.FindStringSubmatch(strings.TrimSpace(remoteURL))
	if match == nil {
		return "", "", "", fmt.Errorf("unrecognized remote url %q", remoteURL)
	}
	host = match[1]
	if host == "" {
		host = match[2]
	}
	return host, match[3], match[4], nil
}

const maxCompareURLLength = 6000
const bodyTruncationNote = "\n\n_(description truncated by lazydiff — full text in the request log tab)_"

// CompareURL builds a GitHub compare-page URL that pre-fills the create-PR
// form with title and body, mirroring lazygit's approach. remoteURL is the
// origin remote's URL (SSH or HTTPS); the host must be github.com. If the
// resulting URL would exceed a conservative safety threshold, body is
// truncated with a note appended (title is never truncated).
func CompareURL(remoteURL, base, head, title, body string) (string, error) {
	host, owner, repo, err := ownerRepo(remoteURL)
	if err != nil {
		return "", err
	}
	if host != "github.com" {
		return "", fmt.Errorf("remote host %q is not github.com; PR flow is GitHub-only", host)
	}
	build := func(b string) string {
		query := url.Values{}
		query.Set("expand", "1")
		query.Set("title", title)
		query.Set("body", b)
		return fmt.Sprintf("https://github.com/%s/%s/compare/%s...%s?%s", owner, repo, base, head, query.Encode())
	}
	if full := build(body); len(full) <= maxCompareURLLength {
		return full, nil
	}
	overhead := len(build(bodyTruncationNote))
	budget := maxCompareURLLength - overhead
	if budget < 0 {
		budget = 0
	}
	if budget < len(body) {
		body = body[:budget]
	}
	return build(body + bodyTruncationNote), nil
}
