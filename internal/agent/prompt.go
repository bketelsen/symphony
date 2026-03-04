package agent

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/bjk/symphony/internal/domain"
)

// PromptData holds template context for rendering prompts.
type PromptData struct {
	Issue   domain.Issue
	Attempt int // 0 for first attempt
}

// templateFuncs provides helper functions for templates.
var templateFuncs = template.FuncMap{
	"deref": func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	},
}

// RenderPrompt parses and executes a Go text/template with the given data.
func RenderPrompt(tmpl string, data PromptData) (string, error) {
	t, err := template.New("prompt").Funcs(templateFuncs).Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("prompt: parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("prompt: execute template: %w", err)
	}

	return buf.String(), nil
}

// BuildContinuationPrompt generates the turn 2+ prompt.
func BuildContinuationPrompt(issue domain.Issue, turnNumber int) string {
	return fmt.Sprintf(
		"Continue working on %s: %s\n\nThis is turn %d. Check the current state of your work and continue where you left off.",
		issue.Identifier, issue.Title, turnNumber,
	)
}
