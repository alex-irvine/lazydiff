package prompt

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"text/template"
)

type Context struct {
	Repository   string
	Mode         string
	OverallDiff  string
	Selection    string
	SelectedDiff string
	StagedDiff   string
	BranchDiff   string
	Ticket       string
	Branch       string
	BaseBranch   string
}

// Sources holds the raw (unparsed) template text for every prompt lazydiff
// can render.
type Sources struct {
	Overall       string
	Detail        string
	CommitMessage string
	PRDescription string
}

type Templates struct {
	overall       *template.Template
	detail        *template.Template
	commitMessage *template.Template
	prDescription *template.Template
}

var placeholderPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}`)

var allowedPlaceholders = map[string]struct{}{
	"repository":    {},
	"mode":          {},
	"overall_diff":  {},
	"selection":     {},
	"selected_diff": {},
	"staged_diff":   {},
	"branch_diff":   {},
	"ticket":        {},
	"branch":        {},
	"base_branch":   {},
}

func Parse(sources Sources) (Templates, error) {
	if err := validate("overall", sources.Overall, "overall_diff"); err != nil {
		return Templates{}, err
	}
	if err := validate("detail", sources.Detail, "overall_diff", "selection", "selected_diff"); err != nil {
		return Templates{}, err
	}
	if err := validate("commit_message", sources.CommitMessage, "staged_diff"); err != nil {
		return Templates{}, err
	}
	if err := validate("pr_description", sources.PRDescription, "branch_diff"); err != nil {
		return Templates{}, err
	}
	funcs := template.FuncMap{}
	for name := range allowedPlaceholders {
		funcs[name] = func() string { return "" }
	}
	overallTemplate, err := template.New("overall").Funcs(funcs).Parse(normalizePlaceholders(sources.Overall))
	if err != nil {
		return Templates{}, fmt.Errorf("overall template malformed: %w", err)
	}
	detailTemplate, err := template.New("detail").Funcs(funcs).Parse(normalizePlaceholders(sources.Detail))
	if err != nil {
		return Templates{}, fmt.Errorf("detail template malformed: %w", err)
	}
	commitMessageTemplate, err := template.New("commit_message").Funcs(funcs).Parse(normalizePlaceholders(sources.CommitMessage))
	if err != nil {
		return Templates{}, fmt.Errorf("commit_message template malformed: %w", err)
	}
	prDescriptionTemplate, err := template.New("pr_description").Funcs(funcs).Parse(normalizePlaceholders(sources.PRDescription))
	if err != nil {
		return Templates{}, fmt.Errorf("pr_description template malformed: %w", err)
	}
	return Templates{
		overall:       overallTemplate,
		detail:        detailTemplate,
		commitMessage: commitMessageTemplate,
		prDescription: prDescriptionTemplate,
	}, nil
}

func (t Templates) RenderOverall(ctx Context) (string, error) { return render(t.overall, ctx) }

func (t Templates) RenderDetail(ctx Context) (string, error) { return render(t.detail, ctx) }

func (t Templates) RenderCommitMessage(ctx Context) (string, error) {
	return render(t.commitMessage, ctx)
}

func (t Templates) RenderPRDescription(ctx Context) (string, error) {
	return render(t.prDescription, ctx)
}

func render(t *template.Template, ctx Context) (string, error) {
	if t == nil {
		return "", fmt.Errorf("prompt template is nil")
	}
	var out bytes.Buffer
	if err := t.Execute(&out, map[string]string{
		"repository":    ctx.Repository,
		"mode":          ctx.Mode,
		"overall_diff":  ctx.OverallDiff,
		"selection":     ctx.Selection,
		"selected_diff": ctx.SelectedDiff,
		"staged_diff":   ctx.StagedDiff,
		"branch_diff":   ctx.BranchDiff,
		"ticket":        ctx.Ticket,
		"branch":        ctx.Branch,
		"base_branch":   ctx.BaseBranch,
	}); err != nil {
		return "", fmt.Errorf("render prompt: %w", err)
	}
	return out.String(), nil
}

func validate(name, source string, required ...string) error {
	if strings.TrimSpace(source) == "" {
		return fmt.Errorf("%s template must not be empty", name)
	}
	for _, match := range placeholderPattern.FindAllStringSubmatch(source, -1) {
		if _, ok := allowedPlaceholders[match[1]]; !ok {
			return fmt.Errorf("%s template contains unknown placeholder %q", name, match[1])
		}
	}
	if strings.Contains(source, "{{") || strings.Contains(source, "}}") {
		funcs := template.FuncMap{}
		for placeholder := range allowedPlaceholders {
			funcs[placeholder] = func() string { return "" }
		}
		if _, err := template.New(name).Funcs(funcs).Parse(normalizePlaceholders(source)); err != nil {
			return fmt.Errorf("%s template malformed: %w", name, err)
		}
	}
	for _, requiredPlaceholder := range required {
		found := false
		for _, match := range placeholderPattern.FindAllStringSubmatch(source, -1) {
			if match[1] == requiredPlaceholder {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%s template must include {{%s}}", name, requiredPlaceholder)
		}
	}
	return nil
}

func normalizePlaceholders(source string) string {
	return placeholderPattern.ReplaceAllString(source, "{{.$1}}")
}
