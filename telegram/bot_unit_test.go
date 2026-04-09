package telegram

import (
	"fmt"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"go-version-rewrite/internal/config"
)

// --- Test 1: registerCommands generates correct command list ---

func TestRegisterCommands_GeneratesCorrectCommandList(t *testing.T) {
	bot, sender := newTestBotWithMock(t, 0)

	err := bot.registerCommands()
	if err != nil {
		t.Fatalf("registerCommands returned error: %v", err)
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()

	if len(sender.requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(sender.requests))
	}

	setCmd, ok := sender.requests[0].(tgbotapi.SetMyCommandsConfig)
	if !ok {
		t.Fatalf("expected SetMyCommandsConfig, got %T", sender.requests[0])
	}

	expectedCommands := []string{
		"start", "help", "list", "bind", "dismiss",
		"unblock_domain", "unblock_sender", "unblock_receiver",
		"list_block_domain", "list_block_sender", "list_block_receiver",
	}

	if len(setCmd.Commands) != len(expectedCommands) {
		t.Fatalf("expected %d commands, got %d", len(expectedCommands), len(setCmd.Commands))
	}

	for i, expected := range expectedCommands {
		if setCmd.Commands[i].Command != expected {
			t.Errorf("command[%d]: expected %q, got %q", i, expected, setCmd.Commands[i].Command)
		}
		if setCmd.Commands[i].Description == "" {
			t.Errorf("command[%d] %q has empty description", i, expected)
		}
	}
}

// --- Test 2: New user /start → welcome message with "Quick Start" button ---

func TestHandleStart_NewUser_WelcomeWithQuickStart(t *testing.T) {
	bot, sender := newTestBotWithMock(t, 0)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 100},
		From: &tgbotapi.User{LanguageCode: "en"},
	}

	bot.handleStart(msg, "en")

	sender.mu.Lock()
	defer sender.mu.Unlock()

	if len(sender.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(sender.messages))
	}

	msgCfg, ok := sender.messages[0].(tgbotapi.MessageConfig)
	if !ok {
		t.Fatalf("expected MessageConfig, got %T", sender.messages[0])
	}

	if !strings.Contains(msgCfg.Text, "Welcome") {
		t.Errorf("expected welcome text, got %q", msgCfg.Text)
	}

	kb, ok := msgCfg.ReplyMarkup.(tgbotapi.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("expected InlineKeyboardMarkup, got %T", msgCfg.ReplyMarkup)
	}

	if len(kb.InlineKeyboard) != 1 || len(kb.InlineKeyboard[0]) != 1 {
		t.Fatalf("expected 1 row with 1 button, got %v", kb.InlineKeyboard)
	}

	btn := kb.InlineKeyboard[0][0]
	if !strings.Contains(btn.Text, "Quick Start") {
		t.Errorf("expected Quick Start button, got %q", btn.Text)
	}
}

// --- Test 3: Old user /start → welcome back with main menu buttons ---

func TestHandleStart_OldUser_WelcomeBackWithMainMenu(t *testing.T) {
	bot, sender := newTestBotWithMock(t, 0)

	// Bind a domain so the user is "old"
	bot.io.BindDomain(200, "test.com")

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 200},
		From: &tgbotapi.User{LanguageCode: "en"},
	}

	bot.handleStart(msg, "en")

	sender.mu.Lock()
	defer sender.mu.Unlock()

	// Find the message sent by handleStart (BindDomain may also send messages)
	var startMsg *tgbotapi.MessageConfig
	for _, m := range sender.messages {
		if cfg, ok := m.(tgbotapi.MessageConfig); ok {
			if strings.Contains(cfg.Text, "Welcome back") {
				startMsg = &cfg
				break
			}
		}
	}

	if startMsg == nil {
		t.Fatalf("expected welcome back message, got messages: %v", sender.sentTexts())
	}

	kb, ok := startMsg.ReplyMarkup.(tgbotapi.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("expected InlineKeyboardMarkup, got %T", startMsg.ReplyMarkup)
	}

	if len(kb.InlineKeyboard) != 3 {
		t.Fatalf("expected 3 rows (domains, blocks, help), got %d", len(kb.InlineKeyboard))
	}

	// Verify button labels
	btnTexts := []string{}
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			btnTexts = append(btnTexts, btn.Text)
		}
	}

	if !strings.Contains(btnTexts[0], "My Domains") {
		t.Errorf("expected My Domains button, got %q", btnTexts[0])
	}
	if !strings.Contains(btnTexts[1], "Block Management") {
		t.Errorf("expected Block Management button, got %q", btnTexts[1])
	}
	if !strings.Contains(btnTexts[2], "Help") {
		t.Errorf("expected Help button, got %q", btnTexts[2])
	}
}

// --- Test 4: /help → category buttons (domain, block, other) ---

func TestHandleHelp_CategoryButtons(t *testing.T) {
	bot, sender := newTestBotWithMock(t, 0)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 100},
		From: &tgbotapi.User{LanguageCode: "en"},
	}

	bot.handleHelp(msg, "en")

	sender.mu.Lock()
	defer sender.mu.Unlock()

	if len(sender.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(sender.messages))
	}

	msgCfg := sender.messages[0].(tgbotapi.MessageConfig)
	kb := msgCfg.ReplyMarkup.(tgbotapi.InlineKeyboardMarkup)

	if len(kb.InlineKeyboard) != 3 {
		t.Fatalf("expected 3 category rows, got %d", len(kb.InlineKeyboard))
	}

	categories := []string{"Domain", "Block", "Other"}
	for i, cat := range categories {
		btn := kb.InlineKeyboard[i][0]
		if !strings.Contains(btn.Text, cat) {
			t.Errorf("row %d: expected button containing %q, got %q", i, cat, btn.Text)
		}
	}
}

// --- Test 5: Help category click → details + back button ---

func TestHandleHelpCategory_DetailsAndBackButton(t *testing.T) {
	bot, sender := newTestBotWithMock(t, 0)

	query := &tgbotapi.CallbackQuery{
		ID: "test-query-id",
		Message: &tgbotapi.Message{
			MessageID: 42,
			Chat:      &tgbotapi.Chat{ID: 100},
		},
		From: &tgbotapi.User{LanguageCode: "en"},
	}

	bot.handleHelpCategory(query, "en", "domain")

	sender.mu.Lock()
	defer sender.mu.Unlock()

	// Should have sent an edit message (via Send) and a callback answer (via Request)
	if len(sender.messages) < 1 {
		t.Fatalf("expected at least 1 message (edit), got %d", len(sender.messages))
	}

	// The edit message should be an EditMessageTextConfig
	editMsg, ok := sender.messages[0].(tgbotapi.EditMessageTextConfig)
	if !ok {
		t.Fatalf("expected EditMessageTextConfig, got %T", sender.messages[0])
	}

	if !strings.Contains(editMsg.Text, "Domain Management") {
		t.Errorf("expected domain help details, got %q", editMsg.Text)
	}

	if editMsg.ReplyMarkup == nil {
		t.Fatal("expected reply markup with back button")
	}

	kb := *editMsg.ReplyMarkup
	if len(kb.InlineKeyboard) != 1 || len(kb.InlineKeyboard[0]) != 1 {
		t.Fatalf("expected 1 row with 1 back button, got %v", kb.InlineKeyboard)
	}

	backBtn := kb.InlineKeyboard[0][0]
	if !strings.Contains(backBtn.Text, "Back") {
		t.Errorf("expected Back button, got %q", backBtn.Text)
	}
}

// --- Test 6: Domain dismiss confirm/cancel flow ---

func TestHandleDismissConfirm_Success(t *testing.T) {
	bot, sender := newTestBotWithMock(t, 0)

	// Bind a domain first
	bot.io.BindDomain(100, "example.com")

	query := &tgbotapi.CallbackQuery{
		ID: "test-query-id",
		Message: &tgbotapi.Message{
			MessageID: 42,
			Chat:      &tgbotapi.Chat{ID: 100},
		},
		From: &tgbotapi.User{LanguageCode: "en"},
	}

	// Clear messages from BindDomain
	sender.mu.Lock()
	sender.messages = nil
	sender.mu.Unlock()

	bot.handleDismissConfirm(query, "en", "example.com")

	sender.mu.Lock()
	defer sender.mu.Unlock()

	// Should have an edit message with success text
	foundSuccess := false
	for _, m := range sender.messages {
		if edit, ok := m.(tgbotapi.EditMessageTextConfig); ok {
			if strings.Contains(edit.Text, "unbound successfully") || strings.Contains(edit.Text, "解绑") {
				foundSuccess = true
			}
		}
	}
	if !foundSuccess {
		t.Error("expected dismiss success message in edit")
	}

	// Should have answered the callback query
	if len(sender.requests) == 0 {
		t.Error("expected AnswerCallbackQuery request")
	}
}

func TestHandleDismissCancel_ShowsCancelled(t *testing.T) {
	bot, sender := newTestBotWithMock(t, 0)

	query := &tgbotapi.CallbackQuery{
		ID: "test-query-id",
		Message: &tgbotapi.Message{
			MessageID: 42,
			Chat:      &tgbotapi.Chat{ID: 100},
		},
		From: &tgbotapi.User{LanguageCode: "en"},
	}

	bot.handleDismissCancel(query, "en")

	sender.mu.Lock()
	defer sender.mu.Unlock()

	foundCancel := false
	for _, m := range sender.messages {
		if edit, ok := m.(tgbotapi.EditMessageTextConfig); ok {
			if strings.Contains(edit.Text, "cancelled") || strings.Contains(edit.Text, "取消") {
				foundCancel = true
			}
		}
	}
	if !foundCancel {
		t.Error("expected cancel message in edit")
	}
}

// --- Test 7: /bind without args → usage hint ---

func TestHandleBind_NoArgs_UsageHint(t *testing.T) {
	bot, sender := newTestBotWithMock(t, 0)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 100},
		From: &tgbotapi.User{LanguageCode: "en"},
	}

	bot.handleBind(msg, "en", []string{})

	sender.mu.Lock()
	defer sender.mu.Unlock()

	if len(sender.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(sender.messages))
	}

	msgCfg := sender.messages[0].(tgbotapi.MessageConfig)
	if !strings.Contains(msgCfg.Text, "/bind") {
		t.Errorf("expected usage hint containing /bind, got %q", msgCfg.Text)
	}
	if !strings.Contains(msgCfg.Text, "example.com") {
		t.Errorf("expected usage hint containing example.com, got %q", msgCfg.Text)
	}
}

// --- Test 8: Block category navigation, empty list shows "No blocked items" ---

func TestHandleBlockCategory_EmptyList_NoBlockedItems(t *testing.T) {
	bot, sender := newTestBotWithMock(t, 0)

	query := &tgbotapi.CallbackQuery{
		ID: "test-query-id",
		Message: &tgbotapi.Message{
			MessageID: 42,
			Chat:      &tgbotapi.Chat{ID: 100},
		},
		From: &tgbotapi.User{LanguageCode: "en"},
	}

	bot.handleBlockCategory(query, "en", "sender")

	sender.mu.Lock()
	defer sender.mu.Unlock()

	foundNoBlocks := false
	for _, m := range sender.messages {
		if edit, ok := m.(tgbotapi.EditMessageTextConfig); ok {
			if strings.Contains(edit.Text, "No blocked items") {
				foundNoBlocks = true
			}
		}
	}
	if !foundNoBlocks {
		var texts []string
		for _, m := range sender.messages {
			texts = append(texts, fmt.Sprintf("%T", m))
		}
		t.Errorf("expected 'No blocked items' message, got message types: %v", texts)
	}
}

// --- Test 9: Admin broadcast → confirmation message ---

func TestHandleSendAll_Admin_ConfirmationMessage(t *testing.T) {
	adminID := int64(12345)
	bot, sender := newTestBotWithMock(t, adminID)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: adminID},
		From: &tgbotapi.User{LanguageCode: "en"},
	}

	bot.handleSendAll(msg, []string{"test", "broadcast"})

	texts := sender.sentTexts()
	found := false
	for _, text := range texts {
		if strings.Contains(text, "Broadcast complete") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected broadcast confirmation, got: %v", texts)
	}
}

// --- Test 10: SetMyCommands failure → bot continues ---

// errorMockSender is a mock that returns errors on Request calls.
type errorMockSender struct {
	mockSender
	requestErr error
}

func (m *errorMockSender) Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, c)
	return nil, m.requestErr
}

func TestRegisterCommands_Failure_ReturnsError(t *testing.T) {
	errSender := &errorMockSender{
		requestErr: fmt.Errorf("API unavailable"),
	}

	bot := NewForTest(
		&config.Config{MailDomain: "test.example.com"},
		nil,
		errSender,
	)

	err := bot.registerCommands()
	if err == nil {
		t.Fatal("expected error from registerCommands, got nil")
	}
	if !strings.Contains(err.Error(), "failed to register commands") {
		t.Errorf("expected 'failed to register commands' error, got: %v", err)
	}

	// Verify the bot can still handle messages after registerCommands failure
	// (i.e., the error doesn't panic or corrupt state)
	errSender.mu.Lock()
	errSender.requests = nil
	errSender.mu.Unlock()

	// Bot should still be functional — test by calling getText
	text := bot.getText("welcome_new", "en")
	if text == "" {
		t.Error("bot should still be functional after registerCommands failure")
	}
}
