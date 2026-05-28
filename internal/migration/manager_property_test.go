package migration

import (
	"fmt"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"

	"go-version-rewrite/internal/config"
	"go-version-rewrite/internal/io"
	"go-version-rewrite/internal/telegram"
)

// mockBotSender implements telegram.BotSender for testing.
type mockBotSender struct {
	name         string
	sentMessages []tgbotapi.Chattable
}

func (m *mockBotSender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	m.sentMessages = append(m.sentMessages, c)
	return tgbotapi.Message{}, nil
}

func (m *mockBotSender) Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	return &tgbotapi.APIResponse{Ok: true}, nil
}

// Verify mockBotSender implements io.TelegramSender
var _ io.TelegramSender = (*mockBotSender)(nil)

// Feature: bot-migration, Property 7: 关闭日期门控
// Validates: Requirements 7.4, 7.5, 7.6
func TestProperty_CloseOldDateGating(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Generator for number of old bots (1 to 5)
	oldBotCountGen := gen.IntRange(1, 5)

	// Property 7a: When now >= closeOldDate, ActiveMailEndpoints should NOT contain old bots
	properties.Property("old bots excluded from ActiveMailEndpoints when now >= closeOldDate", prop.ForAll(
		func(nowUnix int64, closeUnix int64, oldBotCount int) bool {
			now := time.Unix(nowUnix, 0)
			closeDate := time.Unix(closeUnix, 0)

			// Ensure now >= closeDate (by date comparison)
			nowDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
			closeDateLocal := time.Date(closeDate.Year(), closeDate.Month(), closeDate.Day(), 0, 0, 0, 0, time.Local)
			if nowDate.Before(closeDateLocal) {
				// Swap to ensure now >= closeDate
				now, closeDate = closeDate, now
				nowDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
				closeDateLocal = time.Date(closeDate.Year(), closeDate.Month(), closeDate.Day(), 0, 0, 0, 0, time.Local)
			}

			// Create mock bots
			cfg := &config.Config{
				CloseOldDateParsed: &closeDate,
				ChangeBotAlertMsg:  map[string]string{"en": "Please switch"},
			}

			newSender := &mockBotSender{name: "new_bot"}
			newBot := telegram.NewForTestWithOptions(cfg, nil, newSender, telegram.BotOptions{})

			oldBots := make([]*telegram.Bot, oldBotCount)
			for i := 0; i < oldBotCount; i++ {
				oldSender := &mockBotSender{name: fmt.Sprintf("old_bot_%d", i)}
				oldBots[i] = telegram.NewForTestWithOptions(cfg, nil, oldSender, telegram.BotOptions{IsOld: true})
			}

			mm := &MigrationManager{
				config:      cfg,
				newBot:      newBot,
				oldBots:     oldBots,
				alertSender: NewAlertSender(cfg),
			}

			endpoints := mm.ActiveMailEndpoints(now)

			// Should only contain the new bot
			if len(endpoints) != 1 {
				t.Logf("Expected 1 endpoint (new bot only), got %d. now=%v, closeDate=%v",
					len(endpoints), nowDate, closeDateLocal)
				return false
			}

			// The single endpoint should be the new bot (not old)
			if endpoints[0].IsOld {
				t.Logf("Expected new bot endpoint, got old bot")
				return false
			}
			if endpoints[0].Name != "new_bot" {
				t.Logf("Expected endpoint name 'new_bot', got %q", endpoints[0].Name)
				return false
			}

			return true
		},
		gen.Int64Range(
			time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local).Unix(),
			time.Date(2030, 12, 31, 23, 59, 59, 0, time.Local).Unix(),
		),
		gen.Int64Range(
			time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local).Unix(),
			time.Date(2030, 12, 31, 23, 59, 59, 0, time.Local).Unix(),
		),
		oldBotCountGen,
	))

	// Property 7b: When now < closeOldDate, ActiveMailEndpoints should contain both new and old bots
	properties.Property("old bots included in ActiveMailEndpoints when now < closeOldDate", prop.ForAll(
		func(nowUnix int64, closeUnix int64, oldBotCount int) bool {
			now := time.Unix(nowUnix, 0)
			closeDate := time.Unix(closeUnix, 0)

			// Ensure now < closeDate (by date comparison)
			nowDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
			closeDateLocal := time.Date(closeDate.Year(), closeDate.Month(), closeDate.Day(), 0, 0, 0, 0, time.Local)
			if !nowDate.Before(closeDateLocal) {
				// Swap to ensure now < closeDate
				now, closeDate = closeDate, now
				nowDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
				closeDateLocal = time.Date(closeDate.Year(), closeDate.Month(), closeDate.Day(), 0, 0, 0, 0, time.Local)
				// After swap, if still not before, skip (same day)
				if !nowDate.Before(closeDateLocal) {
					return true // skip same-day case
				}
			}

			// Create mock bots
			cfg := &config.Config{
				CloseOldDateParsed: &closeDate,
				ChangeBotAlertMsg:  map[string]string{"en": "Please switch"},
			}

			newSender := &mockBotSender{name: "new_bot"}
			newBot := telegram.NewForTestWithOptions(cfg, nil, newSender, telegram.BotOptions{})

			oldBots := make([]*telegram.Bot, oldBotCount)
			for i := 0; i < oldBotCount; i++ {
				oldSender := &mockBotSender{name: fmt.Sprintf("old_bot_%d", i)}
				oldBots[i] = telegram.NewForTestWithOptions(cfg, nil, oldSender, telegram.BotOptions{IsOld: true})
			}

			mm := &MigrationManager{
				config:      cfg,
				newBot:      newBot,
				oldBots:     oldBots,
				alertSender: NewAlertSender(cfg),
			}

			endpoints := mm.ActiveMailEndpoints(now)

			// Should contain 1 new bot + oldBotCount old bots
			expectedCount := 1 + oldBotCount
			if len(endpoints) != expectedCount {
				t.Logf("Expected %d endpoints, got %d. now=%v, closeDate=%v",
					expectedCount, len(endpoints), nowDate, closeDateLocal)
				return false
			}

			// First endpoint should be new bot
			if endpoints[0].IsOld {
				t.Logf("Expected first endpoint to be new bot")
				return false
			}

			// Remaining endpoints should be old bots
			for i := 1; i < len(endpoints); i++ {
				if !endpoints[i].IsOld {
					t.Logf("Expected endpoint %d to be old bot", i)
					return false
				}
				expectedName := fmt.Sprintf("old_bot_%d", i-1)
				if endpoints[i].Name != expectedName {
					t.Logf("Expected endpoint name %q, got %q", expectedName, endpoints[i].Name)
					return false
				}
			}

			return true
		},
		gen.Int64Range(
			time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local).Unix(),
			time.Date(2030, 12, 31, 23, 59, 59, 0, time.Local).Unix(),
		),
		gen.Int64Range(
			time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local).Unix(),
			time.Date(2030, 12, 31, 23, 59, 59, 0, time.Local).Unix(),
		),
		oldBotCountGen,
	))

	// Property 7c: AfterInteraction alert fires regardless of close_old_date
	// The AfterInteraction hook is set on old bots and should fire even when close_old_date is reached
	properties.Property("AfterInteraction alert fires regardless of close_old_date", prop.ForAll(
		func(nowUnix int64, closeUnix int64) bool {
			now := time.Unix(nowUnix, 0)
			closeDate := time.Unix(closeUnix, 0)

			// Ensure now >= closeDate (close date reached)
			nowDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
			closeDateLocal := time.Date(closeDate.Year(), closeDate.Month(), closeDate.Day(), 0, 0, 0, 0, time.Local)
			if nowDate.Before(closeDateLocal) {
				now, closeDate = closeDate, now
			}

			cfg := &config.Config{
				CloseOldDateParsed: &closeDate,
				ChangeBotAlertMsg:  map[string]string{"en": "Please switch to new bot"},
			}

			alertSender := NewAlertSender(cfg)

			// Simulate what happens when a user interacts with an old bot:
			// The AfterInteraction hook calls alertSender.SendAlert
			// This should work regardless of close_old_date
			oldBotSender := &mockBotSender{name: "old_bot_0"}

			// Verify alert sender has alert configured
			if !alertSender.HasAlert() {
				t.Log("Expected alertSender to have alert")
				return false
			}

			// Simulate the AfterInteraction hook firing
			alertSender.SendAlert(oldBotSender, 12345, "en")

			// Alert should have been sent (1 message)
			if len(oldBotSender.sentMessages) != 1 {
				t.Logf("Expected 1 alert message sent, got %d", len(oldBotSender.sentMessages))
				return false
			}

			// Also verify that the MigrationManager's isCloseOldDateReachedAt returns true
			mm := &MigrationManager{
				config:      cfg,
				alertSender: alertSender,
			}
			if !mm.isCloseOldDateReachedAt(now) {
				t.Log("Expected isCloseOldDateReachedAt to return true")
				return false
			}

			// The key property: even though close date is reached (old bots excluded from mail),
			// the alert still fires on interaction
			return true
		},
		gen.Int64Range(
			time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local).Unix(),
			time.Date(2030, 12, 31, 23, 59, 59, 0, time.Local).Unix(),
		),
		gen.Int64Range(
			time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local).Unix(),
			time.Date(2030, 12, 31, 23, 59, 59, 0, time.Local).Unix(),
		),
	))

	// Property 7d: When closeOldDate is nil (not configured), old bots are always included
	properties.Property("old bots always included when closeOldDate is nil", prop.ForAll(
		func(nowUnix int64, oldBotCount int) bool {
			now := time.Unix(nowUnix, 0)

			cfg := &config.Config{
				CloseOldDateParsed: nil, // not configured
				ChangeBotAlertMsg:  map[string]string{"en": "Please switch"},
			}

			newSender := &mockBotSender{name: "new_bot"}
			newBot := telegram.NewForTestWithOptions(cfg, nil, newSender, telegram.BotOptions{})

			oldBots := make([]*telegram.Bot, oldBotCount)
			for i := 0; i < oldBotCount; i++ {
				oldSender := &mockBotSender{name: fmt.Sprintf("old_bot_%d", i)}
				oldBots[i] = telegram.NewForTestWithOptions(cfg, nil, oldSender, telegram.BotOptions{IsOld: true})
			}

			mm := &MigrationManager{
				config:      cfg,
				newBot:      newBot,
				oldBots:     oldBots,
				alertSender: NewAlertSender(cfg),
			}

			endpoints := mm.ActiveMailEndpoints(now)

			// Should always contain 1 new + all old bots
			expectedCount := 1 + oldBotCount
			if len(endpoints) != expectedCount {
				t.Logf("Expected %d endpoints when closeOldDate is nil, got %d", expectedCount, len(endpoints))
				return false
			}

			return true
		},
		gen.Int64Range(
			time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local).Unix(),
			time.Date(2030, 12, 31, 23, 59, 59, 0, time.Local).Unix(),
		),
		oldBotCountGen,
	))

	properties.TestingRun(t)
}
