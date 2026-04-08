package db

import (
	"path/filepath"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Operation types for the property test state machine.
const (
	opInsertDomain = iota
	opDeleteDomain
	opInsertBlockDomain
	opDeleteBlockDomain
	opInsertBlockSender
	opDeleteBlockSender
	opInsertBlockReceiver
	opDeleteBlockReceiver
)

// decodeOp decodes an encoded integer into operation kind, name, and tg ID.
// Encoding: kind(0-7) * 25 + nameIdx(0-4) * 5 + tgIdx(0-4), range 0..199
func decodeOp(encoded int) (kind int, name string, tgID int64) {
	names := []string{"aa.com", "bb.org", "cc.net", "dd.io", "ee.xyz"}
	tgIDs := []int64{1, 2, 3, 4, 5}
	kind = encoded / 25
	nameIdx := (encoded % 25) / 5
	tgIdx := encoded % 5
	return kind, names[nameIdx], tgIDs[tgIdx]
}

// Feature: go-version-rewrite, Property 3: Database operation consistency
// Validates: Requirements 2.3, 2.5
func TestProperty_DatabaseOperationConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("random operation sequences produce consistent query results", prop.ForAll(
		func(encodedOps []int) bool {
			if len(encodedOps) < 5 {
				return true // skip trivially small sequences
			}

			// Create a fresh temp DB.
			dir := t.TempDir()
			dbPath := filepath.Join(dir, "test.db")
			d, err := New(dbPath)
			if err != nil {
				t.Logf("New() failed: %v", err)
				return false
			}
			defer d.Close()

			// Track expected state.
			// domain_tg: domain is PRIMARY KEY → one tg per domain, no duplicates.
			expectedDomains := make(map[string]int64)

			// Block tables have NO unique constraint → duplicates allowed.
			// Track count of rows per (tg, name) pair.
			// Key format: "tg:name"
			type blockKey struct {
				tg   int64
				name string
			}
			expectedBlockDomain := make(map[blockKey]int)
			expectedBlockSender := make(map[blockKey]int)
			expectedBlockReceiver := make(map[blockKey]int)

			for _, enc := range encodedOps {
				kind, name, tgID := decodeOp(enc)
				bk := blockKey{tg: tgID, name: name}

				switch kind {
				case opInsertDomain:
					// PRIMARY KEY constraint: skip if domain already exists.
					if _, exists := expectedDomains[name]; !exists {
						_, err := d.InsertDomain(name, tgID)
						if err != nil {
							t.Logf("InsertDomain(%q, %d) error: %v", name, tgID, err)
							return false
						}
						expectedDomains[name] = tgID
					}

				case opDeleteDomain:
					_, err := d.DeleteDomain(name)
					if err != nil {
						t.Logf("DeleteDomain(%q) error: %v", name, err)
						return false
					}
					delete(expectedDomains, name)

				case opInsertBlockDomain:
					_, err := d.InsertBlockDomain(name, tgID)
					if err != nil {
						t.Logf("InsertBlockDomain error: %v", err)
						return false
					}
					expectedBlockDomain[bk]++

				case opDeleteBlockDomain:
					// DELETE removes ALL matching rows for (domain, tg).
					_, err := d.DeleteBlockDomain(name, tgID)
					if err != nil {
						t.Logf("DeleteBlockDomain error: %v", err)
						return false
					}
					delete(expectedBlockDomain, bk)

				case opInsertBlockSender:
					_, err := d.InsertBlockSender(name, tgID)
					if err != nil {
						t.Logf("InsertBlockSender error: %v", err)
						return false
					}
					expectedBlockSender[bk]++

				case opDeleteBlockSender:
					_, err := d.DeleteBlockSender(name, tgID)
					if err != nil {
						t.Logf("DeleteBlockSender error: %v", err)
						return false
					}
					delete(expectedBlockSender, bk)

				case opInsertBlockReceiver:
					_, err := d.InsertBlockReceiver(name, tgID)
					if err != nil {
						t.Logf("InsertBlockReceiver error: %v", err)
						return false
					}
					expectedBlockReceiver[bk]++

				case opDeleteBlockReceiver:
					_, err := d.DeleteBlockReceiver(name, tgID)
					if err != nil {
						t.Logf("DeleteBlockReceiver error: %v", err)
						return false
					}
					delete(expectedBlockReceiver, bk)
				}
			}

			// Verify domain_tg state.
			allDomains, err := d.SelectAllDomain()
			if err != nil {
				t.Logf("SelectAllDomain() error: %v", err)
				return false
			}
			if len(allDomains) != len(expectedDomains) {
				t.Logf("domain count mismatch: got %d, expected %d", len(allDomains), len(expectedDomains))
				return false
			}
			for _, r := range allDomains {
				expectedTg, ok := expectedDomains[r.Domain]
				if !ok {
					t.Logf("unexpected domain in DB: %q", r.Domain)
					return false
				}
				if r.Tg != expectedTg {
					t.Logf("domain %q tg mismatch: got %d, expected %d", r.Domain, r.Tg, expectedTg)
					return false
				}
			}

			// Verify block tables for each tg ID (1-5).
			for tg := int64(1); tg <= 5; tg++ {
				// Block domain: count rows per name for this tg.
				bdResults, err := d.SelectUserAllBlockDomain(tg)
				if err != nil {
					t.Logf("SelectUserAllBlockDomain(%d) error: %v", tg, err)
					return false
				}
				bdCounts := make(map[string]int)
				for _, r := range bdResults {
					bdCounts[r.Domain]++
				}
				// Compare with expected.
				expectedBDForTg := make(map[string]int)
				for bk, count := range expectedBlockDomain {
					if bk.tg == tg {
						expectedBDForTg[bk.name] = count
					}
				}
				if len(bdCounts) != len(expectedBDForTg) {
					t.Logf("block_domain unique names for tg=%d: got %d, expected %d", tg, len(bdCounts), len(expectedBDForTg))
					return false
				}
				for name, count := range expectedBDForTg {
					if bdCounts[name] != count {
						t.Logf("block_domain %q for tg=%d: got %d rows, expected %d", name, tg, bdCounts[name], count)
						return false
					}
				}

				// Block sender.
				bsResults, err := d.SelectUserAllBlockSender(tg)
				if err != nil {
					t.Logf("SelectUserAllBlockSender(%d) error: %v", tg, err)
					return false
				}
				bsCounts := make(map[string]int)
				for _, r := range bsResults {
					bsCounts[r.Sender]++
				}
				expectedBSForTg := make(map[string]int)
				for bk, count := range expectedBlockSender {
					if bk.tg == tg {
						expectedBSForTg[bk.name] = count
					}
				}
				if len(bsCounts) != len(expectedBSForTg) {
					t.Logf("block_sender unique names for tg=%d: got %d, expected %d", tg, len(bsCounts), len(expectedBSForTg))
					return false
				}
				for name, count := range expectedBSForTg {
					if bsCounts[name] != count {
						t.Logf("block_sender %q for tg=%d: got %d rows, expected %d", name, tg, bsCounts[name], count)
						return false
					}
				}

				// Block receiver.
				brResults, err := d.SelectUserAllBlockReceiver(tg)
				if err != nil {
					t.Logf("SelectUserAllBlockReceiver(%d) error: %v", tg, err)
					return false
				}
				brCounts := make(map[string]int)
				for _, r := range brResults {
					brCounts[r.Receiver]++
				}
				expectedBRForTg := make(map[string]int)
				for bk, count := range expectedBlockReceiver {
					if bk.tg == tg {
						expectedBRForTg[bk.name] = count
					}
				}
				if len(brCounts) != len(expectedBRForTg) {
					t.Logf("block_receiver unique names for tg=%d: got %d, expected %d", tg, len(brCounts), len(expectedBRForTg))
					return false
				}
				for name, count := range expectedBRForTg {
					if brCounts[name] != count {
						t.Logf("block_receiver %q for tg=%d: got %d rows, expected %d", name, tg, brCounts[name], count)
						return false
					}
				}
			}

			return true
		},
		gen.SliceOfN(30, gen.IntRange(0, 199)),
	))

	properties.TestingRun(t)
}
