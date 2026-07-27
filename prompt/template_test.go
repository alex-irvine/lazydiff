package prompt

import (
	"strings"
	"testing"
)

func testSources(overall, detail string) Sources {
	return Sources{
		Overall:       overall,
		Detail:        detail,
		CommitMessage: "{{staged_diff}}",
		PRDescription: "{{branch_diff}}",
	}
}

func TestParseAndRenderTemplates(t *testing.T) {
	templates, err := Parse(testSources(
		"Repo={{repository}} Mode={{mode}}\n{{overall_diff}}",
		"Target={{selection}}\n{{overall_diff}}\nSelected={{selected_diff}}",
	))
	if err != nil {
		t.Fatal(err)
	}
	ctx := Context{
		Repository:   "/tmp/repo",
		Mode:         "working tree / HEAD",
		OverallDiff:  "diff --git a/a.go b/a.go\n{{literal}}\n",
		Selection:    "a.go hunk 1",
		SelectedDiff: "@@ -1 +1 @@\n-old\n+new\n",
	}
	overall, err := templates.RenderOverall(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(overall, "/tmp/repo") || !strings.Contains(overall, "{{literal}}") {
		t.Fatalf("overall = %q", overall)
	}
	detail, err := templates.RenderDetail(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(detail, "a.go hunk 1") || !strings.Contains(detail, "+new") {
		t.Fatalf("detail = %q", detail)
	}
}

func TestParseRejectsUnknownPlaceholder(t *testing.T) {
	_, err := Parse(testSources("{{unknown}} {{overall_diff}}", "{{overall_diff}} {{selection}} {{selected_diff}}"))
	if err == nil || !strings.Contains(err.Error(), "placeholder") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseRejectsMissingRequiredFields(t *testing.T) {
	_, err := Parse(testSources("{{repository}}", "{{overall_diff}} {{selection}} {{selected_diff}}"))
	if err == nil || !strings.Contains(err.Error(), "overall_diff") {
		t.Fatalf("error = %v", err)
	}
	_, err = Parse(testSources("{{overall_diff}}", "{{overall_diff}} {{selection}}"))
	if err == nil || !strings.Contains(err.Error(), "selected_diff") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseRejectsMalformedTemplate(t *testing.T) {
	_, err := Parse(testSources("{{overall_diff}", "{{overall_diff}} {{selection}} {{selected_diff}}"))
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("error = %v", err)
	}
}

func TestRenderCommitMessageAndPRDescription(t *testing.T) {
	templates, err := Parse(Sources{
		Overall:       "{{overall_diff}}",
		Detail:        "{{overall_diff}} {{selection}} {{selected_diff}}",
		CommitMessage: "Ticket={{ticket}}\n{{staged_diff}}",
		PRDescription: "Branch={{branch}} Base={{base_branch}}\n{{branch_diff}}",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := Context{
		StagedDiff: "+new line\n",
		BranchDiff: "+branch change\n",
		Ticket:     "869d6rn69",
		Branch:     "feature/869d6rn69-thing",
		BaseBranch: "main",
	}
	commitMessage, err := templates.RenderCommitMessage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(commitMessage, "Ticket=869d6rn69") || !strings.Contains(commitMessage, "+new line") {
		t.Fatalf("commit message = %q", commitMessage)
	}
	prDescription, err := templates.RenderPRDescription(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prDescription, "Branch=feature/869d6rn69-thing") || !strings.Contains(prDescription, "Base=main") || !strings.Contains(prDescription, "+branch change") {
		t.Fatalf("pr description = %q", prDescription)
	}
}

func TestParseRejectsMissingCommitMessageRequiredField(t *testing.T) {
	_, err := Parse(Sources{
		Overall:       "{{overall_diff}}",
		Detail:        "{{overall_diff}} {{selection}} {{selected_diff}}",
		CommitMessage: "no placeholder here",
		PRDescription: "{{branch_diff}}",
	})
	if err == nil || !strings.Contains(err.Error(), "staged_diff") {
		t.Fatalf("error = %v", err)
	}
}
