package pr

import (
	"net/url"
	"strings"
	"testing"
)

func TestCompareURLFromSSHRemote(t *testing.T) {
	got, err := CompareURL("git@github.com:alex-irvine/lazydiff.git", "main", "feature/x", "My title", "My body")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "https://github.com/alex-irvine/lazydiff/compare/main...feature/x?") {
		t.Fatalf("url = %q", got)
	}
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("title") != "My title" || parsed.Query().Get("body") != "My body" || parsed.Query().Get("expand") != "1" {
		t.Fatalf("query = %v", parsed.Query())
	}
}

func TestCompareURLFromHTTPSRemoteWithoutGitSuffix(t *testing.T) {
	got, err := CompareURL("https://github.com/alex-irvine/lazydiff", "main", "feature/x", "t", "b")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "https://github.com/alex-irvine/lazydiff/compare/main...feature/x?") {
		t.Fatalf("url = %q", got)
	}
}

func TestCompareURLRejectsNonGitHubHost(t *testing.T) {
	_, err := CompareURL("git@gitlab.com:group/project.git", "main", "feature/x", "t", "b")
	if err == nil || !strings.Contains(err.Error(), "github.com") {
		t.Fatalf("err = %v", err)
	}
}

func TestCompareURLTruncatesOversizedBody(t *testing.T) {
	body := strings.Repeat("a", 10000)
	got, err := CompareURL("git@github.com:owner/repo.git", "main", "feature/x", "title", body)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > maxCompareURLLength {
		t.Fatalf("url length = %d, want <= %d", len(got), maxCompareURLLength)
	}
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(parsed.Query().Get("body"), "truncated by lazydiff") {
		t.Fatalf("body missing truncation note: %q", parsed.Query().Get("body"))
	}
}
