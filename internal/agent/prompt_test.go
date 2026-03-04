package agent

import (
	"strings"
	"testing"

	"github.com/bjk/symphony/internal/domain"
)

func strPtr(s string) *string { return &s }

func TestRenderPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		tmpl     string
		data     PromptData
		wantSub  string // substring expected in output
		wantErr  bool
	}{
		{
			name: "basic substitution",
			tmpl: "Work on {{ .Issue.Identifier }}: {{ .Issue.Title }}",
			data: PromptData{
				Issue: domain.Issue{Identifier: "owner/repo#1", Title: "Fix bug"},
			},
			wantSub: "owner/repo#1: Fix bug",
		},
		{
			name: "attempt > 0 includes continuation",
			tmpl: `{{ if gt .Attempt 0 }}Continuation attempt {{ .Attempt }}.{{ end }}`,
			data: PromptData{
				Issue:   domain.Issue{Identifier: "repo#1"},
				Attempt: 3,
			},
			wantSub: "Continuation attempt 3.",
		},
		{
			name: "attempt 0 omits continuation",
			tmpl: `Start.{{ if gt .Attempt 0 }}Continuation.{{ end }}End.`,
			data: PromptData{
				Issue:   domain.Issue{Identifier: "repo#1"},
				Attempt: 0,
			},
			wantSub: "Start.End.",
		},
		{
			name: "deref nil description",
			tmpl: `Desc: {{ deref .Issue.Description }}`,
			data: PromptData{
				Issue: domain.Issue{Description: nil},
			},
			wantSub: "Desc: ",
		},
		{
			name: "deref non-nil description",
			tmpl: `Desc: {{ deref .Issue.Description }}`,
			data: PromptData{
				Issue: domain.Issue{Description: strPtr("Fix the login page")},
			},
			wantSub: "Fix the login page",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := RenderPrompt(tt.tmpl, tt.data)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("output = %q, want to contain %q", got, tt.wantSub)
			}
		})
	}
}

func TestRenderPromptInvalidTemplate(t *testing.T) {
	t.Parallel()

	_, err := RenderPrompt("{{ .Invalid", PromptData{})
	if err == nil {
		t.Fatal("expected error for invalid template syntax")
	}
}

func TestRenderPromptMissingField(t *testing.T) {
	t.Parallel()

	_, err := RenderPrompt("{{ .Nonexistent }}", PromptData{})
	if err == nil {
		t.Fatal("expected error for missing field")
	}
}

func TestBuildContinuationPrompt(t *testing.T) {
	t.Parallel()

	issue := domain.Issue{Identifier: "owner/repo#42", Title: "Add feature"}
	got := BuildContinuationPrompt(issue, 3)

	if !strings.Contains(got, "owner/repo#42") {
		t.Errorf("missing identifier in: %q", got)
	}
	if !strings.Contains(got, "Add feature") {
		t.Errorf("missing title in: %q", got)
	}
	if !strings.Contains(got, "turn 3") {
		t.Errorf("missing turn number in: %q", got)
	}
}
