package prompt

import (
	"strings"
	"testing"
)

func TestParseAndRenderTemplates(t *testing.T) {
	templates, err := Parse(
		"Repo={{repository}} Mode={{mode}}\n{{overall_diff}}",
		"Target={{selection}}\n{{overall_diff}}\nSelected={{selected_diff}}",
	)
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
	_, err := Parse("{{unknown}} {{overall_diff}}", "{{overall_diff}} {{selection}} {{selected_diff}}")
	if err == nil || !strings.Contains(err.Error(), "placeholder") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseRejectsMissingRequiredFields(t *testing.T) {
	_, err := Parse("{{repository}}", "{{overall_diff}} {{selection}} {{selected_diff}}")
	if err == nil || !strings.Contains(err.Error(), "overall_diff") {
		t.Fatalf("error = %v", err)
	}
	_, err = Parse("{{overall_diff}}", "{{overall_diff}} {{selection}}")
	if err == nil || !strings.Contains(err.Error(), "selected_diff") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseRejectsMalformedTemplate(t *testing.T) {
	_, err := Parse("{{overall_diff}", "{{overall_diff}} {{selection}} {{selected_diff}}")
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("error = %v", err)
	}
}
