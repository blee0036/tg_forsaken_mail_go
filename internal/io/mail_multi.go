package io

import (
	"time"

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
	if mail == nil {
		return
	}
	for _, target := range o.mailTargets(mail) {
		delivery, ok := o.prepareMailDelivery(mail, target)
		if ok {
			o.handleMailMultiDelivery(mail, delivery, endpoints, sendAlert, now)
		}
	}
}

func (o *IO) handleMailMultiDelivery(
	mail *smtpmod.ParsedMail,
	delivery mailDelivery,
	endpoints []DeliveryEndpoint,
	sendAlert AlertFunc,
	now time.Time,
) {
	tgID := delivery.tgID

	// Step 3: Get user language
	lang := o.GetUserLang(tgID)

	// Step 4: Format notification (FormatMailNotification uses FormatMailTime internally)
	email := o.FormatMailNotification(mail, lang, now)

	keyboard := o.mailActionKeyboard(delivery)

	// Step 5: Deliver through each endpoint
	for _, ep := range endpoints {
		if ep.Sender == nil {
			continue
		}

		succeeded := o.deliverMailToSender(mail, delivery, ep.Sender, ep.Name, email, keyboard)
		if ep.IsOld && sendAlert != nil && succeeded {
			sendAlert(ep.Sender, tgID, lang)
		}
	}
}
