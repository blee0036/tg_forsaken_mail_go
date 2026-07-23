package io

import (
	"fmt"
	"html"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/brianvoe/gofakeit/v6"
	"github.com/bwmarrin/snowflake"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"go-version-rewrite/internal/config"
	"go-version-rewrite/internal/db"
	"go-version-rewrite/internal/i18n"
	"go-version-rewrite/internal/lrucache"
	smtpmod "go-version-rewrite/internal/smtp"
)

// numericRegex matches pure numeric strings (including decimals), used by getTrueBlockData.
var numericRegex = regexp.MustCompile(`^[0-9]+\.?[0-9]*$`)

// emailRegex matches email addresses, used by HandleMail.
var emailRegex = regexp.MustCompile(`(?i)[\w._\-+]+@[\w._\-+]+`)

// mdImageRegex matches Markdown image syntax ![alt](url)
var mdImageRegex = regexp.MustCompile(`!\[[^\]]*\]\([^\)]*\)`)

// mdHeadingRegex matches Markdown heading prefixes (# ## ### etc.)
var mdHeadingRegex = regexp.MustCompile(`(?m)^#{1,6}\s+`)

// emptyLinkRegex matches empty-text Markdown links [](url)
var emptyLinkRegex = regexp.MustCompile(`\[\]\([^\)]*\)`)

// zeroWidthRegex matches zero-width characters and invisible Unicode used as preheader padding
var zeroWidthRegex = regexp.MustCompile(`[\x{200B}\x{200C}\x{200D}\x{FEFF}\x{034F}\x{00AD}]+`)

// IO is the core business logic module, corresponding to Node version modules/io.js.
type IO struct {
	domainMu       sync.Mutex
	domainToUser   sync.Map
	domainConflict sync.Map
	db             *db.DB
	bot            TelegramSender
	config         *config.Config
	blockDomain    *lrucache.Cache
	blockSender    *lrucache.Cache
	blockReceiver  *lrucache.Cache
	blockCache     *lrucache.Cache
	langCache      *lrucache.Cache
	snowflakeNode  *snowflake.Node
	mailRetry      mailRetryPolicy
	// UploadHTMLFunc is an optional function for uploading HTML content.
	// It takes HTML bytes and returns a uuid string (empty on failure) and an error.
	// Set this after creating the upload module (Task 7.1).
	UploadHTMLFunc func(htmlContent []byte) (string, error)
}

// New creates a new IO instance with initialized caches and snowflake node.
func New(database *db.DB, cfg *config.Config) *IO {
	node, err := snowflake.NewNode(1)
	if err != nil {
		log.Fatalf("failed to create snowflake node: %v", err)
	}

	return &IO{
		db:            database,
		config:        cfg,
		blockDomain:   lrucache.New(1024),
		blockSender:   lrucache.New(1024),
		blockReceiver: lrucache.New(1024),
		blockCache:    lrucache.New(9000),
		langCache:     lrucache.New(256),
		snowflakeNode: node,
		mailRetry:     defaultMailRetryPolicy(),
	}
}

// Init loads all domain mappings from the database into the in-memory map.
func (o *IO) Init() error {
	o.domainMu.Lock()
	defer o.domainMu.Unlock()

	domains, err := o.db.SelectAllDomain()
	if err != nil {
		return fmt.Errorf("failed to load domain mappings: %w", err)
	}

	for _, d := range domains {
		domain := normalizeDomain(d.Domain)
		if domain == "" {
			continue
		}
		if _, conflicted := o.domainConflict.Load(domain); conflicted {
			continue
		}
		if existing, loaded := o.domainToUser.LoadOrStore(domain, d.Tg); loaded && existing.(int64) != d.Tg {
			o.domainToUser.Delete(domain)
			o.domainConflict.Store(domain, struct{}{})
			log.Printf("conflicting case-insensitive domain bindings for %s; refusing to route until the database is corrected", domain)
		}
	}

	return nil
}

// SetBot sets the Telegram Bot instance.
func (o *IO) SetBot(bot TelegramSender) {
	o.bot = bot
}

// FormatMailNotification formats a parsed email into a Markdown notification string for Telegram.
// Strategy:
//  1. If HTML is available, convert to Markdown and clean for Telegram compatibility.
//  2. If md exceeds limit and text is empty → truncate md to 3800 + hint to view full email.
//  3. If md exceeds limit and text is non-empty → use text instead.
//  4. If text also exceeds limit → truncate text to 3800 + hint to view full email.
func (o *IO) FormatMailNotification(mail *smtpmod.ParsedMail, lang string, now ...time.Time) string {
	t := time.Now()
	if len(now) > 0 {
		t = now[0]
	}
	timeStr := FormatMailTime(mail.DateTime, mail.RawDate, lang, t)
	headers := "*From:* " + escapeMdV2(mail.From) + "\n" +
		"*To:* " + escapeMdV2(mail.To) + "\n"
	if mail.Cc != "" {
		headers += "*Cc:* " + escapeMdV2(mail.Cc) + "\n"
	}
	headers += "*Subject:* " + escapeMdV2(mail.Subject) + "\n" +
		"*Time:* " + escapeMdV2(timeStr)

	truncHint := "\n\n\\.\\.\\.✂️ " + escapeMdV2(i18n.Get("msg_truncated", lang))

	var body string
	if mail.HTML != "" {
		md, err := htmltomarkdown.ConvertString(mail.HTML)
		if err == nil {
			body = cleanMdForTelegram(strings.TrimSpace(md))
		}
	}

	// If html2md produced a result, check length
	if body != "" {
		full := headers + "\n\n" + body
		if len(full) <= 4000 {
			return full
		}
		// md too long
		if strings.TrimSpace(mail.Text) == "" {
			// No text fallback available, truncate md
			maxBody := 3800 - len(headers) - len("\n\n")
			if maxBody > 0 && len(body) > maxBody {
				body = body[:maxBody]
				// Fix unclosed bold markers from truncation
				body = fixUnclosedBold(body)
			}
			return headers + "\n\n" + body + truncHint
		}
		// Has text, fall through to text fallback
	}

	// Use plain text
	textBody := escapeMdV2(mail.Text)
	full := headers + "\n\n" + textBody
	if len(full) <= 4000 {
		return full
	}

	// Text also too long, truncate
	maxBody := 3800 - len(headers) - len("\n\n")
	if maxBody > 0 && len(textBody) > maxBody {
		textBody = textBody[:maxBody]
	}
	return headers + "\n\n" + textBody + truncHint
}

// escapeMdV2 escapes special characters for Telegram MarkdownV2 parse mode.
func escapeMdV2(s string) string {
	// Characters that must be escaped in MarkdownV2:
	// _ * [ ] ( ) ~ ` > # + - = | { } . !
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	)
	return replacer.Replace(s)
}

// cleanMdForTelegram takes html2md output and makes it Telegram MarkdownV2 compatible:
// - Removes image syntax ![alt](url) (Telegram can't render images in text)
// - Strips heading markers (# ## ###) keeping the text
// - Escapes special chars in plain text segments while preserving [text](url) links
// - Collapses excessive blank lines
func cleanMdForTelegram(md string) string {
	// Remove images (tracking pixels, logos, etc.)
	md = mdImageRegex.ReplaceAllString(md, "")

	// Remove empty-text links [](url) — leftover from image-in-link patterns like [![](img)](link)
	md = emptyLinkRegex.ReplaceAllString(md, "")

	// Remove zero-width/invisible characters (preheader padding junk)
	md = zeroWidthRegex.ReplaceAllString(md, "")

	// Remove heading markers, keep text
	md = mdHeadingRegex.ReplaceAllString(md, "")

	// Remove lines that are only whitespace (U+2002, U+00A0, regular spaces, tabs)
	lines := strings.Split(md, "\n")
	var cleaned []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip lines that are only invisible/whitespace chars
		if trimmed == "" || trimmed == "‌" || strings.Trim(trimmed, " \t\u00A0\u2002\u2003\u200B‌") == "" {
			cleaned = append(cleaned, "")
		} else {
			cleaned = append(cleaned, line)
		}
	}
	md = strings.Join(cleaned, "\n")

	// Collapse 3+ consecutive newlines into 2
	for strings.Contains(md, "\n\n\n") {
		md = strings.ReplaceAll(md, "\n\n\n", "\n\n")
	}

	md = strings.TrimSpace(md)

	// Now we need to escape special chars for MarkdownV2, but preserve [text](url) links.
	return escapeMdV2PreserveLinks(md)
}

// linkRegex matches Markdown links [text](url)
var linkRegex = regexp.MustCompile(`\[([^\]]+)\]\(([^\)]+)\)`)

// boldRegex matches **text** (bold in Markdown, also bold in Telegram MarkdownV2)
var boldRegex = regexp.MustCompile(`\*\*([^\*]+)\*\*`)

// italicRegex matches *text* (italic in Markdown, also italic in Telegram MarkdownV2)
var italicRegex = regexp.MustCompile(`(?:^|[^*])\*([^*]+)\*(?:[^*]|$)`)

// mdFormattingRegex matches all Markdown formatting we want to preserve: **bold**, *italic*, [link](url)
// We process them in order of appearance to handle interleaving correctly.
var mdTokenRegex = regexp.MustCompile(`\*\*[^\*]+\*\*|\[[^\]]+\]\([^\)]+\)`)

// escapeMdV2PreserveFormatting escapes MarkdownV2 special chars but keeps
// **bold**, *italic* (single), and [text](url) links intact.
func escapeMdV2PreserveLinks(s string) string {
	var result strings.Builder
	lastIdx := 0

	matches := mdTokenRegex.FindAllStringIndex(s, -1)
	for _, match := range matches {
		// Escape text before this token
		if match[0] > lastIdx {
			segment := s[lastIdx:match[0]]
			result.WriteString(escapeMdV2KeepSingleBold(segment))
		}
		token := s[match[0]:match[1]]
		if strings.HasPrefix(token, "[") {
			// It's a link [text](url)
			// Extract text and url
			submatches := linkRegex.FindStringSubmatch(token)
			if len(submatches) == 3 {
				result.WriteString("[")
				result.WriteString(escapeMdV2KeepSingleBold(submatches[1]))
				result.WriteString("](")
				result.WriteString(submatches[2])
				result.WriteString(")")
			} else {
				result.WriteString(escapeMdV2KeepSingleBold(token))
			}
		} else if strings.HasPrefix(token, "**") {
			// It's bold **text**
			inner := token[2 : len(token)-2]
			result.WriteString("*")
			result.WriteString(escapeMdV2NoBold(inner))
			result.WriteString("*")
		}
		lastIdx = match[1]
	}
	// Escape remaining text
	if lastIdx < len(s) {
		result.WriteString(escapeMdV2KeepSingleBold(s[lastIdx:]))
	}
	return result.String()
}

// singleBoldRegex matches *text* that is NOT part of **text**
var singleBoldRegex = regexp.MustCompile(`(?:^|[^*])\*([^*]+)\*(?:[^*]|$)`)

// escapeMdV2KeepSingleBold escapes all MdV2 special chars except preserves *text* as bold.
func escapeMdV2KeepSingleBold(s string) string {
	// Find *text* patterns (single asterisk bold/italic)
	var result strings.Builder
	lastIdx := 0

	// Simple approach: find pairs of single * that are not **
	inBold := false
	runes := []byte(s)
	i := 0
	segStart := 0

	for i < len(runes) {
		if runes[i] == '*' {
			// Check if it's ** (already handled by outer function)
			if i+1 < len(runes) && runes[i+1] == '*' {
				i += 2
				continue
			}
			if !inBold {
				// Opening *: escape everything before it
				result.WriteString(escapeMdV2NoBold(s[segStart:i]))
				result.WriteString("*")
				inBold = true
				segStart = i + 1
			} else {
				// Closing *: escape inner content, close bold
				result.WriteString(escapeMdV2NoBold(s[segStart:i]))
				result.WriteString("*")
				inBold = false
				segStart = i + 1
			}
		}
		i++
	}
	// Remaining text
	if segStart < len(s) {
		if inBold {
			// Unclosed *, treat the opening * as literal
			result.WriteString(escapeMdV2NoBold("*" + s[segStart:]))
		} else {
			result.WriteString(escapeMdV2NoBold(s[segStart:]))
		}
	}
	_ = lastIdx
	return result.String()
}

// escapeMdV2NoBold escapes all MarkdownV2 special characters including *.
func escapeMdV2NoBold(s string) string {
	return escapeMdV2(s)
}

// fixUnclosedBold checks if a truncated MarkdownV2 string has an odd number of
// unescaped * markers (meaning a bold/italic was cut in half). If so, removes
// the last unmatched * to prevent Telegram parse errors.
func fixUnclosedBold(s string) string {
	count := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '*' && (i == 0 || s[i-1] != '\\') {
			count++
		}
	}
	if count%2 == 0 {
		return s // balanced
	}
	// Remove the last unescaped *
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '*' && (i == 0 || s[i-1] != '\\') {
			return s[:i] + s[i+1:]
		}
	}
	return s
}

// HandleMail processes an incoming email: looks up the recipient domain,
// checks block lists, formats and sends the message via Telegram bot.
// Corresponds to the mailin.on('message', ...) handler in Node version io.js.
func (o *IO) HandleMail(mail *smtpmod.ParsedMail) {
	if mail == nil {
		return
	}
	for _, target := range o.mailTargets(mail) {
		delivery, ok := o.prepareMailDelivery(mail, target)
		if ok {
			o.handleMailDelivery(mail, delivery)
		}
	}
}

func (o *IO) handleMailDelivery(mail *smtpmod.ParsedMail, delivery mailDelivery) {
	tgID := delivery.tgID

	// Format email message using FormatMailNotification with user's language preference
	lang := o.GetUserLang(tgID)
	email := o.FormatMailNotification(mail, lang)

	keyboard := o.mailActionKeyboard(delivery)

	if o.bot != nil {
		o.deliverMailToSender(mail, delivery, o.bot, "primary", email, keyboard)
	}
}

// getBlockSet retrieves a block set from cache, or loads it from DB using the loader function.
// Returns the set (map[string]bool) or nil if loading fails.
func (o *IO) getBlockSet(tgKey string, c *lrucache.Cache, loader func() (map[string]bool, error)) map[string]bool {
	if val, found := c.Get(tgKey); found {
		return val.(map[string]bool)
	}
	set, err := loader()
	if err != nil {
		log.Printf("failed to load block list: %v", err)
		return nil
	}
	c.Set(tgKey, set)
	return set
}

// BindDefaultDomainWith generates a random domain using gofakeit, binds it to the user,
// and sends success messages via the given sender. Returns the domain string (empty if failed).
func (o *IO) BindDefaultDomainWith(sender TelegramSender, tgID int64) string {
	baseDomain := o.config.DefaultDomain()
	domain := ""

	o.domainMu.Lock()
	for tryTime := 0; tryTime < 5; tryTime++ {
		candidate := strings.ToLower(gofakeit.FirstName()+gofakeit.LastName()) + "." + baseDomain
		if _, exists := o.domainToUser.Load(candidate); exists {
			continue
		}
		if _, conflicted := o.domainConflict.Load(candidate); conflicted {
			continue
		}
		if _, err := o.db.InsertDomain(candidate, tgID); err != nil {
			o.domainMu.Unlock()
			log.Printf("failed to persist default domain binding for %s: %v", candidate, err)
			return ""
		}
		o.domainToUser.Store(candidate, tgID)
		domain = candidate
		break
	}
	o.domainMu.Unlock()

	if domain == "" {
		return ""
	}

	// Send English success message
	enMsg := "bind default domain : <code>" + domain + "</code> Success! \n\n" +
		"you can send any mail toward this domain. \n\n Example:<code>someone@" + domain + "</code> \n\n"
	// Send Chinese success message
	cnMsg := "绑定默认域名 : <code>" + domain + "</code> 成功! \n\n" +
		"你可以发送邮件到该域名下的任何邮箱 \n\n 例如：<code>someone@" + domain + "</code> \n\n"

	o.sendHTMLMessageWith(sender, tgID, enMsg)
	o.sendHTMLMessageWith(sender, tgID, cnMsg)

	return domain
}

// BindDefaultDomain is a convenience method that uses o.bot as the sender.
func (o *IO) BindDefaultDomain(tgID int64) string {
	return o.BindDefaultDomainWith(o.bot, tgID)
}

// BindDomainWith binds a domain to the user, sending replies via the given sender. Returns the reply message string.
func (o *IO) BindDomainWith(sender TelegramSender, tgID int64, domain string) string {
	domain = normalizeDomain(domain)
	o.domainMu.Lock()

	if _, conflicted := o.domainConflict.Load(domain); conflicted {
		o.domainMu.Unlock()
		msg := "This domain has conflicting bindings in the database!"
		o.sendMessageWith(sender, tgID, msg)
		return msg
	}
	if val, exists := o.domainToUser.Load(domain); exists {
		o.domainMu.Unlock()
		if val.(int64) == tgID {
			msg := "This domain has already bind on your account!"
			o.sendMessageWith(sender, tgID, msg)
			return msg
		}
		msg := "This domain has already bind on another account!"
		o.sendMessageWith(sender, tgID, msg)
		return msg
	}

	if _, err := o.db.InsertDomain(domain, tgID); err != nil {
		o.domainMu.Unlock()
		log.Printf("failed to persist domain binding for %s: %v", domain, err)
		msg := "Bind failed, please try again!"
		o.sendMessageWith(sender, tgID, msg)
		return msg
	}
	o.domainToUser.Store(domain, tgID)
	o.domainMu.Unlock()

	msg := "Bind Success!"
	o.sendMessageWith(sender, tgID, msg)
	return msg
}

// BindDomain is a convenience method that uses o.bot as the sender.
func (o *IO) BindDomain(tgID int64, domain string) string {
	return o.BindDomainWith(o.bot, tgID, domain)
}

// RemoveDomainWith removes a domain binding for the user, sending replies via the given sender. Returns the reply message string.
func (o *IO) RemoveDomainWith(sender TelegramSender, tgID int64, domain string) string {
	domain = normalizeDomain(domain)
	o.domainMu.Lock()

	if val, exists := o.domainToUser.Load(domain); exists {
		if val.(int64) == tgID {
			if _, err := o.db.DeleteDomain(domain); err != nil {
				o.domainMu.Unlock()
				log.Printf("failed to remove persisted domain binding for %s: %v", domain, err)
				msg := "Release failed, please try again!"
				o.sendMessageWith(sender, tgID, msg)
				return msg
			}
			o.domainToUser.Delete(domain)
			o.domainMu.Unlock()
			msg := "Release Success!"
			o.sendMessageWith(sender, tgID, msg)
			return msg
		}
	}
	o.domainMu.Unlock()

	msg := "This domain has not bind to your account!"
	o.sendMessageWith(sender, tgID, msg)
	return msg
}

// RemoveDomain is a convenience method that uses o.bot as the sender.
func (o *IO) RemoveDomain(tgID int64, domain string) string {
	return o.RemoveDomainWith(o.bot, tgID, domain)
}

// ListDomainWith lists all domains bound to the user, sending via the given sender. Returns the formatted message string.
func (o *IO) ListDomainWith(sender TelegramSender, tgID int64) string {
	msg := "<b>Your domain :</b> \n\n"
	count := 0

	o.domainToUser.Range(func(key, value interface{}) bool {
		if value.(int64) == tgID {
			count++
			msg += fmt.Sprintf("%d: <code> %s</code> \n", count, html.EscapeString(key.(string)))
		}
		return true
	})

	o.sendHTMLMessageWith(sender, tgID, msg)

	return msg
}

// ListDomain is a convenience method that uses o.bot as the sender.
func (o *IO) ListDomain(tgID int64) string {
	return o.ListDomainWith(o.bot, tgID)
}

// GetUserDomains returns the list of domains bound to the given user.
func (o *IO) GetUserDomains(tgID int64) []string {
	var domains []string
	o.domainToUser.Range(func(key, value interface{}) bool {
		if value.(int64) == tgID {
			domains = append(domains, key.(string))
		}
		return true
	})
	return domains
}

// GetAllDomainCount returns the number of domains bound to the given user.
func (o *IO) GetAllDomainCount(tgID int64) int {
	count := 0
	o.domainToUser.Range(func(key, value interface{}) bool {
		if value.(int64) == tgID {
			count++
		}
		return true
	})
	return count
}

// sendMessageWith sends a plain text message via the given sender.
func (o *IO) sendMessageWith(sender TelegramSender, tgID int64, text string) {
	if sender != nil {
		msg := tgbotapi.NewMessage(tgID, text)
		if _, err := sender.Send(msg); err != nil {
			log.Printf("failed to send message: %v", err)
		}
	}
}

// sendMessage is a convenience helper that sends a plain text message via o.bot.
func (o *IO) sendMessage(tgID int64, text string) {
	o.sendMessageWith(o.bot, tgID, text)
}

// sendHTMLMessageWith sends an HTML-formatted message via the given sender.
func (o *IO) sendHTMLMessageWith(sender TelegramSender, tgID int64, text string) {
	if sender != nil {
		msg := tgbotapi.NewMessage(tgID, text)
		msg.ParseMode = "HTML"
		if _, err := sender.Send(msg); err != nil {
			log.Printf("failed to send HTML message: %v", err)
		}
	}
}

// sendHTMLMessage is a convenience helper that sends an HTML-formatted message via o.bot.
func (o *IO) sendHTMLMessage(tgID int64, text string) {
	o.sendHTMLMessageWith(o.bot, tgID, text)
}

// getTrueBlockData checks if data is a pure numeric string. If so, retrieves the
// real data from blockCache (like Node's cache.take — get then delete). Otherwise
// returns data as-is.
func (o *IO) getTrueBlockData(data string) string {
	if numericRegex.MatchString(data) {
		if val, found := o.blockCache.Take(data); found {
			return val.(string)
		}
	}
	return data
}

// CheckButtonData checks if the combined key+data exceeds 64 characters. If so,
// generates a Snowflake ID, caches the data, and returns key + snowflakeID.
// Otherwise returns key + data directly.
func (o *IO) CheckButtonData(data string, key string) string {
	buttonData := key + " " + data
	if len(buttonData) >= 64 {
		snowflakeID := o.snowflakeNode.Generate().String()
		o.blockCache.Set(snowflakeID, data)
		return key + " " + snowflakeID
	}
	return buttonData
}

// BlockSenderWith blocks a sender for the given user, sending replies via the given sender. Returns the success message.
func (o *IO) BlockSenderWith(sender TelegramSender, tgID int64, senderAddr string) string {
	data := normalizeMailboxIdentity(o.getTrueBlockData(senderAddr))
	o.db.InsertBlockSender(data, tgID)
	o.blockSender.Delete(fmt.Sprintf("%d", tgID))

	msg := "Block sender " + data + " Success!"
	o.sendMessageWith(sender, tgID, msg)
	return msg
}

// BlockSender is a convenience method that uses o.bot as the sender.
func (o *IO) BlockSender(tgID int64, senderAddr string) string {
	return o.BlockSenderWith(o.bot, tgID, senderAddr)
}

// BlockDomainWith blocks a domain for the given user, sending replies via the given sender. Returns the success message.
func (o *IO) BlockDomainWith(sender TelegramSender, tgID int64, domain string) string {
	data := normalizeDomain(o.getTrueBlockData(domain))
	o.db.InsertBlockDomain(data, tgID)
	o.blockDomain.Delete(fmt.Sprintf("%d", tgID))

	msg := "Block domain " + data + " Success!"
	o.sendMessageWith(sender, tgID, msg)
	return msg
}

// BlockDomain is a convenience method that uses o.bot as the sender.
func (o *IO) BlockDomain(tgID int64, domain string) string {
	return o.BlockDomainWith(o.bot, tgID, domain)
}

// BlockReceiverWith blocks a receiver for the given user, sending replies via the given sender. Returns the success message.
func (o *IO) BlockReceiverWith(sender TelegramSender, tgID int64, receiver string) string {
	data := normalizeMailboxIdentity(o.getTrueBlockData(receiver))
	o.db.InsertBlockReceiver(data, tgID)
	o.blockReceiver.Delete(fmt.Sprintf("%d", tgID))

	msg := "Block receiver " + data + " Success!"
	o.sendMessageWith(sender, tgID, msg)
	return msg
}

// BlockReceiver is a convenience method that uses o.bot as the sender.
func (o *IO) BlockReceiver(tgID int64, receiver string) string {
	return o.BlockReceiverWith(o.bot, tgID, receiver)
}

// RemoveBlockSenderWith removes a sender block for the given user, sending replies via the given sender. Returns the success message.
func (o *IO) RemoveBlockSenderWith(sender TelegramSender, tgID int64, senderAddr string) string {
	senderAddr = normalizeMailboxIdentity(senderAddr)
	o.db.DeleteBlockSender(senderAddr, tgID)
	o.blockSender.Delete(fmt.Sprintf("%d", tgID))

	msg := "Remove block sender " + senderAddr + " Success!"
	o.sendMessageWith(sender, tgID, msg)
	return msg
}

// RemoveBlockSender is a convenience method that uses o.bot as the sender.
func (o *IO) RemoveBlockSender(tgID int64, senderAddr string) string {
	return o.RemoveBlockSenderWith(o.bot, tgID, senderAddr)
}

// RemoveBlockDomainWith removes a domain block for the given user, sending replies via the given sender. Returns the success message.
func (o *IO) RemoveBlockDomainWith(sender TelegramSender, tgID int64, domain string) string {
	domain = normalizeDomain(domain)
	o.db.DeleteBlockDomain(domain, tgID)
	o.blockDomain.Delete(fmt.Sprintf("%d", tgID))

	msg := "Remove block domain " + domain + " Success!"
	o.sendMessageWith(sender, tgID, msg)
	return msg
}

// RemoveBlockDomain is a convenience method that uses o.bot as the sender.
func (o *IO) RemoveBlockDomain(tgID int64, domain string) string {
	return o.RemoveBlockDomainWith(o.bot, tgID, domain)
}

// RemoveBlockReceiverWith removes a receiver block for the given user, sending replies via the given sender. Returns the success message.
func (o *IO) RemoveBlockReceiverWith(sender TelegramSender, tgID int64, receiver string) string {
	receiver = normalizeMailboxIdentity(receiver)
	o.db.DeleteBlockReceiver(receiver, tgID)
	o.blockReceiver.Delete(fmt.Sprintf("%d", tgID))

	msg := "Remove block receiver " + receiver + " Success!"
	o.sendMessageWith(sender, tgID, msg)
	return msg
}

// RemoveBlockReceiver is a convenience method that uses o.bot as the sender.
func (o *IO) RemoveBlockReceiver(tgID int64, receiver string) string {
	return o.RemoveBlockReceiverWith(o.bot, tgID, receiver)
}

// ListBlockSenderWith lists all blocked senders for the given user, sending via the given sender. Returns the formatted HTML message.
func (o *IO) ListBlockSenderWith(sender TelegramSender, tgID int64) string {
	msg := "<b>Your block sender :</b> \n\n"
	count := 0

	senders, err := o.db.SelectUserAllBlockSender(tgID)
	if err != nil {
		log.Printf("failed to query block senders: %v", err)
	} else {
		for _, info := range senders {
			count++
			msg += fmt.Sprintf("%d: <code> %s</code> \n", count, html.EscapeString(info.Sender))
		}
	}

	o.sendHTMLMessageWith(sender, tgID, msg)
	return msg
}

// ListBlockSender is a convenience method that uses o.bot as the sender.
func (o *IO) ListBlockSender(tgID int64) string {
	return o.ListBlockSenderWith(o.bot, tgID)
}

// ListBlockDomainWith lists all blocked domains for the given user, sending via the given sender. Returns the formatted HTML message.
func (o *IO) ListBlockDomainWith(sender TelegramSender, tgID int64) string {
	msg := "<b>Your block domain :</b> \n\n"
	count := 0

	domains, err := o.db.SelectUserAllBlockDomain(tgID)
	if err != nil {
		log.Printf("failed to query block domains: %v", err)
	} else {
		for _, info := range domains {
			count++
			msg += fmt.Sprintf("%d: <code> %s</code> \n", count, html.EscapeString(info.Domain))
		}
	}

	o.sendHTMLMessageWith(sender, tgID, msg)
	return msg
}

// ListBlockDomain is a convenience method that uses o.bot as the sender.
func (o *IO) ListBlockDomain(tgID int64) string {
	return o.ListBlockDomainWith(o.bot, tgID)
}

// ListBlockReceiverWith lists all blocked receivers for the given user, sending via the given sender. Returns the formatted HTML message.
func (o *IO) ListBlockReceiverWith(sender TelegramSender, tgID int64) string {
	msg := "<b>Your block receiver :</b> \n\n"
	count := 0

	receivers, err := o.db.SelectUserAllBlockReceiver(tgID)
	if err != nil {
		log.Printf("failed to query block receivers: %v", err)
	} else {
		for _, info := range receivers {
			count++
			msg += fmt.Sprintf("%d: <code> %s</code> \n", count, html.EscapeString(info.Receiver))
		}
	}

	o.sendHTMLMessageWith(sender, tgID, msg)
	return msg
}

// ListBlockReceiver is a convenience method that uses o.bot as the sender.
func (o *IO) ListBlockReceiver(tgID int64) string {
	return o.ListBlockReceiverWith(o.bot, tgID)
}

// GetBlockedSenders returns a slice of all blocked sender addresses for the given user.
func (o *IO) GetBlockedSenders(tgID int64) []string {
	senders, err := o.db.SelectUserAllBlockSender(tgID)
	if err != nil {
		log.Printf("failed to query block senders: %v", err)
		return nil
	}
	result := make([]string, 0, len(senders))
	for _, info := range senders {
		result = append(result, info.Sender)
	}
	return result
}

// GetBlockedDomains returns a slice of all blocked domain names for the given user.
func (o *IO) GetBlockedDomains(tgID int64) []string {
	domains, err := o.db.SelectUserAllBlockDomain(tgID)
	if err != nil {
		log.Printf("failed to query block domains: %v", err)
		return nil
	}
	result := make([]string, 0, len(domains))
	for _, info := range domains {
		result = append(result, info.Domain)
	}
	return result
}

// GetBlockedReceivers returns a slice of all blocked receiver addresses for the given user.
func (o *IO) GetBlockedReceivers(tgID int64) []string {
	receivers, err := o.db.SelectUserAllBlockReceiver(tgID)
	if err != nil {
		log.Printf("failed to query block receivers: %v", err)
		return nil
	}
	result := make([]string, 0, len(receivers))
	for _, info := range receivers {
		result = append(result, info.Receiver)
	}
	return result
}

// StoreCallbackData generates a Snowflake ID, stores the data in blockCache,
// and returns the Snowflake ID string. Used by Bot.encodeCallback when callback
// data exceeds Telegram's 64-byte limit.
func (o *IO) StoreCallbackData(data string) string {
	id := o.snowflakeNode.Generate().String()
	o.blockCache.Set(id, data)
	return id
}

// RetrieveCallbackData retrieves callback data from blockCache by its Snowflake ID.
// Returns the data and true if found, or empty string and false if not found.
// Unlike getTrueBlockData, this does NOT delete the entry (callback buttons may be
// clicked multiple times).
func (o *IO) RetrieveCallbackData(id string) (string, bool) {
	val, found := o.blockCache.Get(id)
	if !found {
		return "", false
	}
	return val.(string), true
}

// SendAllWith sends a message to all unique users that have domain bindings, via the given sender.
func (o *IO) SendAllWith(sender TelegramSender, msg string) {
	allUsers := make(map[int64]bool)
	o.domainToUser.Range(func(key, value interface{}) bool {
		allUsers[value.(int64)] = true
		return true
	})

	for tgID := range allUsers {
		o.sendHTMLMessageWith(sender, tgID, msg)
	}
}

// SendAll is a convenience method that uses o.bot as the sender.
func (o *IO) SendAll(msg string) {
	o.SendAllWith(o.bot, msg)
}

// GetUserLang returns the stored language preference for a user.
// Uses LRU cache to avoid repeated DB queries.
// Returns empty string if not set (caller should fall back to Telegram's language_code).
func (o *IO) GetUserLang(tgID int64) string {
	key := fmt.Sprintf("%d", tgID)
	if val, found := o.langCache.Get(key); found {
		return val.(string)
	}
	lang := o.db.GetUserLang(tgID)
	o.langCache.Set(key, lang)
	return lang
}

// SetUserLang stores the language preference for a user and updates the cache.
func (o *IO) SetUserLang(tgID int64, lang string) error {
	err := o.db.SetUserLang(tgID, lang)
	if err == nil {
		key := fmt.Sprintf("%d", tgID)
		o.langCache.Set(key, lang)
	}
	return err
}
