// Package mailer handles delivering emails via SMTP.
//
// This file implements an SMTP Connection Pool.
//
// Concept: why connection pooling?
// Opening a new TCP connection and negotiating TLS/AUTH for EVERY single email
// adds 50-200ms of network overhead per email. If sending 1,000 emails:
//   - Without pool: 1,000 connections created and destroyed.
//   - With pool: 3 persistent connections shared across workers, delivering
//     emails in rapid succession.
//
// SMTP Nuance: SMTP is a stateful TCP protocol. If a connection sits idle
// for 30-60 seconds, SMTP servers (including Mailpit, Mailtrap, and Gmail)
// will drop it. Therefore, our pool:
//  1. Verifies a connection is still alive (via NOOP) before handing it out.
//  2. If dead, closes it and dials a fresh connection automatically.
//  3. Sends RSET (reset) between messages to clear the previous envelope.
package mailer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"sendmail/internal/config"
)

// ErrPoolClosed is returned when trying to acquire a connection from a closed pool.
var ErrPoolClosed = errors.New("smtp connection pool is closed")

// Pool manages a bounded set of reusable SMTP client connections.
type Pool struct {
	cfg config.Config

	// maxConns is the upper limit on concurrent SMTP sockets.
	maxConns int

	// conns holds idle, ready-to-use SMTP clients.
	// A buffered channel acts as an efficient, thread-safe queue.
	conns chan *smtp.Client

	// mu protects activeCount and closed state.
	mu          sync.Mutex
	activeCount int
	closed      bool
}

// NewPool creates an SMTP connection pool with maxConns capacity.
func NewPool(cfg config.Config, maxConns int) *Pool {
	if maxConns <= 0 {
		maxConns = 1
	}

	return &Pool{
		cfg:      cfg,
		maxConns: maxConns,
		conns:    make(chan *smtp.Client, maxConns),
	}
}

// dial creates and authenticates a single new SMTP client.
func (p *Pool) dial(ctx context.Context) (*smtp.Client, error) {
	addr := net.JoinHostPort(p.cfg.SMTPHost, p.cfg.SMTPPort)

	// net.Dialer allows respecting the context deadline during connection.
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial smtp %s: %w", addr, err)
	}

	client, err := smtp.NewClient(conn, p.cfg.SMTPHost)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("init smtp client: %w", err)
	}

	// Authenticate ONLY if credentials are provided.
	// Mailpit by default does not require or advertise authentication.
	if p.cfg.SMTPUsername != "" && p.cfg.SMTPPassword != "" {
		auth := smtp.PlainAuth("", p.cfg.SMTPUsername, p.cfg.SMTPPassword, p.cfg.SMTPHost)
		if ok, _ := client.Extension("AUTH"); ok {
			if err := client.Auth(auth); err != nil {
				_ = client.Close()
				return nil, fmt.Errorf("smtp auth: %w", err)
			}
		}
	}

	return client, nil
}

// Acquire retrieves an active SMTP client from the pool or creates a new one.
// If all connections are in use, it waits until one is released or ctx expires.
func (p *Pool) Acquire(ctx context.Context) (*smtp.Client, error) {
	for {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return nil, ErrPoolClosed
		}

		// Check if an idle connection is already in the channel.
		select {
		case client := <-p.conns:
			p.mu.Unlock()

			// Health check: send NOOP to verify the connection is still alive.
			// If the remote server timed out and hung up, Noop() will return an error.
			if err := client.Noop(); err != nil {
				// Connection died while idle; close it and decrement activeCount.
				_ = client.Close()
				p.mu.Lock()
				p.activeCount--
				p.mu.Unlock()
				continue // Loop again to get or dial another connection.
			}
			return client, nil

		default:
			// No idle connection available in the channel.
			// Can we create a new one without exceeding maxConns?
			if p.activeCount < p.maxConns {
				p.activeCount++
				p.mu.Unlock()

				client, err := p.dial(ctx)
				if err != nil {
					p.mu.Lock()
					p.activeCount--
					p.mu.Unlock()
					return nil, err
				}
				return client, nil
			}
			p.mu.Unlock()
		}

		// Pool is at capacity: wait for an existing connection to be returned,
		// or for the caller's context to cancel/timeout.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case client := <-p.conns:
			// Check if alive before returning
			if err := client.Noop(); err != nil {
				_ = client.Close()
				p.mu.Lock()
				p.activeCount--
				p.mu.Unlock()
				continue
			}
			return client, nil
		}
	}
}

// Release returns a borrowed SMTP client back to the pool.
// If the connection is broken or the pool is closed, it closes the connection.
func (p *Pool) Release(client *smtp.Client) {
	if client == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		_ = client.Close()
		p.activeCount--
		return
	}

	// RSET tells the SMTP server to abort any ongoing mail transaction
	// and reset the envelope state for the next message.
	if err := client.Reset(); err != nil {
		// If RSET fails, the socket is likely broken; discard it.
		_ = client.Close()
		p.activeCount--
		return
	}

	// Return client to idle channel
	select {
	case p.conns <- client:
	default:
		// Channel unexpectedly full; discard
		_ = client.Close()
		p.activeCount--
	}
}

// Discard closes a client without returning it to the pool (used on fatal send errors).
func (p *Pool) Discard(client *smtp.Client) {
	if client == nil {
		return
	}
	_ = client.Close()

	p.mu.Lock()
	p.activeCount--
	p.mu.Unlock()
}

// Send delivers an email using an acquired pooled connection.
// It satisfies the mailer.Mailer interface.
func (p *Pool) Send(to, subject, body string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := p.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire smtp connection: %w", err)
	}

	if err := p.deliverMessage(client, to, subject, body); err != nil {
		// If delivering failed with network error, discard connection rather than reusing.
		p.Discard(client)
		return err
	}

	p.Release(client)
	return nil
}

// deliverMessage sends the SMTP envelope and body over an open SMTP client.
func (p *Pool) deliverMessage(client *smtp.Client, to, subject, body string) error {
	// 1. MAIL FROM:<sender>
	if err := client.Mail(p.cfg.MailFrom); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}

	// 2. RCPT TO:<recipient>
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("rcpt to %q: %w", to, err)
	}

	// 3. DATA
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("data command: %w", err)
	}

	message := buildMessage(p.cfg.MailFrom, to, subject, body)
	if _, err := w.Write(message); err != nil {
		_ = w.Close()
		return fmt.Errorf("write message: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("close message data: %w", err)
	}

	return nil
}

// Close gracefully closes all active SMTP connections in the pool.
func (p *Pool) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	close(p.conns)
	p.mu.Unlock()

	var firstErr error
	for client := range p.conns {
		if err := client.Quit(); err != nil {
			_ = client.Close()
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	log.Printf("smtp pool closed")
	return firstErr
}

// buildMessage formats the RFC 5322 MIME message bytes with CRLF line endings.
func buildMessage(from, to, subject, body string) []byte {
	// Clean line breaks inside body to use standard SMTP CRLF
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\n", "\r\n")

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n",
		from, to, subject, body)
	return []byte(msg)
}

// Ensure interface compliance
var _ io.Closer = (*Pool)(nil)
