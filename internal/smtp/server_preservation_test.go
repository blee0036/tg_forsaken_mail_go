package smtp

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-message/mail"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Feature: high-cpu-usage-fix, Property 4: SMTP Mail Parsing Preservation
// **Validates: Requirements 3.1**
//
// For any valid MIME email input, ParseMail results (From, To, Subject, Date, Text, HTML)
// must remain identical. This captures the baseline behavior that must be preserved
// after the SMTP timeout configuration changes.

// TestProperty_Preservation_ParseMailFieldConsistency verifies that for any valid MIME
// email input, calling ParseMail twice on the same input produces identical results.
// This ensures parsing is deterministic and stable.
func TestProperty_Preservation_ParseMailFieldConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("ParseMail produces identical results for the same MIME input", prop.ForAll(
		func(fromUser, fromDomain, toUser, toDomain, subject, textBody, htmlBody string,
			year, month, day, hour, minute, second int) bool {

			// Clamp date components to valid ranges
			year = 2000 + (absVal(year) % 100)
			month = 1 + (absVal(month) % 12)
			day = 1 + (absVal(day) % 28)
			hour = absVal(hour) % 24
			minute = absVal(minute) % 60
			second = absVal(second) % 60

			fromAddr := fromUser + "@" + fromDomain + ".com"
			toAddr := toUser + "@" + toDomain + ".com"
			dateVal := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)

			// Build MIME message
			var buf bytes.Buffer
			var h mail.Header
			h.SetAddressList("From", []*mail.Address{{Address: fromAddr}})
			h.SetAddressList("To", []*mail.Address{{Address: toAddr}})
			h.SetSubject(subject)
			h.SetDate(dateVal)

			mw, err := mail.CreateWriter(&buf, h)
			if err != nil {
				t.Logf("CreateWriter error: %v", err)
				return false
			}

			var textHeader mail.InlineHeader
			textHeader.Set("Content-Type", "text/plain; charset=utf-8")
			tw, err := mw.CreateInline()
			if err != nil {
				return false
			}
			pw, err := tw.CreatePart(textHeader)
			if err != nil {
				return false
			}
			pw.Write([]byte(textBody))
			pw.Close()

			var htmlHeader mail.InlineHeader
			htmlHeader.Set("Content-Type", "text/html; charset=utf-8")
			pw2, err := tw.CreatePart(htmlHeader)
			if err != nil {
				return false
			}
			pw2.Write([]byte(htmlBody))
			pw2.Close()
			tw.Close()
			mw.Close()

			rawBytes := buf.Bytes()

			// Parse twice
			parsed1, err := ParseMail(bytes.NewReader(rawBytes))
			if err != nil {
				t.Logf("ParseMail (1st) error: %v", err)
				return false
			}
			parsed2, err := ParseMail(bytes.NewReader(rawBytes))
			if err != nil {
				t.Logf("ParseMail (2nd) error: %v", err)
				return false
			}

			// Compare all fields
			if parsed1.From != parsed2.From {
				t.Logf("From mismatch: %q vs %q", parsed1.From, parsed2.From)
				return false
			}
			if parsed1.To != parsed2.To {
				t.Logf("To mismatch: %q vs %q", parsed1.To, parsed2.To)
				return false
			}
			if parsed1.Subject != parsed2.Subject {
				t.Logf("Subject mismatch: %q vs %q", parsed1.Subject, parsed2.Subject)
				return false
			}
			if parsed1.Date != parsed2.Date {
				t.Logf("Date mismatch: %q vs %q", parsed1.Date, parsed2.Date)
				return false
			}
			if parsed1.Text != parsed2.Text {
				t.Logf("Text mismatch: %q vs %q", parsed1.Text, parsed2.Text)
				return false
			}
			if parsed1.HTML != parsed2.HTML {
				t.Logf("HTML mismatch: %q vs %q", parsed1.HTML, parsed2.HTML)
				return false
			}

			return true
		},
		gen.AlphaString(),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString(),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString(),
		gen.AlphaString(),
		gen.AlphaString(),
		gen.IntRange(2000, 2099),
		gen.IntRange(1, 12),
		gen.IntRange(1, 28),
		gen.IntRange(0, 23),
		gen.IntRange(0, 59),
		gen.IntRange(0, 59),
	))

	properties.TestingRun(t)
}

func absVal(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// TestProperty_Preservation_ParseMailExtractsCorrectFields verifies that ParseMail
// correctly extracts From, To, Subject, Date, Text, and HTML from generated MIME emails.
// This captures the baseline parsing behavior.
func TestProperty_Preservation_ParseMailExtractsCorrectFields(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("ParseMail extracts all fields correctly from valid MIME input", prop.ForAll(
		func(fromUser, fromDomain, toUser, toDomain, subject, textBody, htmlBody string,
			year, month, day, hour, minute, second int) bool {

			year = 2000 + (absVal(year) % 100)
			month = 1 + (absVal(month) % 12)
			day = 1 + (absVal(day) % 28)
			hour = absVal(hour) % 24
			minute = absVal(minute) % 60
			second = absVal(second) % 60

			fromAddr := fromUser + "@" + fromDomain + ".com"
			toAddr := toUser + "@" + toDomain + ".com"
			dateVal := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)

			var buf bytes.Buffer
			var h mail.Header
			h.SetAddressList("From", []*mail.Address{{Address: fromAddr}})
			h.SetAddressList("To", []*mail.Address{{Address: toAddr}})
			h.SetSubject(subject)
			h.SetDate(dateVal)

			mw, err := mail.CreateWriter(&buf, h)
			if err != nil {
				return false
			}

			var textHeader mail.InlineHeader
			textHeader.Set("Content-Type", "text/plain; charset=utf-8")
			tw, err := mw.CreateInline()
			if err != nil {
				return false
			}
			pw, err := tw.CreatePart(textHeader)
			if err != nil {
				return false
			}
			pw.Write([]byte(textBody))
			pw.Close()

			var htmlHeader mail.InlineHeader
			htmlHeader.Set("Content-Type", "text/html; charset=utf-8")
			pw2, err := tw.CreatePart(htmlHeader)
			if err != nil {
				return false
			}
			pw2.Write([]byte(htmlBody))
			pw2.Close()
			tw.Close()
			mw.Close()

			parsed, err := ParseMail(&buf)
			if err != nil {
				t.Logf("ParseMail error: %v", err)
				return false
			}

			// Verify From contains the email address
			if !strings.Contains(parsed.From, fromAddr) {
				t.Logf("From %q does not contain %q", parsed.From, fromAddr)
				return false
			}

			// Verify To contains the email address
			if !strings.Contains(parsed.To, toAddr) {
				t.Logf("To %q does not contain %q", parsed.To, toAddr)
				return false
			}

			// Verify Subject
			if parsed.Subject != subject {
				t.Logf("Subject mismatch: got %q, expected %q", parsed.Subject, subject)
				return false
			}

			// Verify Date represents the same point in time
			parsedTime, err := time.Parse("2006-01-02 15:04:05 -0700 MST", parsed.Date)
			if err != nil {
				parsedTime, err = time.Parse("2006-01-02 15:04:05 -0700 -0700", parsed.Date)
				if err != nil {
					t.Logf("Date parse error: %v, raw: %q", err, parsed.Date)
					return false
				}
			}
			if !parsedTime.Equal(dateVal) {
				t.Logf("Date mismatch: parsed %v, expected %v", parsedTime, dateVal)
				return false
			}

			// Verify Text body
			if strings.TrimSpace(parsed.Text) != strings.TrimSpace(textBody) {
				t.Logf("Text mismatch: got %q, expected %q", parsed.Text, textBody)
				return false
			}

			// Verify HTML body
			if strings.TrimSpace(parsed.HTML) != strings.TrimSpace(htmlBody) {
				t.Logf("HTML mismatch: got %q, expected %q", parsed.HTML, htmlBody)
				return false
			}

			return true
		},
		gen.AlphaString(),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString(),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString(),
		gen.AlphaString(),
		gen.AlphaString(),
		gen.IntRange(2000, 2099),
		gen.IntRange(1, 12),
		gen.IntRange(1, 28),
		gen.IntRange(0, 23),
		gen.IntRange(0, 59),
		gen.IntRange(0, 59),
	))

	properties.TestingRun(t)
}

// TestProperty_Preservation_ParseMailTextOnlyEmail verifies that text-only emails
// (no HTML part) are parsed correctly with empty HTML field.
func TestProperty_Preservation_ParseMailTextOnlyEmail(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("text-only MIME email has empty HTML field", prop.ForAll(
		func(fromUser, toDomain, subject, textBody string) bool {
			if len(toDomain) == 0 {
				return true // skip
			}

			fromAddr := fromUser + "@sender.com"
			toAddr := "user@" + toDomain + ".com"

			// Build text-only MIME
			var sb strings.Builder
			sb.WriteString("From: " + fromAddr + "\r\n")
			sb.WriteString("To: " + toAddr + "\r\n")
			sb.WriteString("Subject: " + subject + "\r\n")
			sb.WriteString("Date: Mon, 01 Jan 2024 12:00:00 +0000\r\n")
			sb.WriteString("MIME-Version: 1.0\r\n")
			sb.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
			sb.WriteString("\r\n")
			sb.WriteString(textBody + "\r\n")

			parsed, err := ParseMail(strings.NewReader(sb.String()))
			if err != nil {
				t.Logf("ParseMail error: %v", err)
				return false
			}

			// HTML should be empty for text-only emails
			if parsed.HTML != "" {
				t.Logf("HTML should be empty for text-only email, got %q", parsed.HTML)
				return false
			}

			// Text should contain the body
			if strings.TrimSpace(parsed.Text) != strings.TrimSpace(textBody) {
				t.Logf("Text mismatch: got %q, expected %q", strings.TrimSpace(parsed.Text), strings.TrimSpace(textBody))
				return false
			}

			return true
		},
		gen.AlphaString(),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString(),
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}
