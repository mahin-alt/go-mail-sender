package mailer

import (
	"strings"
	"testing"

	"sendmail/internal/config"
)

func poolCfg(srv *fakeSMTP, from string) config.Config {
	return config.Config{SMTPHost: srv.host(), SMTPPort: srv.port(), SMTPTLSMode: TLSNone, MailFrom: from}
}

func activeCount(p *Pool) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.activeCount
}

func TestPool_SendsAndReusesConnection(t *testing.T) {
	srv := newFakeSMTP(t, func(f *fakeSMTP) {})
	p := NewPool(poolCfg(srv, "a@example.com"), 2)
	defer p.Close()

	for i := 0; i < 3; i++ {
		if err := p.Send("to@example.com", "hi", "body"); err != nil {
			t.Fatal(err)
		}
	}
	_, _, msgs, conns := srv.snapshot()
	if len(msgs) != 3 || conns != 1 {
		t.Fatalf("messages=%d connections=%d, want 3 messages over 1 reused connection", len(msgs), conns)
	}
}

func TestPool_ReconfigureSwitchesServerAndSender(t *testing.T) {
	a := newFakeSMTP(t, func(f *fakeSMTP) {})
	b := newFakeSMTP(t, func(f *fakeSMTP) {})
	p := NewPool(poolCfg(a, "old@example.com"), 2)
	defer p.Close()

	if err := p.Send("to@example.com", "one", "x"); err != nil {
		t.Fatal(err)
	}
	p.Reconfigure(poolCfg(b, "new@example.com"))
	if got := activeCount(p); got != 0 {
		t.Fatalf("idle connection should be dropped on reconfigure, active=%d", got)
	}
	if err := p.Send("to@example.com", "two", "x"); err != nil {
		t.Fatal(err)
	}

	_, _, aMsgs, _ := a.snapshot()
	_, _, bMsgs, _ := b.snapshot()
	if len(aMsgs) != 1 || len(bMsgs) != 1 {
		t.Fatalf("a got %d, b got %d messages; want 1 each", len(aMsgs), len(bMsgs))
	}
	if want := "From: new@example.com"; !strings.Contains(bMsgs[0], want) {
		t.Fatalf("second message should use the new sender, got %q", bMsgs[0])
	}
}

func TestPool_ConnectionInUseDuringReconfigureIsNotReused(t *testing.T) {
	a := newFakeSMTP(t, func(f *fakeSMTP) {})
	b := newFakeSMTP(t, func(f *fakeSMTP) {})
	p := NewPool(poolCfg(a, "x@example.com"), 2)
	defer p.Close()

	client, err := p.Acquire(ctx5(t)) // checked out, talking to A
	if err != nil {
		t.Fatal(err)
	}
	p.Reconfigure(poolCfg(b, "x@example.com"))
	p.Release(client) // old-generation client: must be closed, not pooled

	if got := activeCount(p); got != 0 {
		t.Fatalf("stale connection should be closed on release, active=%d", got)
	}
	if err := p.Send("to@example.com", "s", "b"); err != nil {
		t.Fatal(err)
	}
	if _, _, msgs, _ := b.snapshot(); len(msgs) != 1 {
		t.Fatalf("message should have gone to the new server B, got %d", len(msgs))
	}
}
