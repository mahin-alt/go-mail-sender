package mailer

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSMTP is a tiny in-process SMTP server for tests. It can offer STARTTLS,
// speak implicit TLS, and require AUTH PLAIN, and it records what it saw.
type fakeSMTP struct {
	addr net.Addr

	offerSTARTTLS bool
	implicitTLS   bool
	user, pass    string // when set, AUTH PLAIN must match

	mu        sync.Mutex
	upgraded  bool     // a client completed STARTTLS
	authed    bool     // a client authenticated successfully
	messages  []string // raw DATA payloads
	conns     int
	serverTLS *tls.Config
	certPool  *x509.CertPool
}

func (f *fakeSMTP) host() string { h, _, _ := net.SplitHostPort(f.addr.String()); return h }
func (f *fakeSMTP) port() string { _, p, _ := net.SplitHostPort(f.addr.String()); return p }

// connConfig returns a ConnConfig pointing at the fake server and trusting its certificate.
func (f *fakeSMTP) connConfig(mode string) ConnConfig {
	return ConnConfig{
		Host: "localhost", Port: f.port(), Username: f.user, Password: f.pass, TLSMode: mode,
		tlsConfig: &tls.Config{ServerName: "localhost", RootCAs: f.certPool},
	}
}

func (f *fakeSMTP) snapshot() (upgraded, authed bool, msgs []string, conns int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.upgraded, f.authed, append([]string(nil), f.messages...), f.conns
}

func newFakeSMTP(t *testing.T, configure func(*fakeSMTP)) *fakeSMTP {
	t.Helper()
	f := &fakeSMTP{}
	configure(f)
	f.serverTLS, f.certPool = selfSignedTLS(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	f.addr = ln.Addr()

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			f.mu.Lock()
			f.conns++
			f.mu.Unlock()
			go f.serve(c)
		}
	}()
	return f
}

func selfSignedTLS(t *testing.T) (*tls.Config, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}, pool
}

func (f *fakeSMTP) serve(raw net.Conn) {
	defer raw.Close()
	var conn net.Conn = raw
	if f.implicitTLS {
		conn = tls.Server(raw, f.serverTLS)
	}
	encrypted := f.implicitTLS
	r := bufio.NewReader(conn)
	say := func(s string) { conn.Write([]byte(s + "\r\n")) }

	say("220 fake ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		cmd := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			resp := "250-fake"
			if f.offerSTARTTLS && !encrypted {
				resp += "\r\n250-STARTTLS"
			}
			// Like real servers, only advertise AUTH once the link is encrypted
			// (or when no TLS is on offer at all, as a local plain server would).
			if f.user != "" && (encrypted || !f.offerSTARTTLS) {
				resp += "\r\n250-AUTH PLAIN"
			}
			say(resp + "\r\n250 OK")
		case cmd == "STARTTLS":
			say("220 go ahead")
			tc := tls.Server(raw, f.serverTLS)
			if err := tc.Handshake(); err != nil {
				return
			}
			conn, encrypted = tc, true
			r = bufio.NewReader(tc)
			f.mu.Lock()
			f.upgraded = true
			f.mu.Unlock()
		case strings.HasPrefix(cmd, "AUTH PLAIN"):
			parts := strings.Fields(line)
			payload, _ := base64.StdEncoding.DecodeString(parts[len(parts)-1])
			fields := strings.Split(string(payload), "\x00") // authzid, user, pass
			if len(fields) == 3 && fields[1] == f.user && fields[2] == f.pass {
				f.mu.Lock()
				f.authed = true
				f.mu.Unlock()
				say("235 ok")
			} else {
				say("535 bad credentials")
			}
		case strings.HasPrefix(cmd, "MAIL FROM"), strings.HasPrefix(cmd, "RCPT TO"),
			cmd == "RSET", cmd == "NOOP":
			say("250 OK")
		case cmd == "DATA":
			say("354 go")
			var sb strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				sb.WriteString(l)
			}
			f.mu.Lock()
			f.messages = append(f.messages, sb.String())
			f.mu.Unlock()
			say("250 queued")
		case cmd == "QUIT":
			say("221 bye")
			return
		default:
			say("500 unknown")
		}
	}
}
