package io

import (
	"strings"
	"testing"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	smtpmod "go-version-rewrite/internal/smtp"
)

func renderedMailMessage(t *testing.T, mail *smtpmod.ParsedMail, mode string) tgbotapi.MessageConfig {
	t.Helper()
	ioModule, _ := newTestIO(t)
	ioModule.domainToUser.Store("recipient.example", int64(101))
	mail.EnvelopeFrom = "sender@example.com"
	mail.EnvelopeRecipients = []string{"user@recipient.example"}
	sender := &recordingMailSender{}
	if mode == "multi" {
		ioModule.HandleMailMulti(mail, []DeliveryEndpoint{{Name: "test", Sender: sender}}, nil, time.Unix(0, 0))
	} else {
		ioModule.SetBot(sender)
		ioModule.HandleMail(mail)
	}
	if len(sender.messages) != 1 {
		t.Fatalf("sent %d notifications, want 1", len(sender.messages))
	}
	return sender.messages[0]
}

func entityText(t *testing.T, message tgbotapi.MessageConfig, entity tgbotapi.MessageEntity) string {
	t.Helper()
	units := utf16.Encode([]rune(message.Text))
	end := entity.Offset + entity.Length
	if entity.Offset < 0 || entity.Length <= 0 || end > len(units) {
		t.Fatalf("invalid entity range: %#v in %d UTF-16 units", entity, len(units))
	}
	if utf16.IsSurrogate(rune(units[entity.Offset])) && units[entity.Offset] >= 0xDC00 {
		t.Fatalf("entity starts inside a surrogate pair: %#v", entity)
	}
	if end < len(units) && units[end] >= 0xDC00 && units[end] <= 0xDFFF {
		t.Fatalf("entity ends inside a surrogate pair: %#v", entity)
	}
	return string(utf16.Decode(units[entity.Offset:end]))
}

func assertNativeMailEntities(t *testing.T, message tgbotapi.MessageConfig) {
	t.Helper()
	if message.ParseMode != "" {
		t.Fatalf("mail uses parse mode %q; content must be literal text with explicit entities", message.ParseMode)
	}
	if !utf8.ValidString(message.Text) || strings.ContainsRune(message.Text, utf8.RuneError) {
		t.Fatal("notification contains broken Unicode")
	}
	if n := len(utf16.Encode([]rune(message.Text))); n > 4096 {
		t.Fatalf("notification has %d UTF-16 units, Telegram allows 4096", n)
	}
	for _, entity := range message.Entities {
		entityText(t, message, entity)
	}
	for i, a := range message.Entities {
		for _, b := range message.Entities[i+1:] {
			aEnd, bEnd := a.Offset+a.Length, b.Offset+b.Length
			if a.Offset < b.Offset && b.Offset < aEnd && aEnd < bEnd {
				t.Fatalf("entities cross instead of nesting: %#v, %#v", a, b)
			}
			if a.Type == "code" || a.Type == "pre" {
				if a.Offset < bEnd && b.Offset < aEnd {
					t.Fatalf("code/pre overlaps another entity: %#v, %#v", a, b)
				}
			}
		}
	}
}

// These fixtures reproduce the structures in the Alibaba, GitHub Actions and
// GMI Cloud reports without retaining personal addresses or tracking tokens.
func TestMailNotificationAlibabaLinksAndLiteralPunctuation(t *testing.T) {
	for _, mode := range []string{"single", "multi"} {
		t.Run(mode, func(t *testing.T) {
			message := renderedMailMessage(t, &smtpmod.ParsedMail{
				From: "Alibaba Cloud <newsletter@example.com>", To: "user@recipient.example",
				Subject: "Qwen4 is Here — Token Plan from $3/mo, Flash Sale, Don't Miss Out",
				HTML: `<h1>Qwen4 is Here</h1><p><strong>First Month</strong> at $3 — Offer ends November 30</p>
<p><a href="https://example.com/offer(foo)?x=1&amp;y=2"><strong>Try It Now →</strong></a></p>
<p>Literal *star*, [bracket], file_name, C:\temp and 👩‍💻.</p><hr>
<a href="www.example.com?campaign=newsletter">www.example.com</a>`,
			}, mode)
			assertNativeMailEntities(t, message)
			for _, want := range []string{"Qwen4 is Here", "Try It Now →", `Literal *star*, [bracket], file_name, C:\temp and 👩‍💻.`} {
				if !strings.Contains(message.Text, want) {
					t.Errorf("missing literal content %q in %q", want, message.Text)
				}
			}
			links := map[string]string{}
			for _, entity := range message.Entities {
				if entity.Type == "text_link" {
					links[entityText(t, message, entity)] = entity.URL
				}
			}
			if links["Try It Now →"] != "https://example.com/offer(foo)?x=1&y=2" {
				t.Errorf("button URL was damaged: %#v", links)
			}
			if links["www.example.com"] != "https://www.example.com?campaign=newsletter" {
				t.Errorf("scheme-less newsletter URL was not normalized: %#v", links)
			}
		})
	}
}

func TestMailNotificationGitHubNoDoubleEscaping(t *testing.T) {
	message := renderedMailMessage(t, &smtpmod.ParsedMail{
		From: "Example <notifications@example.com>", To: "project <project@example.com>",
		Subject: "[example/cf_outlook] Run failed: Build & Release dist.zip - main (8be4328)",
		HTML: `<html><head><style>.foo { color:red; }</style><title>Hidden title</title></head><body>
<div style="display: none !important">Hidden preheader &zwnj;&nbsp;&shy;</div>
<p>[example/cf_outlook] Build &amp; Release dist.zip workflow run</p>
<p><strong>Build &amp; Release dist.zip: All jobs have failed</strong></p>
<a href="https://example.com/actions/runs/123">View workflow run</a>
<table><tr><th>Status</th><th>Job</th><th>Annotations</th></tr>
<tr><td>Failed in 13 seconds</td><td>Build &amp; Release ZIP</td><td>3</td></tr></table>
<script>not mail content</script></body></html>`,
	}, "single")
	assertNativeMailEntities(t, message)
	for _, want := range []string{"[example/cf_outlook] Build & Release", "View workflow run", "Status", "Failed in 13 seconds", "Build & Release ZIP"} {
		if !strings.Contains(message.Text, want) {
			t.Errorf("missing %q in %q", want, message.Text)
		}
	}
	for _, unwanted := range []string{`\[`, `\_`, "Hidden preheader", "Hidden title", ".foo", "not mail content", "| ---"} {
		if strings.Contains(message.Text, unwanted) {
			t.Errorf("leaked %q into notification", unwanted)
		}
	}
}

func TestMailNotificationChineseDoesNotSpendBudgetOnTrackingURLs(t *testing.T) {
	paragraph := "首屆 APAC GMI Cloud Day 在台北舉行，產業夥伴共同探索 AI 技術創新與企業應用。"
	trackingURL := "https://example.com/Ctc/" + strings.Repeat("tracking_0123456789", 80)
	body := `<h1>精彩回顧</h1><p>` + strings.Repeat(paragraph, 30) + `</p>`
	for _, label := range []string{"精彩回顧影片", "LinkedIn", "Facebook", "Instagram"} {
		body += `<p><a href="` + trackingURL + `">` + label + `</a></p>`
	}
	message := renderedMailMessage(t, &smtpmod.ParsedMail{
		Subject: "2026 APAC GMI Cloud Day 精彩回顧｜共探企業 AI 新未來", HTML: body,
		Text: strings.Repeat(paragraph, 30) + " Instagram (" + trackingURL + ")",
	}, "multi")
	assertNativeMailEntities(t, message)
	if strings.Count(message.Text, paragraph) != 30 || !strings.Contains(message.Text, "Instagram") {
		t.Fatal("readable newsletter content was lost to URL or byte budgeting")
	}
	if strings.Contains(message.Text, "https://") || strings.Contains(message.Text, "✂️") {
		t.Fatal("tracking URL was expanded into text or caused premature truncation")
	}
}

func TestMailNotificationUnicodeTruncationAndOversizedHeaders(t *testing.T) {
	for _, tc := range []struct{ name, subject, body string }{
		{"chinese", "中文長郵件", strings.Repeat("中文👩‍💻 * [ ] \\ ", 1000)},
		{"headers", strings.Repeat("很長的標題😀", 1000), "Important body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mail := &smtpmod.ParsedMail{From: "sender@example.com", To: "user@recipient.example", Subject: tc.subject, HTML: "<p><b>" + tc.body + "</b></p>"}
			message := renderedMailMessage(t, mail, "single")
			assertNativeMailEntities(t, message)
			if !strings.Contains(message.Text, "✂️") {
				t.Fatal("long notification has no truncation hint")
			}
			if tc.name == "headers" && !strings.Contains(message.Text, "Important body") {
				t.Fatal("oversized headers consumed the entire body budget")
			}
		})
	}
}

func TestMailNotificationPlainTextRemainsLiteral(t *testing.T) {
	body := "*literal* [repo_name] `code` <not-html> &amp; C:\\temp\n👨‍👩‍👧‍👦 中文"
	message := renderedMailMessage(t, &smtpmod.ParsedMail{Subject: "Plain", Text: body}, "single")
	assertNativeMailEntities(t, message)
	if !strings.Contains(message.Text, body) {
		t.Fatalf("plain mail was interpreted or escaped: %q", message.Text)
	}
}

func TestMailNotificationPlainNewsletterUsesShortClickableLabels(t *testing.T) {
	tracking := "https://example.com/redirect/(video)?token=" + strings.Repeat("tracking", 300)
	body := "精彩回顧影片\n\n2026_APAC_GMI_Cloud_Day_Event_and_Speeches_v5 (" + tracking + " )\n\n感謝所有來賓！\nLinkedIn (" + tracking + ")\nhttps://example.com/docs?campaign=newsletter\n"
	message := renderedMailMessage(t, &smtpmod.ParsedMail{Subject: "Plain-text newsletter", Text: body}, "single")
	assertNativeMailEntities(t, message)
	for _, want := range []string{"2026_APAC_GMI_Cloud_Day_Event_and_Speeches_v5", "感謝所有來賓！", "LinkedIn", "example.com/docs"} {
		if !strings.Contains(message.Text, want) {
			t.Errorf("missing %q in %q", want, message.Text)
		}
	}
	if strings.Contains(message.Text, "tracking") || strings.Contains(message.Text, "✂️") {
		t.Fatal("a text-only newsletter still spends its visible budget on tracking URLs")
	}
	links := map[string]string{}
	for _, entity := range message.Entities {
		if entity.Type == "text_link" {
			links[entityText(t, message, entity)] = entity.URL
		}
	}
	if links["LinkedIn"] != tracking || links["example.com/docs"] != "https://example.com/docs?campaign=newsletter" {
		t.Fatalf("plain-text links lost their original destinations: %#v", links)
	}
}

func TestMailNotificationNestedStylesCodeAndLists(t *testing.T) {
	message := renderedMailMessage(t, &smtpmod.ParsedMail{
		HTML: `<h2>Notes 👨‍👩‍👧‍👦</h2><p><a href="https://example.com/(release)"><b>Release <i>one</i></b></a></p>
<ol start="3"><li>
<div><p>First item</p></div><ul><li>Nested item</li></ul></li><li>Second item</li></ol>
<p><strong>Literal <code>a_*[]()\b</code> code</strong></p><pre><code>line 1
  line 2 &lt;x&gt;
</code></pre><pre>script</pre><pre>style</pre>`,
	}, "single")
	assertNativeMailEntities(t, message)
	for _, want := range []string{"Notes 👨‍👩‍👧‍👦", "3. First item", "  • Nested item", "4. Second item", "a_*[]()\\b", "line 1\n  line 2 <x>"} {
		if !strings.Contains(message.Text, want) {
			t.Errorf("missing %q in %q", want, message.Text)
		}
	}
	foundLink, foundCode, foundPre := false, false, false
	for _, entity := range message.Entities {
		text := entityText(t, message, entity)
		switch entity.Type {
		case "text_link":
			foundLink = text == "Release one"
		case "code":
			foundCode = text == `a_*[]()\b`
		case "pre":
			foundPre = foundPre || strings.Contains(text, "line 1\n  line 2 <x>")
		}
	}
	if !strings.Contains(message.Text, "script") || !strings.Contains(message.Text, "style") {
		t.Fatal("code text that resembles an HTML tag was discarded")
	}
	if !foundLink || !foundCode || !foundPre {
		t.Fatalf("missing link/code/pre entities: %#v", message.Entities)
	}
}

func TestMailTextPrefixPreservesGraphemeClusters(t *testing.T) {
	for _, cluster := range []string{"👩‍💻", "👨‍👩‍👧‍👦", "🇸🇬", "👍🏽", "e\u0301", "中文"} {
		for limit := 0; limit <= len(utf16.Encode([]rune(cluster)))+1; limit++ {
			got := mailTextPrefix(strings.Repeat(cluster, 3), limit)
			if !utf8.ValidString(got) || len(utf16.Encode([]rune(got))) > limit {
				t.Fatalf("invalid prefix %q for %q with budget %d", got, cluster, limit)
			}
			if cluster != "中文" && got != "" && got != cluster && got != cluster+cluster {
				t.Fatalf("split grapheme %q: prefix %q with budget %d", cluster, got, limit)
			}
		}
	}
}

func TestMailNotificationTruncatedPlainMailIncludesFullDocument(t *testing.T) {
	for _, mode := range []string{"single", "multi"} {
		t.Run(mode, func(t *testing.T) {
			ioModule, _ := newTestIO(t)
			ioModule.domainToUser.Store("recipient.example", int64(101))
			sender := &scriptedMailSender{}
			mail := &smtpmod.ParsedMail{
				From: "sender@example.com", To: "user@recipient.example", Subject: "Long text mail",
				EnvelopeFrom: "sender@example.com", EnvelopeRecipients: []string{"user@recipient.example"},
				Text: strings.Repeat("完整的中文和 emoji 👩‍💻。", 500) + "END OF ORIGINAL MAIL",
			}
			if mode == "single" {
				ioModule.SetBot(sender)
				ioModule.HandleMail(mail)
			} else {
				ioModule.HandleMailMulti(mail, []DeliveryEndpoint{{Name: "test", Sender: sender}}, nil, time.Unix(0, 0))
			}
			if len(sender.calls) != 2 {
				t.Fatalf("got %d sends, want preview plus complete text document", len(sender.calls))
			}
			message := sender.calls[0].(tgbotapi.MessageConfig)
			assertNativeMailEntities(t, message)
			document, ok := sender.calls[1].(tgbotapi.DocumentConfig)
			if !ok {
				t.Fatalf("second send is %T, want document", sender.calls[1])
			}
			file := document.File.(tgbotapi.FileBytes)
			if file.Name != "content.txt" || !strings.Contains(string(file.Bytes), mail.Text) {
				t.Fatal("full text document is missing original content")
			}
		})
	}
}
