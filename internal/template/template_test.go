package template_test

import (
	"testing"

	"sendmail/internal/template"
)

func TestRender(t *testing.T) {
	data := template.RecipientData{
		Name:  "Jane Doe",
		Email: "jane@example.com",
	}

	tests := []struct {
		name     string
		tmpl     string
		expected string
		wantErr  bool
	}{
		{
			name:     "renders name successfully",
			tmpl:     "Hello {{.Name}}!",
			expected: "Hello Jane Doe!",
			wantErr:  false,
		},
		{
			name:     "renders name and email",
			tmpl:     "Hi {{.Name}}, sent to {{.Email}}",
			expected: "Hi Jane Doe, sent to jane@example.com",
			wantErr:  false,
		},
		{
			name:     "syntax error",
			tmpl:     "Hello {{.Name",
			expected: "",
			wantErr:  true,
		},
		{
			name:     "unknown field returns error or executes",
			tmpl:     "Hello {{.Unknown}}",
			expected: "",
			wantErr:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := template.Render(tc.tmpl, data)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	if err := template.Validate("Hello {{.Name}}"); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}

	if err := template.Validate("Hello {{.Name"); err == nil {
		t.Errorf("expected error for unclosed action, got nil")
	}
}
