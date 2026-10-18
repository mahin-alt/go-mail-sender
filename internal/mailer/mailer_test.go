package mailer

import (
	"strings"
	"testing"
)

func TestBuildMessage(t *testing.T) {
	from := "sender@example.com"
	to := "recipient@example.com"
	subject := "Hello World"
	body := "This is a test body with\nmultiple\nlines."

	msg := string(buildMessage(from, to, subject, body))

	if !strings.Contains(msg, "From: "+from+"\r\n") {
		t.Errorf("expected From header, got %q", msg)
	}
	if !strings.Contains(msg, "To: "+to+"\r\n") {
		t.Errorf("expected To header, got %q", msg)
	}
	if !strings.Contains(msg, "Subject: "+subject+"\r\n") {
		t.Errorf("expected Subject header, got %q", msg)
	}
	if !strings.Contains(msg, "Content-Type: text/plain; charset=UTF-8\r\n") {
		t.Errorf("expected Content-Type header, got %q", msg)
	}
	if !strings.Contains(msg, "\r\n\r\nThis is a test body with\r\nmultiple\r\nlines.\r\n") {
		t.Errorf("expected CRLF line endings in body, got %q", msg)
	}
}
