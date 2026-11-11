package worker

import (
	"errors"
	"fmt"
	"net/textproto"
	"testing"
	"time"

	"sendmail/internal/store"
)

func TestSendRate(t *testing.T) {
	cases := []struct {
		n    int
		d    time.Duration
		want string
	}{
		{5, 400 * time.Millisecond, "12.5/s"},
		{780000, 13 * time.Minute, "1000.0/s"},
		{3, 0, "3 in <1ms"},
	}
	for _, c := range cases {
		if got := sendRate(c.n, c.d); got != c.want {
			t.Errorf("sendRate(%d, %v) = %q, want %q", c.n, c.d, got, c.want)
		}
	}
}

func TestRender(t *testing.T) {
	job := &store.Job{Name: "Aisha\r\nBcc: evil@example.com", Email: "a@example.com",
		SubjectTemplate: "Hi {{.Name}}", BodyTemplate: "Dear {{.Name}},\nyour address is {{.Email}}"}
	subject, body, err := render(job)
	if err != nil {
		t.Fatal(err)
	}
	if subject != "Hi Aisha Bcc: evil@example.com" {
		t.Errorf("subject = %q: line breaks from a name must not reach the header", subject)
	}
	if body != "Dear Aisha\r\nBcc: evil@example.com,\nyour address is a@example.com" {
		t.Errorf("body = %q", body)
	}
	job.SubjectTemplate = "{{.Phone}}"
	if _, _, err := render(job); err == nil {
		t.Error("an unknown field must fail")
	}
}

func TestPermanent(t *testing.T) {
	for _, c := range []struct {
		err  error
		want bool
	}{
		{fmt.Errorf("rcpt to: %w", &textproto.Error{Code: 550, Msg: "no such user"}), true},
		{fmt.Errorf("rcpt to: %w", &textproto.Error{Code: 421, Msg: "try later"}), false},
		{errors.New("connection reset"), false},
	} {
		if got := permanent(c.err); got != c.want {
			t.Errorf("permanent(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}

func TestBackoff(t *testing.T) {
	for attempt, base := range map[int]time.Duration{
		1: 5 * time.Second, 2: 10 * time.Second, 3: 20 * time.Second, 9: 10 * time.Minute, 50: 10 * time.Minute,
	} {
		for range 20 {
			d := backoff(attempt)
			if d < base || d > base+base/5 {
				t.Fatalf("backoff(%d) = %v, want %v plus at most 20%%", attempt, d, base)
			}
		}
	}
}
