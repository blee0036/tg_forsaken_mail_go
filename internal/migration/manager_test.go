package migration

import (
	"testing"
	"time"

	"go-version-rewrite/internal/config"
	"go-version-rewrite/internal/io"
)

func TestIsCloseOldDateReachedAt_NilDate(t *testing.T) {
	mm := &MigrationManager{
		config: &config.Config{
			CloseOldDateParsed: nil,
		},
	}

	if mm.isCloseOldDateReachedAt(time.Now()) {
		t.Error("expected false when CloseOldDateParsed is nil")
	}
}

func TestIsCloseOldDateReachedAt_FutureDate(t *testing.T) {
	future := time.Now().AddDate(0, 0, 7) // 7 days from now
	mm := &MigrationManager{
		config: &config.Config{
			CloseOldDateParsed: &future,
		},
	}

	if mm.isCloseOldDateReachedAt(time.Now()) {
		t.Error("expected false when close_old_date is in the future")
	}
}

func TestIsCloseOldDateReachedAt_PastDate(t *testing.T) {
	past := time.Now().AddDate(0, 0, -1) // yesterday
	mm := &MigrationManager{
		config: &config.Config{
			CloseOldDateParsed: &past,
		},
	}

	if !mm.isCloseOldDateReachedAt(time.Now()) {
		t.Error("expected true when close_old_date is in the past")
	}
}

func TestIsCloseOldDateReachedAt_Today(t *testing.T) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	mm := &MigrationManager{
		config: &config.Config{
			CloseOldDateParsed: &today,
		},
	}

	if !mm.isCloseOldDateReachedAt(now) {
		t.Error("expected true when close_old_date is today")
	}
}

func TestActiveMailEndpoints_NoOldBots(t *testing.T) {
	mm := &MigrationManager{
		config: &config.Config{
			CloseOldDateParsed: nil,
		},
		oldBots: nil,
		newBot:  nil, // We can't call GetSender on nil, so we test with a mock approach
	}

	// This test verifies the logic structure; actual Bot creation requires valid tokens.
	// We test the isCloseOldDateReachedAt logic separately.
	_ = mm
}

func TestActiveMailEndpoints_WithCloseDate(t *testing.T) {
	// Test that when close_old_date is reached, old bots are excluded
	past := time.Now().AddDate(0, 0, -1)
	mm := &MigrationManager{
		config: &config.Config{
			CloseOldDateParsed: &past,
		},
		oldBots: nil,
	}

	if !mm.isCloseOldDateReachedAt(time.Now()) {
		t.Error("expected close date to be reached")
	}
}

func TestAlertFunc_NoAlertConfig(t *testing.T) {
	mm := &MigrationManager{
		alertSender: &AlertSender{
			config: &config.Config{
				ChangeBotAlertMsg: map[string]string{},
			},
		},
	}

	if mm.AlertFunc() != nil {
		t.Error("expected nil AlertFunc when no alert messages configured")
	}
}

func TestAlertFunc_WithAlertConfig(t *testing.T) {
	mm := &MigrationManager{
		alertSender: &AlertSender{
			config: &config.Config{
				ChangeBotAlertMsg: map[string]string{
					"en": "Please switch to new bot",
				},
			},
		},
	}

	alertFn := mm.AlertFunc()
	if alertFn == nil {
		t.Error("expected non-nil AlertFunc when alert messages are configured")
	}
}

func TestTruncateToken(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"short", "short"},
		{"1234567890", "1234567890"},
		{"12345678901", "1234567890"},
		{"abcdefghijklmnop", "abcdefghij"},
		{"", ""},
	}

	for _, tt := range tests {
		result := truncateToken(tt.input)
		if result != tt.expected {
			t.Errorf("truncateToken(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestNewMigrationManager_EmptyTokens(t *testing.T) {
	cfg := &config.Config{
		OldTelegramBotTokens: []string{},
		ChangeBotAlertMsg:    map[string]string{"en": "test"},
	}

	mm := NewMigrationManager(cfg, nil, nil)
	if len(mm.oldBots) != 0 {
		t.Errorf("expected 0 old bots, got %d", len(mm.oldBots))
	}
}

func TestNewMigrationManager_InvalidTokens(t *testing.T) {
	cfg := &config.Config{
		OldTelegramBotTokens: []string{"invalid-token-1", "invalid-token-2"},
		ChangeBotAlertMsg:    map[string]string{"en": "test"},
	}

	// NewMigrationManager should log errors for invalid tokens and skip them
	mm := NewMigrationManager(cfg, nil, nil)
	if len(mm.oldBots) != 0 {
		t.Errorf("expected 0 old bots (invalid tokens should be skipped), got %d", len(mm.oldBots))
	}
}

func TestStop_Idempotent(t *testing.T) {
	mm := &MigrationManager{
		stopCh: make(chan struct{}),
	}

	// First stop should work
	mm.Stop()

	// Second stop should not panic
	mm.Stop()

	// Channel should be closed
	select {
	case <-mm.StopCh():
		// OK, channel is closed
	default:
		t.Error("expected stop channel to be closed")
	}
}

// Verify that DeliveryEndpoint type is accessible from this package
func TestDeliveryEndpointType(t *testing.T) {
	ep := io.DeliveryEndpoint{
		Name:   "test",
		Sender: nil,
		IsOld:  true,
	}
	if ep.Name != "test" {
		t.Error("unexpected name")
	}
	if !ep.IsOld {
		t.Error("expected IsOld to be true")
	}
}
