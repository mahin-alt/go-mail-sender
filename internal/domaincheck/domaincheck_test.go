package domaincheck

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
)

// fakeResolver returns canned DNS answers so tests never touch the network.
type fakeResolver struct {
	mx      []*net.MX
	mxErr   error
	hosts   []string
	hostErr error
}

func (f fakeResolver) LookupMX(context.Context, string) ([]*net.MX, error) {
	return f.mx, f.mxErr
}

func (f fakeResolver) LookupHost(context.Context, string) ([]string, error) {
	return f.hosts, f.hostErr
}

var (
	errNotFound  = &net.DNSError{Err: "no such host", IsNotFound: true}
	errTimeout   = &net.DNSError{Err: "i/o timeout", IsTimeout: true}
	errTemporary = &net.DNSError{Err: "server misbehaving", IsTemporary: true}
	mxRecord     = []*net.MX{{Host: "mx.example.com.", Pref: 10}}
)

func TestCheck(t *testing.T) {
	tests := []struct {
		name     string
		domain   string
		resolver fakeResolver
		want     Status
		wantMsg  string // substring expected in Reason; "" = not checked
	}{
		{"has MX records", "example.com", fakeResolver{mx: mxRecord}, Valid, ""},
		{"null MX", "example.com", fakeResolver{mx: []*net.MX{{Host: ".", Pref: 0}}}, Invalid, "does not accept mail"},
		{"no MX but has A record", "example.com", fakeResolver{mxErr: errNotFound, hosts: []string{"192.0.2.1"}}, Valid, ""},
		{"no MX and no A record", "example.com", fakeResolver{mxErr: errNotFound, hostErr: errNotFound}, Invalid, "does not exist"},
		{"empty MX answer and no host", "example.com", fakeResolver{}, Invalid, "does not exist"},
		{"MX timeout", "example.com", fakeResolver{mxErr: errTimeout}, Unknown, "could not verify"},
		{"MX temporary failure", "example.com", fakeResolver{mxErr: errTemporary}, Unknown, "could not verify"},
		{"MX not found then host timeout", "example.com", fakeResolver{mxErr: errNotFound, hostErr: errTimeout}, Unknown, "could not verify"},
		{"records returned with an error", "example.com", fakeResolver{mx: mxRecord, mxErr: errors.New("partial")}, Valid, ""},
		{"trailing dot is ignored", "example.com.", fakeResolver{mx: mxRecord}, Valid, ""},
		{"empty domain", "", fakeResolver{}, Invalid, "no domain"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Check(context.Background(), tc.resolver, tc.domain)
			if got.Status != tc.want {
				t.Fatalf("status = %v (%q), want %v", got.Status, got.Reason, tc.want)
			}
			if tc.wantMsg != "" && !strings.Contains(got.Reason, tc.wantMsg) {
				t.Errorf("reason = %q, want it to contain %q", got.Reason, tc.wantMsg)
			}
			if tc.want == Valid && got.Reason != "" {
				t.Errorf("valid result should have no reason, got %q", got.Reason)
			}
		})
	}
}

// *net.Resolver must satisfy Resolver, or wiring the real thing in later breaks.
var _ Resolver = (*net.Resolver)(nil)
