package telegram

import (
	"os"
	"strings"
	"testing"

	"go-version-rewrite/internal/config"
	"go-version-rewrite/internal/db"
	"go-version-rewrite/internal/io"
)

// newTestBotWithIO creates a Bot with a real IO instance backed by a temp SQLite DB.
// This is needed for encodeCallback/decodeCallback which use IO's blockCache.
func newTestBotWithIO(t *testing.T) *Bot {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "test-bot-cb-*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	database, err := db.New(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfg := &config.Config{MailDomain: "test.example.com"}
	ioModule := io.New(database, cfg)

	return &Bot{
		io:     ioModule,
		config: cfg,
	}
}

func TestEncodeCallback_Short(t *testing.T) {
	b := newTestBotWithIO(t)

	result := b.encodeCallback("dismiss_yes", "example.com")
	if result != "dismiss_yes:example.com" {
		t.Errorf("expected 'dismiss_yes:example.com', got %q", result)
	}
}

func TestEncodeCallback_NoParams(t *testing.T) {
	b := newTestBotWithIO(t)

	result := b.encodeCallback("help_back")
	if result != "help_back" {
		t.Errorf("expected 'help_back', got %q", result)
	}
}

func TestEncodeCallback_MultipleParams(t *testing.T) {
	b := newTestBotWithIO(t)

	result := b.encodeCallback("action", "p1", "p2", "p3")
	if result != "action:p1:p2:p3" {
		t.Errorf("expected 'action:p1:p2:p3', got %q", result)
	}
}

func TestEncodeCallback_ExactlyAt64Bytes(t *testing.T) {
	b := newTestBotWithIO(t)

	// Create data that is exactly 64 bytes: "a:" + 62 chars of padding
	action := "a"
	param := strings.Repeat("x", 62) // "a:xxx...x" = 64 bytes
	result := b.encodeCallback(action, param)

	// Should NOT be cached (<=64)
	if strings.HasPrefix(result, callbackCachePrefix) {
		t.Errorf("64-byte data should not be cached, got %q", result)
	}
	expected := action + ":" + param
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestEncodeCallback_Over64Bytes(t *testing.T) {
	b := newTestBotWithIO(t)

	// Create data that exceeds 64 bytes
	action := "dismiss_yes"
	param := strings.Repeat("a", 60) // "dismiss_yes:aaa...a" > 64 bytes
	result := b.encodeCallback(action, param)

	if !strings.HasPrefix(result, callbackCachePrefix) {
		t.Errorf("over-64-byte data should be cached with prefix %q, got %q", callbackCachePrefix, result)
	}
	// The cached result should be short enough for Telegram
	if len(result) > 64 {
		t.Errorf("cached result should be <= 64 bytes, got %d bytes: %q", len(result), result)
	}
}

func TestDecodeCallback_Simple(t *testing.T) {
	b := newTestBotWithIO(t)

	cd := b.decodeCallback("dismiss_yes:example.com")
	if cd.Action != "dismiss_yes" {
		t.Errorf("expected action 'dismiss_yes', got %q", cd.Action)
	}
	if len(cd.Params) != 1 || cd.Params[0] != "example.com" {
		t.Errorf("expected params ['example.com'], got %v", cd.Params)
	}
}

func TestDecodeCallback_NoParams(t *testing.T) {
	b := newTestBotWithIO(t)

	cd := b.decodeCallback("help_back")
	if cd.Action != "help_back" {
		t.Errorf("expected action 'help_back', got %q", cd.Action)
	}
	if cd.Params != nil {
		t.Errorf("expected nil params, got %v", cd.Params)
	}
}

func TestDecodeCallback_MultipleParams(t *testing.T) {
	b := newTestBotWithIO(t)

	cd := b.decodeCallback("action:p1:p2:p3")
	if cd.Action != "action" {
		t.Errorf("expected action 'action', got %q", cd.Action)
	}
	if len(cd.Params) != 3 || cd.Params[0] != "p1" || cd.Params[1] != "p2" || cd.Params[2] != "p3" {
		t.Errorf("expected params [p1 p2 p3], got %v", cd.Params)
	}
}

func TestEncodeDecodeCallback_RoundTrip_Short(t *testing.T) {
	b := newTestBotWithIO(t)

	action := "dismiss_yes"
	params := []string{"example.com"}

	encoded := b.encodeCallback(action, params...)
	decoded := b.decodeCallback(encoded)

	if decoded.Action != action {
		t.Errorf("round-trip action: expected %q, got %q", action, decoded.Action)
	}
	if len(decoded.Params) != len(params) {
		t.Fatalf("round-trip params length: expected %d, got %d", len(params), len(decoded.Params))
	}
	for i, p := range params {
		if decoded.Params[i] != p {
			t.Errorf("round-trip param[%d]: expected %q, got %q", i, p, decoded.Params[i])
		}
	}
}

func TestEncodeDecodeCallback_RoundTrip_Long(t *testing.T) {
	b := newTestBotWithIO(t)

	action := "block_sender"
	params := []string{strings.Repeat("very-long-email-address@", 3) + "example.com"}

	encoded := b.encodeCallback(action, params...)
	decoded := b.decodeCallback(encoded)

	if decoded.Action != action {
		t.Errorf("round-trip action: expected %q, got %q", action, decoded.Action)
	}
	if len(decoded.Params) != len(params) {
		t.Fatalf("round-trip params length: expected %d, got %d", len(params), len(decoded.Params))
	}
	for i, p := range params {
		if decoded.Params[i] != p {
			t.Errorf("round-trip param[%d]: expected %q, got %q", i, p, decoded.Params[i])
		}
	}
}

func TestDecodeCallback_EmptyString(t *testing.T) {
	b := newTestBotWithIO(t)

	cd := b.decodeCallback("")
	if cd.Action != "" {
		t.Errorf("expected empty action, got %q", cd.Action)
	}
}

func TestDecodeCallback_CachedNotFound(t *testing.T) {
	b := newTestBotWithIO(t)

	// "cb:" prefix with a non-existent ID should fall back to splitting the raw data
	cd := b.decodeCallback("cb:99999999")
	// Since the ID isn't in cache, it decodes "cb:99999999" as-is
	if cd.Action != "cb" {
		t.Errorf("expected action 'cb', got %q", cd.Action)
	}
}
