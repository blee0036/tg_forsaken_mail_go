package migration

import (
	"log"
	"sort"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"go-version-rewrite/internal/config"
	"go-version-rewrite/internal/io"
)

// AlertSender is responsible for sending migration alert messages to users.
type AlertSender struct {
	config *config.Config
}

// NewAlertSender creates a new AlertSender with the given config.
func NewAlertSender(cfg *config.Config) *AlertSender {
	return &AlertSender{config: cfg}
}

// ResolveAlertMessage resolves the alert message for the given language.
// Fallback order: exact match → prefix normalization (e.g., zh-Hans → zh) → "en" → "zh" → first key alphabetically.
// Returns empty string only if ChangeBotAlertMsg is empty.
func (a *AlertSender) ResolveAlertMessage(lang string) string {
	msgs := a.config.ChangeBotAlertMsg
	if len(msgs) == 0 {
		return ""
	}

	// 1. Exact match
	if msg, ok := msgs[lang]; ok {
		return msg
	}

	// 2. Prefix normalization: split on "-" or "_", take first part
	if idx := strings.IndexAny(lang, "-_"); idx > 0 {
		prefix := lang[:idx]
		if msg, ok := msgs[prefix]; ok {
			return msg
		}
	}

	// 3. Fallback to "en"
	if msg, ok := msgs["en"]; ok {
		return msg
	}

	// 4. Fallback to "zh"
	if msg, ok := msgs["zh"]; ok {
		return msg
	}

	// 5. First key alphabetically
	keys := make([]string, 0, len(msgs))
	for k := range msgs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return msgs[keys[0]]
}

// SendAlert sends the migration alert via the given sender to the specified user.
// Uses Markdown parse mode first; on failure, retries with plain text; on second failure, logs the error.
func (a *AlertSender) SendAlert(sender io.TelegramSender, tgID int64, lang string) {
	msg := a.ResolveAlertMessage(lang)
	if msg == "" {
		return
	}

	// Try Markdown mode first
	mdMsg := tgbotapi.NewMessage(tgID, msg)
	mdMsg.ParseMode = tgbotapi.ModeMarkdown
	_, err := sender.Send(mdMsg)
	if err == nil {
		return
	}
	log.Printf("[migration] alert markdown send failed for tgID=%d: %v, retrying as plain text", tgID, err)

	// Retry with plain text (no parse mode)
	plainMsg := tgbotapi.NewMessage(tgID, msg)
	_, err = sender.Send(plainMsg)
	if err != nil {
		log.Printf("[migration] alert plain text send also failed for tgID=%d: %v", tgID, err)
	}
}

// HasAlert returns true if there are valid alert messages configured.
func (a *AlertSender) HasAlert() bool {
	return len(a.config.ChangeBotAlertMsg) > 0
}

// AsAlertFunc returns an io.AlertFunc closure wrapping SendAlert.
// Returns nil if no alert messages are configured.
func (a *AlertSender) AsAlertFunc() io.AlertFunc {
	if !a.HasAlert() {
		return nil
	}
	return func(sender io.TelegramSender, tgID int64, lang string) {
		a.SendAlert(sender, tgID, lang)
	}
}
