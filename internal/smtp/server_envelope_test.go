package smtp

import (
	"reflect"
	"strings"
	"testing"
)

func TestSessionDataPreservesEnvelopeIndependentlyFromMIMEHeaders(t *testing.T) {
	var received *ParsedMail
	s := &session{handler: func(mail *ParsedMail) {
		received = mail
	}}

	if err := s.Mail("bounce@envelope.example", nil); err != nil {
		t.Fatalf("Mail() error: %v", err)
	}
	for _, recipient := range []string{"bcc@first.example", "other@second.example"} {
		if err := s.Rcpt(recipient, nil); err != nil {
			t.Fatalf("Rcpt(%q) error: %v", recipient, err)
		}
	}

	raw := strings.Join([]string{
		"From: Display Sender <forged@header.example>",
		"To: Visible <visible@header.example>, second@header.example",
		"Cc: copied@header.example",
		"Subject: envelope test",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"body",
	}, "\r\n")
	if err := s.Data(strings.NewReader(raw)); err != nil {
		t.Fatalf("Data() error: %v", err)
	}

	if received == nil {
		t.Fatal("handler was not called")
	}
	if received.EnvelopeFrom != "bounce@envelope.example" {
		t.Errorf("EnvelopeFrom = %q, want %q", received.EnvelopeFrom, "bounce@envelope.example")
	}
	wantRecipients := []string{"bcc@first.example", "other@second.example"}
	if !reflect.DeepEqual(received.EnvelopeRecipients, wantRecipients) {
		t.Errorf("EnvelopeRecipients = %#v, want %#v", received.EnvelopeRecipients, wantRecipients)
	}
	if received.From != "Display Sender <forged@header.example>" {
		t.Errorf("display From = %q", received.From)
	}
	if received.To != "Visible <visible@header.example>, second@header.example" {
		t.Errorf("display To = %q", received.To)
	}
	if received.Cc != "copied@header.example" {
		t.Errorf("display Cc = %q", received.Cc)
	}
}

func TestSessionDataDoesNotFillMissingDisplayHeadersFromEnvelope(t *testing.T) {
	var received *ParsedMail
	s := &session{handler: func(mail *ParsedMail) {
		received = mail
	}}

	_ = s.Mail("sender@envelope.example", nil)
	_ = s.Rcpt("bcc@recipient.example", nil)
	raw := "Subject: Bcc only\r\nContent-Type: text/plain\r\n\r\nbody"
	if err := s.Data(strings.NewReader(raw)); err != nil {
		t.Fatalf("Data() error: %v", err)
	}

	if received == nil {
		t.Fatal("handler was not called")
	}
	if received.From != "" || received.To != "" || received.Cc != "" {
		t.Errorf("display headers were filled from envelope: From=%q To=%q Cc=%q", received.From, received.To, received.Cc)
	}
	if received.EnvelopeFrom != "sender@envelope.example" || !reflect.DeepEqual(received.EnvelopeRecipients, []string{"bcc@recipient.example"}) {
		t.Errorf("envelope was not preserved: from=%q recipients=%#v", received.EnvelopeFrom, received.EnvelopeRecipients)
	}
}

func TestServerAppliesSMTPResourceLimits(t *testing.T) {
	srv := New("127.0.0.1", 2525).smtpServer()

	if srv.MaxMessageBytes != maxMessageBytes {
		t.Errorf("MaxMessageBytes = %d, want %d", srv.MaxMessageBytes, maxMessageBytes)
	}
	if srv.MaxRecipients != maxRecipients {
		t.Errorf("MaxRecipients = %d, want %d", srv.MaxRecipients, maxRecipients)
	}
	if srv.ReadTimeout <= 0 || srv.WriteTimeout <= 0 {
		t.Errorf("SMTP timeouts must be positive: read=%s write=%s", srv.ReadTimeout, srv.WriteTimeout)
	}
}
