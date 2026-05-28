package io

import (
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	smtpmod "go-version-rewrite/internal/smtp"
)

// HandleMailMulti delivers an incoming email through multiple DeliveryEndpoints.
//
// Flow:
//  1. Parse recipient email to get domain → look up tgID
//  2. Check block lists (same as HandleMail)
//  3. Get user language via GetUserLang(tgID)
//  4. Format notification using FormatMailNotification (uses FormatMailTime internally)
//  5. For each endpoint: send message, attachments, HTML doc
//  6. For IsOld endpoints: call sendAlert after successful delivery
//  7. Failure on one endpoint does not affect others
//
// The close_old_date gating is handled by the caller (MigrationManager.ActiveMailEndpoints).
// HandleMailMulti only delivers to the endpoints it receives.
func (o *IO) HandleMailMulti(
	mail *smtpmod.ParsedMail,
	endpoints []DeliveryEndpoint,
	sendAlert AlertFunc,
	now time.Time,
) {
	// Step 1: Parse recipient to get tgID (same logic as HandleMail)
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

	// Step 2: Check block lists
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

	// Step 3: Get user language
	lang := o.GetUserLang(tgID)

	// Step 4: Format notification (FormatMailNotification uses FormatMailTime internally)
	email := o.FormatMailNotification(mail, lang, now)

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

	// Step 5: Deliver through each endpoint
	for _, ep := range endpoints {
		if ep.Sender == nil {
			continue
		}

		deliveryFailed := false

		// Send formatted message with inline keyboard
		msg := tgbotapi.NewMessage(tgID, email)
		msg.ParseMode = "MarkdownV2"
		msg.ReplyMarkup = keyboard
		if _, err := ep.Sender.Send(msg); err != nil {
			log.Printf("[%s] failed to send mail message with MarkdownV2 mode: %v", ep.Name, err)
			// Retry without parse mode as fallback
			msg.ParseMode = ""
			if _, err := ep.Sender.Send(msg); err != nil {
				log.Printf("[%s] failed to send mail message (fallback): %v", ep.Name, err)
				deliveryFailed = true
			}
		}

		// Send attachments
		if !deliveryFailed {
			for _, attachment := range mail.Attachments {
				if len(attachment.Content) > 0 {
					doc := tgbotapi.NewDocument(tgID, tgbotapi.FileBytes{
						Name:  attachment.Filename,
						Bytes: attachment.Content,
					})
					if _, err := ep.Sender.Send(doc); err != nil {
						log.Printf("[%s] failed to send attachment %s: %v", ep.Name, attachment.Filename, err)
						deliveryFailed = true
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
						log.Printf("[%s] failed to upload HTML: %v", ep.Name, err)
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
				if _, err := ep.Sender.Send(doc); err != nil {
					log.Printf("[%s] failed to send HTML document: %v", ep.Name, err)
					deliveryFailed = true
				}
			}
		}

		// Step 6: For old endpoints, call sendAlert only after successful delivery
		if ep.IsOld && sendAlert != nil && !deliveryFailed {
			sendAlert(ep.Sender, tgID, lang)
		}
	}
}
