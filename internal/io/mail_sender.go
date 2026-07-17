package io

import (
	"fmt"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	smtpmod "go-version-rewrite/internal/smtp"
)

func (o *IO) deliverMailToSender(
	mail *smtpmod.ParsedMail,
	delivery mailDelivery,
	sender TelegramSender,
	botName string,
	formattedMail string,
	keyboard tgbotapi.InlineKeyboardMarkup,
) bool {
	message := tgbotapi.NewMessage(delivery.tgID, formattedMail)
	message.ParseMode = "MarkdownV2"
	message.ReplyMarkup = keyboard
	if err := o.sendMailMessageWithRetry(sender, botName, message); err != nil {
		log.Printf("[%s] mail delivery stopped after message failure: %v", botName, err)
		return false
	}

	succeeded := true
	for _, attachment := range mail.Attachments {
		if len(attachment.Content) == 0 {
			continue
		}
		document := tgbotapi.NewDocument(delivery.tgID, tgbotapi.FileBytes{
			Name:  attachment.Filename,
			Bytes: attachment.Content,
		})
		operation := fmt.Sprintf("attachment %q", attachment.Filename)
		if err := o.sendMailWithRetry(sender, botName, operation, document); err != nil {
			log.Printf("[%s] %s delivery exhausted retries: %v", botName, operation, err)
			succeeded = false
		}
	}

	if mail.HTML == "" {
		return succeeded
	}

	htmlBytes := []byte(mail.HTML)
	document := tgbotapi.NewDocument(delivery.tgID, tgbotapi.FileBytes{
		Name:  "content.html",
		Bytes: htmlBytes,
	})
	if o.config.UploadURL != "" && o.UploadHTMLFunc != nil {
		uploadUUID, err := o.UploadHTMLFunc(htmlBytes)
		if err != nil {
			log.Printf("[%s] failed to upload HTML: %v", botName, err)
		} else if uploadUUID != "" {
			viewURL := o.config.UploadURL + "/mail/" + uploadUUID
			document.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonURL("View Directly", viewURL),
				),
			)
		}
	}
	if err := o.sendMailWithRetry(sender, botName, "HTML document", document); err != nil {
		log.Printf("[%s] HTML document delivery exhausted retries: %v", botName, err)
		return false
	}

	return succeeded
}
