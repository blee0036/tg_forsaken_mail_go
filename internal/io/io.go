package io

import (
	"fmt"
	"html"
	"log"
	"regexp"
	"strings"
	"sync"

	"github.com/brianvoe/gofakeit/v6"
	"github.com/bwmarrin/snowflake"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"go-version-rewrite/internal/config"
	"go-version-rewrite/internal/db"
	"go-version-rewrite/internal/lrucache"
	smtpmod "go-version-rewrite/internal/smtp"
)

// numericRegex matches pure numeric strings (including decimals), used by getTrueBlockData.
var numericRegex = regexp.MustCompile(`^[0-9]+\.?[0-9]*$`)

// emailRegex matches email addresses, used by HandleMail.
var emailRegex = regexp.MustCompile(`(?i)[\w._\-+]+@[\w._\-+]+`)

// IO is the core business logic module, corresponding to Node version modules/io.js.
type IO struct {
	domainToUser  sync.Map
	db            *db.DB
	bot           *tgbotapi.BotAPI
	config        *config.Config
	blockDomain   *lrucache.Cache
	blockSender   *lrucache.Cache
	blockReceiver *lrucache.Cache
	blockCache    *lrucache.Cache
	langCache     *lrucache.Cache
	snowflakeNode *snowflake.Node
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
	}
}

// Init loads all domain mappings from the database into the in-memory map.
func (o *IO) Init() error {
	domains, err := o.db.SelectAllDomain()
	if err != nil {
		return fmt.Errorf("failed to load domain mappings: %w", err)
	}

	for _, d := range domains {
		o.domainToUser.Store(d.Domain, d.Tg)
	}

	return nil
}

// SetBot sets the Telegram Bot instance.
func (o *IO) SetBot(bot *tgbotapi.BotAPI) {
	o.bot = bot
}

// FormatMailNotification formats a parsed email into an HTML notification string.
// Uses bold field labels for From, To, Subject, and Time.
// If the total message (headers + body) exceeds 4000 characters, truncates to headers only.
// The lang parameter is reserved for future use (button labels are handled in HandleMail).
func (o *IO) FormatMailNotification(mail *smtpmod.ParsedMail, lang string) string {
	headers := "<b>From:</b> " + html.EscapeString(mail.From) + "\n" +
		"<b>To:</b> " + html.EscapeString(mail.To) + "\n" +
		"<b>Subject:</b> " + html.EscapeString(mail.Subject) + "\n" +
		"<b>Time:</b> " + html.EscapeString(mail.Date)

	full := headers + "\n\n" + html.EscapeString(mail.Text)

	if len(full) > 4000 {
		return headers
	}
	return full
}

// HandleMail processes an incoming email: looks up the recipient domain,
// checks block lists, formats and sends the message via Telegram bot.
// Corresponds to the mailin.on('message', ...) handler in Node version io.js.
func (o *IO) HandleMail(mail *smtpmod.ParsedMail) {
	to := strings.ToLower(mail.To)
	matches := emailRegex.FindString(to)
	if matches == "" {
		return
	}
	receiver := matches
	mailPart := strings.SplitN(receiver, "@", 2)
	if len(mailPart) != 2 {
		log.Printf("error domain %s", to)
		return
	}
	domain := mailPart[1]

	val, exists := o.domainToUser.Load(domain)
	if !exists {
		return
	}
	tgID := val.(int64)

	from := strings.ToLower(mail.From)
	senderMatch := emailRegex.FindString(from)
	if senderMatch == "" {
		return
	}
	sender := senderMatch
	senderPart := strings.SplitN(sender, "@", 2)
	if len(senderPart) != 2 {
		log.Printf("error sender domain %s", from)
		return
	}
	sendDomain := senderPart[1]

	tgKey := fmt.Sprintf("%d", tgID)

	// Check block receiver
	receiverBlockMatch := o.getBlockSet(tgKey, o.blockReceiver, func() (map[string]bool, error) {
		list, err := o.db.SelectUserAllBlockReceiver(tgID)
		if err != nil {
			return nil, err
		}
		set := make(map[string]bool)
		for _, info := range list {
			set[info.Receiver] = true
		}
		return set, nil
	})
	if receiverBlockMatch != nil && receiverBlockMatch[receiver] {
		return
	}

	// Check block domain
	domainBlockMatch := o.getBlockSet(tgKey, o.blockDomain, func() (map[string]bool, error) {
		list, err := o.db.SelectUserAllBlockDomain(tgID)
		if err != nil {
			return nil, err
		}
		set := make(map[string]bool)
		for _, info := range list {
			set[info.Domain] = true
		}
		return set, nil
	})
	if domainBlockMatch != nil && domainBlockMatch[sendDomain] {
		return
	}

	// Check block sender
	senderBlockMatch := o.getBlockSet(tgKey, o.blockSender, func() (map[string]bool, error) {
		list, err := o.db.SelectUserAllBlockSender(tgID)
		if err != nil {
			return nil, err
		}
		set := make(map[string]bool)
		for _, info := range list {
			set[info.Sender] = true
		}
		return set, nil
	})
	if senderBlockMatch != nil && senderBlockMatch[sender] {
		return
	}

	// Format email message using FormatMailNotification (default to English)
	email := o.FormatMailNotification(mail, "en")

	// Create inline keyboard with block buttons
	blockSenderData := o.CheckButtonData(sender, "block_sender")
	blockReceiverData := o.CheckButtonData(receiver, "block_receiver")
	blockDomainData := o.CheckButtonData(sendDomain, "block_domain")

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Block Sender", blockSenderData),
			tgbotapi.NewInlineKeyboardButtonData("Block Receiver", blockReceiverData),
			tgbotapi.NewInlineKeyboardButtonData("Block Domain", blockDomainData),
		),
	)

	// Send message with inline keyboard using HTML parse mode
	if o.bot != nil {
		msg := tgbotapi.NewMessage(tgID, email)
		msg.ParseMode = "HTML"
		msg.ReplyMarkup = keyboard
		if _, err := o.bot.Send(msg); err != nil {
			log.Printf("failed to send mail message with HTML mode: %v\nMessage content:\n%s", err, email)
			// Retry without HTML parse mode as fallback
			msg.ParseMode = ""
			if _, err := o.bot.Send(msg); err != nil {
				log.Printf("failed to send mail message (fallback): %v", err)
			}
		}

		// Send attachments
		for _, attachment := range mail.Attachments {
			if len(attachment.Content) > 0 {
				doc := tgbotapi.NewDocument(tgID, tgbotapi.FileBytes{
					Name:  attachment.Filename,
					Bytes: attachment.Content,
				})
				if _, err := o.bot.Send(doc); err != nil {
					log.Printf("failed to send attachment: %v", err)
				}
			}
		}

		// Handle HTML content
		if mail.HTML != "" {
			htmlBytes := []byte(mail.HTML)
			fileBytes := tgbotapi.FileBytes{
				Name:  "content.html",
				Bytes: htmlBytes,
			}

			var uploadUUID string
			if o.config.UploadURL != "" && o.UploadHTMLFunc != nil {
				uuid, err := o.UploadHTMLFunc(htmlBytes)
				if err != nil {
					log.Printf("failed to upload HTML: %v", err)
				} else {
					uploadUUID = uuid
				}
			}

			doc := tgbotapi.NewDocument(tgID, fileBytes)
			if uploadUUID != "" {
				viewURL := o.config.UploadURL + "/mail/" + uploadUUID
				docKeyboard := tgbotapi.NewInlineKeyboardMarkup(
					tgbotapi.NewInlineKeyboardRow(
						tgbotapi.NewInlineKeyboardButtonURL("View Directly", viewURL),
					),
				)
				doc.ReplyMarkup = docKeyboard
			}
			if _, err := o.bot.Send(doc); err != nil {
				log.Printf("failed to send HTML document: %v", err)
			}
		}
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

// BindDefaultDomain generates a random domain using gofakeit, binds it to the user,
// and sends success messages via bot. Returns the domain string (empty if failed).
func (o *IO) BindDefaultDomain(tgID int64) string {
	tryTime := 0
	domain := strings.ToLower(gofakeit.FirstName()+gofakeit.LastName()) + "." + o.config.MailDomain

	for tryTime < 5 {
		if _, exists := o.domainToUser.Load(domain); exists {
			tryTime++
			domain = strings.ToLower(gofakeit.FirstName()+gofakeit.LastName()) + "." + o.config.MailDomain
			continue
		}
		break
	}

	if tryTime >= 5 {
		return ""
	}

	o.domainToUser.Store(domain, tgID)
	o.db.InsertDomain(domain, tgID)

	// Send English success message
	enMsg := "bind default domain : <code>" + domain + "</code> Success! \n\n" +
		"you can send any mail toward this domain. \n\n Example:<code>someone@" + domain + "</code> \n\n"
	// Send Chinese success message
	cnMsg := "绑定默认域名 : <code>" + domain + "</code> 成功! \n\n" +
		"你可以发送邮件到该域名下的任何邮箱 \n\n 例如：<code>someone@" + domain + "</code> \n\n"

	if o.bot != nil {
		enMsgCfg := tgbotapi.NewMessage(tgID, enMsg)
		enMsgCfg.ParseMode = "HTML"
		if _, err := o.bot.Send(enMsgCfg); err != nil {
			log.Printf("failed to send bind default domain EN message: %v", err)
		}

		cnMsgCfg := tgbotapi.NewMessage(tgID, cnMsg)
		cnMsgCfg.ParseMode = "HTML"
		if _, err := o.bot.Send(cnMsgCfg); err != nil {
			log.Printf("failed to send bind default domain CN message: %v", err)
		}
	}

	return domain
}

// BindDomain binds a domain to the user. Returns the reply message string.
func (o *IO) BindDomain(tgID int64, domain string) string {
	if val, exists := o.domainToUser.Load(domain); exists {
		if val.(int64) == tgID {
			msg := "This domain has already bind on your account!"
			o.sendMessage(tgID, msg)
			return msg
		}
		msg := "This domain has already bind on another account!"
		o.sendMessage(tgID, msg)
		return msg
	}

	o.domainToUser.Store(domain, tgID)
	o.db.InsertDomain(domain, tgID)

	msg := "Bind Success!"
	o.sendMessage(tgID, msg)
	return msg
}

// RemoveDomain removes a domain binding for the user. Returns the reply message string.
func (o *IO) RemoveDomain(tgID int64, domain string) string {
	if val, exists := o.domainToUser.Load(domain); exists {
		if val.(int64) == tgID {
			o.domainToUser.Delete(domain)
			o.db.DeleteDomain(domain)
			msg := "Release Success!"
			o.sendMessage(tgID, msg)
			return msg
		}
	}

	msg := "This domain has not bind to your account!"
	o.sendMessage(tgID, msg)
	return msg
}

// ListDomain lists all domains bound to the user. Returns the formatted message string.
func (o *IO) ListDomain(tgID int64) string {
	msg := "<b>Your domain :</b> \n\n"
	count := 0

	o.domainToUser.Range(func(key, value interface{}) bool {
		if value.(int64) == tgID {
			count++
			msg += fmt.Sprintf("%d: <code> %s</code> \n", count, key.(string))
		}
		return true
	})

	if o.bot != nil {
		msgCfg := tgbotapi.NewMessage(tgID, msg)
		msgCfg.ParseMode = "HTML"
		if _, err := o.bot.Send(msgCfg); err != nil {
			log.Printf("failed to send list domain message: %v", err)
		}
	}

	return msg
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

// sendMessage is a helper that sends a plain text message via bot if bot is set.
func (o *IO) sendMessage(tgID int64, text string) {
	if o.bot != nil {
		msg := tgbotapi.NewMessage(tgID, text)
		if _, err := o.bot.Send(msg); err != nil {
			log.Printf("failed to send message: %v", err)
		}
	}
}

// sendHTMLMessage is a helper that sends an HTML-formatted message via bot if bot is set.
func (o *IO) sendHTMLMessage(tgID int64, text string) {
	if o.bot != nil {
		msg := tgbotapi.NewMessage(tgID, text)
		msg.ParseMode = "HTML"
		if _, err := o.bot.Send(msg); err != nil {
			log.Printf("failed to send HTML message: %v", err)
		}
	}
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

// BlockSender blocks a sender for the given user. Returns the success message.
func (o *IO) BlockSender(tgID int64, sender string) string {
	data := o.getTrueBlockData(sender)
	o.db.InsertBlockSender(data, tgID)
	o.blockSender.Delete(fmt.Sprintf("%d", tgID))

	msg := "Block sender " + data + " Success!"
	o.sendMessage(tgID, msg)
	return msg
}

// BlockDomain blocks a domain for the given user. Returns the success message.
func (o *IO) BlockDomain(tgID int64, domain string) string {
	data := o.getTrueBlockData(domain)
	o.db.InsertBlockDomain(data, tgID)
	o.blockDomain.Delete(fmt.Sprintf("%d", tgID))

	msg := "Block domain " + data + " Success!"
	o.sendMessage(tgID, msg)
	return msg
}

// BlockReceiver blocks a receiver for the given user. Returns the success message.
func (o *IO) BlockReceiver(tgID int64, receiver string) string {
	data := o.getTrueBlockData(receiver)
	o.db.InsertBlockReceiver(data, tgID)
	o.blockReceiver.Delete(fmt.Sprintf("%d", tgID))

	msg := "Block receiver " + data + " Success!"
	o.sendMessage(tgID, msg)
	return msg
}

// RemoveBlockSender removes a sender block for the given user. Returns the success message.
func (o *IO) RemoveBlockSender(tgID int64, sender string) string {
	o.db.DeleteBlockSender(sender, tgID)
	o.blockSender.Delete(fmt.Sprintf("%d", tgID))

	msg := "Remove block sender " + sender + " Success!"
	o.sendMessage(tgID, msg)
	return msg
}

// RemoveBlockDomain removes a domain block for the given user. Returns the success message.
func (o *IO) RemoveBlockDomain(tgID int64, domain string) string {
	o.db.DeleteBlockDomain(domain, tgID)
	o.blockDomain.Delete(fmt.Sprintf("%d", tgID))

	msg := "Remove block domain " + domain + " Success!"
	o.sendMessage(tgID, msg)
	return msg
}

// RemoveBlockReceiver removes a receiver block for the given user. Returns the success message.
func (o *IO) RemoveBlockReceiver(tgID int64, receiver string) string {
	o.db.DeleteBlockReceiver(receiver, tgID)
	o.blockReceiver.Delete(fmt.Sprintf("%d", tgID))

	msg := "Remove block receiver " + receiver + " Success!"
	o.sendMessage(tgID, msg)
	return msg
}

// ListBlockSender lists all blocked senders for the given user. Returns the formatted HTML message.
func (o *IO) ListBlockSender(tgID int64) string {
	msg := "<b>Your block sender :</b> \n\n"
	count := 0

	senders, err := o.db.SelectUserAllBlockSender(tgID)
	if err != nil {
		log.Printf("failed to query block senders: %v", err)
	} else {
		for _, info := range senders {
			count++
			msg += fmt.Sprintf("%d: <code> %s</code> \n", count, info.Sender)
		}
	}

	o.sendHTMLMessage(tgID, msg)
	return msg
}

// ListBlockDomain lists all blocked domains for the given user. Returns the formatted HTML message.
func (o *IO) ListBlockDomain(tgID int64) string {
	msg := "<b>Your block domain :</b> \n\n"
	count := 0

	domains, err := o.db.SelectUserAllBlockDomain(tgID)
	if err != nil {
		log.Printf("failed to query block domains: %v", err)
	} else {
		for _, info := range domains {
			count++
			msg += fmt.Sprintf("%d: <code> %s</code> \n", count, info.Domain)
		}
	}

	o.sendHTMLMessage(tgID, msg)
	return msg
}

// ListBlockReceiver lists all blocked receivers for the given user. Returns the formatted HTML message.
func (o *IO) ListBlockReceiver(tgID int64) string {
	msg := "<b>Your block receiver :</b> \n\n"
	count := 0

	receivers, err := o.db.SelectUserAllBlockReceiver(tgID)
	if err != nil {
		log.Printf("failed to query block receivers: %v", err)
	} else {
		for _, info := range receivers {
			count++
			msg += fmt.Sprintf("%d: <code> %s</code> \n", count, info.Receiver)
		}
	}

	o.sendHTMLMessage(tgID, msg)
	return msg
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

// SendAll sends a message to all unique users that have domain bindings.
func (o *IO) SendAll(msg string) {
	allUsers := make(map[int64]bool)
	o.domainToUser.Range(func(key, value interface{}) bool {
		allUsers[value.(int64)] = true
		return true
	})

	for tgID := range allUsers {
		o.sendHTMLMessage(tgID, msg)
	}
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
