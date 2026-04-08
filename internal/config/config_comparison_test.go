package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Task 1.4: 对比验证：配置模块
// Validates: Requirement 1.6
// Verifies that Go config.Load() parses every field identically to Node require('config.json')

func TestComparison_ConfigSimpleByteLevelIdentity(t *testing.T) {
	// Verify go_version/config-simple.json and root config-simple.json are byte-level identical.
	goVersionPath := filepath.Join("..", "..", "config-simple.json")
	rootPath := filepath.Join("..", "..", "..", "config-simple.json")

	goBytes, err := os.ReadFile(goVersionPath)
	if err != nil {
		t.Fatalf("failed to read go_version/config-simple.json: %v", err)
	}

	rootBytes, err := os.ReadFile(rootPath)
	if err != nil {
		t.Fatalf("failed to read root config-simple.json: %v", err)
	}

	if len(goBytes) != len(rootBytes) {
		t.Fatalf("file sizes differ: go_version=%d bytes, root=%d bytes", len(goBytes), len(rootBytes))
	}

	for i := range goBytes {
		if goBytes[i] != rootBytes[i] {
			t.Fatalf("files differ at byte offset %d: go_version=0x%02x, root=0x%02x", i, goBytes[i], rootBytes[i])
		}
	}
}

func TestComparison_GoConfigMatchesNodeRequire(t *testing.T) {
	// Load config-simple.json using Go's config.Load() and verify every field
	// matches what Node.js require('config-simple.json') would produce.
	cfg, err := Load(filepath.Join("..", "..", "config-simple.json"))
	if err != nil {
		t.Fatalf("config.Load() failed: %v", err)
	}

	// Expected values from Node.js require('./config-simple.json'):
	// mailin.host = "0.0.0.0"
	if cfg.Mailin.Host != "0.0.0.0" {
		t.Errorf("Mailin.Host = %q, want %q (Node: mailin.host)", cfg.Mailin.Host, "0.0.0.0")
	}

	// mailin.port = 25
	if cfg.Mailin.Port != 25 {
		t.Errorf("Mailin.Port = %d, want %d (Node: mailin.port)", cfg.Mailin.Port, 25)
	}

	// mailin.disableWebhook = true
	if cfg.Mailin.DisableWebhook != true {
		t.Errorf("Mailin.DisableWebhook = %v, want true (Node: mailin.disableWebhook)", cfg.Mailin.DisableWebhook)
	}

	// mail_domain = "tgmail.party"
	if cfg.MailDomain != "tgmail.party" {
		t.Errorf("MailDomain = %q, want %q (Node: mail_domain)", cfg.MailDomain, "tgmail.party")
	}

	// telegram_bot_token = ""
	if cfg.TelegramBotToken != "" {
		t.Errorf("TelegramBotToken = %q, want %q (Node: telegram_bot_token)", cfg.TelegramBotToken, "")
	}

	// admin_tg_id = "" in JSON → Node gets "" (string), Go parses to 0 (int64)
	// This is the expected mapping: empty string → 0 for int64
	if cfg.AdminTgID != 0 {
		t.Errorf("AdminTgID = %d, want 0 (Node: admin_tg_id is empty string)", cfg.AdminTgID)
	}

	// upload_url = ""
	if cfg.UploadURL != "" {
		t.Errorf("UploadURL = %q, want %q (Node: upload_url)", cfg.UploadURL, "")
	}

	// upload_token = ""
	if cfg.UploadToken != "" {
		t.Errorf("UploadToken = %q, want %q (Node: upload_token)", cfg.UploadToken, "")
	}
}
