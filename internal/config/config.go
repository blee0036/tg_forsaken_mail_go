package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
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

	// Migration-related fields (all optional, zero-value defaults when absent)
	OldTelegramBotTokens []string          `json:"-"` // custom parsed from old_telegram_bot_token
	ChangeBotAlertMsg    map[string]string  `json:"-"` // custom parsed from change_bot_alert_msg
	CloseOldDate         string            `json:"-"` // raw close_old_date string
	CloseOldDateParsed   *time.Time        `json:"-"` // parsed date (nil if not configured)
}

// configJSON is an intermediate struct for unmarshaling, where admin_tg_id is raw JSON.
type configJSON struct {
	Mailin           MailinConfig    `json:"mailin"`
	MailDomain       string          `json:"mail_domain"`
	TelegramBotToken string          `json:"telegram_bot_token"`
	AdminTgID        json.RawMessage `json:"admin_tg_id"`
	UploadURL        string          `json:"upload_url"`
	UploadToken      string          `json:"upload_token"`

	// Migration-related raw fields
	OldTelegramBotToken json.RawMessage `json:"old_telegram_bot_token"`
	ChangeBotAlertMsg   json.RawMessage `json:"change_bot_alert_msg"`
	CloseOldDate        string          `json:"close_old_date"`
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

	// Parse old_telegram_bot_token: string array, filter empty, deduplicate, exclude main token.
	oldTokens, err := parseOldTelegramBotTokens(raw.OldTelegramBotToken, cfg.TelegramBotToken)
	if err != nil {
		return nil, fmt.Errorf("failed to parse old_telegram_bot_token: %w", err)
	}
	cfg.OldTelegramBotTokens = oldTokens

	// Parse change_bot_alert_msg: map with blank key/value filtering.
	alertMsg, err := parseChangeBotAlertMsg(raw.ChangeBotAlertMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse change_bot_alert_msg: %w", err)
	}
	cfg.ChangeBotAlertMsg = alertMsg

	// Parse close_old_date: validate YYYY-MM-DD format if non-empty.
	closeDate, parsedDate, err := parseCloseOldDate(raw.CloseOldDate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse close_old_date: %w", err)
	}
	cfg.CloseOldDate = closeDate
	cfg.CloseOldDateParsed = parsedDate

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

// parseOldTelegramBotTokens parses old_telegram_bot_token from raw JSON.
// Filters empty strings, deduplicates, and excludes the main token.
func parseOldTelegramBotTokens(raw json.RawMessage, mainToken string) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return []string{}, nil
	}

	var tokens []string
	if err := json.Unmarshal(raw, &tokens); err != nil {
		return nil, fmt.Errorf("old_telegram_bot_token must be a JSON string array: %w", err)
	}

	seen := make(map[string]bool)
	result := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if t == "" {
			continue
		}
		if t == mainToken {
			continue
		}
		if seen[t] {
			continue
		}
		seen[t] = true
		result = append(result, t)
	}
	return result, nil
}

// parseChangeBotAlertMsg parses change_bot_alert_msg from raw JSON.
// Filters entries with blank keys or blank values.
func parseChangeBotAlertMsg(raw json.RawMessage) (map[string]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]string{}, nil
	}

	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("change_bot_alert_msg must be a JSON object with string values: %w", err)
	}

	result := make(map[string]string)
	for k, v := range m {
		if strings.TrimSpace(k) == "" {
			continue
		}
		if strings.TrimSpace(v) == "" {
			continue
		}
		result[k] = v
	}
	return result, nil
}

// parseCloseOldDate validates and parses the close_old_date string.
// Empty string returns ("", nil, nil). Non-empty must be valid YYYY-MM-DD format.
func parseCloseOldDate(raw string) (string, *time.Time, error) {
	if raw == "" {
		return "", nil, nil
	}

	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return "", nil, fmt.Errorf("close_old_date must be in YYYY-MM-DD format, got %q: %w", raw, err)
	}
	return raw, &t, nil
}
