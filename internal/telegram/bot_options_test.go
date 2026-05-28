package telegram

import (
	"os"
	"sync"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"go-version-rewrite/internal/config"
	"go-version-rewrite/internal/db"
	"go-version-rewrite/internal/io"
)

// hookCall records a single AfterInteraction hook invocation.
type hookCall struct {
	tgID int64
	lang string
}

// TestNewForTestWithOptions_AfterInteraction_Message verifies that the AfterInteraction hook
// fires after handleMessage is called, including for non-text messages (empty text).
func TestNewForTestWithOptions_AfterInteraction_Message(t *testing.T) {
	var mu sync.Mutex
	var calls []hookCall

	hook := func(sender BotSender, tgID int64, lang string) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, hookCall{tgID: tgID, lang: lang})
	}

	bot, _ := newTestBotWithMockAndOptions(t, 0, BotOptions{
		IsOld:            true,
		AfterInteraction: hook,
	})

	// Simulate processing a text message
	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 12345},
		From: &tgbotapi.User{ID: 12345, LanguageCode: "zh"},
		Text: "/start",
	}
	bot.processUpdate(tgbotapi.Update{Message: msg})

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 1 {
		t.Fatalf("expected 1 hook call, got %d", len(calls))
	}
	if calls[0].tgID != 12345 {
		t.Errorf("expected tgID=12345, got %d", calls[0].tgID)
	}
	if calls[0].lang != "zh" {
		t.Errorf("expected lang=zh, got %q", calls[0].lang)
	}
}

// TestNewForTestWithOptions_AfterInteraction_EmptyText verifies hook fires even for empty text messages.
func TestNewForTestWithOptions_AfterInteraction_EmptyText(t *testing.T) {
	var mu sync.Mutex
	var calls []hookCall

	hook := func(sender BotSender, tgID int64, lang string) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, hookCall{tgID: tgID, lang: lang})
	}

	bot, _ := newTestBotWithMockAndOptions(t, 0, BotOptions{
		IsOld:            true,
		AfterInteraction: hook,
	})

	// Simulate a non-text message (e.g., image/sticker) — Text is empty
	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 99999},
		From: &tgbotapi.User{ID: 99999, LanguageCode: "en"},
		Text: "", // empty text — handleMessage returns early, but hook should still fire
	}
	bot.processUpdate(tgbotapi.Update{Message: msg})

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 1 {
		t.Fatalf("expected 1 hook call for empty text message, got %d", len(calls))
	}
	if calls[0].tgID != 99999 {
		t.Errorf("expected tgID=99999, got %d", calls[0].tgID)
	}
}

// TestNewForTestWithOptions_AfterInteraction_CallbackQuery verifies hook fires for callback queries.
func TestNewForTestWithOptions_AfterInteraction_CallbackQuery(t *testing.T) {
	var mu sync.Mutex
	var calls []hookCall

	hook := func(sender BotSender, tgID int64, lang string) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, hookCall{tgID: tgID, lang: lang})
	}

	bot, _ := newTestBotWithMockAndOptions(t, 0, BotOptions{
		IsOld:            true,
		AfterInteraction: hook,
	})

	// Simulate a callback query with Message present
	cb := &tgbotapi.CallbackQuery{
		ID:   "cb1",
		From: &tgbotapi.User{ID: 777, LanguageCode: "zh-Hans"},
		Message: &tgbotapi.Message{
			Chat:      &tgbotapi.Chat{ID: 888},
			MessageID: 1,
		},
		Data: "noop",
	}
	bot.processUpdate(tgbotapi.Update{CallbackQuery: cb})

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 1 {
		t.Fatalf("expected 1 hook call for callback query, got %d", len(calls))
	}
	if calls[0].tgID != 888 {
		t.Errorf("expected tgID=888 (from Message.Chat.ID), got %d", calls[0].tgID)
	}
}

// TestNewForTestWithOptions_AfterInteraction_CallbackQuery_NoMessage verifies
// that when CallbackQuery.Message is nil, tgID falls back to From.ID.
func TestNewForTestWithOptions_AfterInteraction_CallbackQuery_NoMessage(t *testing.T) {
	var mu sync.Mutex
	var calls []hookCall

	hook := func(sender BotSender, tgID int64, lang string) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, hookCall{tgID: tgID, lang: lang})
	}

	bot, _ := newTestBotWithMockAndOptions(t, 0, BotOptions{
		IsOld:            true,
		AfterInteraction: hook,
	})

	// Simulate a callback query without Message (rare but possible)
	cb := &tgbotapi.CallbackQuery{
		ID:      "cb2",
		From:    &tgbotapi.User{ID: 555, LanguageCode: "en"},
		Message: nil,
		Data:    "noop",
	}
	bot.processUpdate(tgbotapi.Update{CallbackQuery: cb})

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 1 {
		t.Fatalf("expected 1 hook call, got %d", len(calls))
	}
	if calls[0].tgID != 555 {
		t.Errorf("expected tgID=555 (from From.ID fallback), got %d", calls[0].tgID)
	}
}

// TestNewForTestWithOptions_NoHook_NoCall verifies that without AfterInteraction set, no hook fires.
func TestNewForTestWithOptions_NoHook_NoCall(t *testing.T) {
	bot, _ := newTestBotWithMockAndOptions(t, 0, BotOptions{})

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 12345},
		From: &tgbotapi.User{ID: 12345, LanguageCode: "en"},
		Text: "/start",
	}
	// Should not panic even without hook
	bot.processUpdate(tgbotapi.Update{Message: msg})
}

// TestNewForTestWithOptions_HookReceivesSameSender verifies the hook receives the bot's own sender.
func TestNewForTestWithOptions_HookReceivesSameSender(t *testing.T) {
	var receivedSender BotSender

	hook := func(sender BotSender, tgID int64, lang string) {
		receivedSender = sender
	}

	bot, mockS := newTestBotWithMockAndOptions(t, 0, BotOptions{
		AfterInteraction: hook,
	})

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 12345},
		From: &tgbotapi.User{ID: 12345, LanguageCode: "en"},
		Text: "/start",
	}
	bot.processUpdate(tgbotapi.Update{Message: msg})

	if receivedSender != mockS {
		t.Error("hook did not receive the bot's own sender")
	}
}

// newTestBotWithMockAndOptions creates a Bot with a mock sender, real IO, and BotOptions.
func newTestBotWithMockAndOptions(t *testing.T, adminID int64, opts BotOptions) (*Bot, *mockSender) {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "test-options-*.db")
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
		AdminTgID:  adminID,
	}
	ioModule := io.New(database, cfg)

	sender := &mockSender{}
	bot := NewForTestWithOptions(cfg, ioModule, sender, opts)

	return bot, sender
}
