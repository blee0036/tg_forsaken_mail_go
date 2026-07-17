package io

import (
	"reflect"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	smtpmod "go-version-rewrite/internal/smtp"
)

type recordingMailSender struct {
	messages []tgbotapi.MessageConfig
}

func (s *recordingMailSender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	if message, ok := c.(tgbotapi.MessageConfig); ok {
		s.messages = append(s.messages, message)
	}
	return tgbotapi.Message{}, nil
}

func (s *recordingMailSender) Request(tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	return &tgbotapi.APIResponse{Ok: true}, nil
}

func (s *recordingMailSender) chatIDs() []int64 {
	ids := make([]int64, 0, len(s.messages))
	for _, message := range s.messages {
		ids = append(ids, message.ChatID)
	}
	return ids
}

func testEnvelopeMail(recipients ...string) *smtpmod.ParsedMail {
	return &smtpmod.ParsedMail{
		From:               "Header Sender <display@header.example>",
		To:                 "Visible Recipient <visible@header.example>",
		EnvelopeFrom:       "bounce@envelope.example",
		EnvelopeRecipients: recipients,
		Subject:            "Envelope routing",
		Text:               "body",
	}
}

func runMailDeliveryModes(
	t *testing.T,
	setup func(*IO),
	mail func() *smtpmod.ParsedMail,
	wantChatIDs []int64,
) {
	t.Helper()
	for _, mode := range []string{"single", "multi"} {
		t.Run(mode, func(t *testing.T) {
			ioModule, _ := newTestIO(t)
			setup(ioModule)
			sender := &recordingMailSender{}

			if mode == "single" {
				ioModule.SetBot(sender)
				ioModule.HandleMail(mail())
			} else {
				endpoints := []DeliveryEndpoint{{Name: "test", Sender: sender}}
				ioModule.HandleMailMulti(mail(), endpoints, nil, time.Now())
			}

			if got := sender.chatIDs(); !reflect.DeepEqual(got, wantChatIDs) {
				t.Fatalf("delivered chat IDs = %v, want %v", got, wantChatIDs)
			}
		})
	}
}

func TestMailDelivery_BccUsesEnvelopeRecipient(t *testing.T) {
	runMailDeliveryModes(t, func(ioModule *IO) {
		ioModule.BindDomain(101, "bcc.example")
	}, func() *smtpmod.ParsedMail {
		mail := testEnvelopeMail("hidden@bcc.example")
		mail.To = "undisclosed-recipients:;"
		return mail
	}, []int64{101})
}

func TestMailDelivery_MultipleEnvelopeRecipientsReachEveryUser(t *testing.T) {
	runMailDeliveryModes(t, func(ioModule *IO) {
		ioModule.BindDomain(101, "first.example")
		ioModule.BindDomain(202, "second.example")
	}, func() *smtpmod.ParsedMail {
		return testEnvelopeMail("one@first.example", "two@second.example")
	}, []int64{101, 202})
}

func TestMailDelivery_ForgedMIMEToCannotChangeRoute(t *testing.T) {
	runMailDeliveryModes(t, func(ioModule *IO) {
		ioModule.BindDomain(101, "actual.example")
		ioModule.BindDomain(999, "forged.example")
	}, func() *smtpmod.ParsedMail {
		mail := testEnvelopeMail("recipient@actual.example")
		mail.To = "attacker-controlled@forged.example"
		return mail
	}, []int64{101})
}

func TestMailDelivery_DeduplicatesMultipleRecipientsForSameUser(t *testing.T) {
	runMailDeliveryModes(t, func(ioModule *IO) {
		ioModule.BindDomain(101, "alias-one.example")
		ioModule.BindDomain(101, "alias-two.example")
	}, func() *smtpmod.ParsedMail {
		return testEnvelopeMail(
			"first@alias-one.example",
			"second@alias-two.example",
			"first@alias-one.example",
		)
	}, []int64{101})
}

func TestMailDelivery_MIMEHeadersCannotCreateRecipientWithoutEnvelope(t *testing.T) {
	runMailDeliveryModes(t, func(ioModule *IO) {
		ioModule.BindDomain(999, "forged.example")
	}, func() *smtpmod.ParsedMail {
		mail := testEnvelopeMail()
		mail.To = "attacker-controlled@forged.example"
		return mail
	}, []int64{})
}

func TestMailDelivery_EnvelopeSenderControlsBlocking(t *testing.T) {
	runMailDeliveryModes(t, func(ioModule *IO) {
		ioModule.BindDomain(101, "recipient.example")
		_, _ = ioModule.db.InsertBlockSender("blocked@header.example", 101)
	}, func() *smtpmod.ParsedMail {
		mail := testEnvelopeMail("user@recipient.example")
		mail.From = "blocked@header.example"
		mail.EnvelopeFrom = "allowed@envelope.example"
		return mail
	}, []int64{101})
}

func TestMailDelivery_NullEnvelopeSenderIsDeliverable(t *testing.T) {
	runMailDeliveryModes(t, func(ioModule *IO) {
		ioModule.BindDomain(101, "recipient.example")
	}, func() *smtpmod.ParsedMail {
		mail := testEnvelopeMail("user@recipient.example")
		mail.EnvelopeFrom = ""
		return mail
	}, []int64{101})
}

func TestMailDelivery_UsesUnblockedAliasForSameUser(t *testing.T) {
	runMailDeliveryModes(t, func(ioModule *IO) {
		ioModule.BindDomain(101, "recipient.example")
		_, _ = ioModule.db.InsertBlockReceiver("blocked@recipient.example", 101)
	}, func() *smtpmod.ParsedMail {
		return testEnvelopeMail("blocked@recipient.example", "allowed@recipient.example")
	}, []int64{101})
}
