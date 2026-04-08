package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

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
