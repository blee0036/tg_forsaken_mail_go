package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ValidConfigSimple(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "config-simple.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Mailin.Host != "0.0.0.0" {
		t.Errorf("Mailin.Host = %q, want %q", cfg.Mailin.Host, "0.0.0.0")
	}
	if cfg.Mailin.Port != 25 {
		t.Errorf("Mailin.Port = %d, want %d", cfg.Mailin.Port, 25)
	}
	if cfg.Mailin.DisableWebhook != true {
		t.Errorf("Mailin.DisableWebhook = %v, want true", cfg.Mailin.DisableWebhook)
	}
	if cfg.MailDomain != "tgmail.party" {
		t.Errorf("MailDomain = %q, want %q", cfg.MailDomain, "tgmail.party")
	}
	if cfg.TelegramBotToken != "" {
		t.Errorf("TelegramBotToken = %q, want empty", cfg.TelegramBotToken)
	}
	if cfg.AdminTgID != 0 {
		t.Errorf("AdminTgID = %d, want 0 (empty string)", cfg.AdminTgID)
	}
	if cfg.UploadURL != "" {
		t.Errorf("UploadURL = %q, want empty", cfg.UploadURL)
	}
	if cfg.UploadToken != "" {
		t.Errorf("UploadToken = %q, want empty", cfg.UploadToken)
	}
}

func TestLoad_AdminTgIDAsNumber(t *testing.T) {
	content := `{
		"mailin": {"host": "127.0.0.1", "port": 2525, "disableWebhook": false},
		"mail_domain": "example.com",
		"telegram_bot_token": "tok123",
		"admin_tg_id": 123456789,
		"upload_url": "https://up.example.com",
		"upload_token": "secret"
	}`
	path := writeTempConfig(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AdminTgID != 123456789 {
		t.Errorf("AdminTgID = %d, want 123456789", cfg.AdminTgID)
	}
}

func TestLoad_AdminTgIDAsString(t *testing.T) {
	content := `{
		"mailin": {"host": "0.0.0.0", "port": 25, "disableWebhook": true},
		"mail_domain": "test.com",
		"telegram_bot_token": "",
		"admin_tg_id": "987654321",
		"upload_url": "",
		"upload_token": ""
	}`
	path := writeTempConfig(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AdminTgID != 987654321 {
		t.Errorf("AdminTgID = %d, want 987654321", cfg.AdminTgID)
	}
}

func TestLoad_AdminTgIDEmptyString(t *testing.T) {
	content := `{
		"mailin": {"host": "0.0.0.0", "port": 25, "disableWebhook": true},
		"mail_domain": "test.com",
		"telegram_bot_token": "",
		"admin_tg_id": "",
		"upload_url": "",
		"upload_token": ""
	}`
	path := writeTempConfig(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AdminTgID != 0 {
		t.Errorf("AdminTgID = %d, want 0", cfg.AdminTgID)
	}
}

func TestLoad_FileNotExist(t *testing.T) {
	_, err := Load("/nonexistent/path/config.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	path := writeTempConfig(t, `{invalid json}`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestLoad_EmptyFile(t *testing.T) {
	path := writeTempConfig(t, "")
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for empty file, got nil")
	}
}

func TestLoad_LargeAdminTgID(t *testing.T) {
	content := `{
		"mailin": {"host": "0.0.0.0", "port": 25, "disableWebhook": true},
		"mail_domain": "test.com",
		"telegram_bot_token": "",
		"admin_tg_id": "5000000000",
		"upload_url": "",
		"upload_token": ""
	}`
	path := writeTempConfig(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AdminTgID != 5000000000 {
		t.Errorf("AdminTgID = %d, want 5000000000", cfg.AdminTgID)
	}
}

// writeTempConfig writes content to a temp file and returns its path.
func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}
	return path
}
