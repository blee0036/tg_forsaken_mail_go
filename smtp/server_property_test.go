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

// Feature: go-version-rewrite, Property 4: Mail parsing field completeness
// Validates: Requirements 3.2, 3.5
func TestProperty_MailParsingFieldCompleteness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("parse MIME email → all fields extracted correctly", prop.ForAll(
		func(fromUser, fromDomain, toUser, toDomain, subject, textBody, htmlBody string, year, month, day, hour, minute, second int) bool {
			// Clamp date components to valid ranges
			if year < 2000 {
				year = 2000
			}
			year = 2000 + (year % 100)
			month = 1 + (abs(month) % 12)
			day = 1 + (abs(day) % 28)
			hour = abs(hour) % 24
			minute = abs(minute) % 60
			second = abs(second) % 60

			fromAddr := fromUser + "@" + fromDomain + ".com"
			toAddr := toUser + "@" + toDomain + ".com"
			dateVal := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)

			// Build MIME message using go-message/mail
			var buf bytes.Buffer

			var h mail.Header
			h.SetAddressList("From", []*mail.Address{{Address: fromAddr}})
			h.SetAddressList("To", []*mail.Address{{Address: toAddr}})
			h.SetSubject(subject)
			h.SetDate(dateVal)

			// Create multipart writer (alternative: text + html)
			mw, err := mail.CreateWriter(&buf, h)
			if err != nil {
				t.Logf("CreateWriter error: %v", err)
				return false
			}

			// Write text/plain part
			var textHeader mail.InlineHeader
			textHeader.Set("Content-Type", "text/plain; charset=utf-8")
			tw, err := mw.CreateInline()
			if err != nil {
				t.Logf("CreateInline error: %v", err)
				return false
			}
			pw, err := tw.CreatePart(textHeader)
			if err != nil {
				t.Logf("CreatePart (text) error: %v", err)
				return false
			}
			pw.Write([]byte(textBody))
			pw.Close()

			// Write text/html part
			var htmlHeader mail.InlineHeader
			htmlHeader.Set("Content-Type", "text/html; charset=utf-8")
			pw2, err := tw.CreatePart(htmlHeader)
			if err != nil {
				t.Logf("CreatePart (html) error: %v", err)
				return false
			}
			pw2.Write([]byte(htmlBody))
			pw2.Close()

			tw.Close()
			mw.Close()

			// Parse the constructed MIME message
			parsed, err := ParseMail(&buf)
			if err != nil {
				t.Logf("ParseMail error: %v\nRaw:\n%s", err, buf.String())
				return false
			}

			// Verify From: should contain the email address
			if !strings.Contains(parsed.From, fromAddr) {
				t.Logf("From mismatch: got %q, expected to contain %q", parsed.From, fromAddr)
				return false
			}

			// Verify To: should contain the email address
			if !strings.Contains(parsed.To, toAddr) {
				t.Logf("To mismatch: got %q, expected to contain %q", parsed.To, toAddr)
				return false
			}

			// Verify Subject
			if parsed.Subject != subject {
				t.Logf("Subject mismatch: got %q, expected %q", parsed.Subject, subject)
				return false
			}

			// Verify Date: the parsed date string represents the same point in time.
			// MIME dates use numeric timezone offsets (+0000) while time.UTC uses "UTC",
			// so we compare by parsing both back to time and checking equality.
			parsedTime, err := time.Parse("2006-01-02 15:04:05 -0700 MST", parsed.Date)
			if err != nil {
				// Try alternative format that Go's time.String() may produce
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
		gen.AlphaString(),                // fromUser
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }), // fromDomain
		gen.AlphaString(),                // toUser
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }), // toDomain
		gen.AlphaString(),                // subject
		gen.AlphaString(),                // textBody
		gen.AlphaString(),                // htmlBody
		gen.IntRange(2000, 2099),         // year
		gen.IntRange(1, 12),              // month
		gen.IntRange(1, 28),              // day
		gen.IntRange(0, 23),              // hour
		gen.IntRange(0, 59),              // minute
		gen.IntRange(0, 59),              // second
	))

	properties.TestingRun(t)
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
