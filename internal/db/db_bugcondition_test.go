package db

import (
	"path/filepath"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Bug Condition Exploration Test: SQLite WAL mode and busy_timeout
// Validates: Requirements 1.3, 1.4
//
// This test verifies that db.New() creates a database with WAL journal mode
// and a positive busy_timeout. On UNFIXED code, these tests are EXPECTED TO
// FAIL because the database uses the default journal_mode ("delete") and
// busy_timeout (0).
//
// Bug condition: journal_mode!="wal" AND busy_timeout==0 → concurrent access
// causes lock contention busy-waiting, manifesting as 100% system call time.

func TestBugCondition_SQLiteWALModeAndBusyTimeout(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 50

	properties := gopter.NewProperties(parameters)

	// Property: For any database created via New(), journal_mode must be "wal"
	properties.Property("SQLite journal_mode must be wal", prop.ForAll(
		func(suffix int) bool {
			dir := t.TempDir()
			dbPath := filepath.Join(dir, "test.db")
			d, err := New(dbPath)
			if err != nil {
				t.Logf("New() failed: %v", err)
				return false
			}
			defer d.Close()

			var journalMode string
			err = d.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
			if err != nil {
				t.Logf("PRAGMA journal_mode query failed: %v", err)
				return false
			}

			// Bug condition check: journal_mode should be "wal"
			return journalMode == "wal"
		},
		gen.IntRange(1, 100), // dummy generator to drive property iterations
	))

	// Property: For any database created via New(), busy_timeout must be > 0
	properties.Property("SQLite busy_timeout must be greater than 0", prop.ForAll(
		func(suffix int) bool {
			dir := t.TempDir()
			dbPath := filepath.Join(dir, "test.db")
			d, err := New(dbPath)
			if err != nil {
				t.Logf("New() failed: %v", err)
				return false
			}
			defer d.Close()

			var busyTimeout int
			err = d.db.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout)
			if err != nil {
				t.Logf("PRAGMA busy_timeout query failed: %v", err)
				return false
			}

			// Bug condition check: busy_timeout should be > 0
			return busyTimeout > 0
		},
		gen.IntRange(1, 100), // dummy generator to drive property iterations
	))

	properties.TestingRun(t)
}
