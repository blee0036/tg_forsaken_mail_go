package io

import (
	"errors"
	"reflect"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	smtpmod "go-version-rewrite/internal/smtp"
)

type scriptedMailSender struct {
	errors []error
	calls  []tgbotapi.Chattable
}

func (s *scriptedMailSender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	callIndex := len(s.calls)
	s.calls = append(s.calls, c)
	if callIndex < len(s.errors) && s.errors[callIndex] != nil {
		return tgbotapi.Message{}, s.errors[callIndex]
	}
	return tgbotapi.Message{}, nil
}

func (s *scriptedMailSender) Request(tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	return &tgbotapi.APIResponse{Ok: true}, nil
}

func testRetryPolicy(ioModule *IO, delays *[]time.Duration) {
	ioModule.mailRetry = mailRetryPolicy{
		maxAttempts: 4,
		baseDelay:   500 * time.Millisecond,
		maxDelay:    10 * time.Second,
		sleep: func(delay time.Duration) {
			*delays = append(*delays, delay)
		},
	}
}

func TestSendMailWithRetryUsesExponentialBackoff(t *testing.T) {
	ioModule, _ := newTestIO(t)
	var delays []time.Duration
	testRetryPolicy(ioModule, &delays)
	sender := &scriptedMailSender{errors: []error{
		errors.New("temporary network failure"),
		errors.New("temporary network failure"),
		nil,
	}}

	err := ioModule.sendMailWithRetry(sender, "test-bot", "message", tgbotapi.NewMessage(1, "test"))
	if err != nil {
		t.Fatalf("sendMailWithRetry() error: %v", err)
	}
	if len(sender.calls) != 3 {
		t.Fatalf("Send calls = %d, want 3", len(sender.calls))
	}
	wantDelays := []time.Duration{500 * time.Millisecond, time.Second}
	if !reflect.DeepEqual(delays, wantDelays) {
		t.Fatalf("retry delays = %v, want %v", delays, wantDelays)
	}
}

func TestSendMailWithRetryHonorsTelegramRetryAfter(t *testing.T) {
	ioModule, _ := newTestIO(t)
	var delays []time.Duration
	testRetryPolicy(ioModule, &delays)
	sender := &scriptedMailSender{errors: []error{
		&tgbotapi.Error{
			Code:    429,
			Message: "Too Many Requests",
			ResponseParameters: tgbotapi.ResponseParameters{
				RetryAfter: 7,
			},
		},
		nil,
	}}

	err := ioModule.sendMailWithRetry(sender, "rate-limited", "message", tgbotapi.NewMessage(1, "test"))
	if err != nil {
		t.Fatalf("sendMailWithRetry() error: %v", err)
	}
	if !reflect.DeepEqual(delays, []time.Duration{7 * time.Second}) {
		t.Fatalf("retry delays = %v, want [7s]", delays)
	}
}

func TestSendMailWithRetryDoesNotRetryPermanentTelegramError(t *testing.T) {
	ioModule, _ := newTestIO(t)
	var delays []time.Duration
	testRetryPolicy(ioModule, &delays)
	sender := &scriptedMailSender{errors: []error{
		&tgbotapi.Error{Code: 403, Message: "Forbidden"},
	}}

	err := ioModule.sendMailWithRetry(sender, "blocked-bot", "message", tgbotapi.NewMessage(1, "test"))
	if err == nil {
		t.Fatal("sendMailWithRetry() unexpectedly succeeded")
	}
	if len(sender.calls) != 1 || len(delays) != 0 {
		t.Fatalf("permanent error calls=%d delays=%v, want one call and no delay", len(sender.calls), delays)
	}
}

func TestSendMailMessageFallsBackAfterMarkdownParseError(t *testing.T) {
	ioModule, _ := newTestIO(t)
	var delays []time.Duration
	testRetryPolicy(ioModule, &delays)
	sender := &scriptedMailSender{errors: []error{
		&tgbotapi.Error{Code: 400, Message: "Bad Request: can't parse entities"},
		nil,
	}}
	message := tgbotapi.NewMessage(1, "test")
	message.ParseMode = "MarkdownV2"

	if err := ioModule.sendMailMessageWithRetry(sender, "test-bot", message); err != nil {
		t.Fatalf("sendMailMessageWithRetry() error: %v", err)
	}
	if len(sender.calls) != 2 {
		t.Fatalf("Send calls = %d, want 2", len(sender.calls))
	}
	fallback, ok := sender.calls[1].(tgbotapi.MessageConfig)
	if !ok || fallback.ParseMode != "" {
		t.Fatalf("fallback message = %#v, want empty ParseMode", sender.calls[1])
	}
	if len(delays) != 0 {
		t.Fatalf("parse fallback should be immediate, delays=%v", delays)
	}
}

func TestHandleMailMultiRetriesEachBotIndependently(t *testing.T) {
	ioModule, _ := newTestIO(t)
	ioModule.BindDomain(101, "recipient.example")
	var delays []time.Duration
	testRetryPolicy(ioModule, &delays)

	temporary := errors.New("temporary network failure")
	failing := &scriptedMailSender{errors: []error{temporary, temporary, temporary, temporary}}
	succeeding := &scriptedMailSender{}
	endpoints := []DeliveryEndpoint{
		{Name: "failing-bot", Sender: failing, IsOld: true},
		{Name: "working-bot", Sender: succeeding, IsOld: true},
	}
	mail := &smtpmod.ParsedMail{
		From:               "display@header.example",
		To:                 "visible@header.example",
		EnvelopeFrom:       "sender@envelope.example",
		EnvelopeRecipients: []string{"user@recipient.example"},
		Subject:            "retry isolation",
		Text:               "body",
		Attachments: []smtpmod.Attachment{{
			Filename: "test.txt",
			Content:  []byte("attachment"),
		}},
	}

	var alerted []TelegramSender
	ioModule.HandleMailMulti(mail, endpoints, func(sender TelegramSender, _ int64, _ string) {
		alerted = append(alerted, sender)
	}, time.Now())

	if len(failing.calls) != 4 {
		t.Fatalf("failing bot calls = %d, want 4 message attempts and no attachment", len(failing.calls))
	}
	if len(succeeding.calls) != 2 {
		t.Fatalf("working bot calls = %d, want message and attachment", len(succeeding.calls))
	}
	if len(alerted) != 1 || alerted[0] != succeeding {
		t.Fatalf("alerted senders = %#v, want only working bot", alerted)
	}
	wantDelays := []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second}
	if !reflect.DeepEqual(delays, wantDelays) {
		t.Fatalf("retry delays = %v, want %v", delays, wantDelays)
	}
}
