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
	if cfg.MailDomain != "mx.tgmail.party" {
		t.Errorf("MailDomain = %q, want %q", cfg.MailDomain, "mx.tgmail.party")
	}
	if cfg.DefaultMailDomain != "tgmail.party" {
		t.Errorf("DefaultMailDomain = %q, want %q", cfg.DefaultMailDomain, "tgmail.party")
	}
	if cfg.DefaultDomain() != "tgmail.party" {
		t.Errorf("DefaultDomain() = %q, want %q", cfg.DefaultDomain(), "tgmail.party")
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

func TestDefaultDomain_Normalization(t *testing.T) {
	cases := []struct {
		name       string
		mailDomain string
		defaultDom string
		want       string
	}{
		{"plain default", "mx.example.com", "example.com", "example.com"},
		{"uppercase lowercased", "mx.example.com", "Example.COM", "example.com"},
		{"trailing dot trimmed", "mx.example.com", "example.com.", "example.com"},
		{"whitespace trimmed", "mx.example.com", "  example.com  ", "example.com"},
		{"fallback to mail_domain", "mx.example.com", "", "mx.example.com"},
		{"fallback normalizes too", "MX.Example.COM.", "", "mx.example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{MailDomain: tc.mailDomain, DefaultMailDomain: tc.defaultDom}
			if got := cfg.DefaultDomain(); got != tc.want {
				t.Errorf("DefaultDomain() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLoad_RejectsWildcardDefaultMailDomain(t *testing.T) {
	path := writeTempConfig(t, `{
		"mailin": {"host": "0.0.0.0", "port": 25, "disableWebhook": true},
		"mail_domain": "mx.example.com",
		"default_mail_domain": "*.example.com",
		"telegram_bot_token": "",
		"admin_tg_id": ""
	}`)

	if _, err := Load(path); err == nil {
		t.Fatal("expected an error for wildcard default_mail_domain, got nil")
	}
}

func TestLoad_NormalizesDefaultMailDomain(t *testing.T) {
	path := writeTempConfig(t, `{
		"mailin": {"host": "0.0.0.0", "port": 25, "disableWebhook": true},
		"mail_domain": "mx.example.com",
		"default_mail_domain": "  Example.COM.  ",
		"telegram_bot_token": "",
		"admin_tg_id": ""
	}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultMailDomain != "example.com" {
		t.Errorf("DefaultMailDomain = %q, want %q (should be normalized)", cfg.DefaultMailDomain, "example.com")
	}
	if cfg.DefaultDomain() != "example.com" {
		t.Errorf("DefaultDomain() = %q, want %q", cfg.DefaultDomain(), "example.com")
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
