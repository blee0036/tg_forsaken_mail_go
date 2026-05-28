package migration

import (
	"fmt"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"go-version-rewrite/internal/config"
	"go-version-rewrite/internal/io"
)

// mockSender implements io.TelegramSender for testing.
type mockSender struct {
	sentMessages []tgbotapi.Chattable
	failCount    int // number of times Send should fail before succeeding
	callCount    int
}

func (m *mockSender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	m.callCount++
	if m.callCount <= m.failCount {
		return tgbotapi.Message{}, fmt.Errorf("mock send error #%d", m.callCount)
	}
	m.sentMessages = append(m.sentMessages, c)
	return tgbotapi.Message{}, nil
}

func (m *mockSender) Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	return &tgbotapi.APIResponse{Ok: true}, nil
}

// Verify mockSender implements io.TelegramSender
var _ io.TelegramSender = (*mockSender)(nil)

func TestResolveAlertMessage_ExactMatch(t *testing.T) {
	cfg := &config.Config{
		ChangeBotAlertMsg: map[string]string{
			"en": "Switch to new bot",
			"zh": "请切换到新 Bot",
		},
	}
	a := NewAlertSender(cfg)

	got := a.ResolveAlertMessage("en")
	if got != "Switch to new bot" {
		t.Errorf("expected exact match for 'en', got %q", got)
	}

	got = a.ResolveAlertMessage("zh")
	if got != "请切换到新 Bot" {
		t.Errorf("expected exact match for 'zh', got %q", got)
	}
}

func TestResolveAlertMessage_PrefixNormalization(t *testing.T) {
	cfg := &config.Config{
		ChangeBotAlertMsg: map[string]string{
			"en": "Switch to new bot",
			"zh": "请切换到新 Bot",
		},
	}
	a := NewAlertSender(cfg)

	// zh-Hans should normalize to zh
	got := a.ResolveAlertMessage("zh-Hans")
	if got != "请切换到新 Bot" {
		t.Errorf("expected prefix normalization 'zh-Hans' → 'zh', got %q", got)
	}

	// en-US should normalize to en
	got = a.ResolveAlertMessage("en-US")
	if got != "Switch to new bot" {
		t.Errorf("expected prefix normalization 'en-US' → 'en', got %q", got)
	}

	// zh_CN should normalize to zh (underscore separator)
	got = a.ResolveAlertMessage("zh_CN")
	if got != "请切换到新 Bot" {
		t.Errorf("expected prefix normalization 'zh_CN' → 'zh', got %q", got)
	}
}

func TestResolveAlertMessage_FallbackToEn(t *testing.T) {
	cfg := &config.Config{
		ChangeBotAlertMsg: map[string]string{
			"en": "Switch to new bot",
			"fr": "Passez au nouveau bot",
		},
	}
	a := NewAlertSender(cfg)

	// Unknown language with no prefix match should fall back to "en"
	got := a.ResolveAlertMessage("ja")
	if got != "Switch to new bot" {
		t.Errorf("expected fallback to 'en', got %q", got)
	}
}

func TestResolveAlertMessage_FallbackToZh(t *testing.T) {
	cfg := &config.Config{
		ChangeBotAlertMsg: map[string]string{
			"zh": "请切换到新 Bot",
			"fr": "Passez au nouveau bot",
		},
	}
	a := NewAlertSender(cfg)

	// No "en" key, should fall back to "zh"
	got := a.ResolveAlertMessage("ja")
	if got != "请切换到新 Bot" {
		t.Errorf("expected fallback to 'zh', got %q", got)
	}
}

func TestResolveAlertMessage_FallbackToFirstAlphabetically(t *testing.T) {
	cfg := &config.Config{
		ChangeBotAlertMsg: map[string]string{
			"fr": "Passez au nouveau bot",
			"de": "Wechseln Sie zum neuen Bot",
		},
	}
	a := NewAlertSender(cfg)

	// No "en" or "zh", should fall back to first alphabetically ("de")
	got := a.ResolveAlertMessage("ja")
	if got != "Wechseln Sie zum neuen Bot" {
		t.Errorf("expected fallback to first alphabetically 'de', got %q", got)
	}
}

func TestResolveAlertMessage_EmptyMap(t *testing.T) {
	cfg := &config.Config{
		ChangeBotAlertMsg: map[string]string{},
	}
	a := NewAlertSender(cfg)

	got := a.ResolveAlertMessage("en")
	if got != "" {
		t.Errorf("expected empty string for empty map, got %q", got)
	}
}

func TestSendAlert_MarkdownSuccess(t *testing.T) {
	cfg := &config.Config{
		ChangeBotAlertMsg: map[string]string{
			"en": "Switch to *new bot*",
		},
	}
	a := NewAlertSender(cfg)
	sender := &mockSender{}

	a.SendAlert(sender, 12345, "en")

	if len(sender.sentMessages) != 1 {
		t.Fatalf("expected 1 message sent, got %d", len(sender.sentMessages))
	}
}

func TestSendAlert_MarkdownFailsPlainTextSucceeds(t *testing.T) {
	cfg := &config.Config{
		ChangeBotAlertMsg: map[string]string{
			"en": "Switch to *new bot*",
		},
	}
	a := NewAlertSender(cfg)
	sender := &mockSender{failCount: 1} // First call fails, second succeeds

	a.SendAlert(sender, 12345, "en")

	if sender.callCount != 2 {
		t.Fatalf("expected 2 send attempts, got %d", sender.callCount)
	}
	if len(sender.sentMessages) != 1 {
		t.Fatalf("expected 1 message sent (plain text retry), got %d", len(sender.sentMessages))
	}
}

func TestSendAlert_BothFail(t *testing.T) {
	cfg := &config.Config{
		ChangeBotAlertMsg: map[string]string{
			"en": "Switch to *new bot*",
		},
	}
	a := NewAlertSender(cfg)
	sender := &mockSender{failCount: 2} // Both calls fail

	// Should not panic, just log errors
	a.SendAlert(sender, 12345, "en")

	if sender.callCount != 2 {
		t.Fatalf("expected 2 send attempts, got %d", sender.callCount)
	}
	if len(sender.sentMessages) != 0 {
		t.Fatalf("expected 0 messages sent, got %d", len(sender.sentMessages))
	}
}

func TestSendAlert_EmptyMessage(t *testing.T) {
	cfg := &config.Config{
		ChangeBotAlertMsg: map[string]string{},
	}
	a := NewAlertSender(cfg)
	sender := &mockSender{}

	a.SendAlert(sender, 12345, "en")

	if sender.callCount != 0 {
		t.Fatalf("expected 0 send attempts for empty message, got %d", sender.callCount)
	}
}

func TestHasAlert(t *testing.T) {
	tests := []struct {
		name     string
		msgs     map[string]string
		expected bool
	}{
		{"with messages", map[string]string{"en": "hello"}, true},
		{"empty map", map[string]string{}, false},
		{"nil map", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{ChangeBotAlertMsg: tt.msgs}
			a := NewAlertSender(cfg)
			if got := a.HasAlert(); got != tt.expected {
				t.Errorf("HasAlert() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestAsAlertFunc_NilWhenNoMessages(t *testing.T) {
	cfg := &config.Config{ChangeBotAlertMsg: map[string]string{}}
	a := NewAlertSender(cfg)

	fn := a.AsAlertFunc()
	if fn != nil {
		t.Error("expected nil AlertFunc when no messages configured")
	}
}

func TestAsAlertFunc_ReturnsWorkingFunc(t *testing.T) {
	cfg := &config.Config{
		ChangeBotAlertMsg: map[string]string{
			"en": "Please switch",
		},
	}
	a := NewAlertSender(cfg)

	fn := a.AsAlertFunc()
	if fn == nil {
		t.Fatal("expected non-nil AlertFunc")
	}

	sender := &mockSender{}
	fn(sender, 12345, "en")

	if len(sender.sentMessages) != 1 {
		t.Fatalf("expected AlertFunc to send 1 message, got %d", len(sender.sentMessages))
	}
}
