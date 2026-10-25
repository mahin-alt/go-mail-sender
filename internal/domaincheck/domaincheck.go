// Package domaincheck answers one question: "can this email domain receive
// mail?". It uses only DNS (the standard library's net.Resolver); it never
// connects to the mail server, so it cannot say whether a particular mailbox
// exists.
//
// Concept: a check has THREE outcomes, not two. A DNS timeout is not proof
// that a domain is dead, so it is reported as Unknown and callers should NOT
// reject the address because of it. Only a definite DNS answer ("no such
// domain", "no mail server") produces Invalid.
package domaincheck

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// DefaultTimeout bounds a single domain check (MX lookup plus fallback).
const DefaultTimeout = 3 * time.Second

// Status is the outcome of a domain check.
type Status int

const (
	// Valid means DNS says the domain can receive mail.
	Valid Status = iota
	// Invalid means DNS definitively says it cannot (no such domain, no mail
	// server, or a null MX).
	Invalid
	// Unknown means we could not tell (timeout, SERVFAIL, network trouble).
	Unknown
)

func (s Status) String() string {
	switch s {
	case Valid:
		return "valid"
	case Invalid:
		return "invalid"
	default:
		return "unknown"
	}
}

// Result is what Check returns. Reason is set for Invalid and Unknown.
type Result struct {
	Status Status
	Reason string
}

// Resolver is the part of *net.Resolver we need. Taking an interface lets
// tests supply canned DNS answers instead of touching the network.
type Resolver interface {
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
	LookupHost(ctx context.Context, host string) ([]string, error)
}

// Checker binds a Resolver so callers can hold one value and call
// Check(ctx, domain) without passing the resolver every time.
type Checker struct {
	resolver Resolver
}

// NewChecker returns a Checker that looks domains up with r
// (use net.DefaultResolver in production).
func NewChecker(r Resolver) *Checker {
	return &Checker{resolver: r}
}

// Check reports whether domain can receive mail. See the package-level Check.
func (c *Checker) Check(ctx context.Context, domain string) Result {
	return Check(ctx, c.resolver, domain)
}

// Check reports whether domain can receive mail.
//
// Order of checks (RFC 5321 section 5.1):
//  1. MX records. A single null MX ("0 .", RFC 7505) means the domain
//     explicitly refuses mail -> Invalid.
//  2. If there are no MX records, the domain's own A/AAAA address is the
//     implicit mail server -> Valid if one exists.
//  3. "Not found" from DNS -> Invalid; any other DNS error -> Unknown.
func Check(ctx context.Context, r Resolver, domain string) Result {
	domain = strings.TrimSuffix(strings.TrimSpace(domain), ".")
	if domain == "" {
		return Result{Invalid, "email has no domain"}
	}

	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	mx, err := r.LookupMX(ctx, domain)

	// LookupMX can return usable records together with an error (for example
	// when some entries are malformed), so look at the records first.
	if len(mx) > 0 {
		if nullMX(mx) {
			return Result{Invalid, fmt.Sprintf("domain %s does not accept mail", domain)}
		}
		return Result{Status: Valid}
	}

	if err != nil && !isNotFound(err) {
		return unknown(domain, err)
	}

	// No MX records: fall back to the implicit MX (A/AAAA).
	hosts, err := r.LookupHost(ctx, domain)
	if len(hosts) > 0 {
		return Result{Status: Valid}
	}
	if err != nil && !isNotFound(err) {
		return unknown(domain, err)
	}

	return Result{Invalid, fmt.Sprintf("domain %s does not exist or has no mail server", domain)}
}

// nullMX reports whether records is exactly one null MX ("."), which means
// "this domain accepts no mail".
func nullMX(records []*net.MX) bool {
	return len(records) == 1 && strings.TrimSuffix(records[0].Host, ".") == ""
}

// isNotFound is true for a definite "no such name / no records" DNS answer.
func isNotFound(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsNotFound
}

func unknown(domain string, err error) Result {
	return Result{Unknown, fmt.Sprintf("could not verify domain %s: %v", domain, err)}
}
