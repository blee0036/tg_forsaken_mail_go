package config

import (
	"testing"
	"time"
)

func TestLoad_OldTokens_BasicParsing(t *testing.T) {
	content := `{
		"mailin": {"host": "0.0.0.0", "port": 25, "disableWebhook": true},
		"mail_domain": "test.com",
		"telegram_bot_token": "main-token",
		"admin_tg_id": "123",
		"old_telegram_bot_token": ["old-token-1", "old-token-2"]
	}`
	path := writeTempConfig(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.OldTelegramBotTokens) != 2 {
		t.Fatalf("OldTelegramBotTokens length = %d, want 2", len(cfg.OldTelegramBotTokens))
	}
	if cfg.OldTelegramBotTokens[0] != "old-token-1" {
		t.Errorf("OldTelegramBotTokens[0] = %q, want %q", cfg.OldTelegramBotTokens[0], "old-token-1")
	}
	if cfg.OldTelegramBotTokens[1] != "old-token-2" {
		t.Errorf("OldTelegramBotTokens[1] = %q, want %q", cfg.OldTelegramBotTokens[1], "old-token-2")
	}
}

func TestLoad_OldTokens_FilterEmpty(t *testing.T) {
	content := `{
		"mailin": {"host": "0.0.0.0", "port": 25, "disableWebhook": true},
		"mail_domain": "test.com",
		"telegram_bot_token": "main-token",
		"admin_tg_id": "123",
		"old_telegram_bot_token": ["", "valid-token", "", "another"]
	}`
	path := writeTempConfig(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.OldTelegramBotTokens) != 2 {
		t.Fatalf("OldTelegramBotTokens length = %d, want 2", len(cfg.OldTelegramBotTokens))
	}
}

func TestLoad_OldTokens_Deduplicate(t *testing.T) {
	content := `{
		"mailin": {"host": "0.0.0.0", "port": 25, "disableWebhook": true},
		"mail_domain": "test.com",
		"telegram_bot_token": "main-token",
		"admin_tg_id": "123",
		"old_telegram_bot_token": ["dup", "dup", "unique"]
	}`
	path := writeTempConfig(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.OldTelegramBotTokens) != 2 {
		t.Fatalf("OldTelegramBotTokens length = %d, want 2", len(cfg.OldTelegramBotTokens))
	}
}

func TestLoad_OldTokens_ExcludeMainToken(t *testing.T) {
	content := `{
		"mailin": {"host": "0.0.0.0", "port": 25, "disableWebhook": true},
		"mail_domain": "test.com",
		"telegram_bot_token": "main-token",
		"admin_tg_id": "123",
		"old_telegram_bot_token": ["main-token", "old-token"]
	}`
	path := writeTempConfig(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.OldTelegramBotTokens) != 1 {
		t.Fatalf("OldTelegramBotTokens length = %d, want 1", len(cfg.OldTelegramBotTokens))
	}
	if cfg.OldTelegramBotTokens[0] != "old-token" {
		t.Errorf("OldTelegramBotTokens[0] = %q, want %q", cfg.OldTelegramBotTokens[0], "old-token")
	}
}

func TestLoad_OldTokens_MissingField(t *testing.T) {
	content := `{
		"mailin": {"host": "0.0.0.0", "port": 25, "disableWebhook": true},
		"mail_domain": "test.com",
		"telegram_bot_token": "main-token",
		"admin_tg_id": "123"
	}`
	path := writeTempConfig(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.OldTelegramBotTokens == nil {
		t.Fatal("OldTelegramBotTokens should not be nil, want empty slice")
	}
	if len(cfg.OldTelegramBotTokens) != 0 {
		t.Errorf("OldTelegramBotTokens length = %d, want 0", len(cfg.OldTelegramBotTokens))
	}
}

func TestLoad_OldTokens_EmptyArray(t *testing.T) {
	content := `{
		"mailin": {"host": "0.0.0.0", "port": 25, "disableWebhook": true},
		"mail_domain": "test.com",
		"telegram_bot_token": "main-token",
		"admin_tg_id": "123",
		"old_telegram_bot_token": []
	}`
	path := writeTempConfig(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.OldTelegramBotTokens) != 0 {
		t.Errorf("OldTelegramBotTokens length = %d, want 0", len(cfg.OldTelegramBotTokens))
	}
}

func TestLoad_AlertMsg_BasicParsing(t *testing.T) {
	content := `{
		"mailin": {"host": "0.0.0.0", "port": 25, "disableWebhook": true},
		"mail_domain": "test.com",
		"telegram_bot_token": "tok",
		"admin_tg_id": "123",
		"change_bot_alert_msg": {"en": "Switch to new bot", "zh": "请切换到新 Bot"}
	}`
	path := writeTempConfig(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.ChangeBotAlertMsg) != 2 {
		t.Fatalf("ChangeBotAlertMsg length = %d, want 2", len(cfg.ChangeBotAlertMsg))
	}
	if cfg.ChangeBotAlertMsg["en"] != "Switch to new bot" {
		t.Errorf("ChangeBotAlertMsg[en] = %q, want %q", cfg.ChangeBotAlertMsg["en"], "Switch to new bot")
	}
}

func TestLoad_AlertMsg_FilterBlankKeyValue(t *testing.T) {
	content := `{
		"mailin": {"host": "0.0.0.0", "port": 25, "disableWebhook": true},
		"mail_domain": "test.com",
		"telegram_bot_token": "tok",
		"admin_tg_id": "123",
		"change_bot_alert_msg": {"": "no key", "en": "", "  ": "blank key", "zh": "  ", "valid": "msg"}
	}`
	path := writeTempConfig(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.ChangeBotAlertMsg) != 1 {
		t.Fatalf("ChangeBotAlertMsg length = %d, want 1 (only 'valid')", len(cfg.ChangeBotAlertMsg))
	}
	if cfg.ChangeBotAlertMsg["valid"] != "msg" {
		t.Errorf("ChangeBotAlertMsg[valid] = %q, want %q", cfg.ChangeBotAlertMsg["valid"], "msg")
	}
}

func TestLoad_AlertMsg_MissingField(t *testing.T) {
	content := `{
		"mailin": {"host": "0.0.0.0", "port": 25, "disableWebhook": true},
		"mail_domain": "test.com",
		"telegram_bot_token": "tok",
		"admin_tg_id": "123"
	}`
	path := writeTempConfig(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ChangeBotAlertMsg == nil {
		t.Fatal("ChangeBotAlertMsg should not be nil, want empty map")
	}
	if len(cfg.ChangeBotAlertMsg) != 0 {
		t.Errorf("ChangeBotAlertMsg length = %d, want 0", len(cfg.ChangeBotAlertMsg))
	}
}

func TestLoad_CloseOldDate_ValidDate(t *testing.T) {
	content := `{
		"mailin": {"host": "0.0.0.0", "port": 25, "disableWebhook": true},
		"mail_domain": "test.com",
		"telegram_bot_token": "tok",
		"admin_tg_id": "123",
		"close_old_date": "2025-03-01"
	}`
	path := writeTempConfig(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CloseOldDate != "2025-03-01" {
		t.Errorf("CloseOldDate = %q, want %q", cfg.CloseOldDate, "2025-03-01")
	}
	if cfg.CloseOldDateParsed == nil {
		t.Fatal("CloseOldDateParsed should not be nil")
	}
	expected := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	if !cfg.CloseOldDateParsed.Equal(expected) {
		t.Errorf("CloseOldDateParsed = %v, want %v", cfg.CloseOldDateParsed, expected)
	}
}

func TestLoad_CloseOldDate_InvalidFormat(t *testing.T) {
	content := `{
		"mailin": {"host": "0.0.0.0", "port": 25, "disableWebhook": true},
		"mail_domain": "test.com",
		"telegram_bot_token": "tok",
		"admin_tg_id": "123",
		"close_old_date": "not-a-date"
	}`
	path := writeTempConfig(t, content)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid close_old_date, got nil")
	}
}

func TestLoad_CloseOldDate_EmptyString(t *testing.T) {
	content := `{
		"mailin": {"host": "0.0.0.0", "port": 25, "disableWebhook": true},
		"mail_domain": "test.com",
		"telegram_bot_token": "tok",
		"admin_tg_id": "123",
		"close_old_date": ""
	}`
	path := writeTempConfig(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CloseOldDate != "" {
		t.Errorf("CloseOldDate = %q, want empty", cfg.CloseOldDate)
	}
	if cfg.CloseOldDateParsed != nil {
		t.Errorf("CloseOldDateParsed = %v, want nil", cfg.CloseOldDateParsed)
	}
}

func TestLoad_CloseOldDate_MissingField(t *testing.T) {
	content := `{
		"mailin": {"host": "0.0.0.0", "port": 25, "disableWebhook": true},
		"mail_domain": "test.com",
		"telegram_bot_token": "tok",
		"admin_tg_id": "123"
	}`
	path := writeTempConfig(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CloseOldDate != "" {
		t.Errorf("CloseOldDate = %q, want empty", cfg.CloseOldDate)
	}
	if cfg.CloseOldDateParsed != nil {
		t.Errorf("CloseOldDateParsed = %v, want nil", cfg.CloseOldDateParsed)
	}
}

func TestLoad_AllMigrationFields_Combined(t *testing.T) {
	content := `{
		"mailin": {"host": "0.0.0.0", "port": 25, "disableWebhook": true},
		"mail_domain": "test.com",
		"telegram_bot_token": "main-token",
		"admin_tg_id": "123",
		"old_telegram_bot_token": ["old-1", "old-2"],
		"change_bot_alert_msg": {"en": "Switch!", "zh": "切换！"},
		"close_old_date": "2025-06-15"
	}`
	path := writeTempConfig(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.OldTelegramBotTokens) != 2 {
		t.Errorf("OldTelegramBotTokens length = %d, want 2", len(cfg.OldTelegramBotTokens))
	}
	if len(cfg.ChangeBotAlertMsg) != 2 {
		t.Errorf("ChangeBotAlertMsg length = %d, want 2", len(cfg.ChangeBotAlertMsg))
	}
	if cfg.CloseOldDate != "2025-06-15" {
		t.Errorf("CloseOldDate = %q, want %q", cfg.CloseOldDate, "2025-06-15")
	}
	if cfg.CloseOldDateParsed == nil {
		t.Fatal("CloseOldDateParsed should not be nil")
	}
}

func TestLoad_NoMigrationFields_BackwardCompatible(t *testing.T) {
	content := `{
		"mailin": {"host": "0.0.0.0", "port": 25, "disableWebhook": true},
		"mail_domain": "test.com",
		"telegram_bot_token": "tok",
		"admin_tg_id": "123"
	}`
	path := writeTempConfig(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// All migration fields should be zero values
	if len(cfg.OldTelegramBotTokens) != 0 {
		t.Errorf("OldTelegramBotTokens length = %d, want 0", len(cfg.OldTelegramBotTokens))
	}
	if len(cfg.ChangeBotAlertMsg) != 0 {
		t.Errorf("ChangeBotAlertMsg length = %d, want 0", len(cfg.ChangeBotAlertMsg))
	}
	if cfg.CloseOldDate != "" {
		t.Errorf("CloseOldDate = %q, want empty", cfg.CloseOldDate)
	}
	if cfg.CloseOldDateParsed != nil {
		t.Errorf("CloseOldDateParsed = %v, want nil", cfg.CloseOldDateParsed)
	}
}
