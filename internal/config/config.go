package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

// MailinConfig holds SMTP server configuration.
type MailinConfig struct {
	Host           string `json:"host"`
	Port           int    `json:"port"`
	DisableWebhook bool   `json:"disableWebhook"`
}

// Config represents the application configuration, matching the Node version's config.json structure.
type Config struct {
	Mailin           MailinConfig `json:"mailin"`
	MailDomain       string       `json:"mail_domain"`
	TelegramBotToken string       `json:"telegram_bot_token"`
	AdminTgID        int64        `json:"-"` // custom unmarshal; can be string or number in JSON
	UploadURL        string       `json:"upload_url"`
	UploadToken      string       `json:"upload_token"`
}

// configJSON is an intermediate struct for unmarshaling, where admin_tg_id is raw JSON.
type configJSON struct {
	Mailin           MailinConfig    `json:"mailin"`
	MailDomain       string          `json:"mail_domain"`
	TelegramBotToken string          `json:"telegram_bot_token"`
	AdminTgID        json.RawMessage `json:"admin_tg_id"`
	UploadURL        string          `json:"upload_url"`
	UploadToken      string          `json:"upload_token"`
}

// Load reads and parses a JSON config file from the given path.
// Returns a clear error when the file doesn't exist or the JSON is invalid.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var raw configJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	cfg := &Config{
		Mailin:           raw.Mailin,
		MailDomain:       raw.MailDomain,
		TelegramBotToken: raw.TelegramBotToken,
		UploadURL:        raw.UploadURL,
		UploadToken:      raw.UploadToken,
	}

	// Parse admin_tg_id which can be a JSON string or number.
	adminID, err := parseAdminTgID(raw.AdminTgID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse admin_tg_id: %w", err)
	}
	cfg.AdminTgID = adminID

	return cfg, nil
}

// parseAdminTgID handles admin_tg_id being either a JSON string or number.
// Empty string "" results in 0.
func parseAdminTgID(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, nil
	}

	// Try as a JSON number first.
	var num json.Number
	if err := json.Unmarshal(raw, &num); err == nil {
		s := num.String()
		if s == "" {
			return 0, nil
		}
		return strconv.ParseInt(s, 10, 64)
	}

	// Try as a JSON string.
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return 0, fmt.Errorf("admin_tg_id must be a string or number, got: %s", string(raw))
	}
	if s == "" {
		return 0, nil
	}
	return strconv.ParseInt(s, 10, 64)
}
