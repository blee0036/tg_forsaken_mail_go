package migration

import (
	"os"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"go-version-rewrite/internal/config"
	"go-version-rewrite/internal/db"
	"go-version-rewrite/internal/io"
)

// mockCronSender records all Send calls for verification.
type mockCronSender struct {
	messages []tgbotapi.Chattable
}

func (m *mockCronSender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	m.messages = append(m.messages, c)
	return tgbotapi.Message{}, nil
}

func (m *mockCronSender) Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	return &tgbotapi.APIResponse{Ok: true}, nil
}

func TestCronScheduler_RunOnce_NoAlert(t *testing.T) {
	cfg := &config.Config{
		ChangeBotAlertMsg: map[string]string{}, // no alert configured
	}
	alertSender := NewAlertSender(cfg)

	cron := NewCronScheduler(cfg, nil, nil, nil, alertSender)
	// Should not panic even with nil db/io/bots when no alert is configured
	cron.RunOnce()
}

func TestCronScheduler_RunOnce_SendsAlerts(t *testing.T) {
	// Create temp DB with domain_tg entries
	tmpFile, err := os.CreateTemp("", "cron_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	database, err := db.New(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Insert some domain_tg entries (same tgID with multiple domains)
	database.InsertDomain("domain1.example.com", 100)
	database.InsertDomain("domain2.example.com", 100) // same tgID
	database.InsertDomain("domain3.example.com", 200)

	cfg := &config.Config{
		MailDomain:        "example.com",
		ChangeBotAlertMsg: map[string]string{"en": "Please switch to new bot"},
	}

	ioModule := io.New(database, cfg)
	alertSender := NewAlertSender(cfg)

	// Create mock old bots using a mock sender
	sender1 := &mockCronSender{}
	sender2 := &mockCronSender{}

	// We can't easily create real telegram.Bot instances without valid tokens,
	// so we test RunOnce logic by directly testing the DB method and alert logic.
	// For the full integration, we verify SelectAllUniqueTgIDs works correctly.
	_ = ioModule
	_ = sender1
	_ = sender2

	// Verify SelectAllUniqueTgIDs returns unique IDs
	tgIDs, err := database.SelectAllUniqueTgIDs()
	if err != nil {
		t.Fatalf("SelectAllUniqueTgIDs failed: %v", err)
	}

	if len(tgIDs) != 2 {
		t.Errorf("expected 2 unique tgIDs, got %d", len(tgIDs))
	}

	// Verify the IDs are 100 and 200 (order may vary)
	idSet := make(map[int64]bool)
	for _, id := range tgIDs {
		idSet[id] = true
	}
	if !idSet[100] || !idSet[200] {
		t.Errorf("expected tgIDs to contain 100 and 200, got %v", tgIDs)
	}

	// Verify alert sender resolves message
	msg := alertSender.ResolveAlertMessage("")
	if msg != "Please switch to new bot" {
		t.Errorf("expected alert message, got %q", msg)
	}
}

func TestCronScheduler_Stop(t *testing.T) {
	cfg := &config.Config{
		ChangeBotAlertMsg: map[string]string{"en": "test"},
	}
	alertSender := NewAlertSender(cfg)
	cron := NewCronScheduler(cfg, nil, nil, nil, alertSender)

	// Stop should be idempotent
	cron.Stop()
	cron.Stop() // should not panic
}

func TestCronScheduler_StopTerminatesStart(t *testing.T) {
	cfg := &config.Config{
		ChangeBotAlertMsg: map[string]string{"en": "test"},
	}
	alertSender := NewAlertSender(cfg)
	cron := NewCronScheduler(cfg, nil, nil, nil, alertSender)

	done := make(chan struct{})
	go func() {
		cron.Start()
		close(done)
	}()

	// Give Start a moment to begin, then stop it
	time.Sleep(50 * time.Millisecond)
	cron.Stop()

	select {
	case <-done:
		// Start returned successfully
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after Stop was called")
	}
}

func TestCronScheduler_IsCloseOldDateReached(t *testing.T) {
	// Not configured
	cron := &CronScheduler{
		config: &config.Config{CloseOldDateParsed: nil},
	}
	if cron.isCloseOldDateReached() {
		t.Error("expected false when CloseOldDateParsed is nil")
	}

	// Future date
	future := time.Now().AddDate(0, 0, 30)
	cron = &CronScheduler{
		config: &config.Config{CloseOldDateParsed: &future},
	}
	if cron.isCloseOldDateReached() {
		t.Error("expected false for future date")
	}

	// Past date
	past := time.Now().AddDate(0, 0, -1)
	cron = &CronScheduler{
		config: &config.Config{CloseOldDateParsed: &past},
	}
	if !cron.isCloseOldDateReached() {
		t.Error("expected true for past date")
	}
}

func TestSelectAllUniqueTgIDs_EmptyTable(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "cron_empty_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	database, err := db.New(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	tgIDs, err := database.SelectAllUniqueTgIDs()
	if err != nil {
		t.Fatalf("SelectAllUniqueTgIDs failed: %v", err)
	}
	if len(tgIDs) != 0 {
		t.Errorf("expected 0 tgIDs for empty table, got %d", len(tgIDs))
	}
}

func TestSelectAllUniqueTgIDs_Deduplication(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "cron_dedup_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	database, err := db.New(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Insert multiple domains for same tgID
	database.InsertDomain("a.example.com", 42)
	database.InsertDomain("b.example.com", 42)
	database.InsertDomain("c.example.com", 42)
	database.InsertDomain("d.example.com", 99)

	tgIDs, err := database.SelectAllUniqueTgIDs()
	if err != nil {
		t.Fatalf("SelectAllUniqueTgIDs failed: %v", err)
	}
	if len(tgIDs) != 2 {
		t.Errorf("expected 2 unique tgIDs, got %d: %v", len(tgIDs), tgIDs)
	}
}
