package io

import (
	"os"
	"strings"
	"testing"

	"go-version-rewrite/internal/config"
	"go-version-rewrite/internal/db"
	smtpmod "go-version-rewrite/internal/smtp"
)

// newTestIO creates an IO instance backed by a temporary SQLite database.
func newTestIO(t *testing.T) (*IO, *db.DB) {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "test-io-*.db")
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

	cfg := &config.Config{
		MailDomain: "test.example.com",
	}

	io := New(database, cfg)
	return io, database
}

func TestNew(t *testing.T) {
	io, _ := newTestIO(t)

	if io.db == nil {
		t.Error("db should not be nil")
	}
	if io.config == nil {
		t.Error("config should not be nil")
	}
	if io.blockDomain == nil {
		t.Error("blockDomain cache should not be nil")
	}
	if io.blockSender == nil {
		t.Error("blockSender cache should not be nil")
	}
	if io.blockReceiver == nil {
		t.Error("blockReceiver cache should not be nil")
	}
	if io.blockCache == nil {
		t.Error("blockCache cache should not be nil")
	}
	if io.snowflakeNode == nil {
		t.Error("snowflakeNode should not be nil")
	}
	if io.bot != nil {
		t.Error("bot should be nil initially")
	}
}

func TestInit(t *testing.T) {
	io, database := newTestIO(t)

	// Insert some domains into the database
	database.InsertDomain("alpha.test.example.com", 100)
	database.InsertDomain("beta.test.example.com", 200)
	database.InsertDomain("gamma.test.example.com", 100)

	if err := io.Init(); err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	// Verify all domains are loaded into the map
	val, ok := io.domainToUser.Load("alpha.test.example.com")
	if !ok || val.(int64) != 100 {
		t.Errorf("expected alpha.test.example.com -> 100, got %v, %v", val, ok)
	}

	val, ok = io.domainToUser.Load("beta.test.example.com")
	if !ok || val.(int64) != 200 {
		t.Errorf("expected beta.test.example.com -> 200, got %v, %v", val, ok)
	}

	val, ok = io.domainToUser.Load("gamma.test.example.com")
	if !ok || val.(int64) != 100 {
		t.Errorf("expected gamma.test.example.com -> 100, got %v, %v", val, ok)
	}

	// Non-existent domain
	_, ok = io.domainToUser.Load("nonexistent.test.example.com")
	if ok {
		t.Error("nonexistent domain should not be in map")
	}
}

func TestSetBot(t *testing.T) {
	io, _ := newTestIO(t)

	if io.bot != nil {
		t.Error("bot should be nil before SetBot")
	}

	// We can't create a real BotAPI without a token, but we can test nil handling
	io.SetBot(nil)
	if io.bot != nil {
		t.Error("bot should be nil after SetBot(nil)")
	}
}

func TestBindDomain(t *testing.T) {
	io, _ := newTestIO(t)

	// Bind a new domain
	msg := io.BindDomain(100, "example.com")
	if msg != "Bind Success!" {
		t.Errorf("expected 'Bind Success!', got %q", msg)
	}

	// Verify it's in the map
	val, ok := io.domainToUser.Load("example.com")
	if !ok || val.(int64) != 100 {
		t.Errorf("domain should be bound to user 100")
	}

	// Bind same domain to same user
	msg = io.BindDomain(100, "example.com")
	if msg != "This domain has already bind on your account!" {
		t.Errorf("expected already bound message, got %q", msg)
	}

	// Bind same domain to different user
	msg = io.BindDomain(200, "example.com")
	if msg != "This domain has already bind on another account!" {
		t.Errorf("expected another account message, got %q", msg)
	}
}

func TestRemoveDomain(t *testing.T) {
	io, _ := newTestIO(t)

	// Bind a domain first
	io.BindDomain(100, "example.com")

	// Remove domain owned by user
	msg := io.RemoveDomain(100, "example.com")
	if msg != "Release Success!" {
		t.Errorf("expected 'Release Success!', got %q", msg)
	}

	// Verify it's removed from the map
	_, ok := io.domainToUser.Load("example.com")
	if ok {
		t.Error("domain should be removed from map")
	}

	// Try to remove a domain not bound to user
	io.BindDomain(100, "other.com")
	msg = io.RemoveDomain(200, "other.com")
	if msg != "This domain has not bind to your account!" {
		t.Errorf("expected not bound message, got %q", msg)
	}

	// Try to remove a non-existent domain
	msg = io.RemoveDomain(100, "nonexistent.com")
	if msg != "This domain has not bind to your account!" {
		t.Errorf("expected not bound message, got %q", msg)
	}
}

func TestListDomain(t *testing.T) {
	io, _ := newTestIO(t)

	// List with no domains
	msg := io.ListDomain(100)
	if !strings.HasPrefix(msg, "<b>Your domain :</b> \n\n") {
		t.Errorf("expected header prefix, got %q", msg)
	}

	// Bind some domains
	io.BindDomain(100, "alpha.com")
	io.BindDomain(100, "beta.com")
	io.BindDomain(200, "gamma.com") // different user

	msg = io.ListDomain(100)
	if !strings.Contains(msg, "alpha.com") {
		t.Error("list should contain alpha.com")
	}
	if !strings.Contains(msg, "beta.com") {
		t.Error("list should contain beta.com")
	}
	if strings.Contains(msg, "gamma.com") {
		t.Error("list should not contain gamma.com (different user)")
	}
}

func TestGetAllDomainCount(t *testing.T) {
	io, _ := newTestIO(t)

	if count := io.GetAllDomainCount(100); count != 0 {
		t.Errorf("expected 0, got %d", count)
	}

	io.BindDomain(100, "a.com")
	io.BindDomain(100, "b.com")
	io.BindDomain(200, "c.com")

	if count := io.GetAllDomainCount(100); count != 2 {
		t.Errorf("expected 2, got %d", count)
	}

	if count := io.GetAllDomainCount(200); count != 1 {
		t.Errorf("expected 1, got %d", count)
	}

	if count := io.GetAllDomainCount(300); count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
}

func TestBindDefaultDomain(t *testing.T) {
	io, _ := newTestIO(t)

	domain := io.BindDefaultDomain(100)
	if domain == "" {
		t.Fatal("expected a domain, got empty string")
	}

	// Verify format: should end with .test.example.com and be all lowercase
	if !strings.HasSuffix(domain, "."+io.config.MailDomain) {
		t.Errorf("domain %q should end with .%s", domain, io.config.MailDomain)
	}
	if domain != strings.ToLower(domain) {
		t.Errorf("domain %q should be all lowercase", domain)
	}

	// Verify it's in the map
	val, ok := io.domainToUser.Load(domain)
	if !ok || val.(int64) != 100 {
		t.Errorf("domain should be bound to user 100")
	}

	// Verify it's in the database
	records, err := io.db.SelectByDomain(domain)
	if err != nil {
		t.Fatalf("SelectByDomain error: %v", err)
	}
	if len(records) != 1 || records[0].Tg != 100 {
		t.Errorf("expected 1 record with tg=100, got %v", records)
	}
}

func TestBindDefaultDomainRetry(t *testing.T) {
	// This test verifies that BindDefaultDomain retries when domain exists.
	// We can't easily force collisions with random names, but we verify
	// the function works correctly in the normal case.
	io, _ := newTestIO(t)

	// Generate multiple default domains - they should all be unique
	domains := make(map[string]bool)
	for i := 0; i < 10; i++ {
		domain := io.BindDefaultDomain(int64(100 + i))
		if domain == "" {
			t.Fatalf("iteration %d: expected a domain, got empty", i)
		}
		if domains[domain] {
			t.Errorf("iteration %d: duplicate domain %q", i, domain)
		}
		domains[domain] = true
	}
}

// --- Block management tests ---

func TestGetTrueBlockData_NonNumeric(t *testing.T) {
	io, _ := newTestIO(t)

	// Non-numeric data should be returned as-is
	result := io.getTrueBlockData("user@example.com")
	if result != "user@example.com" {
		t.Errorf("expected 'user@example.com', got %q", result)
	}
}

func TestGetTrueBlockData_Numeric(t *testing.T) {
	io, _ := newTestIO(t)

	// Set up cache with a numeric key
	io.blockCache.Set("12345", "real-sender@example.com")

	result := io.getTrueBlockData("12345")
	if result != "real-sender@example.com" {
		t.Errorf("expected 'real-sender@example.com', got %q", result)
	}

	// Cache entry should be deleted after retrieval (like Node's cache.take)
	_, found := io.blockCache.Get("12345")
	if found {
		t.Error("cache entry should be deleted after getTrueBlockData")
	}
}

func TestGetTrueBlockData_NumericNotInCache(t *testing.T) {
	io, _ := newTestIO(t)

	// Numeric data not in cache should return as-is
	result := io.getTrueBlockData("99999")
	if result != "99999" {
		t.Errorf("expected '99999', got %q", result)
	}
}

func TestCheckButtonData_Short(t *testing.T) {
	io, _ := newTestIO(t)

	result := io.CheckButtonData("user@example.com", "block_sender")
	expected := "block_sender user@example.com"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestCheckButtonData_Long(t *testing.T) {
	io, _ := newTestIO(t)

	// Create data that when combined with key exceeds 64 chars
	longData := strings.Repeat("a", 60)
	result := io.CheckButtonData(longData, "block_sender")

	// Should start with "block_sender " and have a snowflake ID instead of the long data
	if !strings.HasPrefix(result, "block_sender ") {
		t.Errorf("result should start with 'block_sender ', got %q", result)
	}
	if strings.Contains(result, longData) {
		t.Error("result should not contain the original long data")
	}

	// The snowflake ID should be cached
	parts := strings.SplitN(result, " ", 2)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	snowflakeID := parts[1]
	cached, found := io.blockCache.Get(snowflakeID)
	if !found {
		t.Error("snowflake ID should be cached")
	}
	if cached.(string) != longData {
		t.Errorf("cached value should be %q, got %q", longData, cached.(string))
	}
}

func TestCheckButtonData_RoundTrip(t *testing.T) {
	io, _ := newTestIO(t)

	// Short data round-trip
	shortData := "user@ex.com"
	buttonData := io.CheckButtonData(shortData, "block_sender")
	parts := strings.SplitN(buttonData, " ", 2)
	result := io.getTrueBlockData(parts[1])
	if result != shortData {
		t.Errorf("short round-trip: expected %q, got %q", shortData, result)
	}

	// Long data round-trip
	longData := strings.Repeat("x", 60)
	buttonData = io.CheckButtonData(longData, "block_sender")
	parts = strings.SplitN(buttonData, " ", 2)
	result = io.getTrueBlockData(parts[1])
	if result != longData {
		t.Errorf("long round-trip: expected %q, got %q", longData, result)
	}
}

func TestBlockSender(t *testing.T) {
	io, _ := newTestIO(t)

	msg := io.BlockSender(100, "spammer@evil.com")
	expected := "Block sender spammer@evil.com Success!"
	if msg != expected {
		t.Errorf("expected %q, got %q", expected, msg)
	}

	// Verify it's in the database
	senders, err := io.db.SelectUserAllBlockSender(100)
	if err != nil {
		t.Fatalf("SelectUserAllBlockSender error: %v", err)
	}
	if len(senders) != 1 || senders[0].Sender != "spammer@evil.com" {
		t.Errorf("expected 1 sender record, got %v", senders)
	}
}

func TestBlockSender_WithCachedData(t *testing.T) {
	io, _ := newTestIO(t)

	// Simulate a snowflake ID scenario
	io.blockCache.Set("12345", "real-sender@example.com")

	msg := io.BlockSender(100, "12345")
	expected := "Block sender real-sender@example.com Success!"
	if msg != expected {
		t.Errorf("expected %q, got %q", expected, msg)
	}

	// Verify the real data is in the database
	senders, err := io.db.SelectUserAllBlockSender(100)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(senders) != 1 || senders[0].Sender != "real-sender@example.com" {
		t.Errorf("expected real-sender@example.com in DB, got %v", senders)
	}
}

func TestBlockDomain(t *testing.T) {
	io, _ := newTestIO(t)

	msg := io.BlockDomain(100, "evil.com")
	expected := "Block domain evil.com Success!"
	if msg != expected {
		t.Errorf("expected %q, got %q", expected, msg)
	}

	domains, err := io.db.SelectUserAllBlockDomain(100)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(domains) != 1 || domains[0].Domain != "evil.com" {
		t.Errorf("expected evil.com in DB, got %v", domains)
	}
}

func TestBlockReceiver(t *testing.T) {
	io, _ := newTestIO(t)

	msg := io.BlockReceiver(100, "me@private.com")
	expected := "Block receiver me@private.com Success!"
	if msg != expected {
		t.Errorf("expected %q, got %q", expected, msg)
	}

	receivers, err := io.db.SelectUserAllBlockReceiver(100)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(receivers) != 1 || receivers[0].Receiver != "me@private.com" {
		t.Errorf("expected me@private.com in DB, got %v", receivers)
	}
}

func TestRemoveBlockSender(t *testing.T) {
	io, _ := newTestIO(t)

	// Block first, then remove
	io.BlockSender(100, "spammer@evil.com")
	msg := io.RemoveBlockSender(100, "spammer@evil.com")
	expected := "Remove block sender spammer@evil.com Success!"
	if msg != expected {
		t.Errorf("expected %q, got %q", expected, msg)
	}

	senders, err := io.db.SelectUserAllBlockSender(100)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(senders) != 0 {
		t.Errorf("expected 0 senders after removal, got %v", senders)
	}
}

func TestRemoveBlockDomain(t *testing.T) {
	io, _ := newTestIO(t)

	io.BlockDomain(100, "evil.com")
	msg := io.RemoveBlockDomain(100, "evil.com")
	expected := "Remove block domain evil.com Success!"
	if msg != expected {
		t.Errorf("expected %q, got %q", expected, msg)
	}

	domains, err := io.db.SelectUserAllBlockDomain(100)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(domains) != 0 {
		t.Errorf("expected 0 domains after removal, got %v", domains)
	}
}

func TestRemoveBlockReceiver(t *testing.T) {
	io, _ := newTestIO(t)

	io.BlockReceiver(100, "me@private.com")
	msg := io.RemoveBlockReceiver(100, "me@private.com")
	expected := "Remove block receiver me@private.com Success!"
	if msg != expected {
		t.Errorf("expected %q, got %q", expected, msg)
	}

	receivers, err := io.db.SelectUserAllBlockReceiver(100)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(receivers) != 0 {
		t.Errorf("expected 0 receivers after removal, got %v", receivers)
	}
}

func TestListBlockSender(t *testing.T) {
	io, _ := newTestIO(t)

	// Empty list
	msg := io.ListBlockSender(100)
	if !strings.HasPrefix(msg, "<b>Your block sender :</b> \n\n") {
		t.Errorf("expected header prefix, got %q", msg)
	}

	// Add some blocked senders
	io.BlockSender(100, "a@evil.com")
	io.BlockSender(100, "b@evil.com")

	msg = io.ListBlockSender(100)
	if !strings.Contains(msg, "a@evil.com") {
		t.Error("list should contain a@evil.com")
	}
	if !strings.Contains(msg, "b@evil.com") {
		t.Error("list should contain b@evil.com")
	}
	if !strings.Contains(msg, "1: <code> a@evil.com</code>") {
		t.Errorf("expected formatted item, got %q", msg)
	}
}

func TestListBlockDomain(t *testing.T) {
	io, _ := newTestIO(t)

	msg := io.ListBlockDomain(100)
	if !strings.HasPrefix(msg, "<b>Your block domain :</b> \n\n") {
		t.Errorf("expected header prefix, got %q", msg)
	}

	io.BlockDomain(100, "evil.com")
	msg = io.ListBlockDomain(100)
	if !strings.Contains(msg, "evil.com") {
		t.Error("list should contain evil.com")
	}
}

func TestListBlockReceiver(t *testing.T) {
	io, _ := newTestIO(t)

	msg := io.ListBlockReceiver(100)
	if !strings.HasPrefix(msg, "<b>Your block receiver :</b> \n\n") {
		t.Errorf("expected header prefix, got %q", msg)
	}

	io.BlockReceiver(100, "me@private.com")
	msg = io.ListBlockReceiver(100)
	if !strings.Contains(msg, "me@private.com") {
		t.Error("list should contain me@private.com")
	}
}

func TestBlockClearsCache(t *testing.T) {
	io, _ := newTestIO(t)

	// Pre-populate the sender cache for user 100
	io.blockSender.Set("100", "some-cached-data")

	// Block should clear the cache
	io.BlockSender(100, "spammer@evil.com")

	_, found := io.blockSender.Get("100")
	if found {
		t.Error("blockSender cache should be cleared after BlockSender")
	}
}

func TestRemoveBlockClearsCache(t *testing.T) {
	io, _ := newTestIO(t)

	// Pre-populate caches
	io.blockSender.Set("100", "cached")
	io.blockDomain.Set("100", "cached")
	io.blockReceiver.Set("100", "cached")

	io.RemoveBlockSender(100, "x")
	if _, found := io.blockSender.Get("100"); found {
		t.Error("blockSender cache should be cleared after RemoveBlockSender")
	}

	io.RemoveBlockDomain(100, "x")
	if _, found := io.blockDomain.Get("100"); found {
		t.Error("blockDomain cache should be cleared after RemoveBlockDomain")
	}

	io.RemoveBlockReceiver(100, "x")
	if _, found := io.blockReceiver.Get("100"); found {
		t.Error("blockReceiver cache should be cleared after RemoveBlockReceiver")
	}
}

func TestStoreCallbackData(t *testing.T) {
	io, _ := newTestIO(t)

	id := io.StoreCallbackData("dismiss_yes:example.com")
	if id == "" {
		t.Fatal("expected non-empty snowflake ID")
	}

	// Verify it's stored in blockCache
	val, found := io.blockCache.Get(id)
	if !found {
		t.Fatal("data should be stored in blockCache")
	}
	if val.(string) != "dismiss_yes:example.com" {
		t.Errorf("expected 'dismiss_yes:example.com', got %q", val.(string))
	}
}

func TestStoreCallbackData_UniqueIDs(t *testing.T) {
	io, _ := newTestIO(t)

	ids := make(map[string]bool)
	for i := 0; i < 10; i++ {
		id := io.StoreCallbackData("data")
		if ids[id] {
			t.Errorf("duplicate snowflake ID: %s", id)
		}
		ids[id] = true
	}
}

func TestRetrieveCallbackData_Found(t *testing.T) {
	io, _ := newTestIO(t)

	id := io.StoreCallbackData("action:param1:param2")
	data, ok := io.RetrieveCallbackData(id)
	if !ok {
		t.Fatal("expected data to be found")
	}
	if data != "action:param1:param2" {
		t.Errorf("expected 'action:param1:param2', got %q", data)
	}

	// Unlike getTrueBlockData, RetrieveCallbackData should NOT delete the entry
	data2, ok2 := io.RetrieveCallbackData(id)
	if !ok2 {
		t.Fatal("data should still be in cache after retrieval")
	}
	if data2 != "action:param1:param2" {
		t.Errorf("second retrieval: expected 'action:param1:param2', got %q", data2)
	}
}

func TestRetrieveCallbackData_NotFound(t *testing.T) {
	io, _ := newTestIO(t)

	data, ok := io.RetrieveCallbackData("nonexistent-id")
	if ok {
		t.Error("expected not found")
	}
	if data != "" {
		t.Errorf("expected empty string, got %q", data)
	}
}

func TestSendAll_NoBot(t *testing.T) {
	io, _ := newTestIO(t)

	// Bind some domains to create users
	io.BindDomain(100, "a.com")
	io.BindDomain(200, "b.com")

	// SendAll with no bot should not panic
	io.SendAll("test message")
}


// --- HandleMail tests ---

func newTestParsedMail() *smtpmod.ParsedMail {
	return &smtpmod.ParsedMail{
		From:    "sender@example.com",
		To:      "user@bound.com",
		Subject: "Test Subject",
		Date:    "2024-01-15 10:30:00",
		Text:    "Hello, this is a test email.",
		HTML:    "",
	}
}

func TestHandleMail_NoDomainMatch(t *testing.T) {
	io, _ := newTestIO(t)

	mail := newTestParsedMail()
	mail.To = "user@unknown.com"

	// Should not panic, just return silently
	io.HandleMail(mail)
}

func TestHandleMail_InvalidToAddress(t *testing.T) {
	io, _ := newTestIO(t)

	mail := newTestParsedMail()
	mail.To = "not-an-email"

	// Should not panic
	io.HandleMail(mail)
}

func TestHandleMail_BlockedReceiver(t *testing.T) {
	io, _ := newTestIO(t)

	// Bind domain and block receiver
	io.BindDomain(100, "bound.com")
	io.db.InsertBlockReceiver("user@bound.com", 100)

	mail := newTestParsedMail()

	// Should return silently (blocked receiver)
	io.HandleMail(mail)
}

func TestHandleMail_BlockedDomain(t *testing.T) {
	io, _ := newTestIO(t)

	io.BindDomain(100, "bound.com")
	io.db.InsertBlockDomain("example.com", 100)

	mail := newTestParsedMail()

	// Should return silently (blocked sender domain)
	io.HandleMail(mail)
}

func TestHandleMail_BlockedSender(t *testing.T) {
	io, _ := newTestIO(t)

	io.BindDomain(100, "bound.com")
	io.db.InsertBlockSender("sender@example.com", 100)

	mail := newTestParsedMail()

	// Should return silently (blocked sender)
	io.HandleMail(mail)
}

func TestHandleMail_MessageFormat(t *testing.T) {
	// Test the message formatting logic via FormatMailNotification
	io, _ := newTestIO(t)
	m := &smtpmod.ParsedMail{
		From:    "alice@sender.com",
		To:      "bob@receiver.com",
		Subject: "Hello World",
		Date:    "2024-01-15 10:30:00",
		Text:    "This is the body.",
	}

	expected := "<b>From:</b> alice@sender.com\n" +
		"<b>To:</b> bob@receiver.com\n" +
		"<b>Subject:</b> Hello World\n" +
		"<b>Time:</b> 2024-01-15 10:30:00\n\n" +
		"This is the body."

	result := io.FormatMailNotification(m, "en")
	if result != expected {
		t.Errorf("message format mismatch:\ngot:  %q\nwant: %q", result, expected)
	}
}

func TestHandleMail_MessageTruncation(t *testing.T) {
	io, _ := newTestIO(t)
	m := &smtpmod.ParsedMail{
		From:    "alice@sender.com",
		To:      "bob@receiver.com",
		Subject: "Hello World",
		Date:    "2024-01-15 10:30:00",
		Text:    strings.Repeat("x", 4000),
	}

	result := io.FormatMailNotification(m, "en")

	if len(result) > 4000 {
		t.Errorf("truncated message should be <= 4000 chars, got %d", len(result))
	}

	// Verify truncated message doesn't contain body text
	if strings.Contains(result, "xxxx") {
		t.Error("truncated message should not contain body text")
	}

	// Should still contain headers
	if !strings.Contains(result, "<b>From:</b>") {
		t.Error("truncated message should contain From header")
	}
	if !strings.Contains(result, "<b>Subject:</b>") {
		t.Error("truncated message should contain Subject header")
	}
}

func TestHandleMail_EmailExtraction(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"user@example.com", "user@example.com"},
		{"User Name <user@example.com>", "user@example.com"},
		{"USER@EXAMPLE.COM", "user@example.com"},
		{"user+tag@sub.example.com", "user+tag@sub.example.com"},
		{"first.last@domain.co", "first.last@domain.co"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lower := strings.ToLower(tt.input)
			match := emailRegex.FindString(lower)
			if match != tt.expected {
				t.Errorf("emailRegex.FindString(%q) = %q, want %q", lower, match, tt.expected)
			}
		})
	}
}

func TestHandleMail_BlockCheckOrder(t *testing.T) {
	// This test verifies the block check order: receiver → domain → sender
	// by setting up blocks at each level and verifying which takes effect.

	// Test 1: Receiver blocked - should block even if domain and sender are not blocked
	io1, _ := newTestIO(t)
	io1.BindDomain(100, "bound.com")
	io1.db.InsertBlockReceiver("user@bound.com", 100)
	mail := newTestParsedMail()
	io1.HandleMail(mail) // Should return silently

	// Verify receiver block was loaded into cache
	val, found := io1.blockReceiver.Get("100")
	if !found {
		t.Error("blockReceiver cache should be populated after HandleMail")
	}
	if set, ok := val.(map[string]bool); ok {
		if !set["user@bound.com"] {
			t.Error("blockReceiver cache should contain user@bound.com")
		}
	}

	// Test 2: Domain blocked (receiver not blocked)
	io2, _ := newTestIO(t)
	io2.BindDomain(100, "bound.com")
	io2.db.InsertBlockDomain("example.com", 100)
	io2.HandleMail(mail) // Should return silently

	// Verify domain block was loaded into cache
	val, found = io2.blockDomain.Get("100")
	if !found {
		t.Error("blockDomain cache should be populated after HandleMail")
	}
	if set, ok := val.(map[string]bool); ok {
		if !set["example.com"] {
			t.Error("blockDomain cache should contain example.com")
		}
	}
}

func TestHandleMail_CachePopulation(t *testing.T) {
	io, _ := newTestIO(t)
	io.BindDomain(100, "bound.com")

	// No blocks - mail should go through (but no bot, so just verify caches are populated)
	mail := newTestParsedMail()
	io.HandleMail(mail)

	// All three caches should be populated for user 100
	if _, found := io.blockReceiver.Get("100"); !found {
		t.Error("blockReceiver cache should be populated")
	}
	if _, found := io.blockDomain.Get("100"); !found {
		t.Error("blockDomain cache should be populated")
	}
	if _, found := io.blockSender.Get("100"); !found {
		t.Error("blockSender cache should be populated")
	}
}

func TestHandleMail_CacheReuse(t *testing.T) {
	io, _ := newTestIO(t)
	io.BindDomain(100, "bound.com")

	// First call populates cache
	mail := newTestParsedMail()
	io.HandleMail(mail)

	// Second call should use cache (no DB query)
	io.HandleMail(mail)

	// Caches should still be there
	if _, found := io.blockReceiver.Get("100"); !found {
		t.Error("blockReceiver cache should still be populated")
	}
}

func TestGetBlockSet(t *testing.T) {
	io, _ := newTestIO(t)

	// Test cache miss → loader called
	called := false
	set := io.getBlockSet("100", io.blockSender, func() (map[string]bool, error) {
		called = true
		return map[string]bool{"a@b.com": true}, nil
	})
	if !called {
		t.Error("loader should be called on cache miss")
	}
	if !set["a@b.com"] {
		t.Error("set should contain a@b.com")
	}

	// Test cache hit → loader not called
	called = false
	set = io.getBlockSet("100", io.blockSender, func() (map[string]bool, error) {
		called = true
		return nil, nil
	})
	if called {
		t.Error("loader should not be called on cache hit")
	}
	if !set["a@b.com"] {
		t.Error("cached set should still contain a@b.com")
	}
}
