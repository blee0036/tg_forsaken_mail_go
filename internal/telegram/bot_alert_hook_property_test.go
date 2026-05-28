package telegram

import (
	"os"
	"sync"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"

	"go-version-rewrite/internal/config"
	"go-version-rewrite/internal/db"
	"go-version-rewrite/internal/io"
)

// Feature: bot-migration, Property 12: 旧 Bot 交互触发 Alert
// Validates: Requirements 3.2

// alertHookRecord records a single AfterInteraction hook invocation.
type alertHookRecord struct {
	sender BotSender
	tgID   int64
	lang   string
}

// alertHookTracker tracks all hook invocations in a thread-safe manner.
type alertHookTracker struct {
	mu    sync.Mutex
	calls []alertHookRecord
}

func (h *alertHookTracker) hook() AfterInteractionFunc {
	return func(sender BotSender, tgID int64, lang string) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.calls = append(h.calls, alertHookRecord{sender: sender, tgID: tgID, lang: lang})
	}
}

func (h *alertHookTracker) callCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.calls)
}

func (h *alertHookTracker) lastCall() alertHookRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.calls[len(h.calls)-1]
}

func (h *alertHookTracker) reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = nil
}

// createOldBotWithHookTracker creates an old Bot with AfterInteraction hook tracking.
func createOldBotWithHookTracker(t *testing.T) (*Bot, *mockSender, *alertHookTracker) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "test-alert-hook-*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	database, err := db.New(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfg := &config.Config{
		MailDomain: "test.example.com",
		AdminTgID:  99999,
	}
	ioModule := io.New(database, cfg)

	tracker := &alertHookTracker{}
	sender := &mockSender{}
	bot := NewForTestWithOptions(cfg, ioModule, sender, BotOptions{
		IsOld:            true,
		AfterInteraction: tracker.hook(),
	})

	return bot, sender, tracker
}

func TestProperty_OldBotInteractionTriggersAlert(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Generator for chat IDs (positive values)
	genChatID := gen.Int64Range(1, 9999999)

	// Generator for language codes
	genLang := gen.OneConstOf("en", "zh", "zh-Hans", "zh-Hant", "fr", "de", "ja", "ko")

	// Generator for message commands (various commands the bot handles)
	genCommand := gen.OneConstOf(
		"/start", "/help", "/list", "/bind example.com",
		"/dismiss example.com", "/unblock_domain spam.com",
		"/unblock_sender bad@evil.com", "/unblock_receiver me@test.com",
		"/list_block_domain", "/list_block_sender", "/list_block_receiver",
		"/lang", "/unknown_command", "random text",
	)

	// Generator for callback actions
	genCallbackAction := gen.OneConstOf(
		"noop", "quick_start", "main_menu:domains", "main_menu:help",
		"help_cat:domain", "help_cat:block", "help_cat:other",
		"help_back", "dismiss_no", "go_main", "set_lang:en", "set_lang:zh",
		"block_cat:sender", "block_cat:domain", "block_cat:receiver",
	)

	// Property: Message interactions trigger exactly one hook call
	properties.Property(
		"old Bot message interaction triggers exactly one AfterInteraction hook call",
		prop.ForAll(
			func(chatID int64, lang string, command string) bool {
				bot, sender, tracker := createOldBotWithHookTracker(t)

				msg := &tgbotapi.Message{
					Chat: &tgbotapi.Chat{ID: chatID},
					From: &tgbotapi.User{ID: chatID, LanguageCode: lang},
					Text: command,
				}
				bot.processUpdate(tgbotapi.Update{Message: msg})

				// Verify exactly one hook call
				count := tracker.callCount()
				if count != 1 {
					t.Logf("expected 1 hook call, got %d (chatID=%d, command=%q)", count, chatID, command)
					return false
				}

				// Verify correct tgID (from Message.Chat.ID)
				record := tracker.lastCall()
				if record.tgID != chatID {
					t.Logf("expected tgID=%d, got %d", chatID, record.tgID)
					return false
				}

				// Verify the hook received the bot's own sender
				if record.sender != sender {
					t.Logf("hook did not receive the bot's own sender")
					return false
				}

				return true
			},
			genChatID,
			genLang,
			genCommand,
		),
	)

	// Property: Callback query interactions trigger exactly one hook call
	properties.Property(
		"old Bot callback query interaction triggers exactly one AfterInteraction hook call",
		prop.ForAll(
			func(chatID int64, fromID int64, lang string, callbackData string) bool {
				bot, sender, tracker := createOldBotWithHookTracker(t)

				cb := &tgbotapi.CallbackQuery{
					ID:   "cb-test",
					From: &tgbotapi.User{ID: fromID, LanguageCode: lang},
					Message: &tgbotapi.Message{
						MessageID: 1,
						Chat:      &tgbotapi.Chat{ID: chatID},
					},
					Data: callbackData,
				}
				bot.processUpdate(tgbotapi.Update{CallbackQuery: cb})

				// Verify exactly one hook call
				count := tracker.callCount()
				if count != 1 {
					t.Logf("expected 1 hook call, got %d (chatID=%d, callbackData=%q)", count, chatID, callbackData)
					return false
				}

				// Verify correct tgID (from CallbackQuery.Message.Chat.ID when Message is present)
				record := tracker.lastCall()
				if record.tgID != chatID {
					t.Logf("expected tgID=%d (from Message.Chat.ID), got %d", chatID, record.tgID)
					return false
				}

				// Verify the hook received the bot's own sender
				if record.sender != sender {
					t.Logf("hook did not receive the bot's own sender")
					return false
				}

				return true
			},
			genChatID,
			gen.Int64Range(1, 9999999),
			genLang,
			genCallbackAction,
		),
	)

	// Property: Callback query without Message falls back to From.ID
	// Note: When Message is nil, only "noop" action is safe (other handlers access Message fields).
	// This tests the tgID fallback logic in processUpdate.
	properties.Property(
		"old Bot callback query without Message uses From.ID as tgID",
		prop.ForAll(
			func(fromID int64, lang string) bool {
				bot, sender, tracker := createOldBotWithHookTracker(t)

				cb := &tgbotapi.CallbackQuery{
					ID:      "cb-no-msg",
					From:    &tgbotapi.User{ID: fromID, LanguageCode: lang},
					Message: nil, // No Message present
					Data:    "noop",
				}
				bot.processUpdate(tgbotapi.Update{CallbackQuery: cb})

				// Verify exactly one hook call
				count := tracker.callCount()
				if count != 1 {
					t.Logf("expected 1 hook call, got %d (fromID=%d)", count, fromID)
					return false
				}

				// Verify tgID falls back to From.ID
				record := tracker.lastCall()
				if record.tgID != int64(fromID) {
					t.Logf("expected tgID=%d (from From.ID fallback), got %d", fromID, record.tgID)
					return false
				}

				// Verify the hook received the bot's own sender
				if record.sender != sender {
					t.Logf("hook did not receive the bot's own sender")
					return false
				}

				return true
			},
			gen.Int64Range(1, 9999999),
			genLang,
		),
	)

	// Property: Empty text messages still trigger the hook
	properties.Property(
		"old Bot empty text message still triggers exactly one AfterInteraction hook call",
		prop.ForAll(
			func(chatID int64, lang string) bool {
				bot, sender, tracker := createOldBotWithHookTracker(t)

				msg := &tgbotapi.Message{
					Chat: &tgbotapi.Chat{ID: chatID},
					From: &tgbotapi.User{ID: chatID, LanguageCode: lang},
					Text: "", // Empty text (e.g., image/sticker)
				}
				bot.processUpdate(tgbotapi.Update{Message: msg})

				// Verify exactly one hook call even for empty text
				count := tracker.callCount()
				if count != 1 {
					t.Logf("expected 1 hook call for empty text, got %d (chatID=%d)", count, chatID)
					return false
				}

				record := tracker.lastCall()
				if record.tgID != chatID {
					t.Logf("expected tgID=%d, got %d", chatID, record.tgID)
					return false
				}

				if record.sender != sender {
					t.Logf("hook did not receive the bot's own sender for empty text message")
					return false
				}

				return true
			},
			genChatID,
			genLang,
		),
	)

	properties.TestingRun(t)
}
