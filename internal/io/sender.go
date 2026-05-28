package io

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

// TelegramSender abstracts the Telegram message sending capability.
// *tgbotapi.BotAPI implicitly implements this interface.
type TelegramSender interface {
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
	Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error)
}

// DeliveryEndpoint encapsulates a Bot's delivery capability and metadata.
// Used by HandleMailMulti to deliver mail through multiple bots.
type DeliveryEndpoint struct {
	Name   string
	Sender TelegramSender
	IsOld  bool
}

// AlertFunc is the migration alert callback type, injected by the migration layer.
// The io package does not directly depend on migration or telegram concrete types.
type AlertFunc func(sender TelegramSender, tgID int64, lang string)
