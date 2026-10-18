// Package template handles personalized email rendering using Go's standard
// text/template package.
//
// Concept: separating template logic into its own package makes it easy
// to unit test without database, network, or HTTP dependencies. Later,
// if you want to support HTML templates (via html/template) or custom
// template helpers, this is the only package that needs to change.
package template

import (
	"bytes"
	"fmt"
	"text/template"
)

// RecipientData holds the variables available inside an email template.
// Field names are capitalized so they are exported and accessible to Go's template engine.
// In your template strings, you access them as {{.Name}} and {{.Email}}.
type RecipientData struct {
	Name  string
	Email string
}

// Render executes a template string with the provided recipient data.
// Example:
//
//	rendered, err := Render("Hello {{.Name}}!", RecipientData{Name: "Alice"})
//	// rendered == "Hello Alice!"
func Render(tmplStr string, data RecipientData) (string, error) {
	// Parse creates a new template and parses the template definition string.
	// We name the template "email" for identification.
	tmpl, err := template.New("email").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	// bytes.Buffer is an in-memory buffer that implements io.Writer.
	// Go's template.Execute writes its output into any io.Writer.
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}

// Validate checks whether a template string has valid syntax before saving it.
// This prevents saving broken templates that would fail on every single job.
func Validate(tmplStr string) error {
	_, err := template.New("validate").Parse(tmplStr)
	if err != nil {
		return fmt.Errorf("invalid template syntax: %w", err)
	}
	return nil
}
