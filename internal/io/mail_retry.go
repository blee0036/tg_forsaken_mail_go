package io

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	defaultMailSendAttempts = 4
	defaultMailRetryBase    = 500 * time.Millisecond
	defaultMailRetryMax     = 10 * time.Second
)

type mailRetryPolicy struct {
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
	sleep       func(time.Duration)
}

func defaultMailRetryPolicy() mailRetryPolicy {
	return mailRetryPolicy{
		maxAttempts: defaultMailSendAttempts,
		baseDelay:   defaultMailRetryBase,
		maxDelay:    defaultMailRetryMax,
		sleep:       time.Sleep,
	}
}

func telegramAPIError(err error) (*tgbotapi.Error, bool) {
	var apiErr *tgbotapi.Error
	if errors.As(err, &apiErr) {
		return apiErr, true
	}
	return nil, false
}

func isRetryableMailSendError(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	if apiErr, ok := telegramAPIError(err); ok {
		return apiErr.Code == 429 || apiErr.Code >= 500
	}
	return true
}

func needsPlainTextFallback(err error) bool {
	apiErr, ok := telegramAPIError(err)
	if !ok || apiErr.Code != 400 {
		return false
	}
	message := strings.ToLower(apiErr.Message)
	return strings.Contains(message, "parse") || strings.Contains(message, "entit")
}

func (o *IO) mailRetryDelay(err error, failedAttempt int) time.Duration {
	policy := o.mailRetry
	delay := policy.baseDelay
	for i := 1; i < failedAttempt && delay < policy.maxDelay; i++ {
		delay *= 2
	}
	if apiErr, ok := telegramAPIError(err); ok && apiErr.RetryAfter > 0 {
		delay = time.Duration(apiErr.RetryAfter) * time.Second
	}
	if delay > policy.maxDelay {
		return policy.maxDelay
	}
	return delay
}

func (o *IO) sendMailWithRetry(
	sender TelegramSender,
	botName string,
	operation string,
	chattable tgbotapi.Chattable,
) error {
	policy := o.mailRetry
	if policy.maxAttempts < 1 {
		policy = defaultMailRetryPolicy()
	}
	if botName == "" {
		botName = "primary"
	}

	var err error
	for attempt := 1; attempt <= policy.maxAttempts; attempt++ {
		if _, err = sender.Send(chattable); err == nil {
			return nil
		}
		if attempt == policy.maxAttempts || !isRetryableMailSendError(err) {
			return fmt.Errorf("%s failed after %d attempt(s): %w", operation, attempt, err)
		}

		delay := o.mailRetryDelay(err, attempt)
		log.Printf("[%s] %s attempt %d/%d failed: %v; retrying in %s", botName, operation, attempt, policy.maxAttempts, err, delay)
		policy.sleep(delay)
	}

	return fmt.Errorf("%s failed: %w", operation, err)
}

func (o *IO) sendMailMessageWithRetry(
	sender TelegramSender,
	botName string,
	message tgbotapi.MessageConfig,
) error {
	err := o.sendMailWithRetry(sender, botName, "mail message", message)
	if err == nil || !needsPlainTextFallback(err) {
		return err
	}

	log.Printf("[%s] mail message MarkdownV2 formatting was rejected; retrying without parse mode", botName)
	message.ParseMode = ""
	return o.sendMailWithRetry(sender, botName, "mail message without parse mode", message)
}
