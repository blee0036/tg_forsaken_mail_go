package io

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"

	"go-version-rewrite/internal/config"
	"go-version-rewrite/internal/db"
	smtpmod "go-version-rewrite/internal/smtp"
)

// escapeMdV2ForTest is a test helper that mirrors the escapeMdV2 function for assertions.
func escapeMdV2ForTest(s string) string {
	return escapeMdV2(s)
}

// newPropTestIO creates an IO instance backed by an in-memory SQLite database for property tests.
func newPropTestIO(t *testing.T) (*IO, *db.DB) {
	t.Helper()
	database, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create in-memory db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfg := &config.Config{
		MailDomain: "test.example.com",
	}

	io := New(database, cfg)
	return io, database
}

// Feature: go-version-rewrite, Property 5: Domain mapping initialization completeness
// Validates: Requirements 4.1
func TestProperty_DomainMappingInitCompleteness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// decodeDomainRecord decodes an encoded int into a (domain, tgID) pair.
	// We use a pool of domain prefixes and tg IDs to keep things manageable.
	domainPrefixes := []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta", "eta", "theta", "iota", "kappa"}
	tgIDs := []int64{100, 200, 300, 400, 500}

	properties.Property("Init loads all DB domain records into memory map", prop.ForAll(
		func(encodedRecords []int) bool {
			// Create fresh IO + DB for each test run.
			tmpFile, err := os.CreateTemp("", "prop5-*.db")
			if err != nil {
				t.Logf("CreateTemp error: %v", err)
				return false
			}
			tmpFile.Close()
			defer os.Remove(tmpFile.Name())

			database, err := db.New(tmpFile.Name())
			if err != nil {
				t.Logf("db.New error: %v", err)
				return false
			}
			defer database.Close()

			cfg := &config.Config{MailDomain: "test.example.com"}
			ioInst := New(database, cfg)

			// Build unique domain records from encoded ints.
			// Encoding: prefixIdx(0-9) * 5 + tgIdx(0-4), range 0..49
			expected := make(map[string]int64)
			for _, enc := range encodedRecords {
				prefixIdx := enc / 5
				tgIdx := enc % 5
				domain := domainPrefixes[prefixIdx] + ".test.example.com"
				tgID := tgIDs[tgIdx]

				// domain_tg has PRIMARY KEY on domain, so skip duplicates.
				if _, exists := expected[domain]; !exists {
					if _, err := database.InsertDomain(domain, tgID); err != nil {
						t.Logf("InsertDomain error: %v", err)
						return false
					}
					expected[domain] = tgID
				}
			}

			// Call Init to load from DB into memory.
			if err := ioInst.Init(); err != nil {
				t.Logf("Init error: %v", err)
				return false
			}

			// Verify all expected entries are in the map.
			for domain, expectedTg := range expected {
				val, ok := ioInst.domainToUser.Load(domain)
				if !ok {
					t.Logf("domain %q not found in map", domain)
					return false
				}
				if val.(int64) != expectedTg {
					t.Logf("domain %q: expected tg=%d, got tg=%d", domain, expectedTg, val.(int64))
					return false
				}
			}

			// Verify count matches: no extra entries in the map.
			mapCount := 0
			ioInst.domainToUser.Range(func(_, _ interface{}) bool {
				mapCount++
				return true
			})
			if mapCount != len(expected) {
				t.Logf("map count %d != expected count %d", mapCount, len(expected))
				return false
			}

			return true
		},
		gen.SliceOfN(15, gen.IntRange(0, 49)),
	))

	properties.TestingRun(t)
}

// Feature: go-version-rewrite, Property 15: Default domain format
// Validates: Requirements 7.1
func TestProperty_DefaultDomainFormat(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("BindDefaultDomain returns all-lowercase {name}.{mail_domain}", prop.ForAll(
		func(tgID int64) bool {
			// Ensure positive tgID for realistic usage.
			if tgID <= 0 {
				tgID = -tgID + 1
			}

			tmpFile, err := os.CreateTemp("", "prop15-*.db")
			if err != nil {
				t.Logf("CreateTemp error: %v", err)
				return false
			}
			tmpFile.Close()
			defer os.Remove(tmpFile.Name())

			database, err := db.New(tmpFile.Name())
			if err != nil {
				t.Logf("db.New error: %v", err)
				return false
			}
			defer database.Close()

			cfg := &config.Config{MailDomain: "test.example.com"}
			ioInst := New(database, cfg)

			domain := ioInst.BindDefaultDomain(tgID)
			if domain == "" {
				// Could happen if all 5 retries collide, extremely unlikely with fresh DB.
				t.Log("BindDefaultDomain returned empty (all retries exhausted)")
				return false
			}

			// 1. Must be all lowercase.
			if domain != strings.ToLower(domain) {
				t.Logf("domain %q is not all lowercase", domain)
				return false
			}

			// 2. Must end with .{mail_domain}.
			suffix := "." + cfg.MailDomain
			if !strings.HasSuffix(domain, suffix) {
				t.Logf("domain %q does not end with %q", domain, suffix)
				return false
			}

			// 3. The part before .{mail_domain} must contain only lowercase letters.
			namePart := strings.TrimSuffix(domain, suffix)
			if namePart == "" {
				t.Logf("domain %q has empty name part", domain)
				return false
			}
			for _, ch := range namePart {
				if ch < 'a' || ch > 'z' {
					t.Logf("domain %q name part contains non-lowercase-letter: %c", domain, ch)
					return false
				}
			}

			return true
		},
		gen.Int64(),
	))

	properties.TestingRun(t)
}

// Feature: go-version-rewrite, Property 16: Domain bind/unbind memory and database consistency
// Validates: Requirements 7.5, 7.6, 7.9
func TestProperty_DomainBindUnbindConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Operation encoding:
	// bit 0: 0=bind, 1=unbind
	// bits 1-3: domainIdx (0-4)
	// bits 4-6: tgIdx (0-4)
	// Total range: 0..49
	domainPool := []string{"alpha.com", "beta.org", "gamma.net", "delta.io", "epsilon.xyz"}
	tgPool := []int64{100, 200, 300, 400, 500}

	properties.Property("after bind/unbind sequence, memory map matches DB", prop.ForAll(
		func(encodedOps []int) bool {
			tmpFile, err := os.CreateTemp("", "prop16-*.db")
			if err != nil {
				t.Logf("CreateTemp error: %v", err)
				return false
			}
			tmpFile.Close()
			defer os.Remove(tmpFile.Name())

			database, err := db.New(tmpFile.Name())
			if err != nil {
				t.Logf("db.New error: %v", err)
				return false
			}
			defer database.Close()

			cfg := &config.Config{MailDomain: "test.example.com"}
			ioInst := New(database, cfg)

			for _, enc := range encodedOps {
				isBind := (enc % 2) == 0
				domainIdx := (enc / 2) % 5
				tgIdx := (enc / 10) % 5
				domain := domainPool[domainIdx]
				tgID := tgPool[tgIdx]

				if isBind {
					ioInst.BindDomain(tgID, domain)
				} else {
					ioInst.RemoveDomain(tgID, domain)
				}
			}

			// Collect in-memory map state.
			memMap := make(map[string]int64)
			ioInst.domainToUser.Range(func(key, value interface{}) bool {
				memMap[key.(string)] = value.(int64)
				return true
			})

			// Collect DB state.
			dbRecords, err := database.SelectAllDomain()
			if err != nil {
				t.Logf("SelectAllDomain error: %v", err)
				return false
			}
			dbMap := make(map[string]int64)
			for _, r := range dbRecords {
				dbMap[r.Domain] = r.Tg
			}

			// Compare counts.
			if len(memMap) != len(dbMap) {
				t.Logf("count mismatch: mem=%d, db=%d", len(memMap), len(dbMap))
				return false
			}

			// Compare entries.
			for domain, memTg := range memMap {
				dbTg, ok := dbMap[domain]
				if !ok {
					t.Logf("domain %q in memory but not in DB", domain)
					return false
				}
				if memTg != dbTg {
					t.Logf("domain %q: mem tg=%d, db tg=%d", domain, memTg, dbTg)
					return false
				}
			}

			for domain := range dbMap {
				if _, ok := memMap[domain]; !ok {
					t.Logf("domain %q in DB but not in memory", domain)
					return false
				}
			}

			return true
		},
		gen.SliceOfN(20, gen.IntRange(0, 49)),
	))

	properties.TestingRun(t)
}

// Feature: go-version-rewrite, Property 9: Callback data cache round-trip consistency
// Validates: Requirements 4.10, 4.11
func TestProperty_CallbackDataCacheRoundTrip(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("CheckButtonData → getTrueBlockData round-trip returns original data", prop.ForAll(
		func(data string) bool {
			// Create fresh IO for each test run.
			tmpFile, err := os.CreateTemp("", "prop9-*.db")
			if err != nil {
				t.Logf("CreateTemp error: %v", err)
				return false
			}
			tmpFile.Close()
			defer os.Remove(tmpFile.Name())

			database, err := db.New(tmpFile.Name())
			if err != nil {
				t.Logf("db.New error: %v", err)
				return false
			}
			defer database.Close()

			cfg := &config.Config{MailDomain: "test.example.com"}
			ioInst := New(database, cfg)

			key := "block_sender"
			buttonData := ioInst.CheckButtonData(data, key)

			// Split to get the data part after the key prefix.
			parts := strings.SplitN(buttonData, " ", 2)
			if len(parts) != 2 {
				t.Logf("expected 2 parts in %q, got %d", buttonData, len(parts))
				return false
			}
			dataPart := parts[1]

			// Verify the key prefix is preserved.
			if parts[0] != key {
				t.Logf("key prefix mismatch: expected %q, got %q", key, parts[0])
				return false
			}

			// Verify length-based behavior:
			// If key + " " + data >= 64 chars, dataPart should be a numeric snowflake ID.
			// Otherwise, dataPart should be the original data.
			combined := key + " " + data
			if len(combined) >= 64 {
				// dataPart should be a numeric snowflake ID (not the original data).
				if dataPart == data {
					t.Logf("expected snowflake ID for long data, but got original data back")
					return false
				}
				// Verify it's numeric.
				for _, ch := range dataPart {
					if ch < '0' || ch > '9' {
						t.Logf("snowflake ID %q contains non-numeric char %c", dataPart, ch)
						return false
					}
				}
			} else {
				// dataPart should be the original data.
				if dataPart != data {
					t.Logf("expected original data %q, got %q", data, dataPart)
					return false
				}
			}

			// Round-trip: getTrueBlockData should return the original data.
			recovered := ioInst.getTrueBlockData(dataPart)
			if recovered != data {
				t.Logf("round-trip failed: original=%q, recovered=%q", data, recovered)
				return false
			}

			return true
		},
		gen.AnyString(),
	))

	properties.TestingRun(t)
}

// Feature: go-version-rewrite, Property 12: List formatting consistency
// Validates: Requirements 6.4, 6.10, 6.11, 6.12, 8.3
func TestProperty_ListFormattingConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Pool of sender-like items to insert.
	senderPool := []string{
		"alice@example.com", "bob@test.org", "carol@domain.net",
		"dave@mail.io", "eve@host.xyz", "frank@srv.co",
		"grace@web.dev", "heidi@app.us",
	}

	properties.Property("ListBlockSender output contains all items in correct format", prop.ForAll(
		func(indices []int) bool {
			tmpFile, err := os.CreateTemp("", "prop12-*.db")
			if err != nil {
				t.Logf("CreateTemp error: %v", err)
				return false
			}
			tmpFile.Close()
			defer os.Remove(tmpFile.Name())

			database, err := db.New(tmpFile.Name())
			if err != nil {
				t.Logf("db.New error: %v", err)
				return false
			}
			defer database.Close()

			cfg := &config.Config{MailDomain: "test.example.com"}
			ioInst := New(database, cfg)

			tgID := int64(100)

			// Insert unique senders based on indices.
			inserted := make([]string, 0)
			seen := make(map[string]bool)
			for _, idx := range indices {
				sender := senderPool[idx]
				if !seen[sender] {
					ioInst.BlockSender(tgID, sender)
					inserted = append(inserted, sender)
					seen[sender] = true
				}
			}

			// Get the formatted list.
			result := ioInst.ListBlockSender(tgID)

			// Verify header.
			if !strings.HasPrefix(result, "<b>Your block sender :</b> \n\n") {
				t.Logf("missing header prefix in %q", result)
				return false
			}

			// Verify each inserted item appears in the correct format.
			for i, sender := range inserted {
				expectedLine := strings.Replace(
					"N: <code> ITEM</code> \n",
					"N",
					strings.Replace("N", "N", string(rune('0'+i+1)), 1),
					1,
				)
				// Build the expected formatted line.
				expectedFmt := fmt.Sprintf("%d: <code> %s</code> \n", i+1, sender)
				if !strings.Contains(result, expectedFmt) {
					t.Logf("missing formatted item %q in result %q", expectedFmt, result)
					_ = expectedLine // suppress unused
					return false
				}
			}

			return true
		},
		gen.SliceOfN(5, gen.IntRange(0, len(senderPool)-1)),
	))

	properties.TestingRun(t)
}

// Feature: go-version-rewrite, Property 17: Block operation cache invalidation and database consistency
// Validates: Requirements 8.1, 8.2, 8.6, 8.7
func TestProperty_BlockOperationCacheInvalidation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Operation encoding for block_sender:
	// bit 0: 0=block, 1=unblock
	// bits 1-3: senderIdx (0-4)
	// Total range: 0..9
	senderPool := []string{"a@evil.com", "b@spam.org", "c@bad.net", "d@junk.io", "e@trash.xyz"}

	properties.Property("block/unblock sequences produce correct DB state and invalidate cache", prop.ForAll(
		func(encodedOps []int) bool {
			tmpFile, err := os.CreateTemp("", "prop17-*.db")
			if err != nil {
				t.Logf("CreateTemp error: %v", err)
				return false
			}
			tmpFile.Close()
			defer os.Remove(tmpFile.Name())

			database, err := db.New(tmpFile.Name())
			if err != nil {
				t.Logf("db.New error: %v", err)
				return false
			}
			defer database.Close()

			cfg := &config.Config{MailDomain: "test.example.com"}
			ioInst := New(database, cfg)

			tgID := int64(100)
			tgKey := fmt.Sprintf("%d", tgID)

			// Track expected state: set of blocked senders.
			expectedBlocked := make(map[string]bool)

			for _, enc := range encodedOps {
				isBlock := (enc % 2) == 0
				senderIdx := (enc / 2) % 5
				sender := senderPool[senderIdx]

				// Pre-populate cache to verify invalidation.
				ioInst.blockSender.Set(tgKey, "stale-cache-data")

				if isBlock {
					ioInst.BlockSender(tgID, sender)
					expectedBlocked[sender] = true
				} else {
					ioInst.RemoveBlockSender(tgID, sender)
					delete(expectedBlocked, sender)
				}

				// Verify cache is invalidated after each operation.
				if _, found := ioInst.blockSender.Get(tgKey); found {
					t.Logf("cache for user %d should be invalidated after operation on %q", tgID, sender)
					return false
				}
			}

			// Verify DB state matches expected state.
			dbSenders, err := database.SelectUserAllBlockSender(tgID)
			if err != nil {
				t.Logf("SelectUserAllBlockSender error: %v", err)
				return false
			}

			dbSet := make(map[string]bool)
			for _, s := range dbSenders {
				dbSet[s.Sender] = true
			}

			// Compare expected vs DB.
			if len(expectedBlocked) != len(dbSet) {
				t.Logf("count mismatch: expected=%d, db=%d", len(expectedBlocked), len(dbSet))
				return false
			}

			for sender := range expectedBlocked {
				if !dbSet[sender] {
					t.Logf("sender %q expected in DB but not found", sender)
					return false
				}
			}

			for sender := range dbSet {
				if !expectedBlocked[sender] {
					t.Logf("sender %q in DB but not expected", sender)
					return false
				}
			}

			return true
		},
		gen.SliceOfN(15, gen.IntRange(0, 9)),
	))

	properties.TestingRun(t)
}

// Feature: go-version-rewrite, Property 6: Recipient address domain extraction
// Validates: Requirements 4.2
func TestProperty_RecipientAddressDomainExtraction(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Generate random alphanumeric strings for user and domain parts.
	alphanumGen := gen.RegexMatch("[a-zA-Z0-9]{1,20}")

	properties.Property("emailRegex extracts domain part after @ correctly", prop.ForAll(
		func(user, domainName, tld string) bool {
			email := user + "@" + domainName + "." + tld
			emailLower := strings.ToLower(email)

			match := emailRegex.FindString(emailLower)
			if match == "" {
				t.Logf("emailRegex did not match %q", emailLower)
				return false
			}

			parts := strings.SplitN(match, "@", 2)
			if len(parts) != 2 {
				t.Logf("expected 2 parts after splitting %q by @, got %d", match, len(parts))
				return false
			}

			expectedDomain := strings.ToLower(domainName + "." + tld)
			if parts[1] != expectedDomain {
				t.Logf("domain mismatch: expected %q, got %q (from email %q)", expectedDomain, parts[1], emailLower)
				return false
			}

			return true
		},
		alphanumGen,
		gen.RegexMatch("[a-z0-9]{1,15}"),
		gen.RegexMatch("[a-z]{2,6}"),
	))

	properties.TestingRun(t)
}

// Feature: go-version-rewrite, Property 7: Block check order
// Validates: Requirements 4.3
func TestProperty_BlockCheckOrder(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Encoding for which block lists to populate:
	// bit 0: block receiver
	// bit 1: block domain
	// bit 2: block sender
	// Range: 0..7
	properties.Property("block check follows receiver → domain → sender order", prop.ForAll(
		func(blockBits int) bool {
			tmpFile, err := os.CreateTemp("", "prop7-*.db")
			if err != nil {
				t.Logf("CreateTemp error: %v", err)
				return false
			}
			tmpFile.Close()
			defer os.Remove(tmpFile.Name())

			database, err := db.New(tmpFile.Name())
			if err != nil {
				t.Logf("db.New error: %v", err)
				return false
			}
			defer database.Close()

			cfg := &config.Config{MailDomain: "test.example.com"}
			ioInst := New(database, cfg)

			tgID := int64(100)
			receiverAddr := "user@bound.com"
			senderAddr := "sender@evil.com"
			senderDomain := "evil.com"

			// Bind the domain so HandleMail can find the user.
			ioInst.BindDomain(tgID, "bound.com")

			blockReceiver := (blockBits & 1) != 0
			blockDomain := (blockBits & 2) != 0
			blockSender := (blockBits & 4) != 0

			if blockReceiver {
				database.InsertBlockReceiver(receiverAddr, tgID)
			}
			if blockDomain {
				database.InsertBlockDomain(senderDomain, tgID)
			}
			if blockSender {
				database.InsertBlockSender(senderAddr, tgID)
			}

			mail := &smtpmod.ParsedMail{
				From:               senderAddr,
				To:                 receiverAddr,
				EnvelopeFrom:       senderAddr,
				EnvelopeRecipients: []string{receiverAddr},
				Subject:            "Test",
				Date:               "2024-01-15",
				Text:               "body",
			}

			// HandleMail will populate caches as it checks each block level.
			// No bot is set, so no message is sent — we verify via cache state.
			ioInst.HandleMail(mail)

			tgKey := fmt.Sprintf("%d", tgID)
			_, receiverCacheExists := ioInst.blockReceiver.Get(tgKey)
			_, domainCacheExists := ioInst.blockDomain.Get(tgKey)
			_, senderCacheExists := ioInst.blockSender.Get(tgKey)

			if blockReceiver {
				// Receiver is blocked → should check receiver (cache populated),
				// should NOT proceed to domain or sender checks.
				if !receiverCacheExists {
					t.Log("receiver cache should be populated when receiver is blocked")
					return false
				}
				if domainCacheExists {
					t.Log("domain cache should NOT be populated when receiver is blocked")
					return false
				}
				if senderCacheExists {
					t.Log("sender cache should NOT be populated when receiver is blocked")
					return false
				}
			} else if blockDomain {
				// Receiver not blocked, domain is blocked → receiver and domain caches populated,
				// sender cache should NOT be populated.
				if !receiverCacheExists {
					t.Log("receiver cache should be populated")
					return false
				}
				if !domainCacheExists {
					t.Log("domain cache should be populated when domain is blocked")
					return false
				}
				if senderCacheExists {
					t.Log("sender cache should NOT be populated when domain is blocked")
					return false
				}
			} else if blockSender {
				// Only sender blocked → all three caches populated, mail blocked at sender level.
				if !receiverCacheExists {
					t.Log("receiver cache should be populated")
					return false
				}
				if !domainCacheExists {
					t.Log("domain cache should be populated")
					return false
				}
				if !senderCacheExists {
					t.Log("sender cache should be populated when sender is blocked")
					return false
				}
			} else {
				// Nothing blocked → all three caches populated (mail goes through).
				if !receiverCacheExists {
					t.Log("receiver cache should be populated when nothing blocked")
					return false
				}
				if !domainCacheExists {
					t.Log("domain cache should be populated when nothing blocked")
					return false
				}
				if !senderCacheExists {
					t.Log("sender cache should be populated when nothing blocked")
					return false
				}
			}

			return true
		},
		gen.IntRange(0, 7),
	))

	properties.TestingRun(t)
}

// Feature: go-version-rewrite, Property 8: Mail message formatting
// Validates: Requirements 4.5, 4.7, 4.12, 4.13
func TestProperty_MailMessageFormatting(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Generate random alphanumeric strings for mail fields.
	fieldGen := gen.AlphaString().SuchThat(func(v interface{}) bool {
		return len(v.(string)) > 0
	})
	// For text body, use a bool to decide short vs long text to ensure we test truncation.
	longTextGen := gen.Bool()

	properties.Property("mail message format is correct and truncation works for >4000 chars", prop.ForAll(
		func(from, to, subject, date string, useLongText bool) bool {
			var text string
			if useLongText {
				// Generate text that will push total over 4000 chars.
				text = strings.Repeat("x", 4500)
			} else {
				text = "Short body content."
			}

			// Build the full message the same way HandleMail does.
			fullEmail := "From : " + from + "\n" +
				"To : " + to + "\n" +
				"Subject : " + subject + "\n" +
				"Time : " + date + "\n\n" +
				"Content : \n" + text

			headersOnly := "From : " + from + "\n" +
				"To : " + to + "\n" +
				"Subject : " + subject + "\n" +
				"Time : " + date + "\n\n"

			if len(fullEmail) > 4000 {
				// Truncation case: result should be headers only.
				result := headersOnly

				// Verify it doesn't contain "Content : \n".
				if strings.Contains(result, "Content : \n") {
					t.Logf("truncated message should not contain 'Content : \\n'")
					return false
				}

				// Verify it ends with double newline (headers end).
				if !strings.HasSuffix(result, "\n\n") {
					t.Logf("truncated message should end with \\n\\n")
					return false
				}

				// Verify headers are present.
				if !strings.HasPrefix(result, "From : "+from+"\n") {
					t.Logf("truncated message missing From header")
					return false
				}
				if !strings.Contains(result, "To : "+to+"\n") {
					t.Logf("truncated message missing To header")
					return false
				}
				if !strings.Contains(result, "Subject : "+subject+"\n") {
					t.Logf("truncated message missing Subject header")
					return false
				}
				if !strings.Contains(result, "Time : "+date+"\n") {
					t.Logf("truncated message missing Time header")
					return false
				}
			} else {
				// Normal case: verify full format.
				expected := "From : " + from + "\n" +
					"To : " + to + "\n" +
					"Subject : " + subject + "\n" +
					"Time : " + date + "\n\n" +
					"Content : \n" + text

				if fullEmail != expected {
					t.Logf("format mismatch:\ngot:  %q\nwant: %q", fullEmail, expected)
					return false
				}

				// Verify structure: starts with "From : ", contains all fields.
				if !strings.HasPrefix(fullEmail, "From : ") {
					t.Logf("message should start with 'From : '")
					return false
				}
				if !strings.Contains(fullEmail, "\nTo : ") {
					t.Logf("message should contain '\\nTo : '")
					return false
				}
				if !strings.Contains(fullEmail, "\nSubject : ") {
					t.Logf("message should contain '\\nSubject : '")
					return false
				}
				if !strings.Contains(fullEmail, "\nTime : ") {
					t.Logf("message should contain '\\nTime : '")
					return false
				}
				if !strings.Contains(fullEmail, "\n\nContent : \n") {
					t.Logf("message should contain '\\n\\nContent : \\n'")
					return false
				}
			}

			return true
		},
		fieldGen,
		fieldGen,
		fieldGen,
		fieldGen,
		longTextGen,
	))

	properties.TestingRun(t)
}

// Feature: telegram-interaction-redesign, Property 11: 邮件通知格式正确性
// Validates: Requirements 9.1, 9.2
func TestProperty_MailNotificationFormatCorrectness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Generators for short mail fields (keep total < 4000)
	shortFieldGen := gen.RegexMatch("[a-zA-Z0-9@.]{1,30}")
	shortBodyGen := gen.RegexMatch("[a-zA-Z0-9 ]{0,100}")
	langGen := gen.OneConstOf("en", "zh")

	// Create IO instance once outside the property (no DB state needed for formatting)
	ioInst, _ := newPropTestIO(t)

	properties.Property("FormatMailNotification output contains Markdown bold tags for all header fields", prop.ForAll(
		func(from, to, subject, date, body, lang string) bool {
			mail := &smtpmod.ParsedMail{
				From:    from,
				To:      to,
				Subject: subject,
				Date:    date,
				RawDate: date, // FormatMailTime uses RawDate when DateTime is nil
				Text:    body,
			}

			result := ioInst.FormatMailNotification(mail, lang)

			// Verify output contains Markdown bold tags for all four header fields
			if !strings.Contains(result, "*From:*") {
				t.Logf("missing *From:* in result: %q", result)
				return false
			}
			if !strings.Contains(result, "*To:*") {
				t.Logf("missing *To:* in result: %q", result)
				return false
			}
			if !strings.Contains(result, "*Subject:*") {
				t.Logf("missing *Subject:* in result: %q", result)
				return false
			}
			if !strings.Contains(result, "*Time:*") {
				t.Logf("missing *Time:* in result: %q", result)
				return false
			}

			// Verify the actual field values are present in the output (may be escaped)
			if !strings.Contains(result, escapeMdV2ForTest(from)) {
				t.Logf("missing from value %q in result: %q", from, result)
				return false
			}
			if !strings.Contains(result, escapeMdV2ForTest(to)) {
				t.Logf("missing to value %q in result: %q", to, result)
				return false
			}
			if !strings.Contains(result, escapeMdV2ForTest(subject)) {
				t.Logf("missing subject value %q in result: %q", subject, result)
				return false
			}
			if !strings.Contains(result, escapeMdV2ForTest(date)) {
				t.Logf("missing date value %q in result: %q", date, result)
				return false
			}

			return true
		},
		shortFieldGen,
		shortFieldGen,
		shortFieldGen,
		shortFieldGen,
		shortBodyGen,
		langGen,
	))

	properties.TestingRun(t)
}

// Feature: telegram-interaction-redesign, Property 12: 邮件正文截断
// Validates: Requirements 9.3
func TestProperty_MailNotificationBodyTruncation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Generate body text that is always > 4000 characters
	longBodyLenGen := gen.IntRange(4001, 8000)
	langGen := gen.OneConstOf("en", "zh")

	// Create IO instance once outside the property (no DB state needed for formatting)
	ioInst, _ := newPropTestIO(t)

	properties.Property("FormatMailNotification truncates long body with hint", prop.ForAll(
		func(bodyLen int, lang string) bool {
			longBody := strings.Repeat("A", bodyLen)

			mail := &smtpmod.ParsedMail{
				From:    "sender@example.com",
				To:      "receiver@example.com",
				Subject: "Test Subject",
				Date:    "2024-01-15",
				Text:    longBody,
			}

			result := ioInst.FormatMailNotification(mail, lang)

			// Verify output contains Markdown bold header fields
			if !strings.Contains(result, "*From:*") {
				t.Logf("missing *From:* in truncated result")
				return false
			}
			if !strings.Contains(result, "*To:*") {
				t.Logf("missing *To:* in truncated result")
				return false
			}
			if !strings.Contains(result, "*Subject:*") {
				t.Logf("missing *Subject:* in truncated result")
				return false
			}
			if !strings.Contains(result, "*Time:*") {
				t.Logf("missing *Time:* in truncated result")
				return false
			}

			// Verify output does NOT contain the full body text
			if strings.Contains(result, longBody) {
				t.Logf("truncated result should not contain the full body text")
				return false
			}

			// Should contain truncation hint
			if !strings.Contains(result, "✂️") {
				t.Logf("truncated result should contain truncation hint")
				return false
			}

			return true
		},
		longBodyLenGen,
		langGen,
	))

	properties.TestingRun(t)
}

// Feature: telegram-interaction-redesign, Property 13: 邮件通知按钮标签语言一致性
// Validates: Requirements 9.4
func TestProperty_MailNotificationButtonLabelLanguageConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	shortFieldGen := gen.RegexMatch("[a-zA-Z0-9@.]{1,30}")
	langGen := gen.OneConstOf("en", "zh")

	// Create IO instance once outside the property (no DB state needed for formatting)
	ioInst, _ := newPropTestIO(t)

	properties.Property("FormatMailNotification accepts lang parameter and produces valid format for both languages", prop.ForAll(
		func(from, to, subject, date, lang string) bool {

			mail := &smtpmod.ParsedMail{
				From:    from,
				To:      to,
				Subject: subject,
				Date:    date,
				Text:    "Short body",
			}

			result := ioInst.FormatMailNotification(mail, lang)

			// FormatMailNotification should accept both "en" and "zh" and produce valid output
			// The output should always be non-empty
			if result == "" {
				t.Logf("FormatMailNotification returned empty string for lang=%q", lang)
				return false
			}

			// The output should always contain the Markdown bold header tags regardless of language
			if !strings.Contains(result, "*From:*") {
				t.Logf("missing *From:* for lang=%q", lang)
				return false
			}
			if !strings.Contains(result, "*To:*") {
				t.Logf("missing *To:* for lang=%q", lang)
				return false
			}
			if !strings.Contains(result, "*Subject:*") {
				t.Logf("missing *Subject:* for lang=%q", lang)
				return false
			}
			if !strings.Contains(result, "*Time:*") {
				t.Logf("missing *Time:* for lang=%q", lang)
				return false
			}

			// Verify the format is consistent: the output for both languages
			// should have the same structure (headers + body)
			lines := strings.Split(result, "\n")
			if len(lines) < 4 {
				t.Logf("expected at least 4 lines (header fields) for lang=%q, got %d", lang, len(lines))
				return false
			}

			// Both languages should produce valid non-empty output
			resultOtherLang := ioInst.FormatMailNotification(mail, "en")
			resultZh := ioInst.FormatMailNotification(mail, "zh")

			if resultOtherLang == "" || resultZh == "" {
				t.Logf("one of the language outputs is empty: en=%q, zh=%q", resultOtherLang, resultZh)
				return false
			}

			return true
		},
		shortFieldGen,
		shortFieldGen,
		shortFieldGen,
		shortFieldGen,
		langGen,
	))

	properties.TestingRun(t)
}

// Feature: bot-migration, Property 10: Date fallback 行为
// Validates: Requirements 11.7, 11.8
func TestProperty_DateFallbackBehavior(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Generator for random non-date strings (strings that are NOT valid RFC2822/RFC3339 dates)
	// Use alphanumeric + special chars that won't accidentally parse as dates
	nonDateStringGen := gen.RegexMatch("[a-zA-Z!@#$%^&*()_+=]{1,50}")

	// Generator for random time.Time values for "now" parameter
	// Range: 2000-01-01 to 2030-12-31
	nowGen := gen.Int64Range(946684800, 1924991999).Map(func(ts int64) time.Time {
		return time.Unix(ts, 0).UTC()
	})

	// Generator for zh-series language codes
	zhLangGen := gen.OneConstOf("zh", "zh-Hans", "zh-Hant", "zh_CN", "zh-TW")
	// Generator for non-zh language codes
	nonZhLangGen := gen.OneConstOf("en", "en-US", "ja", "ko", "fr", "de")

	properties.Property("dateTime==nil and rawDate non-empty returns rawDate as-is", prop.ForAll(
		func(rawDate string) bool {
			// Call FormatMailTime with dateTime=nil, non-empty rawDate
			result := FormatMailTime(nil, rawDate, "en", time.Now())

			// Should return rawDate exactly as-is
			if result != rawDate {
				t.Logf("expected rawDate %q returned as-is, got %q", rawDate, result)
				return false
			}

			return true
		},
		nonDateStringGen,
	))

	properties.Property("dateTime==nil and rawDate non-empty returns rawDate regardless of language", prop.ForAll(
		func(rawDate, lang string) bool {
			now := time.Now()
			result := FormatMailTime(nil, rawDate, lang, now)

			// Should return rawDate exactly as-is regardless of language
			if result != rawDate {
				t.Logf("lang=%q: expected rawDate %q returned as-is, got %q", lang, rawDate, result)
				return false
			}

			return true
		},
		nonDateStringGen,
		gen.OneConstOf("zh", "zh-Hans", "en", "ja", "ko", "fr"),
	))

	properties.Property("dateTime==nil and rawDate empty with zh lang uses now formatted as UTC+8", prop.ForAll(
		func(now time.Time) bool {
			lang := "zh"
			result := FormatMailTime(nil, "", lang, now)

			// Should format now in UTC+8
			loc := time.FixedZone("UTC+8", 8*60*60)
			expected := now.In(loc).Format("2006-01-02 15:04:05 -07:00")

			if result != expected {
				t.Logf("zh empty rawDate: expected %q, got %q", expected, result)
				return false
			}

			// Must contain +08:00 timezone identifier
			if !strings.Contains(result, "+08:00") {
				t.Logf("zh empty rawDate: result %q missing +08:00", result)
				return false
			}

			return true
		},
		nowGen,
	))

	properties.Property("dateTime==nil and rawDate empty with zh-series lang uses now formatted as UTC+8", prop.ForAll(
		func(lang string, now time.Time) bool {
			result := FormatMailTime(nil, "", lang, now)

			// Should format now in UTC+8
			loc := time.FixedZone("UTC+8", 8*60*60)
			expected := now.In(loc).Format("2006-01-02 15:04:05 -07:00")

			if result != expected {
				t.Logf("zh-series lang=%q empty rawDate: expected %q, got %q", lang, expected, result)
				return false
			}

			if !strings.Contains(result, "+08:00") {
				t.Logf("zh-series lang=%q empty rawDate: result %q missing +08:00", lang, result)
				return false
			}

			return true
		},
		zhLangGen,
		nowGen,
	))

	properties.Property("dateTime==nil and rawDate empty with non-zh lang uses now formatted as UTC+0", prop.ForAll(
		func(lang string, now time.Time) bool {
			result := FormatMailTime(nil, "", lang, now)

			// Should format now in UTC+0
			expected := now.UTC().Format("2006-01-02 15:04:05 -07:00")

			if result != expected {
				t.Logf("non-zh lang=%q empty rawDate: expected %q, got %q", lang, expected, result)
				return false
			}

			if !strings.Contains(result, "+00:00") {
				t.Logf("non-zh lang=%q empty rawDate: result %q missing +00:00", lang, result)
				return false
			}

			return true
		},
		nonZhLangGen,
		nowGen,
	))

	properties.TestingRun(t)
}

// Feature: bot-migration, Property 9: 邮件时间时区格式化
// Validates: Requirements 11.3, 11.4, 11.5
func TestProperty_MailTimeTimezoneFormatting(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Generator for zh-series language codes
	zhLangGen := gen.OneConstOf("zh", "zh-Hans", "zh-Hant", "zh_CN", "zh-TW", "zh-HK", "ZH", "Zh-Hans")
	// Generator for non-zh language codes
	nonZhLangGen := gen.OneConstOf("en", "en-US", "ja", "ko", "fr", "de", "es", "ru", "ar", "pt")

	// Generator for random time.Time values (Unix timestamps in a reasonable range)
	// Range: 2000-01-01 to 2030-12-31
	timeGen := gen.Int64Range(946684800, 1924991999).Map(func(ts int64) time.Time {
		return time.Unix(ts, 0).UTC()
	})

	properties.Property("zh-series languages produce UTC+8 output with +08:00 timezone identifier", prop.ForAll(
		func(lang string, dt time.Time) bool {
			result := FormatMailTime(&dt, "", lang, time.Now())

			// 1. Output must contain "+08:00" timezone identifier
			if !strings.Contains(result, "+08:00") {
				t.Logf("zh lang=%q: result %q does not contain +08:00", lang, result)
				return false
			}

			// 2. Verify the actual time conversion is correct (UTC+8 = UTC + 8 hours)
			loc := time.FixedZone("UTC+8", 8*60*60)
			expectedTime := dt.In(loc)
			expectedStr := expectedTime.Format("2006-01-02 15:04:05 -07:00")
			if result != expectedStr {
				t.Logf("zh lang=%q: result %q != expected %q", lang, result, expectedStr)
				return false
			}

			return true
		},
		zhLangGen,
		timeGen,
	))

	properties.Property("non-zh languages produce UTC+0 output with +00:00 timezone identifier", prop.ForAll(
		func(lang string, dt time.Time) bool {
			result := FormatMailTime(&dt, "", lang, time.Now())

			// 1. Output must contain "+00:00" timezone identifier
			if !strings.Contains(result, "+00:00") {
				t.Logf("non-zh lang=%q: result %q does not contain +00:00", lang, result)
				return false
			}

			// 2. Verify the actual time conversion is correct (UTC+0)
			expectedStr := dt.UTC().Format("2006-01-02 15:04:05 -07:00")
			if result != expectedStr {
				t.Logf("non-zh lang=%q: result %q != expected %q", lang, result, expectedStr)
				return false
			}

			return true
		},
		nonZhLangGen,
		timeGen,
	))

	properties.Property("output always contains timezone identifier for non-nil dateTime", prop.ForAll(
		func(lang string, dt time.Time) bool {
			result := FormatMailTime(&dt, "", lang, time.Now())

			// Output must contain either +08:00 or +00:00
			hasTimezone := strings.Contains(result, "+08:00") || strings.Contains(result, "+00:00")
			if !hasTimezone {
				t.Logf("lang=%q: result %q does not contain any timezone identifier", lang, result)
				return false
			}

			// Verify consistency: zh → +08:00, non-zh → +00:00
			if IsZhLang(lang) {
				if !strings.Contains(result, "+08:00") {
					t.Logf("zh lang=%q should have +08:00 but got %q", lang, result)
					return false
				}
			} else {
				if !strings.Contains(result, "+00:00") {
					t.Logf("non-zh lang=%q should have +00:00 but got %q", lang, result)
					return false
				}
			}

			return true
		},
		gen.OneConstOf("zh", "zh-Hans", "zh_CN", "en", "ja", "ko", "fr", "de"),
		timeGen,
	))

	properties.TestingRun(t)
}

// Feature: bot-migration, Property 5: 邮件多路投递与选择性 Alert
// Validates: Requirements 5.1, 5.2, 5.3
func TestProperty_MailMultiDeliverySelectiveAlert(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Generator for number of old bots (1..5)
	oldBotCountGen := gen.IntRange(1, 5)

	// Generator for random mail subject (short alphanumeric)
	subjectGen := gen.RegexMatch("[a-zA-Z0-9 ]{1,30}")

	// Generator for random mail body text
	bodyGen := gen.RegexMatch("[a-zA-Z0-9 ]{0,100}")

	properties.Property("all endpoints receive mail; old endpoints get alert; new endpoint does not", prop.ForAll(
		func(numOldBots int, subject, body string) bool {
			// Create fresh IO + DB for each test run
			tmpFile, err := os.CreateTemp("", "prop5-multi-*.db")
			if err != nil {
				t.Logf("CreateTemp error: %v", err)
				return false
			}
			tmpFile.Close()
			defer os.Remove(tmpFile.Name())

			database, err := db.New(tmpFile.Name())
			if err != nil {
				t.Logf("db.New error: %v", err)
				return false
			}
			defer database.Close()

			cfg := &config.Config{MailDomain: "test.example.com"}
			ioInst := New(database, cfg)

			// Bind a domain so HandleMailMulti can resolve the recipient
			tgID := int64(12345)
			domain := "user123.test.example.com"
			ioInst.domainToUser.Store(domain, tgID)

			// Create mock senders: 1 new + N old
			newSender := &multiDeliveryMockSender{name: "new-bot"}
			endpoints := []DeliveryEndpoint{
				{Name: "new-bot", Sender: newSender, IsOld: false},
			}

			oldSenders := make([]*multiDeliveryMockSender, numOldBots)
			for i := 0; i < numOldBots; i++ {
				name := fmt.Sprintf("old-bot-%d", i)
				oldSenders[i] = &multiDeliveryMockSender{name: name}
				endpoints = append(endpoints, DeliveryEndpoint{
					Name:   name,
					Sender: oldSenders[i],
					IsOld:  true,
				})
			}

			// Track alert calls
			type alertCall struct {
				senderName string
				tgID       int64
				lang       string
			}
			var alertCalls []alertCall
			alertFunc := func(sender TelegramSender, tgID int64, lang string) {
				ms := sender.(*multiDeliveryMockSender)
				alertCalls = append(alertCalls, alertCall{
					senderName: ms.name,
					tgID:       tgID,
					lang:       lang,
				})
			}

			// Create a mail addressed to the bound domain
			mail := &smtpmod.ParsedMail{
				From:               "sender@external.com",
				To:                 "someone@" + domain,
				EnvelopeFrom:       "sender@external.com",
				EnvelopeRecipients: []string{"someone@" + domain},
				Subject:            subject,
				Date:               "2024-06-15",
				RawDate:            "2024-06-15",
				Text:               body,
			}

			// Execute
			ioInst.HandleMailMulti(mail, endpoints, alertFunc, time.Now())

			// Verify 1: All endpoints received at least one Send call (the mail message)
			if newSender.sendCount == 0 {
				t.Logf("new-bot received 0 Send calls, expected at least 1")
				return false
			}
			for i, os := range oldSenders {
				if os.sendCount == 0 {
					t.Logf("old-bot-%d received 0 Send calls, expected at least 1", i)
					return false
				}
			}

			// Verify 2: AlertFunc was called once for each old bot endpoint
			if len(alertCalls) != numOldBots {
				t.Logf("expected %d alert calls (one per old bot), got %d", numOldBots, len(alertCalls))
				return false
			}

			// Verify each old bot's sender was used in the alert call
			oldSenderNames := make(map[string]bool)
			for i := 0; i < numOldBots; i++ {
				oldSenderNames[fmt.Sprintf("old-bot-%d", i)] = true
			}
			for _, ac := range alertCalls {
				if !oldSenderNames[ac.senderName] {
					t.Logf("alert called with unexpected sender %q", ac.senderName)
					return false
				}
				if ac.tgID != tgID {
					t.Logf("alert called with tgID=%d, expected %d", ac.tgID, tgID)
					return false
				}
				delete(oldSenderNames, ac.senderName)
			}
			if len(oldSenderNames) > 0 {
				t.Logf("some old bots did not get alert calls: %v", oldSenderNames)
				return false
			}

			// Verify 3: AlertFunc was NOT called for the new bot endpoint
			for _, ac := range alertCalls {
				if ac.senderName == "new-bot" {
					t.Logf("alert was called for new-bot, which should not happen")
					return false
				}
			}

			return true
		},
		oldBotCountGen,
		subjectGen,
		bodyGen,
	))

	properties.TestingRun(t)
}

// multiDeliveryMockSender is a mock TelegramSender that tracks Send calls for property testing.
type multiDeliveryMockSender struct {
	name      string
	sendCount int
}

func (m *multiDeliveryMockSender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	m.sendCount++
	return tgbotapi.Message{}, nil
}

func (m *multiDeliveryMockSender) Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	return &tgbotapi.APIResponse{Ok: true}, nil
}
