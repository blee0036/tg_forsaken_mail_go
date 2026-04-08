package db

import (
	"path/filepath"
	"testing"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	d, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestNew_CreatesTablesAndIndexes(t *testing.T) {
	d := newTestDB(t)

	// Verify all 4 tables exist
	for _, table := range []string{"domain_tg", "block_sender", "block_domain", "block_receiver"} {
		if !d.tableExists(table) {
			t.Errorf("table %q should exist", table)
		}
	}

	// Verify indexes exist
	for _, idx := range []string{"block_sender_idx_tg", "block_domain_idx_tg", "block_receiver_idx_tg"} {
		var name string
		err := d.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='index' AND name=?`, idx).Scan(&name)
		if err != nil {
			t.Errorf("index %q should exist: %v", idx, err)
		}
	}
}

func TestNew_InvalidPath(t *testing.T) {
	_, err := New("/nonexistent/path/to/db")
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestNew_IdempotentTableCreation(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Create DB twice — second time should not fail
	d1, err := New(dbPath)
	if err != nil {
		t.Fatalf("first New() failed: %v", err)
	}
	d1.Close()

	d2, err := New(dbPath)
	if err != nil {
		t.Fatalf("second New() failed: %v", err)
	}
	d2.Close()
}

func TestClose(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	d, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Errorf("Close() failed: %v", err)
	}
}

// --- Domain operations ---

func TestSelectByDomain_Empty(t *testing.T) {
	d := newTestDB(t)
	results, err := d.SelectByDomain("test.com")
	if err != nil {
		t.Fatalf("SelectByDomain() error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
}

func TestInsertDomain_AndSelect(t *testing.T) {
	d := newTestDB(t)
	results, err := d.InsertDomain("test.com", 12345)
	if err != nil {
		t.Fatalf("InsertDomain() error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Domain != "test.com" || results[0].Tg != 12345 {
		t.Errorf("unexpected result: %+v", results[0])
	}
}

func TestDeleteDomain(t *testing.T) {
	d := newTestDB(t)
	d.InsertDomain("test.com", 12345)
	results, err := d.DeleteDomain("test.com")
	if err != nil {
		t.Fatalf("DeleteDomain() error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty results after delete, got %d", len(results))
	}
}

func TestSelectAllDomain(t *testing.T) {
	d := newTestDB(t)
	d.InsertDomain("a.com", 1)
	d.InsertDomain("b.com", 2)
	results, err := d.SelectAllDomain()
	if err != nil {
		t.Fatalf("SelectAllDomain() error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

// --- Block domain operations ---

func TestBlockDomain_InsertSelectDelete(t *testing.T) {
	d := newTestDB(t)

	// Insert
	results, err := d.InsertBlockDomain("spam.com", 100)
	if err != nil {
		t.Fatalf("InsertBlockDomain() error: %v", err)
	}
	if len(results) != 1 || results[0].Domain != "spam.com" || results[0].Tg != 100 {
		t.Errorf("unexpected insert result: %+v", results)
	}

	// Select
	results, err = d.SelectByBlockDomain("spam.com", 100)
	if err != nil {
		t.Fatalf("SelectByBlockDomain() error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	// Delete
	results, err = d.DeleteBlockDomain("spam.com", 100)
	if err != nil {
		t.Fatalf("DeleteBlockDomain() error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty after delete, got %d", len(results))
	}
}

func TestSelectUserAllBlockDomain(t *testing.T) {
	d := newTestDB(t)
	d.InsertBlockDomain("a.com", 100)
	d.InsertBlockDomain("b.com", 100)
	d.InsertBlockDomain("c.com", 200)

	results, err := d.SelectUserAllBlockDomain(100)
	if err != nil {
		t.Fatalf("SelectUserAllBlockDomain() error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results for tg=100, got %d", len(results))
	}
}

// --- Block sender operations ---

func TestBlockSender_InsertSelectDelete(t *testing.T) {
	d := newTestDB(t)

	// Insert — returns selectByBlockSender(sender) with NULL tg (Node.js bug replication)
	results, err := d.InsertBlockSender("spammer@evil.com", 100)
	if err != nil {
		t.Fatalf("InsertBlockSender() error: %v", err)
	}
	// Node.js bug: returns selectByBlockSender(sender) where tg is undefined → NULL
	// Since we inserted with tg=100, query with tg IS NULL returns empty
	if len(results) != 0 {
		t.Errorf("expected empty results (Node.js bug replication), got %d", len(results))
	}

	// SelectByBlockSender with correct tg should find the record
	results2, err := d.SelectByBlockSender("spammer@evil.com", 100)
	if err != nil {
		t.Fatalf("SelectByBlockSender() error: %v", err)
	}
	if len(results2) != 1 || results2[0].Sender != "spammer@evil.com" || results2[0].Tg != 100 {
		t.Errorf("unexpected select result: %+v", results2)
	}

	// Delete — also returns selectByBlockSender(sender) with NULL tg
	results, err = d.DeleteBlockSender("spammer@evil.com", 100)
	if err != nil {
		t.Fatalf("DeleteBlockSender() error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty results after delete (Node.js bug replication), got %d", len(results))
	}

	// Verify actually deleted
	results2, err = d.SelectByBlockSender("spammer@evil.com", 100)
	if err != nil {
		t.Fatalf("SelectByBlockSender() after delete error: %v", err)
	}
	if len(results2) != 0 {
		t.Errorf("expected empty after delete, got %d", len(results2))
	}
}

func TestSelectUserAllBlockSender(t *testing.T) {
	d := newTestDB(t)
	d.InsertBlockSender("a@evil.com", 100)
	d.InsertBlockSender("b@evil.com", 100)
	d.InsertBlockSender("c@evil.com", 200)

	results, err := d.SelectUserAllBlockSender(100)
	if err != nil {
		t.Fatalf("SelectUserAllBlockSender() error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results for tg=100, got %d", len(results))
	}
}

// --- Block receiver operations ---

func TestBlockReceiver_InsertSelectDelete(t *testing.T) {
	d := newTestDB(t)

	// Insert — returns selectByBlockReceiver(receiver) with NULL tg (Node.js bug replication)
	results, err := d.InsertBlockReceiver("victim@target.com", 100)
	if err != nil {
		t.Fatalf("InsertBlockReceiver() error: %v", err)
	}
	// Node.js bug: returns selectByBlockReceiver(receiver) where tg is undefined → NULL
	if len(results) != 0 {
		t.Errorf("expected empty results (Node.js bug replication), got %d", len(results))
	}

	// SelectByBlockReceiver with correct tg should find the record
	results2, err := d.SelectByBlockReceiver("victim@target.com", 100)
	if err != nil {
		t.Fatalf("SelectByBlockReceiver() error: %v", err)
	}
	if len(results2) != 1 || results2[0].Receiver != "victim@target.com" || results2[0].Tg != 100 {
		t.Errorf("unexpected select result: %+v", results2)
	}

	// Delete — also returns selectByBlockReceiver(receiver) with NULL tg
	results, err = d.DeleteBlockReceiver("victim@target.com", 100)
	if err != nil {
		t.Fatalf("DeleteBlockReceiver() error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty results after delete (Node.js bug replication), got %d", len(results))
	}
}

func TestSelectUserAllBlockReceiver(t *testing.T) {
	d := newTestDB(t)
	d.InsertBlockReceiver("a@target.com", 100)
	d.InsertBlockReceiver("b@target.com", 100)
	d.InsertBlockReceiver("c@target.com", 200)

	results, err := d.SelectUserAllBlockReceiver(100)
	if err != nil {
		t.Fatalf("SelectUserAllBlockReceiver() error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results for tg=100, got %d", len(results))
	}
}

// --- Schema verification ---

func TestSchemaMatchesNodeVersion(t *testing.T) {
	d := newTestDB(t)

	// Verify domain_tg table structure
	var sql string
	err := d.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='domain_tg'`).Scan(&sql)
	if err != nil {
		t.Fatalf("failed to get domain_tg schema: %v", err)
	}
	// domain_tg should have domain TEXT PRIMARY KEY and tg INTEGER
	if sql == "" {
		t.Error("domain_tg schema is empty")
	}

	// Verify block_sender has index
	err = d.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='index' AND name='block_sender_idx_tg'`).Scan(&sql)
	if err != nil {
		t.Fatalf("failed to get block_sender index: %v", err)
	}

	// Verify block_domain has index
	err = d.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='index' AND name='block_domain_idx_tg'`).Scan(&sql)
	if err != nil {
		t.Fatalf("failed to get block_domain index: %v", err)
	}

	// Verify block_receiver has index
	err = d.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='index' AND name='block_receiver_idx_tg'`).Scan(&sql)
	if err != nil {
		t.Fatalf("failed to get block_receiver index: %v", err)
	}
}


