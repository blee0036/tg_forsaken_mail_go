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
	formattedMail mailContent,
	keyboard tgbotapi.InlineKeyboardMarkup,
) bool {
	message := formattedMail.message(delivery.tgID)
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
		if attachment.InlineResource && (mail.HTML != "" || mail.Text != "") {
			// Newsletter logos and tracking resources must not flood the chat
			// with documents. Explicit user attachments still go through.
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
		if formattedMail.truncated {
			// A text-only mail has no content.html to recover the preview's
			// omitted text from, so deliver a complete readable copy as well.
			fullText := "From: " + mail.From + "\nTo: " + mail.To + "\n"
			if mail.Cc != "" {
				fullText += "Cc: " + mail.Cc + "\n"
			}
			date := mail.RawDate
			if date == "" {
				date = mail.Date
			}
			fullText += "Subject: " + mail.Subject + "\nTime: " + date + "\n\n" + mail.Text
			document := tgbotapi.NewDocument(delivery.tgID, tgbotapi.FileBytes{Name: "content.txt", Bytes: []byte(fullText)})
			if err := o.sendMailWithRetry(sender, botName, "full text document", document); err != nil {
				log.Printf("[%s] full text document delivery exhausted retries: %v", botName, err)
				return false
			}
		}
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
