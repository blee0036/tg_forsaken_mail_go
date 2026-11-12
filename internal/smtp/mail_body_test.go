package smtp

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

func TestParseMailBodyClassification(t *testing.T) {
	for _, tc := range []struct {
		name, raw, text, html string
		attachments           int
	}{
		{"case-insensitive HTML", "Content-Type: Text/HTML; Charset=UTF-8\r\n\r\n<p>中文</p>", "", "<p>中文</p>", 0},
		{"inline image", "Content-Type: multipart/mixed; boundary=x\r\n\r\n--x\r\nContent-Type: text/plain\r\n\r\nReal body\r\n--x\r\nContent-Type: image/png; name=logo.png\r\nContent-Disposition: inline; filename=logo.png\r\nContent-Transfer-Encoding: base64\r\n\r\nUE5H\r\n--x--\r\n", "Real body", "", 1},
		{"named inline text", "Content-Type: multipart/mixed; boundary=x\r\n\r\n--x\r\nContent-Type: text/plain\r\n\r\nReal body\r\n--x\r\nContent-Type: text/plain; name=notes.txt\r\nContent-Disposition: inline; filename=notes.txt\r\n\r\nAttachment, not the body\r\n--x--\r\n", "Real body", "", 1},
		{"calendar", "Content-Type: multipart/mixed; boundary=x\r\n\r\n--x\r\nContent-Type: text/plain\r\n\r\nReal body\r\n--x\r\nContent-Type: text/calendar\r\n\r\nBEGIN:VCALENDAR\r\n--x--\r\n", "Real body", "", 1},
		{"unknown root charset", "Content-Type: text/plain; charset=x-unknown\r\n\r\nReadable UTF-8 中文", "Readable UTF-8 中文", "", 0},
		{"unknown part charset", "Content-Type: multipart/alternative; boundary=x\r\n\r\n--x\r\nContent-Type: text/plain; charset=x-unknown\r\n\r\nReadable UTF-8 中文\r\n--x\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>HTML is still readable</p>\r\n--x--\r\n", "Readable UTF-8 中文", "<p>HTML is still readable</p>", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mail, err := ParseMail(strings.NewReader(tc.raw))
			if err != nil {
				t.Fatalf("ParseMail: %v", err)
			}
			if strings.TrimSpace(mail.Text) != tc.text || strings.TrimSpace(mail.HTML) != tc.html || len(mail.Attachments) != tc.attachments {
				t.Fatalf("got Text=%q HTML=%q attachments=%d, want %q %q %d", mail.Text, mail.HTML, len(mail.Attachments), tc.text, tc.html, tc.attachments)
			}
			if tc.name == "inline image" && (!mail.Attachments[0].InlineResource || string(mail.Attachments[0].Content) != "PNG") {
				t.Fatal("inline resource bytes or classification were lost")
			}
		})
	}
}

func TestParseMailMixedBodyPartsAreNotOverwritten(t *testing.T) {
	raw := "Content-Type: multipart/mixed; boundary=x\r\n\r\n--x\r\nContent-Type: text/plain\r\n\r\nFirst paragraph\r\n--x\r\nContent-Type: text/plain\r\n\r\nSecond paragraph\r\n--x--\r\n"
	mail, err := ParseMail(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mail.Text, "First paragraph") || !strings.Contains(mail.Text, "Second paragraph") {
		t.Fatalf("mixed body was overwritten: %q", mail.Text)
	}
}

func TestParseMailDecodesTransferEncodingAndLegacyCharset(t *testing.T) {
	// GBK bytes for 中文; transport encoding and charset must each be decoded once.
	raw := "Subject: =?UTF-8?B?5Lit5paH?=\r\nContent-Type: text/plain; charset=gbk\r\nContent-Transfer-Encoding: base64\r\n\r\n" + base64.StdEncoding.EncodeToString([]byte{0xd6, 0xd0, 0xce, 0xc4})
	mail, err := ParseMail(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if mail.Text != "中文" || mail.Subject != "中文" {
		t.Fatalf("decoded mail = Subject %q, Text %q", mail.Subject, mail.Text)
	}
}

func TestParseMailRelatedRootAndAlternativeSelection(t *testing.T) {
	for _, start := range []string{"", "; start=\"<root>\""} {
		t.Run("start"+start, func(t *testing.T) {
			resource := "--outer\r\nContent-Type: text/plain\r\nContent-ID: <resource>\r\n\r\nMust not become the body\r\n"
			root := "--outer\r\nContent-Type: multipart/alternative; boundary=inner\r\nContent-ID: <root>\r\n\r\n--inner\r\nContent-Type: text/plain\r\n\r\nPlain version\r\n--inner\r\nContent-Type: text/html\r\n\r\n<p>Old HTML</p>\r\n--inner\r\nContent-Type: text/html\r\n\r\n<p>Preferred HTML</p>\r\n--inner--\r\n"
			body := root + resource
			if start != "" {
				body = resource + root
			}
			raw := "Content-Type: multipart/related; boundary=outer" + start + "\r\n\r\n" + body + "--outer--\r\n"
			mail, err := ParseMail(strings.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			if mail.Text != "Plain version" || mail.HTML != "<p>Preferred HTML</p>" || len(mail.Attachments) != 1 {
				t.Fatalf("incorrect MIME root/alternative: %#v", mail)
			}
		})
	}
}

func TestParseMailMixedPlainIntroductionIsIncludedInHTML(t *testing.T) {
	raw := "Content-Type: multipart/mixed; boundary=x\r\n\r\n--x\r\nContent-Type: text/plain\r\n\r\nIntroduction <literal>\r\n--x\r\nContent-Type: text/html\r\n\r\n<p>HTML details</p>\r\n--x--\r\n"
	mail, err := ParseMail(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mail.HTML, "Introduction &lt;literal&gt;") || !strings.Contains(mail.HTML, "HTML details") {
		t.Fatalf("mixed content disappeared from the HTML representation: %q", mail.HTML)
	}
}

func TestParseMailHTMLMetaCharsetDoesNotDoubleDecode(t *testing.T) {
	for _, mimeCharset := range []string{"", "; charset=gbk"} {
		t.Run(mimeCharset, func(t *testing.T) {
			raw := "Content-Type: text/html" + mimeCharset + "\r\n\r\n<meta charset=gbk><p>" + string([]byte{0xd6, 0xd0, 0xce, 0xc4}) + "</p>"
			mail, err := ParseMail(strings.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(mail.HTML, "<p>中文</p>") {
				t.Fatalf("HTML was not decoded exactly once: %q", mail.HTML)
			}
		})
	}
}

func TestParseMailRejectsTruncatedTransferInsteadOfSilentlyLosingBody(t *testing.T) {
	raw := "Content-Type: text/plain\r\nContent-Transfer-Encoding: base64\r\n\r\nSGVsbG8$broken"
	if mail, err := ParseMail(strings.NewReader(raw)); err == nil {
		t.Fatalf("corrupt transfer was accepted as a complete mail: %#v", mail)
	}
}

func TestParseMailPartLimitDoesNotSilentlyAcceptPartialMail(t *testing.T) {
	var raw strings.Builder
	raw.WriteString("Content-Type: multipart/mixed; boundary=x\r\n\r\n")
	for i := 0; i < 101; i++ {
		fmt.Fprintf(&raw, "--x\r\nContent-Type: text/plain\r\n\r\nPart %d\r\n", i)
	}
	raw.WriteString("--x--\r\n")
	if mail, err := ParseMail(strings.NewReader(raw.String())); err == nil {
		t.Fatalf("over-limit mail was accepted as complete: %#v", mail)
	}
}
