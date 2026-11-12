package io

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rivo/uniseg"
	"golang.org/x/net/html"

	"go-version-rewrite/internal/i18n"
	smtpmod "go-version-rewrite/internal/smtp"
)

const (
	mailMessageLimit     = 4096 // Telegram text/entity offsets are measured in UTF-16 units.
	mailHeaderValueLimit = 512  // Leave room for the body even with very large recipient lists.
)

type mailStyle uint8

const (
	mailBold mailStyle = 1 << iota
	mailItalic
	mailUnderline
	mailStrike
	mailCode
	mailPre
)

// mailRun contains literal text, never Markdown or HTML. Formatting and URLs are
// kept separately so punctuation and long tracking URLs cannot affect parsing
// or consume the visible-text budget.
type mailRun struct {
	text  string
	style mailStyle
	link  string
}

type mailContent struct {
	runs      []mailRun
	truncated bool
}

func (c *mailContent) append(text string, style mailStyle, link string) {
	if text == "" {
		return
	}
	if style&(mailCode|mailPre) != 0 {
		// Telegram does not allow other entities inside code/pre entities.
		style &= mailCode | mailPre
		link = ""
	}
	if n := len(c.runs); n > 0 && c.runs[n-1].style == style && c.runs[n-1].link == link {
		c.runs[n-1].text += text
		return
	}
	c.runs = append(c.runs, mailRun{text: text, style: style, link: link})
}

func (c *mailContent) appendContent(other mailContent) {
	for _, run := range other.runs {
		c.append(run.text, run.style, run.link)
	}
}

func (c mailContent) text() string {
	var b strings.Builder
	for _, run := range c.runs {
		b.WriteString(run.text)
	}
	return b.String()
}

func mailUTF16Len(text string) int {
	n := 0
	for _, r := range text {
		n++
		if r > 0xFFFF {
			n++
		}
	}
	return n
}

// mailTextPrefix never splits a UTF-8 sequence, a surrogate pair, or an emoji /
// combining-character grapheme. The budget is independent of serialized markup.
func mailTextPrefix(text string, limit int) string {
	if mailUTF16Len(text) <= limit {
		return text
	}
	g := uniseg.NewGraphemes(text)
	end, used := 0, 0
	for g.Next() {
		n := mailUTF16Len(g.Str())
		if used+n > limit {
			break
		}
		used += n
		_, end = g.Positions()
	}
	return text[:end]
}

func (c mailContent) prefix(limit int) mailContent {
	remaining := len(mailTextPrefix(c.text(), limit))
	var result mailContent
	for _, run := range c.runs {
		if remaining == 0 {
			break
		}
		text := run.text
		if len(text) > remaining {
			text = text[:remaining]
		}
		result.append(text, run.style, run.link)
		remaining -= len(text)
	}
	return result
}

func (c *mailContent) trimSpace() {
	for len(c.runs) > 0 {
		c.runs[0].text = strings.TrimLeftFunc(c.runs[0].text, unicode.IsSpace)
		if c.runs[0].text != "" {
			break
		}
		c.runs = c.runs[1:]
	}
	for n := len(c.runs); n > 0; n = len(c.runs) {
		c.runs[n-1].text = strings.TrimRightFunc(c.runs[n-1].text, unicode.IsSpace)
		if c.runs[n-1].text != "" {
			break
		}
		c.runs = c.runs[:n-1]
	}
}

func formatMailNotification(mail *smtpmod.ParsedMail, lang string, now ...time.Time) mailContent {
	t := time.Now()
	if len(now) > 0 {
		t = now[0]
	}
	var content mailContent
	headerTruncated := false
	addHeader := func(label, value string) {
		value = strings.Join(strings.Fields(value), " ")
		if mailUTF16Len(value) > mailHeaderValueLimit {
			value = mailTextPrefix(value, mailHeaderValueLimit-1) + "…"
			headerTruncated = true
		}
		content.append(label+":", mailBold, "")
		content.append(" "+value+"\n", 0, "")
	}
	addHeader("From", mail.From)
	addHeader("To", mail.To)
	if mail.Cc != "" {
		addHeader("Cc", mail.Cc)
	}
	addHeader("Subject", mail.Subject)
	addHeader("Time", FormatMailTime(mail.DateTime, mail.RawDate, lang, t))

	body := renderMailHTML(mail.HTML)
	if len(body.runs) == 0 {
		body = renderPlainMailText(mail.Text)
	}
	body.trimSpace()
	if len(body.runs) > 0 {
		content.append("\n", 0, "")
		content.appendContent(body)
	}
	if !headerTruncated && mailUTF16Len(content.text()) <= mailMessageLimit {
		return content
	}
	hint := "\n\n…✂️ " + i18n.Get("msg_truncated", lang)
	content = content.prefix(mailMessageLimit - mailUTF16Len(hint))
	content.trimSpace()
	content.append(hint, 0, "")
	content.truncated = true
	return content
}

func renderPlainMailText(source string) mailContent {
	var content mailContent
	for _, line := range strings.SplitAfter(strings.ReplaceAll(source, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		label, target := "", ""
		// Newsletter text alternatives commonly put one named link on a line:
		// "LinkedIn (https://...tracking...)". Keep the name and URL, not the
		// tracking string on screen. Other plain text is not parsed as markup.
		if index := strings.LastIndex(trimmed, " ("); index > 0 && strings.HasSuffix(trimmed, ")") {
			label = trimmed[:index]
			target = mailLink(trimmed[index+2 : len(trimmed)-1])
		} else if link := mailLink(trimmed); strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://") {
			target = link
			u, _ := url.Parse(link)
			label = u.Hostname()
			// A short path carries useful meaning; a tracking query does not.
			if mailUTF16Len(u.EscapedPath()) <= 60 {
				label += u.EscapedPath()
			}
		}
		if target == "" {
			content.append(line, 0, "")
			continue
		}
		content.append(label, 0, target)
		if strings.HasSuffix(line, "\n") {
			content.append("\n", 0, "")
		}
	}
	return content
}

func (c mailContent) message(chatID int64) tgbotapi.MessageConfig {
	message := tgbotapi.NewMessage(chatID, c.text())
	message.DisableWebPagePreview = true
	offset := 0
	lastLink := -1
	for _, run := range c.runs {
		length := mailUTF16Len(run.text)
		if run.link != "" {
			// Keep a label clickable as one link even when it contains nested styles.
			if lastLink >= 0 && message.Entities[lastLink].URL == run.link &&
				message.Entities[lastLink].Offset+message.Entities[lastLink].Length == offset {
				message.Entities[lastLink].Length += length
			} else {
				lastLink = len(message.Entities)
				message.Entities = append(message.Entities, tgbotapi.MessageEntity{Type: "text_link", Offset: offset, Length: length, URL: run.link})
			}
		}
		for _, format := range []struct {
			style mailStyle
			kind  string
		}{
			{mailBold, "bold"}, {mailItalic, "italic"}, {mailUnderline, "underline"},
			{mailStrike, "strikethrough"}, {mailCode, "code"}, {mailPre, "pre"},
		} {
			if run.style&format.style != 0 {
				message.Entities = append(message.Entities, tgbotapi.MessageEntity{Type: format.kind, Offset: offset, Length: length})
			}
		}
		offset += length
	}
	// Outer entities precede contained ones; run-local styles can never cross.
	sort.SliceStable(message.Entities, func(i, j int) bool {
		a, b := message.Entities[i], message.Entities[j]
		if a.Offset != b.Offset {
			return a.Offset < b.Offset
		}
		return a.Length > b.Length
	})
	return message
}

// markdownV2 preserves FormatMailNotification's existing string-returning API.
// Delivery itself uses message() and never parses this serialization.
func (c mailContent) markdownV2() string {
	var b strings.Builder
	for i, run := range c.runs {
		if i > 0 {
			// CR is Telegram's documented delimiter for ambiguous adjacent markers.
			b.WriteByte('\r')
		}
		text := escapeMdV2(run.text)
		if run.style&(mailCode|mailPre) != 0 {
			text = strings.NewReplacer("\\", "\\\\", "`", "\\`").Replace(run.text)
			if run.style&mailPre != 0 {
				text = "```\n" + text + "\n```"
			} else {
				text = "`" + text + "`"
			}
		} else {
			for _, format := range []struct {
				style  mailStyle
				marker string
			}{{mailStrike, "~"}, {mailUnderline, "__"}, {mailItalic, "_"}, {mailBold, "*"}} {
				if run.style&format.style != 0 {
					if strings.HasPrefix(text, "_") {
						text = "\r" + text + "\r"
					}
					text = format.marker + text + format.marker
				}
			}
			if run.link != "" {
				link := strings.NewReplacer("\\", "\\\\", ")", "\\)").Replace(run.link)
				text = "[" + text + "](" + link + ")"
			}
		}
		b.WriteString(text)
	}
	return b.String()
}

type mailHTMLRenderer struct {
	content    mailContent
	space      bool
	spaceAt    mailRun
	breaks     int
	listMarker bool
	lists      []int // -1 for unordered lists; otherwise the next ordered-list index.
}

func renderMailHTML(source string) mailContent {
	if strings.TrimSpace(source) == "" {
		return mailContent{}
	}
	doc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return mailContent{}
	}
	renderer := &mailHTMLRenderer{}
	renderer.walk(doc, 0, "")
	renderer.content.trimSpace()
	return renderer.content
}

func (r *mailHTMLRenderer) text(text string, style mailStyle, link string) {
	// Invisible padding-only text nodes are common in newsletter preheaders.
	if strings.TrimFunc(text, func(c rune) bool {
		return unicode.IsSpace(c) || strings.ContainsRune("\u200b\u200c\u200d\ufeff\u034f\u00ad", c)
	}) == "" {
		r.pendingSpace(style, link)
		return
	}
	var b strings.Builder
	for _, c := range text {
		if unicode.IsSpace(c) {
			r.pendingSpace(style, link)
			continue
		}
		if unicode.IsControl(c) || strings.ContainsRune("\u200b\ufeff\u034f\u00ad", c) {
			continue
		}
		if r.space && !r.listMarker && r.breaks == 0 && (len(r.content.runs) > 0 || b.Len() > 0) {
			if r.spaceAt.style == style && r.spaceAt.link == link {
				b.WriteByte(' ')
			} else {
				r.content.append(b.String(), style, link)
				b.Reset()
				r.content.append(" ", r.spaceAt.style, r.spaceAt.link)
			}
		}
		r.space = false
		r.breaks = 0
		r.listMarker = false
		b.WriteRune(c)
	}
	r.content.append(b.String(), style, link)
}

func (r *mailHTMLRenderer) pendingSpace(style mailStyle, link string) {
	if !r.space {
		r.spaceAt = mailRun{style: style, link: link}
	}
	r.space = true
}

func (r *mailHTMLRenderer) lineBreak(count int) {
	if r.listMarker {
		r.space = false
		return
	}
	if len(r.content.runs) > 0 && count > r.breaks {
		r.content.append(strings.Repeat("\n", count-r.breaks), 0, "")
		r.breaks = count
	}
	r.space = false
}

func mailAttr(node *html.Node, name string) string {
	for _, attr := range node.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}

func mailCSS(node *html.Node) map[string]string {
	result := map[string]string{}
	for _, declaration := range strings.Split(mailAttr(node, "style"), ";") {
		key, value, ok := strings.Cut(declaration, ":")
		if ok {
			value, _, _ = strings.Cut(strings.ToLower(value), "!important")
			result[strings.TrimSpace(strings.ToLower(key))] = strings.TrimSpace(value)
		}
	}
	return result
}

func hiddenMailNode(node *html.Node) bool {
	for _, attr := range node.Attr {
		if attr.Key == "hidden" {
			return true
		}
	}
	css := mailCSS(node)
	return css["display"] == "none" || css["visibility"] == "hidden" ||
		css["visibility"] == "collapse" || css["mso-hide"] == "all"
}

func mailLink(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "//") {
		value = "https:" + value
	} else if strings.HasPrefix(strings.ToLower(value), "www.") {
		value = "https://" + value
	}
	u, err := url.Parse(value)
	if err != nil {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Hostname() != "" {
			return u.String()
		}
	case "mailto", "tel":
		if u.Opaque != "" {
			return u.String()
		}
	}
	return ""
}

func (r *mailHTMLRenderer) walk(node *html.Node, style mailStyle, link string) {
	if node.Type == html.TextNode {
		r.text(node.Data, style, link)
		return
	}
	if node.Type != html.ElementNode && node.Type != html.DocumentNode {
		return
	}
	if hiddenMailNode(node) {
		return
	}
	block := 0
	switch node.Data {
	case "head", "script", "style", "title", "meta", "link", "base", "iframe", "object", "template", "noscript", "svg", "canvas":
		return
	case "p", "article", "section", "main", "header", "footer", "blockquote":
		block = 2
	case "div", "address", "table", "tr", "dl", "dt", "dd":
		block = 1
	case "h1", "h2", "h3", "h4", "h5", "h6":
		block = 2
		style |= mailBold
	case "br":
		r.lineBreak(1)
		return
	case "hr":
		r.lineBreak(2)
		r.text("────────", 0, "")
		r.lineBreak(2)
		return
	case "strong", "b":
		style |= mailBold
	case "em", "i":
		style |= mailItalic
	case "u", "ins":
		style |= mailUnderline
	case "s", "strike", "del":
		style |= mailStrike
	case "code", "tt", "kbd", "samp":
		style = mailCode
	case "pre":
		r.lineBreak(2)
		var text strings.Builder
		var collect func(*html.Node)
		collect = func(n *html.Node) {
			if n.Type == html.ElementNode && (hiddenMailNode(n) || n.Data == "script" || n.Data == "style") {
				return
			}
			if n.Type == html.TextNode {
				text.WriteString(n.Data)
			} else if n.Type == html.ElementNode && n.Data == "br" {
				text.WriteByte('\n')
			}
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				collect(child)
			}
		}
		collect(node)
		value := strings.ReplaceAll(text.String(), "\r\n", "\n")
		r.content.append(value, mailPre, "")
		if value != "" {
			r.listMarker = false
		}
		r.breaks = len(value) - len(strings.TrimRight(value, "\n"))
		r.lineBreak(2)
		return
	case "a":
		link = mailLink(mailAttr(node, "href"))
	case "img":
		if mailAttr(node, "width") == "1" || mailAttr(node, "height") == "1" {
			return
		}
		alt := strings.TrimSpace(mailAttr(node, "alt"))
		switch strings.ToLower(alt) {
		case "", "logo", "image", "spacer", "pixel":
			return
		}
		r.text(alt, style, link)
		return
	case "ul", "ol":
		r.lineBreak(1)
		next := -1
		if node.Data == "ol" {
			next = 1
			if start, err := strconv.Atoi(mailAttr(node, "start")); err == nil {
				next = start
			}
		}
		r.lists = append(r.lists, next)
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			r.walk(child, style, link)
		}
		r.lists = r.lists[:len(r.lists)-1]
		r.lineBreak(1)
		return
	case "li":
		r.listMarker = false
		r.lineBreak(1)
		prefix := "• "
		if n := len(r.lists); n > 0 && r.lists[n-1] >= 0 {
			prefix = strconv.Itoa(r.lists[n-1]) + ". "
			r.lists[n-1]++
		}
		// Emit the marker without a block break between it and a child <p>.
		r.content.append(strings.Repeat("  ", max(0, len(r.lists)-1))+prefix, 0, "")
		r.breaks = 0
		r.space = false
		r.listMarker = true
		block = 1
	case "td", "th":
		if node.PrevSibling != nil && r.breaks == 0 {
			r.text(" | ", 0, "")
		}
		if node.Data == "th" {
			style |= mailBold
		}
	}
	css := mailCSS(node)
	if css["font-weight"] == "bold" || css["font-weight"] == "bolder" {
		style |= mailBold
	} else if weight, err := strconv.Atoi(css["font-weight"]); err == nil && weight >= 600 {
		style |= mailBold
	}
	if css["font-style"] == "italic" {
		style |= mailItalic
	}
	if block > 0 && node.Data != "li" {
		r.lineBreak(block)
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		r.walk(child, style, link)
	}
	if block > 0 {
		if node.Data == "li" {
			r.listMarker = false
		}
		r.lineBreak(block)
	}
}
