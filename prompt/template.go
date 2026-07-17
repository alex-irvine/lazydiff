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
}

type Templates struct {
	overall *template.Template
	detail  *template.Template
}

var placeholderPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}`)

var allowedPlaceholders = map[string]struct{}{
	"repository":    {},
	"mode":          {},
	"overall_diff":  {},
	"selection":     {},
	"selected_diff": {},
}

func Parse(overall, detail string) (Templates, error) {
	if err := validate("overall", overall, "overall_diff"); err != nil {
		return Templates{}, err
	}
	if err := validate("detail", detail, "overall_diff", "selection", "selected_diff"); err != nil {
		return Templates{}, err
	}
	funcs := template.FuncMap{}
	for name := range allowedPlaceholders {
		funcs[name] = func() string { return "" }
	}
	overallTemplate, err := template.New("overall").Funcs(funcs).Parse(normalizePlaceholders(overall))
	if err != nil {
		return Templates{}, fmt.Errorf("overall template malformed: %w", err)
	}
	detailTemplate, err := template.New("detail").Funcs(funcs).Parse(normalizePlaceholders(detail))
	if err != nil {
		return Templates{}, fmt.Errorf("detail template malformed: %w", err)
	}
	return Templates{overall: overallTemplate, detail: detailTemplate}, nil
}

func (t Templates) RenderOverall(ctx Context) (string, error) {
	return render(t.overall, ctx)
}

func (t Templates) RenderDetail(ctx Context) (string, error) {
	return render(t.detail, ctx)
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
