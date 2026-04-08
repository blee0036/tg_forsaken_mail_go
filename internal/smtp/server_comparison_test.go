package smtp

import (
	"strings"
	"testing"
)

// Task 4.3: 对比验证：SMTP 模块
// Validates: Requirement 3.5
// Verifies that Go ParseMail() extracts the same header fields and body content
// as the Node.js mailin library would produce for identical MIME messages.

// buildMultipartMIME constructs a raw MIME email string with text and optional HTML parts.
func buildMultipartMIME(from, to, subject, date, textBody, htmlBody string) string {
	boundary := "----=_Part_12345"
	var sb strings.Builder

	sb.WriteString("From: " + from + "\r\n")
	sb.WriteString("To: " + to + "\r\n")
	sb.WriteString("Subject: " + subject + "\r\n")
	sb.WriteString("Date: " + date + "\r\n")
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n")
	sb.WriteString("\r\n")

	// Text part
	sb.WriteString("--" + boundary + "\r\n")
	sb.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(textBody + "\r\n")

	// HTML part (if provided)
	if htmlBody != "" {
		sb.WriteString("--" + boundary + "\r\n")
		sb.WriteString("Content-Type: text/html; charset=utf-8\r\n")
		sb.WriteString("\r\n")
		sb.WriteString(htmlBody + "\r\n")
	}

	sb.WriteString("--" + boundary + "--\r\n")
	return sb.String()
}

// buildTextOnlyMIME constructs a raw MIME email with only a text/plain body.
func buildTextOnlyMIME(from, to, subject, date, textBody string) string {
	var sb strings.Builder
	sb.WriteString("From: " + from + "\r\n")
	sb.WriteString("To: " + to + "\r\n")
	sb.WriteString("Subject: " + subject + "\r\n")
	sb.WriteString("Date: " + date + "\r\n")
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(textBody + "\r\n")
	return sb.String()
}

// buildMIMEWithAttachment constructs a MIME email with text, HTML, and an attachment.
func buildMIMEWithAttachment(from, to, subject, date, textBody, htmlBody, attachName, attachContent string) string {
	outerBoundary := "----=_Mixed_67890"
	innerBoundary := "----=_Alt_12345"
	var sb strings.Builder

	sb.WriteString("From: " + from + "\r\n")
	sb.WriteString("To: " + to + "\r\n")
	sb.WriteString("Subject: " + subject + "\r\n")
	sb.WriteString("Date: " + date + "\r\n")
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: multipart/mixed; boundary=\"" + outerBoundary + "\"\r\n")
	sb.WriteString("\r\n")

	// Multipart/alternative (text + html)
	sb.WriteString("--" + outerBoundary + "\r\n")
	sb.WriteString("Content-Type: multipart/alternative; boundary=\"" + innerBoundary + "\"\r\n")
	sb.WriteString("\r\n")

	// Text part
	sb.WriteString("--" + innerBoundary + "\r\n")
	sb.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(textBody + "\r\n")

	// HTML part
	sb.WriteString("--" + innerBoundary + "\r\n")
	sb.WriteString("Content-Type: text/html; charset=utf-8\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(htmlBody + "\r\n")

	sb.WriteString("--" + innerBoundary + "--\r\n")

	// Attachment
	sb.WriteString("--" + outerBoundary + "\r\n")
	sb.WriteString("Content-Type: application/octet-stream\r\n")
	sb.WriteString("Content-Disposition: attachment; filename=\"" + attachName + "\"\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(attachContent + "\r\n")

	sb.WriteString("--" + outerBoundary + "--\r\n")
	return sb.String()
}

// buildMIMENoHeaders constructs a MIME email with missing From/To/Subject/Date headers.
func buildMIMENoHeaders(textBody string) string {
	var sb strings.Builder
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(textBody + "\r\n")
	return sb.String()
}

// TestComparison_StandardEmail verifies that Go ParseMail extracts the same fields
// as Node.js mailin would for a standard multipart email with text and HTML.
func TestComparison_StandardEmail(t *testing.T) {
	raw := buildMultipartMIME(
		"sender@example.com",
		"receiver@test.tgmail.party",
		"Test Email Subject",
		"Mon, 01 Jan 2024 12:00:00 +0000",
		"Hello, this is a test email.",
		"<html><body><p>Hello, this is a test email.</p></body></html>",
	)

	parsed, err := ParseMail(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("ParseMail failed: %v", err)
	}

	// Node mailin parses From as the raw address string.
	// go-message parses it into mail.Address and .String() returns "<addr>" format.
	if !strings.Contains(parsed.From, "sender@example.com") {
		t.Errorf("From: got %q, want it to contain %q", parsed.From, "sender@example.com")
	}

	if !strings.Contains(parsed.To, "receiver@test.tgmail.party") {
		t.Errorf("To: got %q, want it to contain %q", parsed.To, "receiver@test.tgmail.party")
	}

	if parsed.Subject != "Test Email Subject" {
		t.Errorf("Subject: got %q, want %q", parsed.Subject, "Test Email Subject")
	}

	// Date should be parseable and represent the same point in time.
	// Node mailin stores the raw date header; Go parses it to time.Time then .String().
	if parsed.Date == "" {
		t.Error("Date: got empty string, want non-empty date")
	}
	// Verify the date string contains the expected components
	if !strings.Contains(parsed.Date, "2024") {
		t.Errorf("Date: got %q, expected it to contain year 2024", parsed.Date)
	}

	// Text body — Node mailin extracts text/plain part
	if strings.TrimSpace(parsed.Text) != "Hello, this is a test email." {
		t.Errorf("Text: got %q, want %q", strings.TrimSpace(parsed.Text), "Hello, this is a test email.")
	}

	// HTML body — Node mailin extracts text/html part
	if strings.TrimSpace(parsed.HTML) != "<html><body><p>Hello, this is a test email.</p></body></html>" {
		t.Errorf("HTML: got %q, want %q", strings.TrimSpace(parsed.HTML), "<html><body><p>Hello, this is a test email.</p></body></html>")
	}

	// No attachments expected
	if len(parsed.Attachments) != 0 {
		t.Errorf("Attachments: got %d, want 0", len(parsed.Attachments))
	}
}

// TestComparison_TextOnlyEmail verifies parsing of an email with only a text body (no HTML).
// Node mailin would set html to empty/undefined and text to the plain text content.
func TestComparison_TextOnlyEmail(t *testing.T) {
	raw := buildTextOnlyMIME(
		"sender@example.com",
		"receiver@test.tgmail.party",
		"Plain Text Only",
		"Tue, 15 Feb 2024 08:30:00 +0000",
		"This email has no HTML body, only plain text.",
	)

	parsed, err := ParseMail(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("ParseMail failed: %v", err)
	}

	if !strings.Contains(parsed.From, "sender@example.com") {
		t.Errorf("From: got %q, want it to contain %q", parsed.From, "sender@example.com")
	}

	if !strings.Contains(parsed.To, "receiver@test.tgmail.party") {
		t.Errorf("To: got %q, want it to contain %q", parsed.To, "receiver@test.tgmail.party")
	}

	if parsed.Subject != "Plain Text Only" {
		t.Errorf("Subject: got %q, want %q", parsed.Subject, "Plain Text Only")
	}

	if strings.TrimSpace(parsed.Text) != "This email has no HTML body, only plain text." {
		t.Errorf("Text: got %q, want %q", strings.TrimSpace(parsed.Text), "This email has no HTML body, only plain text.")
	}

	// HTML should be empty — Node mailin would not populate html for text-only emails
	if parsed.HTML != "" {
		t.Errorf("HTML: got %q, want empty string", parsed.HTML)
	}

	if len(parsed.Attachments) != 0 {
		t.Errorf("Attachments: got %d, want 0", len(parsed.Attachments))
	}
}

// TestComparison_EmailWithAttachment verifies parsing of an email with text, HTML, and an attachment.
// Node mailin would populate attachments array with filename, contentType, and content.
func TestComparison_EmailWithAttachment(t *testing.T) {
	raw := buildMIMEWithAttachment(
		"sender@example.com",
		"receiver@test.tgmail.party",
		"Email With Attachment",
		"Wed, 20 Mar 2024 14:00:00 +0000",
		"See the attached file.",
		"<html><body><p>See the attached file.</p></body></html>",
		"report.txt",
		"This is the attachment content.",
	)

	parsed, err := ParseMail(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("ParseMail failed: %v", err)
	}

	if !strings.Contains(parsed.From, "sender@example.com") {
		t.Errorf("From: got %q, want it to contain %q", parsed.From, "sender@example.com")
	}

	if parsed.Subject != "Email With Attachment" {
		t.Errorf("Subject: got %q, want %q", parsed.Subject, "Email With Attachment")
	}

	if strings.TrimSpace(parsed.Text) != "See the attached file." {
		t.Errorf("Text: got %q, want %q", strings.TrimSpace(parsed.Text), "See the attached file.")
	}

	if strings.TrimSpace(parsed.HTML) != "<html><body><p>See the attached file.</p></body></html>" {
		t.Errorf("HTML: got %q, want %q", strings.TrimSpace(parsed.HTML), "<html><body><p>See the attached file.</p></body></html>")
	}

	// Node mailin would produce one attachment with the filename and content
	if len(parsed.Attachments) != 1 {
		t.Fatalf("Attachments: got %d, want 1", len(parsed.Attachments))
	}

	att := parsed.Attachments[0]
	if att.Filename != "report.txt" {
		t.Errorf("Attachment filename: got %q, want %q", att.Filename, "report.txt")
	}

	if strings.TrimSpace(string(att.Content)) != "This is the attachment content." {
		t.Errorf("Attachment content: got %q, want %q", strings.TrimSpace(string(att.Content)), "This is the attachment content.")
	}
}

// TestComparison_MissingHeaders verifies that missing headers produce empty strings,
// matching Node mailin behavior where undefined headers become empty/undefined.
func TestComparison_MissingHeaders(t *testing.T) {
	raw := buildMIMENoHeaders("Body with no headers.")

	parsed, err := ParseMail(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("ParseMail failed: %v", err)
	}

	// Node mailin would return undefined/empty for missing headers.
	// Go version should use empty strings for missing fields.
	if parsed.From != "" {
		t.Errorf("From: got %q, want empty string for missing header", parsed.From)
	}

	if parsed.To != "" {
		t.Errorf("To: got %q, want empty string for missing header", parsed.To)
	}

	if parsed.Subject != "" {
		t.Errorf("Subject: got %q, want empty string for missing header", parsed.Subject)
	}

	if parsed.Date != "" {
		t.Errorf("Date: got %q, want empty string for missing header", parsed.Date)
	}

	if strings.TrimSpace(parsed.Text) != "Body with no headers." {
		t.Errorf("Text: got %q, want %q", strings.TrimSpace(parsed.Text), "Body with no headers.")
	}

	if parsed.HTML != "" {
		t.Errorf("HTML: got %q, want empty string", parsed.HTML)
	}

	if len(parsed.Attachments) != 0 {
		t.Errorf("Attachments: got %d, want 0", len(parsed.Attachments))
	}
}
