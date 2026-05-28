package migration

import (
	"fmt"
	"os"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"

	"go-version-rewrite/internal/config"
	"go-version-rewrite/internal/db"
	"go-version-rewrite/internal/io"
	"go-version-rewrite/internal/telegram"
)

// cronMockSender records all Send calls for verification in cron property tests.
type cronMockSender struct {
	name         string
	sentMessages []tgbotapi.Chattable
}

func (m *cronMockSender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	m.sentMessages = append(m.sentMessages, c)
	return tgbotapi.Message{}, nil
}

func (m *cronMockSender) Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	return &tgbotapi.APIResponse{Ok: true}, nil
}

// Verify cronMockSender implements io.TelegramSender
var _ io.TelegramSender = (*cronMockSender)(nil)

// Feature: bot-migration, Property 6: 定时任务唯一用户投递且仅通过旧 Bot
// Validates: Requirements 6.1, 6.2
func TestProperty_CronUniqueUserDeliveryOnlyOldBots(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Generator for number of old bots (1 to 4)
	oldBotCountGen := gen.IntRange(1, 4)

	// Generator for number of domain bindings (1 to 20)
	bindingCountGen := gen.IntRange(1, 20)

	// Generator for tgID values (use a small range to ensure collisions/deduplication)
	tgIDGen := gen.Int64Range(100, 110)

	// Property 6a: Each unique tgID receives exactly one alert per old bot
	properties.Property("each unique tgID receives exactly one alert per old bot", prop.ForAll(
		func(oldBotCount int, bindingCount int, tgIDs []int64) bool {
			// Ensure we have enough tgIDs for the bindings
			if len(tgIDs) < bindingCount {
				return true // skip
			}
			tgIDs = tgIDs[:bindingCount]

			// Create temp DB
			tmpFile, err := os.CreateTemp("", "cron_prop_*.db")
			if err != nil {
				t.Logf("Failed to create temp file: %v", err)
				return false
			}
			defer os.Remove(tmpFile.Name())
			tmpFile.Close()

			database, err := db.New(tmpFile.Name())
			if err != nil {
				t.Logf("Failed to create DB: %v", err)
				return false
			}
			defer database.Close()

			// Insert domain bindings (multiple domains can map to same tgID)
			for i := 0; i < bindingCount; i++ {
				domain := fmt.Sprintf("domain%d.example.com", i)
				database.InsertDomain(domain, tgIDs[i])
			}

			// Calculate expected unique tgIDs
			uniqueTgIDs := make(map[int64]bool)
			for _, id := range tgIDs {
				uniqueTgIDs[id] = true
			}
			expectedUniqueCount := len(uniqueTgIDs)

			// Create config with alert message
			cfg := &config.Config{
				MailDomain:        "example.com",
				ChangeBotAlertMsg: map[string]string{"en": "Please switch to new bot"},
			}

			// Create IO module
			ioModule := io.New(database, cfg)

			// Create mock old bot senders
			oldSenders := make([]*cronMockSender, oldBotCount)
			oldBots := make([]*telegram.Bot, oldBotCount)
			for i := 0; i < oldBotCount; i++ {
				oldSenders[i] = &cronMockSender{name: fmt.Sprintf("old_bot_%d", i)}
				oldBots[i] = telegram.NewForTestWithOptions(cfg, ioModule, oldSenders[i], telegram.BotOptions{IsOld: true})
			}

			// Create AlertSender and CronScheduler
			alertSender := NewAlertSender(cfg)
			cron := NewCronScheduler(cfg, ioModule, database, oldBots, alertSender)

			// Run once
			cron.RunOnce()

			// Verify: each old bot sender should have sent exactly expectedUniqueCount messages
			for i, sender := range oldSenders {
				if len(sender.sentMessages) != expectedUniqueCount {
					t.Logf("Old bot %d: expected %d messages (one per unique tgID), got %d",
						i, expectedUniqueCount, len(sender.sentMessages))
					return false
				}
			}

			return true
		},
		oldBotCountGen,
		bindingCountGen,
		gen.SliceOfN(20, tgIDGen),
	))

	// Property 6b: Only old bot senders are used (no new bot sender involved)
	properties.Property("only old bot senders are used, no new bot sender involved", prop.ForAll(
		func(oldBotCount int, bindingCount int, tgIDs []int64) bool {
			if len(tgIDs) < bindingCount {
				return true // skip
			}
			tgIDs = tgIDs[:bindingCount]

			// Create temp DB
			tmpFile, err := os.CreateTemp("", "cron_prop_newbot_*.db")
			if err != nil {
				t.Logf("Failed to create temp file: %v", err)
				return false
			}
			defer os.Remove(tmpFile.Name())
			tmpFile.Close()

			database, err := db.New(tmpFile.Name())
			if err != nil {
				t.Logf("Failed to create DB: %v", err)
				return false
			}
			defer database.Close()

			// Insert domain bindings
			for i := 0; i < bindingCount; i++ {
				domain := fmt.Sprintf("domain%d.test.com", i)
				database.InsertDomain(domain, tgIDs[i])
			}

			cfg := &config.Config{
				MailDomain:        "test.com",
				ChangeBotAlertMsg: map[string]string{"en": "Switch now"},
			}

			ioModule := io.New(database, cfg)

			// Create a new bot sender to verify it's NOT used
			newBotSender := &cronMockSender{name: "new_bot"}
			_ = telegram.NewForTestWithOptions(cfg, ioModule, newBotSender, telegram.BotOptions{})

			// Create old bot senders
			oldSenders := make([]*cronMockSender, oldBotCount)
			oldBots := make([]*telegram.Bot, oldBotCount)
			for i := 0; i < oldBotCount; i++ {
				oldSenders[i] = &cronMockSender{name: fmt.Sprintf("old_bot_%d", i)}
				oldBots[i] = telegram.NewForTestWithOptions(cfg, ioModule, oldSenders[i], telegram.BotOptions{IsOld: true})
			}

			// CronScheduler only has old bots (by design, it doesn't use new bot)
			alertSender := NewAlertSender(cfg)
			cron := NewCronScheduler(cfg, ioModule, database, oldBots, alertSender)

			// Run once
			cron.RunOnce()

			// Verify: new bot sender should have received NO messages
			if len(newBotSender.sentMessages) != 0 {
				t.Logf("New bot sender received %d messages, expected 0", len(newBotSender.sentMessages))
				return false
			}

			// Verify: old bot senders should have received messages (if there are bindings)
			uniqueTgIDs := make(map[int64]bool)
			for _, id := range tgIDs {
				uniqueTgIDs[id] = true
			}
			expectedPerBot := len(uniqueTgIDs)

			for i, sender := range oldSenders {
				if len(sender.sentMessages) != expectedPerBot {
					t.Logf("Old bot %d: expected %d messages, got %d", i, expectedPerBot, len(sender.sentMessages))
					return false
				}
			}

			return true
		},
		oldBotCountGen,
		bindingCountGen,
		gen.SliceOfN(20, tgIDGen),
	))

	// Property 6c: Total alerts = unique_tgIDs * num_old_bots
	properties.Property("total alerts equals unique_tgIDs times num_old_bots", prop.ForAll(
		func(oldBotCount int, bindingCount int, tgIDs []int64) bool {
			if len(tgIDs) < bindingCount {
				return true // skip
			}
			tgIDs = tgIDs[:bindingCount]

			// Create temp DB
			tmpFile, err := os.CreateTemp("", "cron_prop_total_*.db")
			if err != nil {
				t.Logf("Failed to create temp file: %v", err)
				return false
			}
			defer os.Remove(tmpFile.Name())
			tmpFile.Close()

			database, err := db.New(tmpFile.Name())
			if err != nil {
				t.Logf("Failed to create DB: %v", err)
				return false
			}
			defer database.Close()

			// Insert domain bindings
			for i := 0; i < bindingCount; i++ {
				domain := fmt.Sprintf("d%d.mail.com", i)
				database.InsertDomain(domain, tgIDs[i])
			}

			// Calculate expected unique tgIDs
			uniqueTgIDs := make(map[int64]bool)
			for _, id := range tgIDs {
				uniqueTgIDs[id] = true
			}
			expectedTotal := len(uniqueTgIDs) * oldBotCount

			cfg := &config.Config{
				MailDomain:        "mail.com",
				ChangeBotAlertMsg: map[string]string{"en": "Migrate now"},
			}

			ioModule := io.New(database, cfg)

			// Create old bot senders
			oldSenders := make([]*cronMockSender, oldBotCount)
			oldBots := make([]*telegram.Bot, oldBotCount)
			for i := 0; i < oldBotCount; i++ {
				oldSenders[i] = &cronMockSender{name: fmt.Sprintf("old_bot_%d", i)}
				oldBots[i] = telegram.NewForTestWithOptions(cfg, ioModule, oldSenders[i], telegram.BotOptions{IsOld: true})
			}

			alertSender := NewAlertSender(cfg)
			cron := NewCronScheduler(cfg, ioModule, database, oldBots, alertSender)

			// Run once
			cron.RunOnce()

			// Count total messages across all old bot senders
			totalSent := 0
			for _, sender := range oldSenders {
				totalSent += len(sender.sentMessages)
			}

			if totalSent != expectedTotal {
				t.Logf("Total alerts: expected %d (unique=%d * bots=%d), got %d",
					expectedTotal, len(uniqueTgIDs), oldBotCount, totalSent)
				return false
			}

			return true
		},
		oldBotCountGen,
		bindingCountGen,
		gen.SliceOfN(20, tgIDGen),
	))

	properties.TestingRun(t)
}
