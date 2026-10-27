package mailer

import (
	"context"
	"crypto/tls"
	"strings"
	"testing"
	"time"
)

func ctx5(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestDial_STARTTLS_UpgradesAndAuthenticates(t *testing.T) {
	srv := newFakeSMTP(t, func(f *fakeSMTP) { f.offerSTARTTLS = true; f.user, f.pass = "me", "secret" })

	for _, mode := range []string{TLSStartTLS, TLSAuto} {
		client, err := Dial(ctx5(t), srv.connConfig(mode))
		if err != nil {
			t.Fatalf("mode %s: %v", mode, err)
		}
		if _, ok := client.TLSConnectionState(); !ok {
			t.Errorf("mode %s: connection should be encrypted", mode)
		}
		client.Close()
	}
	upgraded, authed, _, _ := srv.snapshot()
	if !upgraded || !authed {
		t.Fatalf("server saw upgraded=%v authed=%v, want both true", upgraded, authed)
	}
}

func TestDial_STARTTLS_RequiredButNotOffered(t *testing.T) {
	srv := newFakeSMTP(t, func(f *fakeSMTP) {})

	_, err := Dial(ctx5(t), srv.connConfig(TLSStartTLS))
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("err = %v, want a STARTTLS error", err)
	}
}

func TestDial_Auto_FallsBackToPlainWhenNotOffered(t *testing.T) {
	srv := newFakeSMTP(t, func(f *fakeSMTP) {})

	client, err := Dial(ctx5(t), srv.connConfig(TLSAuto))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, ok := client.TLSConnectionState(); ok {
		t.Fatal("connection should be plain when the server offers no STARTTLS")
	}
}

func TestDial_None_IgnoresSTARTTLSOffer(t *testing.T) {
	srv := newFakeSMTP(t, func(f *fakeSMTP) { f.offerSTARTTLS = true })

	client, err := Dial(ctx5(t), srv.connConfig(TLSNone))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if upgraded, _, _, _ := srv.snapshot(); upgraded {
		t.Fatal("mode none must not upgrade")
	}
}

func TestDial_ImplicitTLS(t *testing.T) {
	srv := newFakeSMTP(t, func(f *fakeSMTP) { f.implicitTLS = true; f.user, f.pass = "me", "secret" })

	client, err := Dial(ctx5(t), srv.connConfig(TLSImplicit))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, ok := client.TLSConnectionState(); !ok {
		t.Fatal("implicit TLS connection should be encrypted")
	}
	if _, authed, _, _ := srv.snapshot(); !authed {
		t.Fatal("should have authenticated over implicit TLS")
	}
}

func TestDial_ImplicitTLS_AgainstPlainServerFails(t *testing.T) {
	srv := newFakeSMTP(t, func(f *fakeSMTP) {})

	if _, err := Dial(ctx5(t), srv.connConfig(TLSImplicit)); err == nil {
		t.Fatal("expected an error when speaking TLS to a plain server")
	}
}

func TestDial_UntrustedCertificateIsRejected(t *testing.T) {
	srv := newFakeSMTP(t, func(f *fakeSMTP) { f.offerSTARTTLS = true })

	cfg := srv.connConfig(TLSStartTLS)
	cfg.tlsConfig = &tls.Config{ServerName: "localhost"} // system roots: self-signed cert not trusted
	_, err := Dial(ctx5(t), cfg)
	if err == nil || !strings.Contains(err.Error(), "starttls") {
		t.Fatalf("err = %v, want a certificate error from starttls", err)
	}
}

func TestDial_WrongPassword(t *testing.T) {
	srv := newFakeSMTP(t, func(f *fakeSMTP) { f.offerSTARTTLS = true; f.user, f.pass = "me", "secret" })

	cfg := srv.connConfig(TLSStartTLS)
	cfg.Password = "wrong"
	_, err := Dial(ctx5(t), cfg)
	if err == nil || !strings.Contains(err.Error(), "smtp auth") {
		t.Fatalf("err = %v, want smtp auth failure", err)
	}
}

func TestDial_ConnectionRefused(t *testing.T) {
	_, err := Dial(ctx5(t), ConnConfig{Host: "127.0.0.1", Port: "1"})
	if err == nil {
		t.Fatal("expected connection error")
	}
}

func TestSendTest_DeliversOverSTARTTLS(t *testing.T) {
	srv := newFakeSMTP(t, func(f *fakeSMTP) { f.offerSTARTTLS = true })

	if err := SendTest(ctx5(t), srv.connConfig(TLSStartTLS), "from@example.com", "to@example.com"); err != nil {
		t.Fatal(err)
	}
	_, _, msgs, _ := srv.snapshot()
	if len(msgs) != 1 || !strings.Contains(msgs[0], "To: to@example.com") || !strings.Contains(msgs[0], "Subject: SMTP test email") {
		t.Fatalf("unexpected messages: %q", msgs)
	}
}

func TestCheck_SendsNothing(t *testing.T) {
	srv := newFakeSMTP(t, func(f *fakeSMTP) { f.offerSTARTTLS = true })

	if err := Check(ctx5(t), srv.connConfig(TLSStartTLS)); err != nil {
		t.Fatal(err)
	}
	if _, _, msgs, _ := srv.snapshot(); len(msgs) != 0 {
		t.Fatalf("Check must not send mail, got %d", len(msgs))
	}
}
