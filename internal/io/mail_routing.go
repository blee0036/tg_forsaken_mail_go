package io

import (
	"fmt"
	netmail "net/mail"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	smtpmod "go-version-rewrite/internal/smtp"
)

type mailTarget struct {
	tgID      int64
	receivers []string
}

type mailDelivery struct {
	tgID         int64
	receiver     string
	sender       string
	senderDomain string
}

func normalizeDomain(raw string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
}

func normalizeMailboxIdentity(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func parseEnvelopeMailbox(raw string) (address string, domain string, ok bool) {
	parsed, err := netmail.ParseAddress(strings.TrimSpace(raw))
	if err != nil {
		return "", "", false
	}

	address = normalizeMailboxIdentity(parsed.Address)
	at := strings.LastIndexByte(address, '@')
	if at <= 0 || at == len(address)-1 {
		return "", "", false
	}
	domain = normalizeDomain(address[at+1:])
	if domain == "" {
		return "", "", false
	}
	return address, domain, true
}

func (o *IO) mailTargets(mail *smtpmod.ParsedMail) []mailTarget {
	targets := make([]mailTarget, 0, len(mail.EnvelopeRecipients))
	targetByUser := make(map[int64]int)
	seenReceiver := make(map[int64]map[string]struct{})

	for _, rawRecipient := range mail.EnvelopeRecipients {
		receiver, domain, ok := parseEnvelopeMailbox(rawRecipient)
		if !ok {
			continue
		}
		value, exists := o.domainToUser.Load(domain)
		if !exists {
			continue
		}
		tgID, ok := value.(int64)
		if !ok {
			continue
		}

		index, exists := targetByUser[tgID]
		if !exists {
			index = len(targets)
			targetByUser[tgID] = index
			targets = append(targets, mailTarget{tgID: tgID})
			seenReceiver[tgID] = make(map[string]struct{})
		}
		if _, duplicate := seenReceiver[tgID][receiver]; duplicate {
			continue
		}
		seenReceiver[tgID][receiver] = struct{}{}
		targets[index].receivers = append(targets[index].receivers, receiver)
	}

	return targets
}

func (o *IO) prepareMailDelivery(mail *smtpmod.ParsedMail, target mailTarget) (mailDelivery, bool) {
	tgKey := fmt.Sprintf("%d", target.tgID)
	receiverBlocks := o.getBlockSet(tgKey, o.blockReceiver, func() (map[string]bool, error) {
		list, err := o.db.SelectUserAllBlockReceiver(target.tgID)
		if err != nil {
			return nil, err
		}
		set := make(map[string]bool)
		for _, info := range list {
			set[strings.ToLower(info.Receiver)] = true
		}
		return set, nil
	})

	receiver := ""
	for _, candidate := range target.receivers {
		if receiverBlocks == nil || !receiverBlocks[candidate] {
			receiver = candidate
			break
		}
	}
	if receiver == "" {
		return mailDelivery{}, false
	}

	delivery := mailDelivery{tgID: target.tgID, receiver: receiver}
	sender, senderDomain, hasSender := parseEnvelopeMailbox(mail.EnvelopeFrom)
	if !hasSender {
		return delivery, true
	}
	delivery.sender = sender
	delivery.senderDomain = senderDomain

	domainBlocks := o.getBlockSet(tgKey, o.blockDomain, func() (map[string]bool, error) {
		list, err := o.db.SelectUserAllBlockDomain(target.tgID)
		if err != nil {
			return nil, err
		}
		set := make(map[string]bool)
		for _, info := range list {
			set[strings.ToLower(info.Domain)] = true
		}
		return set, nil
	})
	if domainBlocks != nil && domainBlocks[senderDomain] {
		return mailDelivery{}, false
	}

	senderBlocks := o.getBlockSet(tgKey, o.blockSender, func() (map[string]bool, error) {
		list, err := o.db.SelectUserAllBlockSender(target.tgID)
		if err != nil {
			return nil, err
		}
		set := make(map[string]bool)
		for _, info := range list {
			set[strings.ToLower(info.Sender)] = true
		}
		return set, nil
	})
	if senderBlocks != nil && senderBlocks[sender] {
		return mailDelivery{}, false
	}

	return delivery, true
}

func (o *IO) mailActionKeyboard(delivery mailDelivery) tgbotapi.InlineKeyboardMarkup {
	buttons := make([]tgbotapi.InlineKeyboardButton, 0, 3)
	if delivery.sender != "" {
		buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData(
			"Block Sender", o.CheckButtonData(delivery.sender, "block_sender"),
		))
	}
	buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData(
		"Block Receiver", o.CheckButtonData(delivery.receiver, "block_receiver"),
	))
	if delivery.senderDomain != "" {
		buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData(
			"Block Domain", o.CheckButtonData(delivery.senderDomain, "block_domain"),
		))
	}
	return tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(buttons...))
}
