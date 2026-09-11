package mail

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/jrvidotti/ddcore/internal/config"
)

// fakeSMTP is the smallest server that can hold a real conversation: enough to
// prove the client says what it should, and to cut the connection at the one
// moment where the outcome stops being knowable.
type fakeSMTP struct {
	addr      string
	dropAtDot bool

	mu   sync.Mutex
	data string
	rcpt []string
	from string
}

func startFakeSMTP(t *testing.T, dropAtDot bool) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeSMTP{addr: ln.Addr().String(), dropAtDot: dropAtDot}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	return s
}

func (s *fakeSMTP) serve(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	say := func(line string) { conn.Write([]byte(line + "\r\n")) }
	say("220 localhost ESMTP")

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			say("250 localhost")
		case strings.HasPrefix(cmd, "MAIL FROM"):
			s.mu.Lock()
			s.from = strings.TrimSpace(line)
			s.mu.Unlock()
			say("250 OK")
		case strings.HasPrefix(cmd, "RCPT TO"):
			s.mu.Lock()
			s.rcpt = append(s.rcpt, strings.TrimSpace(line))
			s.mu.Unlock()
			say("250 OK")
		case strings.HasPrefix(cmd, "DATA"):
			say("354 Go ahead")
			var body strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				body.WriteString(l)
			}
			s.mu.Lock()
			s.data = body.String()
			s.mu.Unlock()
			if s.dropAtDot {
				// The message may well have been accepted; the sender will
				// never find out. This is the case the design calls Uncertain.
				return
			}
			say("250 OK")
		case strings.HasPrefix(cmd, "QUIT"):
			say("221 Bye")
			return
		default:
			say("250 OK")
		}
	}
}

func (s *fakeSMTP) body() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data
}

func senderFor(t *testing.T, s *fakeSMTP) *smtpSender {
	t.Helper()
	host, port, _ := net.SplitHostPort(s.addr)
	cfg := config.Mail{From: "ddcore <no-reply@x.com>", Host: host, TLS: config.TLSNone}
	cfg.Port = atoi(t, port)
	return &smtpSender{cfg: cfg, log: slog.Default()}
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}

func TestSMTPDeliversAMessage(t *testing.T) {
	srv := startFakeSMTP(t, false)
	err := senderFor(t, srv).Send(context.Background(), Message{
		To: []string{"ana@x.com"}, Subject: "Hello", Text: "body",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	body := srv.body()
	for _, want := range []string{"Subject: Hello", "To: ana@x.com", "body"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in delivered message:\n%s", want, body)
		}
	}
}

// Everything up to the final dot is a clean failure: the message did not go.
func TestSMTPRefusedRecipientIsNotUncertain(t *testing.T) {
	srv := startFakeSMTP(t, false)
	s := senderFor(t, srv)
	err := s.Send(context.Background(), Message{})
	if err == nil {
		t.Fatal("expected an error with no recipient")
	}
	if errors.Is(err, ErrUncertain) {
		t.Errorf("a message that never left must not be uncertain: %v", err)
	}
}

// Losing the connection while waiting for the answer to the final dot is the
// one outcome the sender cannot resolve. Retrying is how the same message gets
// delivered twice, so it is reported as its own thing.
func TestSMTPLostAnswerIsUncertain(t *testing.T) {
	srv := startFakeSMTP(t, true)
	err := senderFor(t, srv).Send(context.Background(), Message{
		To: []string{"ana@x.com"}, Subject: "Hello", Text: "body",
	})
	if err == nil {
		t.Fatal("expected an error when the answer never arrives")
	}
	if !errors.Is(err, ErrUncertain) {
		t.Errorf("expected ErrUncertain, got %v", err)
	}
}

func TestSMTPSendsAttachments(t *testing.T) {
	srv := startFakeSMTP(t, false)
	err := senderFor(t, srv).Send(context.Background(), Message{
		To: []string{"ana@x.com"}, Subject: "Invoice", Text: "see attached", HTML: "<p>see attached</p>",
		Attachments: []Attachment{{Filename: "nota fiscal.pdf", ContentType: "application/pdf", Content: []byte("%PDF-1.4 pretend")}},
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	body := srv.body()
	for _, want := range []string{
		"multipart/mixed",
		"multipart/alternative",
		`Content-Disposition: attachment; filename="nota fiscal.pdf"`,
		"Content-Transfer-Encoding: base64",
		"JVBERi0xLjQgcHJldGVuZA==", // "%PDF-1.4 pretend"
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in delivered message:\n%s", want, body)
		}
	}
}
