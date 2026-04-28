package db

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Feature: high-cpu-usage-fix, Property 3: Database CRUD Preservation
// **Validates: Requirements 3.1, 3.3, 3.4**
//
// For any sequence of database operations (InsertDomain/DeleteDomain/InsertBlockSender/
// DeleteBlockSender/InsertBlockDomain/DeleteBlockDomain/InsertBlockReceiver/DeleteBlockReceiver),
// the results must remain consistent:
// - insert → select returns the inserted data
// - insert → delete → select returns empty
// - All CRUD operations produce correct, consistent results

// TestProperty_Preservation_DBInsertThenSelectReturnsSameData verifies that for any
// randomly generated domain name and chat ID, inserting then selecting returns the inserted data.
func TestProperty_Preservation_DBInsertThenSelectReturnsSameData(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	domainPool := []string{
		"alpha.com", "beta.org", "gamma.net", "delta.io", "epsilon.xyz",
		"zeta.dev", "eta.co", "theta.app", "iota.me", "kappa.us",
	}
	tgPool := []int64{100, 200, 300, 400, 500, 600, 700, 800, 900, 1000}

	properties.Property("insert domain then select returns the inserted data", prop.ForAll(
		func(domainIdx, tgIdx int) bool {
			dir := t.TempDir()
			dbPath := filepath.Join(dir, "test.db")
			d, err := New(dbPath)
			if err != nil {
				t.Logf("New() failed: %v", err)
				return false
			}
			defer d.Close()

			domain := domainPool[domainIdx]
			tgID := tgPool[tgIdx]

			// Insert
			insertResult, err := d.InsertDomain(domain, tgID)
			if err != nil {
				t.Logf("InsertDomain(%q, %d) error: %v", domain, tgID, err)
				return false
			}
			if len(insertResult) != 1 {
				t.Logf("InsertDomain result count: expected 1, got %d", len(insertResult))
				return false
			}
			if insertResult[0].Domain != domain || insertResult[0].Tg != tgID {
				t.Logf("InsertDomain result mismatch: got %+v", insertResult[0])
				return false
			}

			// Select
			selectResult, err := d.SelectByDomain(domain)
			if err != nil {
				t.Logf("SelectByDomain(%q) error: %v", domain, err)
				return false
			}
			if len(selectResult) != 1 {
				t.Logf("SelectByDomain result count: expected 1, got %d", len(selectResult))
				return false
			}
			if selectResult[0].Domain != domain || selectResult[0].Tg != tgID {
				t.Logf("SelectByDomain result mismatch: got %+v", selectResult[0])
				return false
			}

			return true
		},
		gen.IntRange(0, len(domainPool)-1),
		gen.IntRange(0, len(tgPool)-1),
	))

	properties.TestingRun(t)
}

// TestProperty_Preservation_DBInsertDeleteThenSelectReturnsEmpty verifies that for any
// randomly generated data, insert → delete → select returns empty.
func TestProperty_Preservation_DBInsertDeleteThenSelectReturnsEmpty(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	domainPool := []string{
		"alpha.com", "beta.org", "gamma.net", "delta.io", "epsilon.xyz",
	}
	tgPool := []int64{100, 200, 300, 400, 500}

	// Domain insert → delete → select returns empty
	properties.Property("insert domain then delete then select returns empty", prop.ForAll(
		func(domainIdx, tgIdx int) bool {
			dir := t.TempDir()
			dbPath := filepath.Join(dir, "test.db")
			d, err := New(dbPath)
			if err != nil {
				t.Logf("New() failed: %v", err)
				return false
			}
			defer d.Close()

			domain := domainPool[domainIdx]
			tgID := tgPool[tgIdx]

			d.InsertDomain(domain, tgID)
			deleteResult, err := d.DeleteDomain(domain)
			if err != nil {
				t.Logf("DeleteDomain error: %v", err)
				return false
			}
			if len(deleteResult) != 0 {
				t.Logf("DeleteDomain result should be empty, got %d", len(deleteResult))
				return false
			}

			selectResult, err := d.SelectByDomain(domain)
			if err != nil {
				t.Logf("SelectByDomain error: %v", err)
				return false
			}
			if len(selectResult) != 0 {
				t.Logf("SelectByDomain after delete should be empty, got %d", len(selectResult))
				return false
			}

			return true
		},
		gen.IntRange(0, len(domainPool)-1),
		gen.IntRange(0, len(tgPool)-1),
	))

	// BlockDomain insert → delete → select returns empty
	properties.Property("insert block domain then delete then select returns empty", prop.ForAll(
		func(domainIdx, tgIdx int) bool {
			dir := t.TempDir()
			dbPath := filepath.Join(dir, "test.db")
			d, err := New(dbPath)
			if err != nil {
				return false
			}
			defer d.Close()

			domain := domainPool[domainIdx]
			tgID := tgPool[tgIdx]

			d.InsertBlockDomain(domain, tgID)
			deleteResult, err := d.DeleteBlockDomain(domain, tgID)
			if err != nil {
				t.Logf("DeleteBlockDomain error: %v", err)
				return false
			}
			if len(deleteResult) != 0 {
				t.Logf("DeleteBlockDomain result should be empty, got %d", len(deleteResult))
				return false
			}

			selectResult, err := d.SelectByBlockDomain(domain, tgID)
			if err != nil {
				return false
			}
			if len(selectResult) != 0 {
				t.Logf("SelectByBlockDomain after delete should be empty, got %d", len(selectResult))
				return false
			}

			return true
		},
		gen.IntRange(0, len(domainPool)-1),
		gen.IntRange(0, len(tgPool)-1),
	))

	// BlockSender insert → delete → select returns empty
	properties.Property("insert block sender then delete then select returns empty", prop.ForAll(
		func(senderIdx, tgIdx int) bool {
			dir := t.TempDir()
			dbPath := filepath.Join(dir, "test.db")
			d, err := New(dbPath)
			if err != nil {
				return false
			}
			defer d.Close()

			senders := []string{"a@evil.com", "b@spam.org", "c@bad.net", "d@junk.io", "e@trash.xyz"}
			sender := senders[senderIdx]
			tgID := tgPool[tgIdx]

			d.InsertBlockSender(sender, tgID)
			d.DeleteBlockSender(sender, tgID)

			selectResult, err := d.SelectByBlockSender(sender, tgID)
			if err != nil {
				return false
			}
			if len(selectResult) != 0 {
				t.Logf("SelectByBlockSender after delete should be empty, got %d", len(selectResult))
				return false
			}

			return true
		},
		gen.IntRange(0, 4),
		gen.IntRange(0, len(tgPool)-1),
	))

	// BlockReceiver insert → delete → select returns empty
	properties.Property("insert block receiver then delete then select returns empty", prop.ForAll(
		func(receiverIdx, tgIdx int) bool {
			dir := t.TempDir()
			dbPath := filepath.Join(dir, "test.db")
			d, err := New(dbPath)
			if err != nil {
				return false
			}
			defer d.Close()

			receivers := []string{"a@target.com", "b@target.org", "c@target.net", "d@target.io", "e@target.xyz"}
			receiver := receivers[receiverIdx]
			tgID := tgPool[tgIdx]

			d.InsertBlockReceiver(receiver, tgID)
			d.DeleteBlockReceiver(receiver, tgID)

			selectResult, err := d.SelectByBlockReceiver(receiver, tgID)
			if err != nil {
				return false
			}
			if len(selectResult) != 0 {
				t.Logf("SelectByBlockReceiver after delete should be empty, got %d", len(selectResult))
				return false
			}

			return true
		},
		gen.IntRange(0, 4),
		gen.IntRange(0, len(tgPool)-1),
	))

	properties.TestingRun(t)
}

// TestProperty_Preservation_DBRandomOperationSequenceConsistency verifies that for any
// random sequence of mixed CRUD operations, the final DB state is consistent with
// the expected state tracked in memory.
func TestProperty_Preservation_DBRandomOperationSequenceConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Operation encoding reuses the same scheme as db_property_test.go:
	// kind(0-7) * 25 + nameIdx(0-4) * 5 + tgIdx(0-4), range 0..199
	properties.Property("random mixed CRUD operation sequences produce consistent final state", prop.ForAll(
		func(encodedOps []int) bool {
			dir := t.TempDir()
			dbPath := filepath.Join(dir, "test.db")
			d, err := New(dbPath)
			if err != nil {
				t.Logf("New() failed: %v", err)
				return false
			}
			defer d.Close()

			// Track expected state
			expectedDomains := make(map[string]int64)
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
					if _, exists := expectedDomains[name]; !exists {
						_, err := d.InsertDomain(name, tgID)
						if err != nil {
							t.Logf("InsertDomain error: %v", err)
							return false
						}
						expectedDomains[name] = tgID
					}
				case opDeleteDomain:
					_, err := d.DeleteDomain(name)
					if err != nil {
						return false
					}
					delete(expectedDomains, name)
				case opInsertBlockDomain:
					_, err := d.InsertBlockDomain(name, tgID)
					if err != nil {
						return false
					}
					expectedBlockDomain[bk]++
				case opDeleteBlockDomain:
					_, err := d.DeleteBlockDomain(name, tgID)
					if err != nil {
						return false
					}
					delete(expectedBlockDomain, bk)
				case opInsertBlockSender:
					_, err := d.InsertBlockSender(name, tgID)
					if err != nil {
						return false
					}
					expectedBlockSender[bk]++
				case opDeleteBlockSender:
					_, err := d.DeleteBlockSender(name, tgID)
					if err != nil {
						return false
					}
					delete(expectedBlockSender, bk)
				case opInsertBlockReceiver:
					_, err := d.InsertBlockReceiver(name, tgID)
					if err != nil {
						return false
					}
					expectedBlockReceiver[bk]++
				case opDeleteBlockReceiver:
					_, err := d.DeleteBlockReceiver(name, tgID)
					if err != nil {
						return false
					}
					delete(expectedBlockReceiver, bk)
				}
			}

			// Verify domain_tg state
			allDomains, err := d.SelectAllDomain()
			if err != nil {
				return false
			}
			if len(allDomains) != len(expectedDomains) {
				t.Logf("domain count mismatch: got %d, expected %d", len(allDomains), len(expectedDomains))
				return false
			}
			sort.Slice(allDomains, func(i, j int) bool { return allDomains[i].Domain < allDomains[j].Domain })
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

			// Verify block tables for each tg ID (1-5)
			for tg := int64(1); tg <= 5; tg++ {
				// Block domain
				bdResults, err := d.SelectUserAllBlockDomain(tg)
				if err != nil {
					return false
				}
				bdCounts := make(map[string]int)
				for _, r := range bdResults {
					bdCounts[r.Domain]++
				}
				expectedBDForTg := make(map[string]int)
				for bk, count := range expectedBlockDomain {
					if bk.tg == tg {
						expectedBDForTg[bk.name] = count
					}
				}
				if len(bdCounts) != len(expectedBDForTg) {
					t.Logf("block_domain count mismatch for tg=%d: got %d, expected %d", tg, len(bdCounts), len(expectedBDForTg))
					return false
				}
				for name, count := range expectedBDForTg {
					if bdCounts[name] != count {
						return false
					}
				}

				// Block sender
				bsResults, err := d.SelectUserAllBlockSender(tg)
				if err != nil {
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
					t.Logf("block_sender count mismatch for tg=%d: got %d, expected %d", tg, len(bsCounts), len(expectedBSForTg))
					return false
				}
				for name, count := range expectedBSForTg {
					if bsCounts[name] != count {
						return false
					}
				}

				// Block receiver
				brResults, err := d.SelectUserAllBlockReceiver(tg)
				if err != nil {
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
					t.Logf("block_receiver count mismatch for tg=%d: got %d, expected %d", tg, len(brCounts), len(expectedBRForTg))
					return false
				}
				for name, count := range expectedBRForTg {
					if brCounts[name] != count {
						return false
					}
				}
			}

			return true
		},
		gen.SliceOfN(25, gen.IntRange(0, 199)),
	))

	properties.TestingRun(t)
}
