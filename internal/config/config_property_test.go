package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Feature: bot-migration, Property 1: Config Token 解析与过滤
// Validates: Requirements 1.1, 1.4
func TestProperty_ConfigTokenParseAndFilter(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("parsed OldTelegramBotTokens contains no empty strings, no duplicates, and no main token", prop.ForAll(
		func(mainToken string, tokens []string, injectMain bool) bool {
			// Optionally inject the main token into the array to test exclusion
			if injectMain && mainToken != "" {
				tokens = append(tokens, mainToken)
			}

			// Build a config JSON with old_telegram_bot_token array and a main token
			raw := map[string]interface{}{
				"mailin": map[string]interface{}{
					"host": "0.0.0.0",
					"port": 25,
				},
				"mail_domain":            "example.com",
				"telegram_bot_token":     mainToken,
				"admin_tg_id":           0,
				"old_telegram_bot_token": tokens,
			}

			data, err := json.Marshal(raw)
			if err != nil {
				return false
			}

			dir := t.TempDir()
			path := filepath.Join(dir, "config.json")
			if err := os.WriteFile(path, data, 0644); err != nil {
				return false
			}

			cfg, err := Load(path)
			if err != nil {
				t.Logf("Load error: %v", err)
				return false
			}

			// Property 1a: No empty strings in result
			for _, tok := range cfg.OldTelegramBotTokens {
				if tok == "" {
					t.Log("Found empty string in OldTelegramBotTokens")
					return false
				}
			}

			// Property 1b: No duplicates in result
			seen := make(map[string]bool)
			for _, tok := range cfg.OldTelegramBotTokens {
				if seen[tok] {
					t.Logf("Found duplicate token: %q", tok)
					return false
				}
				seen[tok] = true
			}

			// Property 1c: No entry equal to main token
			for _, tok := range cfg.OldTelegramBotTokens {
				if tok == mainToken {
					t.Logf("Found main token %q in OldTelegramBotTokens", mainToken)
					return false
				}
			}

			return true
		},
		gen.AnyString(), // mainToken
		gen.SliceOf(gen.Weighted([]gen.WeightedGen{
			{Weight: 3, Gen: gen.AnyString()},  // random tokens (may include duplicates)
			{Weight: 2, Gen: gen.Const("")},    // explicitly inject empty strings
		})),
		gen.Bool(), // injectMain: whether to inject main token into the array
	))

	properties.TestingRun(t)
}

// Feature: bot-migration, Property 2: Config Alert 消息解析与过滤
// Validates: Requirements 2.1, 2.4
func TestProperty_ConfigAlertMsgParseAndFilter(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("parsed ChangeBotAlertMsg contains no blank key or blank value entries", prop.ForAll(
		func(entries map[string]string) bool {
			// Build a config JSON with change_bot_alert_msg
			raw := map[string]interface{}{
				"mailin": map[string]interface{}{
					"host": "0.0.0.0",
					"port": 25,
				},
				"mail_domain":          "example.com",
				"telegram_bot_token":   "test-token",
				"admin_tg_id":         0,
				"change_bot_alert_msg": entries,
			}

			data, err := json.Marshal(raw)
			if err != nil {
				return false
			}

			dir := t.TempDir()
			path := filepath.Join(dir, "config.json")
			if err := os.WriteFile(path, data, 0644); err != nil {
				return false
			}

			cfg, err := Load(path)
			if err != nil {
				t.Logf("Load error: %v", err)
				return false
			}

			// Property 2a: No blank key entries in result
			for k := range cfg.ChangeBotAlertMsg {
				if strings.TrimSpace(k) == "" {
					t.Logf("Found blank key in ChangeBotAlertMsg: %q", k)
					return false
				}
			}

			// Property 2b: No blank value entries in result
			for k, v := range cfg.ChangeBotAlertMsg {
				if strings.TrimSpace(v) == "" {
					t.Logf("Found blank value for key %q in ChangeBotAlertMsg: %q", k, v)
					return false
				}
			}

			return true
		},
		gen.MapOf(
			gen.Weighted([]gen.WeightedGen{
				{Weight: 3, Gen: gen.AnyString()},       // random keys
				{Weight: 2, Gen: gen.Const("")},         // empty key
				{Weight: 1, Gen: gen.Const("   ")},      // whitespace-only key
				{Weight: 1, Gen: gen.Const("\t\n")},     // whitespace-only key (tabs/newlines)
			}),
			gen.Weighted([]gen.WeightedGen{
				{Weight: 3, Gen: gen.AnyString()},       // random values
				{Weight: 2, Gen: gen.Const("")},         // empty value
				{Weight: 1, Gen: gen.Const("   ")},      // whitespace-only value
				{Weight: 1, Gen: gen.Const("\t\n")},     // whitespace-only value (tabs/newlines)
			}),
		),
	))

	properties.TestingRun(t)
}

// Feature: bot-migration, Property 8: 关闭日期配置解析
// Validates: Requirements 7.1, 7.3
func TestProperty_CloseOldDateParsing(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Sub-property 8a: Valid YYYY-MM-DD dates parse successfully
	properties.Property("valid YYYY-MM-DD dates parse successfully with correct CloseOldDateParsed", prop.ForAll(
		func(year int, month int, day int) bool {
			// Clamp to valid date ranges
			if year < 1 {
				year = 1
			}
			if year > 9999 {
				year = year % 9999 + 1
			}
			if month < 1 || month > 12 {
				month = (month%12 + 12) % 12
				if month == 0 {
					month = 1
				}
			}
			if day < 1 || day > 28 {
				// Use 28 as safe max to avoid invalid dates like Feb 30
				day = (day%28 + 28) % 28
				if day == 0 {
					day = 1
				}
			}

			dateStr := fmt.Sprintf("%04d-%02d-%02d", year, month, day)

			raw := map[string]interface{}{
				"mailin": map[string]interface{}{
					"host": "0.0.0.0",
					"port": 25,
				},
				"mail_domain":        "example.com",
				"telegram_bot_token": "test-token",
				"admin_tg_id":       0,
				"close_old_date":    dateStr,
			}

			data, err := json.Marshal(raw)
			if err != nil {
				return false
			}

			dir := t.TempDir()
			path := filepath.Join(dir, "config.json")
			if err := os.WriteFile(path, data, 0644); err != nil {
				return false
			}

			cfg, err := Load(path)
			if err != nil {
				t.Logf("Valid date %q caused Load error: %v", dateStr, err)
				return false
			}

			// CloseOldDateParsed should not be nil for valid dates
			if cfg.CloseOldDateParsed == nil {
				t.Logf("Valid date %q resulted in nil CloseOldDateParsed", dateStr)
				return false
			}

			// CloseOldDate raw string should be preserved
			if cfg.CloseOldDate != dateStr {
				t.Logf("CloseOldDate mismatch: got %q, want %q", cfg.CloseOldDate, dateStr)
				return false
			}

			return true
		},
		gen.IntRange(1, 9999), // year
		gen.IntRange(1, 12),   // month
		gen.IntRange(1, 28),   // day
	))

	// Sub-property 8b: Invalid non-empty strings return error
	properties.Property("invalid non-empty close_old_date strings cause Load to return error", prop.ForAll(
		func(kind int, randomStr string) bool {
			var dateStr string
			switch kind % 6 {
			case 0:
				// Random string (very unlikely to be valid YYYY-MM-DD)
				dateStr = randomStr + "x" // append 'x' to ensure it's not accidentally valid
			case 1:
				// Wrong separator
				dateStr = "2025/01/15"
			case 2:
				// Incomplete date
				dateStr = "2025-01"
			case 3:
				// Invalid month
				dateStr = "2025-13-01"
			case 4:
				// Invalid day
				dateStr = "2025-02-30"
			case 5:
				// Reversed format
				dateStr = "15-01-2025"
			}

			// Skip if dateStr happens to be empty (shouldn't happen with our cases)
			if dateStr == "" {
				return true
			}

			raw := map[string]interface{}{
				"mailin": map[string]interface{}{
					"host": "0.0.0.0",
					"port": 25,
				},
				"mail_domain":        "example.com",
				"telegram_bot_token": "test-token",
				"admin_tg_id":       0,
				"close_old_date":    dateStr,
			}

			data, err := json.Marshal(raw)
			if err != nil {
				return false
			}

			dir := t.TempDir()
			path := filepath.Join(dir, "config.json")
			if err := os.WriteFile(path, data, 0644); err != nil {
				return false
			}

			_, err = Load(path)
			if err == nil {
				t.Logf("Invalid date %q did not cause error", dateStr)
				return false
			}

			return true
		},
		gen.IntRange(0, 5), // kind selector
		gen.AnyString(),    // random string for case 0
	))

	// Sub-property 8c: Empty string parses to nil CloseOldDateParsed
	properties.Property("empty close_old_date string results in nil CloseOldDateParsed", prop.ForAll(
		func(token string) bool {
			raw := map[string]interface{}{
				"mailin": map[string]interface{}{
					"host": "0.0.0.0",
					"port": 25,
				},
				"mail_domain":        "example.com",
				"telegram_bot_token": token,
				"admin_tg_id":       0,
				"close_old_date":    "",
			}

			data, err := json.Marshal(raw)
			if err != nil {
				return false
			}

			dir := t.TempDir()
			path := filepath.Join(dir, "config.json")
			if err := os.WriteFile(path, data, 0644); err != nil {
				return false
			}

			cfg, err := Load(path)
			if err != nil {
				t.Logf("Empty close_old_date caused Load error: %v", err)
				return false
			}

			// CloseOldDateParsed should be nil
			if cfg.CloseOldDateParsed != nil {
				t.Logf("Empty close_old_date resulted in non-nil CloseOldDateParsed: %v", cfg.CloseOldDateParsed)
				return false
			}

			// CloseOldDate raw string should be empty
			if cfg.CloseOldDate != "" {
				t.Logf("CloseOldDate should be empty, got %q", cfg.CloseOldDate)
				return false
			}

			return true
		},
		gen.AnyString(), // token (just to vary the config slightly)
	))

	properties.TestingRun(t)
}

// Feature: bot-migration, Property 11: 向后兼容性
// Validates: Requirements 10.1, 10.3
func TestProperty_BackwardCompatibility(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("config without migration fields loads successfully with zero-value migration fields", prop.ForAll(
		func(host string, port int, disableWebhook bool, mailDomain string,
			token string, adminID int64, uploadURL string, uploadToken string) bool {

			// Clamp port to valid range
			if port < 0 {
				port = -port
			}
			port = port % 65536

			// Build a valid config JSON WITHOUT any migration-related fields
			// (no old_telegram_bot_token, no change_bot_alert_msg, no close_old_date)
			raw := map[string]interface{}{
				"mailin": map[string]interface{}{
					"host":           host,
					"port":           port,
					"disableWebhook": disableWebhook,
				},
				"mail_domain":        mailDomain,
				"telegram_bot_token": token,
				"admin_tg_id":        adminID,
				"upload_url":         uploadURL,
				"upload_token":       uploadToken,
			}

			data, err := json.Marshal(raw)
			if err != nil {
				return false
			}

			dir := t.TempDir()
			path := filepath.Join(dir, "config.json")
			if err := os.WriteFile(path, data, 0644); err != nil {
				return false
			}

			cfg, err := Load(path)
			if err != nil {
				t.Logf("Load error for config without migration fields: %v\nJSON: %s", err, string(data))
				return false
			}

			// Property 11a: OldTelegramBotTokens should be empty (zero value)
			if len(cfg.OldTelegramBotTokens) != 0 {
				t.Logf("OldTelegramBotTokens should be empty, got %v", cfg.OldTelegramBotTokens)
				return false
			}

			// Property 11b: ChangeBotAlertMsg should be empty (zero value)
			if len(cfg.ChangeBotAlertMsg) != 0 {
				t.Logf("ChangeBotAlertMsg should be empty, got %v", cfg.ChangeBotAlertMsg)
				return false
			}

			// Property 11c: CloseOldDate should be empty string (zero value)
			if cfg.CloseOldDate != "" {
				t.Logf("CloseOldDate should be empty, got %q", cfg.CloseOldDate)
				return false
			}

			// Property 11d: CloseOldDateParsed should be nil (zero value)
			if cfg.CloseOldDateParsed != nil {
				t.Logf("CloseOldDateParsed should be nil, got %v", cfg.CloseOldDateParsed)
				return false
			}

			// Also verify that the non-migration fields loaded correctly
			if cfg.Mailin.Host != host || cfg.Mailin.Port != port || cfg.Mailin.DisableWebhook != disableWebhook {
				t.Log("Mailin fields mismatch")
				return false
			}
			if cfg.MailDomain != mailDomain || cfg.TelegramBotToken != token || cfg.AdminTgID != adminID {
				t.Log("Core config fields mismatch")
				return false
			}
			if cfg.UploadURL != uploadURL || cfg.UploadToken != uploadToken {
				t.Log("Upload fields mismatch")
				return false
			}

			return true
		},
		gen.AnyString(), // host
		gen.Int(),       // port
		gen.Bool(),      // disableWebhook
		gen.AnyString(), // mailDomain
		gen.AnyString(), // token
		gen.Int64(),     // adminID
		gen.AnyString(), // uploadURL
		gen.AnyString(), // uploadToken
	))

	properties.TestingRun(t)
}

// Feature: go-version-rewrite, Property 1: Config parse round-trip consistency
// Validates: Requirements 1.2, 1.6
func TestProperty_ConfigParseRoundTrip(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("round-trip: serialize → Load → fields match", prop.ForAll(
		func(host string, port int, disableWebhook bool, mailDomain string,
			token string, adminID int64, uploadURL string, uploadToken string) bool {

			// Clamp port to valid range to avoid JSON issues
			if port < 0 {
				port = -port
			}
			port = port % 65536

			// Build JSON using the intermediate struct format that Load expects.
			// admin_tg_id is serialized as a number since Load handles both string and number.
			raw := map[string]interface{}{
				"mailin": map[string]interface{}{
					"host":           host,
					"port":           port,
					"disableWebhook": disableWebhook,
				},
				"mail_domain":        mailDomain,
				"telegram_bot_token": token,
				"admin_tg_id":        adminID,
				"upload_url":         uploadURL,
				"upload_token":       uploadToken,
			}

			data, err := json.Marshal(raw)
			if err != nil {
				return false
			}

			dir := t.TempDir()
			path := filepath.Join(dir, "config.json")
			if err := os.WriteFile(path, data, 0644); err != nil {
				return false
			}

			cfg, err := Load(path)
			if err != nil {
				t.Logf("Load error for valid config: %v\nJSON: %s", err, string(data))
				return false
			}

			return cfg.Mailin.Host == host &&
				cfg.Mailin.Port == port &&
				cfg.Mailin.DisableWebhook == disableWebhook &&
				cfg.MailDomain == mailDomain &&
				cfg.TelegramBotToken == token &&
				cfg.AdminTgID == adminID &&
				cfg.UploadURL == uploadURL &&
				cfg.UploadToken == uploadToken
		},
		gen.AnyString(),          // host
		gen.Int(),                // port
		gen.Bool(),               // disableWebhook
		gen.AnyString(),          // mailDomain
		gen.AnyString(),          // token
		gen.Int64(),              // adminID
		gen.AnyString(),          // uploadURL
		gen.AnyString(),          // uploadToken
	))

	properties.TestingRun(t)
}


// Feature: go-version-rewrite, Property 2: Invalid config rejection
// Validates: Requirements 1.4
func TestProperty_InvalidConfigRejection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("invalid JSON strings cause Load to return error", prop.ForAll(
		func(kind int, randomBytes string) bool {
			var content string
			switch kind % 5 {
			case 0:
				// Empty string
				content = ""
			case 1:
				// Random bytes (unlikely to be valid JSON config)
				content = randomBytes
			case 2:
				// Truncated JSON
				content = `{"mailin": {"host": "0.0.0.0"`
			case 3:
				// Valid JSON but wrong type (array instead of object)
				content = `[1, 2, 3]`
			case 4:
				// Valid JSON object but with wrong field types
				content = `{"mailin": "not_an_object", "admin_tg_id": [1,2]}`
			}

			dir := t.TempDir()
			path := filepath.Join(dir, "config.json")
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return false
			}

			_, err := Load(path)
			return err != nil
		},
		gen.IntRange(0, 4),  // kind selector
		gen.AnyString(),     // random bytes
	))

	// Also test that nonexistent file returns error
	properties.Property("nonexistent file returns error", prop.ForAll(
		func(randomPath string) bool {
			_, err := Load(filepath.Join("/nonexistent", randomPath, "config.json"))
			return err != nil
		},
		gen.AnyString(),
	))

	properties.TestingRun(t)
}
