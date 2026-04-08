package db

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Task 2.3: 对比验证：数据库模块
// Validates: Requirements 2.5, 2.6
// Verifies that Go db.New() creates the same schema as Node.js db.js,
// and that identical operation sequences produce identical results.

// normalizeSQL strips extra whitespace and newlines from SQL for comparison.
func normalizeSQL(s string) string {
	// Replace all whitespace sequences (spaces, tabs, newlines) with a single space.
	re := regexp.MustCompile(`\s+`)
	s = re.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	return s
}

// --- Schema Comparison Tests ---

func TestComparison_SchemaTablesMatchNodeVersion(t *testing.T) {
	d := newTestDB(t)

	// Query all tables and indexes from sqlite_master, ordered by type and name.
	rows, err := d.db.Query(`SELECT type, name, sql FROM sqlite_master WHERE type IN ('table', 'index') ORDER BY type, name`)
	if err != nil {
		t.Fatalf("failed to query sqlite_master: %v", err)
	}
	defer rows.Close()

	type schemaEntry struct {
		Type string
		Name string
		SQL  string
	}
	var entries []schemaEntry
	for rows.Next() {
		var e schemaEntry
		var sqlStr *string
		if err := rows.Scan(&e.Type, &e.Name, &sqlStr); err != nil {
			t.Fatalf("failed to scan sqlite_master row: %v", err)
		}
		if sqlStr != nil {
			e.SQL = *sqlStr
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows iteration error: %v", err)
	}

	// Expected tables from Node.js db.js
	expectedTables := map[string]string{
		"block_domain":   "CREATE TABLE block_domain ( domain TEXT, tg INTEGER )",
		"block_receiver": "CREATE TABLE block_receiver ( receiver TEXT, tg INTEGER )",
		"block_sender":   "CREATE TABLE block_sender ( sender TEXT, tg INTEGER )",
		"domain_tg":      "CREATE TABLE domain_tg ( domain TEXT PRIMARY KEY, tg INTEGER )",
	}

	// Expected indexes from Node.js db.js
	expectedIndexes := map[string]string{
		"block_domain_idx_tg":   "CREATE INDEX block_domain_idx_tg ON block_domain ( tg )",
		"block_receiver_idx_tg": "CREATE INDEX block_receiver_idx_tg ON block_receiver ( tg )",
		"block_sender_idx_tg":   "CREATE INDEX block_sender_idx_tg ON block_sender ( tg )",
	}

	foundTables := make(map[string]string)
	foundIndexes := make(map[string]string)

	for _, e := range entries {
		switch e.Type {
		case "table":
			foundTables[e.Name] = normalizeSQL(e.SQL)
		case "index":
			// Skip auto-created indexes (sqlite_autoindex_*)
			if strings.HasPrefix(e.Name, "sqlite_autoindex_") {
				continue
			}
			foundIndexes[e.Name] = normalizeSQL(e.SQL)
		}
	}

	// Verify all expected tables exist with correct structure
	for name, expectedSQL := range expectedTables {
		gotSQL, ok := foundTables[name]
		if !ok {
			t.Errorf("expected table %q not found in sqlite_master", name)
			continue
		}
		if normalizeSQL(expectedSQL) != gotSQL {
			t.Errorf("table %q SQL mismatch:\n  expected: %s\n  got:      %s", name, normalizeSQL(expectedSQL), gotSQL)
		}
	}

	// Verify no extra tables
	for name := range foundTables {
		if _, ok := expectedTables[name]; !ok {
			t.Errorf("unexpected table %q found in sqlite_master", name)
		}
	}

	// Verify all expected indexes exist with correct structure
	for name, expectedSQL := range expectedIndexes {
		gotSQL, ok := foundIndexes[name]
		if !ok {
			t.Errorf("expected index %q not found in sqlite_master", name)
			continue
		}
		if normalizeSQL(expectedSQL) != gotSQL {
			t.Errorf("index %q SQL mismatch:\n  expected: %s\n  got:      %s", name, normalizeSQL(expectedSQL), gotSQL)
		}
	}

	// Verify no extra user-defined indexes
	for name := range foundIndexes {
		if _, ok := expectedIndexes[name]; !ok {
			t.Errorf("unexpected index %q found in sqlite_master", name)
		}
	}
}

func TestComparison_SchemaTableCount(t *testing.T) {
	d := newTestDB(t)

	// Node.js db.js creates exactly 4 tables
	var tableCount int
	err := d.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&tableCount)
	if err != nil {
		t.Fatalf("failed to count tables: %v", err)
	}
	if tableCount != 4 {
		t.Errorf("expected 4 tables (matching Node.js db.js), got %d", tableCount)
	}

	// Node.js db.js creates exactly 3 explicit indexes
	var indexCount int
	err = d.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name NOT LIKE 'sqlite_autoindex_%'`).Scan(&indexCount)
	if err != nil {
		t.Fatalf("failed to count indexes: %v", err)
	}
	if indexCount != 3 {
		t.Errorf("expected 3 indexes (matching Node.js db.js), got %d", indexCount)
	}
}

// --- Operation Comparison Tests ---

func TestComparison_DomainOperationsMatchNodeBehavior(t *testing.T) {
	d := newTestDB(t)

	// Step 1: Insert domains (same sequence Node version would execute)
	res, err := d.InsertDomain("alice.example.com", 100)
	if err != nil {
		t.Fatalf("InsertDomain(alice) error: %v", err)
	}
	// Node: insertDomain returns selectByDomain result → [{domain: "alice.example.com", tg: 100}]
	if len(res) != 1 || res[0].Domain != "alice.example.com" || res[0].Tg != 100 {
		t.Errorf("InsertDomain(alice) result mismatch: got %+v", res)
	}

	res, err = d.InsertDomain("bob.example.com", 200)
	if err != nil {
		t.Fatalf("InsertDomain(bob) error: %v", err)
	}
	if len(res) != 1 || res[0].Domain != "bob.example.com" || res[0].Tg != 200 {
		t.Errorf("InsertDomain(bob) result mismatch: got %+v", res)
	}

	// Step 2: Query all domains
	// Node: selectAllDomain() returns all rows
	all, err := d.SelectAllDomain()
	if err != nil {
		t.Fatalf("SelectAllDomain() error: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("SelectAllDomain() expected 2 results, got %d", len(all))
	}
	// Sort for deterministic comparison
	sort.Slice(all, func(i, j int) bool { return all[i].Domain < all[j].Domain })
	if all[0].Domain != "alice.example.com" || all[0].Tg != 100 {
		t.Errorf("first domain mismatch: got %+v", all[0])
	}
	if all[1].Domain != "bob.example.com" || all[1].Tg != 200 {
		t.Errorf("second domain mismatch: got %+v", all[1])
	}

	// Step 3: Query specific domain
	// Node: selectByDomain("alice.example.com") returns [{domain: "alice.example.com", tg: 100}]
	res, err = d.SelectByDomain("alice.example.com")
	if err != nil {
		t.Fatalf("SelectByDomain(alice) error: %v", err)
	}
	if len(res) != 1 || res[0].Domain != "alice.example.com" || res[0].Tg != 100 {
		t.Errorf("SelectByDomain(alice) mismatch: got %+v", res)
	}

	// Step 4: Query non-existent domain
	// Node: selectByDomain("nonexistent") returns []
	res, err = d.SelectByDomain("nonexistent.com")
	if err != nil {
		t.Fatalf("SelectByDomain(nonexistent) error: %v", err)
	}
	if len(res) != 0 {
		t.Errorf("SelectByDomain(nonexistent) expected empty, got %d results", len(res))
	}

	// Step 5: Delete domain
	// Node: deleteDomain("alice.example.com") deletes then returns selectByDomain → []
	res, err = d.DeleteDomain("alice.example.com")
	if err != nil {
		t.Fatalf("DeleteDomain(alice) error: %v", err)
	}
	if len(res) != 0 {
		t.Errorf("DeleteDomain(alice) expected empty result, got %d", len(res))
	}

	// Step 6: Verify only bob remains
	all, err = d.SelectAllDomain()
	if err != nil {
		t.Fatalf("SelectAllDomain() after delete error: %v", err)
	}
	if len(all) != 1 || all[0].Domain != "bob.example.com" || all[0].Tg != 200 {
		t.Errorf("after delete, expected only bob: got %+v", all)
	}
}

func TestComparison_BlockDomainOperationsMatchNodeBehavior(t *testing.T) {
	d := newTestDB(t)

	// Insert block domains
	res, err := d.InsertBlockDomain("spam.com", 100)
	if err != nil {
		t.Fatalf("InsertBlockDomain error: %v", err)
	}
	// Node: insertBlockDomain returns selectByBlockDomain(domain, tg) → [{domain: "spam.com", tg: 100}]
	if len(res) != 1 || res[0].Domain != "spam.com" || res[0].Tg != 100 {
		t.Errorf("InsertBlockDomain result mismatch: got %+v", res)
	}

	res, err = d.InsertBlockDomain("ads.com", 100)
	if err != nil {
		t.Fatalf("InsertBlockDomain(ads) error: %v", err)
	}
	if len(res) != 1 || res[0].Domain != "ads.com" || res[0].Tg != 100 {
		t.Errorf("InsertBlockDomain(ads) result mismatch: got %+v", res)
	}

	// Query all block domains for user 100
	allBD, err := d.SelectUserAllBlockDomain(100)
	if err != nil {
		t.Fatalf("SelectUserAllBlockDomain error: %v", err)
	}
	if len(allBD) != 2 {
		t.Errorf("expected 2 block domains for tg=100, got %d", len(allBD))
	}

	// Delete one
	res, err = d.DeleteBlockDomain("spam.com", 100)
	if err != nil {
		t.Fatalf("DeleteBlockDomain error: %v", err)
	}
	// Node: deleteBlockDomain returns selectByBlockDomain(domain, tg) → []
	if len(res) != 0 {
		t.Errorf("DeleteBlockDomain expected empty result, got %d", len(res))
	}

	// Verify only ads.com remains
	allBD, err = d.SelectUserAllBlockDomain(100)
	if err != nil {
		t.Fatalf("SelectUserAllBlockDomain after delete error: %v", err)
	}
	if len(allBD) != 1 || allBD[0].Domain != "ads.com" {
		t.Errorf("after delete, expected only ads.com: got %+v", allBD)
	}
}

func TestComparison_BlockSenderOperationsMatchNodeBehavior(t *testing.T) {
	d := newTestDB(t)

	// Insert block sender
	res, err := d.InsertBlockSender("spammer@evil.com", 100)
	if err != nil {
		t.Fatalf("InsertBlockSender error: %v", err)
	}
	// Node.js bug: insertBlockSender calls selectByBlockSender(sender) with only sender arg,
	// tg is undefined → NULL. So the return query is WHERE sender IS ? AND tg IS NULL → empty.
	if len(res) != 0 {
		t.Errorf("InsertBlockSender should return empty (Node.js bug: tg=NULL query), got %d", len(res))
	}

	// But the record IS in the database — verify with proper query
	res2, err := d.SelectByBlockSender("spammer@evil.com", 100)
	if err != nil {
		t.Fatalf("SelectByBlockSender error: %v", err)
	}
	if len(res2) != 1 || res2[0].Sender != "spammer@evil.com" || res2[0].Tg != 100 {
		t.Errorf("SelectByBlockSender mismatch: got %+v", res2)
	}

	// Insert another for same user
	d.InsertBlockSender("phisher@bad.com", 100)

	// Query all block senders for user 100
	allBS, err := d.SelectUserAllBlockSender(100)
	if err != nil {
		t.Fatalf("SelectUserAllBlockSender error: %v", err)
	}
	if len(allBS) != 2 {
		t.Errorf("expected 2 block senders for tg=100, got %d", len(allBS))
	}

	// Delete — also returns empty due to Node.js bug
	res, err = d.DeleteBlockSender("spammer@evil.com", 100)
	if err != nil {
		t.Fatalf("DeleteBlockSender error: %v", err)
	}
	if len(res) != 0 {
		t.Errorf("DeleteBlockSender should return empty (Node.js bug), got %d", len(res))
	}

	// Verify only phisher remains
	allBS, err = d.SelectUserAllBlockSender(100)
	if err != nil {
		t.Fatalf("SelectUserAllBlockSender after delete error: %v", err)
	}
	if len(allBS) != 1 || allBS[0].Sender != "phisher@bad.com" {
		t.Errorf("after delete, expected only phisher: got %+v", allBS)
	}
}

func TestComparison_BlockReceiverOperationsMatchNodeBehavior(t *testing.T) {
	d := newTestDB(t)

	// Insert block receiver
	res, err := d.InsertBlockReceiver("me@mymail.com", 100)
	if err != nil {
		t.Fatalf("InsertBlockReceiver error: %v", err)
	}
	// Node.js bug: same as block_sender — returns empty due to tg=NULL query
	if len(res) != 0 {
		t.Errorf("InsertBlockReceiver should return empty (Node.js bug), got %d", len(res))
	}

	// Verify record exists with proper query
	res2, err := d.SelectByBlockReceiver("me@mymail.com", 100)
	if err != nil {
		t.Fatalf("SelectByBlockReceiver error: %v", err)
	}
	if len(res2) != 1 || res2[0].Receiver != "me@mymail.com" || res2[0].Tg != 100 {
		t.Errorf("SelectByBlockReceiver mismatch: got %+v", res2)
	}

	// Insert another
	d.InsertBlockReceiver("other@mymail.com", 100)

	// Query all
	allBR, err := d.SelectUserAllBlockReceiver(100)
	if err != nil {
		t.Fatalf("SelectUserAllBlockReceiver error: %v", err)
	}
	if len(allBR) != 2 {
		t.Errorf("expected 2 block receivers for tg=100, got %d", len(allBR))
	}

	// Delete
	res, err = d.DeleteBlockReceiver("me@mymail.com", 100)
	if err != nil {
		t.Fatalf("DeleteBlockReceiver error: %v", err)
	}
	if len(res) != 0 {
		t.Errorf("DeleteBlockReceiver should return empty (Node.js bug), got %d", len(res))
	}

	// Verify only other remains
	allBR, err = d.SelectUserAllBlockReceiver(100)
	if err != nil {
		t.Fatalf("SelectUserAllBlockReceiver after delete error: %v", err)
	}
	if len(allBR) != 1 || allBR[0].Receiver != "other@mymail.com" {
		t.Errorf("after delete, expected only other: got %+v", allBR)
	}
}

func TestComparison_FullOperationSequenceMatchesNodeBehavior(t *testing.T) {
	d := newTestDB(t)

	// Execute a fixed sequence of mixed operations and verify results match Node behavior.

	// 1. Insert domains
	d.InsertDomain("user1.tgmail.party", 1001)
	d.InsertDomain("user2.tgmail.party", 1002)
	d.InsertDomain("shared.tgmail.party", 1001)

	// 2. Insert block entries
	d.InsertBlockDomain("spam.org", 1001)
	d.InsertBlockDomain("ads.net", 1001)
	d.InsertBlockSender("bad@evil.com", 1001)
	d.InsertBlockReceiver("noreply@service.com", 1002)

	// 3. Verify domain state
	all, _ := d.SelectAllDomain()
	if len(all) != 3 {
		t.Fatalf("expected 3 domains, got %d", len(all))
	}

	// 4. Delete one domain
	res, _ := d.DeleteDomain("shared.tgmail.party")
	if len(res) != 0 {
		t.Errorf("DeleteDomain(shared) should return empty, got %d", len(res))
	}

	// 5. Verify 2 domains remain
	all, _ = d.SelectAllDomain()
	if len(all) != 2 {
		t.Errorf("expected 2 domains after delete, got %d", len(all))
	}

	// 6. Delete one block domain
	d.DeleteBlockDomain("spam.org", 1001)
	bdAll, _ := d.SelectUserAllBlockDomain(1001)
	if len(bdAll) != 1 || bdAll[0].Domain != "ads.net" {
		t.Errorf("after deleting spam.org block, expected only ads.net: got %+v", bdAll)
	}

	// 7. Verify block sender still exists
	bsAll, _ := d.SelectUserAllBlockSender(1001)
	if len(bsAll) != 1 || bsAll[0].Sender != "bad@evil.com" {
		t.Errorf("block sender mismatch: got %+v", bsAll)
	}

	// 8. Verify block receiver for user 1002
	brAll, _ := d.SelectUserAllBlockReceiver(1002)
	if len(brAll) != 1 || brAll[0].Receiver != "noreply@service.com" {
		t.Errorf("block receiver mismatch: got %+v", brAll)
	}

	// 9. Cross-user isolation: user 1002 should have no block domains
	bdUser2, _ := d.SelectUserAllBlockDomain(1002)
	if len(bdUser2) != 0 {
		t.Errorf("user 1002 should have no block domains, got %d", len(bdUser2))
	}

	// 10. Delete all remaining and verify empty
	d.DeleteDomain("user1.tgmail.party")
	d.DeleteDomain("user2.tgmail.party")
	d.DeleteBlockDomain("ads.net", 1001)
	d.DeleteBlockSender("bad@evil.com", 1001)
	d.DeleteBlockReceiver("noreply@service.com", 1002)

	all, _ = d.SelectAllDomain()
	if len(all) != 0 {
		t.Errorf("expected 0 domains after full cleanup, got %d", len(all))
	}
	bdAll, _ = d.SelectUserAllBlockDomain(1001)
	if len(bdAll) != 0 {
		t.Errorf("expected 0 block domains after cleanup, got %d", len(bdAll))
	}
	bsAll, _ = d.SelectUserAllBlockSender(1001)
	if len(bsAll) != 0 {
		t.Errorf("expected 0 block senders after cleanup, got %d", len(bsAll))
	}
	brAll, _ = d.SelectUserAllBlockReceiver(1002)
	if len(brAll) != 0 {
		t.Errorf("expected 0 block receivers after cleanup, got %d", len(brAll))
	}
}
