package telegram

import (
	"os"
	"strings"
	"sync"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"go-version-rewrite/internal/config"
	"go-version-rewrite/internal/db"
	"go-version-rewrite/internal/io"
)

// mockSender captures all sent messages and request calls for test assertions.
type mockSender struct {
	mu       sync.Mutex
	messages []tgbotapi.Chattable
	requests []tgbotapi.Chattable
}

func (m *mockSender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, c)
	return tgbotapi.Message{}, nil
}

func (m *mockSender) Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, c)
	return &tgbotapi.APIResponse{Ok: true}, nil
}

func (m *mockSender) sentTexts() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var texts []string
	for _, c := range m.messages {
		if msg, ok := c.(tgbotapi.MessageConfig); ok {
			texts = append(texts, msg.Text)
		}
	}
	return texts
}

// newTestBotWithMock creates a Bot with a mock sender and real IO backed by a temp SQLite DB.
func newTestBotWithMock(t *testing.T, adminID int64) (*Bot, *mockSender) {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "test-sendall-*.db")
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
	bot := NewForTest(cfg, ioModule, sender)

	return bot, sender
}

func TestHandleSendAll_AdminWithArgs(t *testing.T) {
	adminID := int64(12345)
	bot, sender := newTestBotWithMock(t, adminID)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: adminID},
		From: &tgbotapi.User{LanguageCode: "en"},
	}

	bot.handleSendAll(msg, []string{"hello", "world"})

	texts := sender.sentTexts()
	// Should have sent the confirmation message to admin
	found := false
	for _, text := range texts {
		if strings.Contains(text, "Broadcast complete") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected broadcast confirmation message, got: %v", texts)
	}
}

func TestHandleSendAll_AdminWithArgsChinese(t *testing.T) {
	adminID := int64(12345)
	bot, sender := newTestBotWithMock(t, adminID)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: adminID},
		From: &tgbotapi.User{LanguageCode: "zh-cn"},
	}

	bot.handleSendAll(msg, []string{"你好", "世界"})

	texts := sender.sentTexts()
	found := false
	for _, text := range texts {
		if strings.Contains(text, "广播已发送") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected Chinese broadcast confirmation, got: %v", texts)
	}
}

func TestHandleSendAll_NonAdminSilentIgnore(t *testing.T) {
	adminID := int64(12345)
	bot, sender := newTestBotWithMock(t, adminID)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 99999}, // not admin
		From: &tgbotapi.User{LanguageCode: "en"},
	}

	bot.handleSendAll(msg, []string{"hello"})

	texts := sender.sentTexts()
	if len(texts) != 0 {
		t.Errorf("expected no messages for non-admin, got: %v", texts)
	}
}

func TestHandleSendAll_AdminEmptyArgs(t *testing.T) {
	adminID := int64(12345)
	bot, sender := newTestBotWithMock(t, adminID)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: adminID},
		From: &tgbotapi.User{LanguageCode: "en"},
	}

	bot.handleSendAll(msg, []string{})

	texts := sender.sentTexts()
	if len(texts) != 0 {
		t.Errorf("expected no messages for empty args, got: %v", texts)
	}
}

func TestHandleSendAll_AdminNilArgs(t *testing.T) {
	adminID := int64(12345)
	bot, sender := newTestBotWithMock(t, adminID)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: adminID},
		From: &tgbotapi.User{LanguageCode: "en"},
	}

	bot.handleSendAll(msg, nil)

	texts := sender.sentTexts()
	if len(texts) != 0 {
		t.Errorf("expected no messages for nil args, got: %v", texts)
	}
}

func TestHandleSendAll_JoinsArgsWithSpace(t *testing.T) {
	adminID := int64(12345)
	bot, sender := newTestBotWithMock(t, adminID)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: adminID},
		From: &tgbotapi.User{LanguageCode: "en"},
	}

	// Verify that multiple args are joined with space by checking the confirmation is sent
	// (IO.SendAll uses its own bot reference for broadcasting; we verify the join logic
	// indirectly — if args were empty after join, no confirmation would be sent)
	bot.handleSendAll(msg, []string{"hello", "beautiful", "world"})

	texts := sender.sentTexts()
	if len(texts) != 1 {
		t.Fatalf("expected exactly 1 confirmation message, got %d: %v", len(texts), texts)
	}
	if !strings.Contains(texts[0], "Broadcast complete") {
		t.Errorf("expected broadcast confirmation, got: %q", texts[0])
	}
}
