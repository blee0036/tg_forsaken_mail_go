package db

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
)

// DomainTg represents a domain-to-telegram mapping record.
type DomainTg struct {
	Domain string
	Tg     int64
}

// BlockSender represents a blocked sender record.
type BlockSender struct {
	Sender string
	Tg     int64
}

// BlockDomain represents a blocked domain record.
type BlockDomain struct {
	Domain string
	Tg     int64
}

// BlockReceiver represents a blocked receiver record.
type BlockReceiver struct {
	Receiver string
	Tg       int64
}

// DB wraps a *sql.DB for SQLite operations.
type DB struct {
	db *sql.DB
}

// New opens the SQLite database at dbPath, auto-detects and creates
// missing tables (domain_tg, block_sender, block_domain, block_receiver)
// with indexes. SQL statements match the Node.js db.js exactly.
func New(dbPath string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	d := &DB{db: sqlDB}
	if err := d.initTables(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to initialize tables: %w", err)
	}
	return d, nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	return d.db.Close()
}

// initTables checks for and creates missing tables, matching Node.js db.js exactly.
func (d *DB) initTables() error {
	// Check and create domain_tg
	if !d.tableExists("domain_tg") {
		log.Println("WARNING: database appears empty, initializing it.")
		_, err := d.db.Exec(`
        CREATE TABLE domain_tg (
            domain TEXT PRIMARY KEY,
            tg INTEGER
        );`)
		if err != nil {
			return fmt.Errorf("create domain_tg: %w", err)
		}
	}

	// Check and create block_sender
	if !d.tableExists("block_sender") {
		log.Println("WARNING: database appears empty, initializing it.")
		_, err := d.db.Exec(`
        CREATE TABLE block_sender (
            sender TEXT,
            tg INTEGER
        );
        CREATE INDEX block_sender_idx_tg ON block_sender (
            tg
        );`)
		if err != nil {
			return fmt.Errorf("create block_sender: %w", err)
		}
	}

	// Check and create block_domain
	if !d.tableExists("block_domain") {
		log.Println("WARNING: database appears empty, initializing it.")
		_, err := d.db.Exec(`
        CREATE TABLE block_domain (
            domain TEXT,
            tg INTEGER
        );
        CREATE INDEX block_domain_idx_tg ON block_domain (
            tg
        );`)
		if err != nil {
			return fmt.Errorf("create block_domain: %w", err)
		}
	}

	// Check and create block_receiver
	if !d.tableExists("block_receiver") {
		log.Println("WARNING: database appears empty, initializing it.")
		_, err := d.db.Exec(`
        CREATE TABLE block_receiver (
            receiver TEXT,
            tg INTEGER
        );
        CREATE INDEX block_receiver_idx_tg ON block_receiver (
            tg
        );`)
		if err != nil {
			return fmt.Errorf("create block_receiver: %w", err)
		}
	}

	// Check and create user_settings (for language preference)
	if !d.tableExists("user_settings") {
		_, err := d.db.Exec(`
        CREATE TABLE user_settings (
            tg INTEGER PRIMARY KEY,
            lang TEXT DEFAULT ''
        );`)
		if err != nil {
			return fmt.Errorf("create user_settings: %w", err)
		}
	}

	return nil
}

func (d *DB) tableExists(name string) bool {
	var n string
	err := d.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' and name=?`, name).Scan(&n)
	return err == nil
}

// --- Domain operations (domain_tg table) ---

// SelectByDomain returns all domain_tg records matching the given domain.
// SQL: SELECT * FROM domain_tg WHERE domain IS $origin
func (d *DB) SelectByDomain(domain string) ([]DomainTg, error) {
	rows, err := d.db.Query("SELECT * FROM domain_tg WHERE domain IS ?", domain)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []DomainTg
	for rows.Next() {
		var r DomainTg
		if err := rows.Scan(&r.Domain, &r.Tg); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// InsertDomain inserts a domain mapping and returns the selectByDomain result.
// SQL: INSERT INTO domain_tg (domain, tg) VALUES ($domain, $tg)
// Then returns selectByDomain(domain).
func (d *DB) InsertDomain(domain string, tg int64) ([]DomainTg, error) {
	_, err := d.db.Exec("INSERT INTO domain_tg (domain, tg) VALUES (?, ?)", domain, tg)
	if err != nil {
		return nil, err
	}
	return d.SelectByDomain(domain)
}

// DeleteDomain deletes a domain mapping and returns the selectByDomain result.
// SQL: DELETE FROM domain_tg WHERE domain IS $domain
// Then returns selectByDomain(domain).
func (d *DB) DeleteDomain(domain string) ([]DomainTg, error) {
	_, err := d.db.Exec("DELETE FROM domain_tg WHERE domain IS ?", domain)
	if err != nil {
		return nil, err
	}
	return d.SelectByDomain(domain)
}

// SelectAllDomain returns all domain_tg records.
// SQL: SELECT * FROM domain_tg
func (d *DB) SelectAllDomain() ([]DomainTg, error) {
	rows, err := d.db.Query("SELECT * FROM domain_tg")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []DomainTg
	for rows.Next() {
		var r DomainTg
		if err := rows.Scan(&r.Domain, &r.Tg); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// --- Block domain operations (block_domain table) ---

// SelectByBlockDomain returns block_domain records matching domain and tg.
// SQL: SELECT * FROM block_domain WHERE domain IS $domain AND tg IS $tg
func (d *DB) SelectByBlockDomain(domain string, tg int64) ([]BlockDomain, error) {
	rows, err := d.db.Query("SELECT * FROM block_domain WHERE domain IS ? AND tg IS ?", domain, tg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []BlockDomain
	for rows.Next() {
		var r BlockDomain
		if err := rows.Scan(&r.Domain, &r.Tg); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// InsertBlockDomain inserts a block_domain record and returns selectByBlockDomain result.
// SQL: INSERT INTO block_domain (domain, tg) VALUES ($domain, $tg)
// Then returns selectByBlockDomain(domain, tg).
func (d *DB) InsertBlockDomain(domain string, tg int64) ([]BlockDomain, error) {
	_, err := d.db.Exec("INSERT INTO block_domain (domain, tg) VALUES (?, ?)", domain, tg)
	if err != nil {
		return nil, err
	}
	return d.SelectByBlockDomain(domain, tg)
}

// DeleteBlockDomain deletes a block_domain record and returns selectByBlockDomain result.
// SQL: DELETE FROM block_domain WHERE domain IS $domain AND tg IS $tg
// Then returns selectByBlockDomain(domain, tg).
func (d *DB) DeleteBlockDomain(domain string, tg int64) ([]BlockDomain, error) {
	_, err := d.db.Exec("DELETE FROM block_domain WHERE domain IS ? AND tg IS ?", domain, tg)
	if err != nil {
		return nil, err
	}
	return d.SelectByBlockDomain(domain, tg)
}

// SelectUserAllBlockDomain returns all block_domain records for a given tg user.
// SQL: SELECT * FROM block_domain WHERE tg IS $origin
func (d *DB) SelectUserAllBlockDomain(tg int64) ([]BlockDomain, error) {
	rows, err := d.db.Query("SELECT * FROM block_domain WHERE tg IS ?", tg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []BlockDomain
	for rows.Next() {
		var r BlockDomain
		if err := rows.Scan(&r.Domain, &r.Tg); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// --- Block sender operations (block_sender table) ---

// SelectByBlockSender returns block_sender records matching sender and tg.
// SQL: SELECT * FROM block_sender WHERE sender IS $sender AND tg IS $tg
func (d *DB) SelectByBlockSender(sender string, tg int64) ([]BlockSender, error) {
	rows, err := d.db.Query("SELECT * FROM block_sender WHERE sender IS ? AND tg IS ?", sender, tg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []BlockSender
	for rows.Next() {
		var r BlockSender
		if err := rows.Scan(&r.Sender, &r.Tg); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// selectByBlockSenderNullTg is an internal helper that replicates the Node.js bug
// where insertBlockSender/deleteBlockSender call selectByBlockSender(sender) with
// only one argument, making tg undefined (NULL in SQLite).
// SQL: SELECT * FROM block_sender WHERE sender IS $sender AND tg IS NULL
func (d *DB) selectByBlockSenderNullTg(sender string) ([]BlockSender, error) {
	rows, err := d.db.Query("SELECT * FROM block_sender WHERE sender IS ? AND tg IS NULL", sender)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []BlockSender
	for rows.Next() {
		var r BlockSender
		if err := rows.Scan(&r.Sender, &r.Tg); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// InsertBlockSender inserts a block_sender record.
// SQL: INSERT INTO block_sender (sender, tg) VALUES ($sender, $tg)
// Note: Node.js version calls selectByBlockSender(sender) with only sender arg
// (tg is undefined → NULL), so this replicates that behavior.
func (d *DB) InsertBlockSender(sender string, tg int64) ([]BlockSender, error) {
	_, err := d.db.Exec("INSERT INTO block_sender (sender, tg) VALUES (?, ?)", sender, tg)
	if err != nil {
		return nil, err
	}
	// Replicate Node.js bug: selectByBlockSender(sender) — tg is undefined → NULL
	return d.selectByBlockSenderNullTg(sender)
}

// DeleteBlockSender deletes a block_sender record.
// SQL: DELETE FROM block_sender WHERE sender IS $sender AND tg IS $tg
// Note: Node.js version calls selectByBlockSender(sender) with only sender arg
// (tg is undefined → NULL), so this replicates that behavior.
func (d *DB) DeleteBlockSender(sender string, tg int64) ([]BlockSender, error) {
	_, err := d.db.Exec("DELETE FROM block_sender WHERE sender IS ? AND tg IS ?", sender, tg)
	if err != nil {
		return nil, err
	}
	// Replicate Node.js bug: selectByBlockSender(sender) — tg is undefined → NULL
	return d.selectByBlockSenderNullTg(sender)
}

// SelectUserAllBlockSender returns all block_sender records for a given tg user.
// SQL: SELECT * FROM block_sender WHERE tg IS $origin
func (d *DB) SelectUserAllBlockSender(tg int64) ([]BlockSender, error) {
	rows, err := d.db.Query("SELECT * FROM block_sender WHERE tg IS ?", tg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []BlockSender
	for rows.Next() {
		var r BlockSender
		if err := rows.Scan(&r.Sender, &r.Tg); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// --- Block receiver operations (block_receiver table) ---

// SelectByBlockReceiver returns block_receiver records matching receiver and tg.
// SQL: SELECT * FROM block_receiver WHERE receiver IS $receiver AND tg IS $tg
func (d *DB) SelectByBlockReceiver(receiver string, tg int64) ([]BlockReceiver, error) {
	rows, err := d.db.Query("SELECT * FROM block_receiver WHERE receiver IS ? AND tg IS ?", receiver, tg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []BlockReceiver
	for rows.Next() {
		var r BlockReceiver
		if err := rows.Scan(&r.Receiver, &r.Tg); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// selectByBlockReceiverNullTg is an internal helper that replicates the Node.js bug
// where insertBlockReceiver/deleteBlockReceiver call selectByBlockReceiver(receiver)
// with only one argument, making tg undefined (NULL in SQLite).
// SQL: SELECT * FROM block_receiver WHERE receiver IS $receiver AND tg IS NULL
func (d *DB) selectByBlockReceiverNullTg(receiver string) ([]BlockReceiver, error) {
	rows, err := d.db.Query("SELECT * FROM block_receiver WHERE receiver IS ? AND tg IS NULL", receiver)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []BlockReceiver
	for rows.Next() {
		var r BlockReceiver
		if err := rows.Scan(&r.Receiver, &r.Tg); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// InsertBlockReceiver inserts a block_receiver record.
// SQL: INSERT INTO block_receiver (receiver, tg) VALUES ($receiver, $tg)
// Note: Node.js version calls selectByBlockReceiver(receiver) with only receiver arg
// (tg is undefined → NULL), so this replicates that behavior.
func (d *DB) InsertBlockReceiver(receiver string, tg int64) ([]BlockReceiver, error) {
	_, err := d.db.Exec("INSERT INTO block_receiver (receiver, tg) VALUES (?, ?)", receiver, tg)
	if err != nil {
		return nil, err
	}
	// Replicate Node.js bug: selectByBlockReceiver(receiver) — tg is undefined → NULL
	return d.selectByBlockReceiverNullTg(receiver)
}

// DeleteBlockReceiver deletes a block_receiver record.
// SQL: DELETE FROM block_receiver WHERE receiver IS $receiver AND tg IS $tg
// Note: Node.js version calls selectByBlockReceiver(receiver) with only receiver arg
// (tg is undefined → NULL), so this replicates that behavior.
func (d *DB) DeleteBlockReceiver(receiver string, tg int64) ([]BlockReceiver, error) {
	_, err := d.db.Exec("DELETE FROM block_receiver WHERE receiver IS ? AND tg IS ?", receiver, tg)
	if err != nil {
		return nil, err
	}
	// Replicate Node.js bug: selectByBlockReceiver(receiver) — tg is undefined → NULL
	return d.selectByBlockReceiverNullTg(receiver)
}

// SelectUserAllBlockReceiver returns all block_receiver records for a given tg user.
// SQL: SELECT * FROM block_receiver WHERE tg IS $origin
func (d *DB) SelectUserAllBlockReceiver(tg int64) ([]BlockReceiver, error) {
	rows, err := d.db.Query("SELECT * FROM block_receiver WHERE tg IS ?", tg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []BlockReceiver
	for rows.Next() {
		var r BlockReceiver
		if err := rows.Scan(&r.Receiver, &r.Tg); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// --- User settings operations (user_settings table) ---

// GetUserLang returns the stored language preference for a user.
// Returns empty string if not set.
func (d *DB) GetUserLang(tg int64) string {
	var lang string
	err := d.db.QueryRow(`SELECT lang FROM user_settings WHERE tg = ?`, tg).Scan(&lang)
	if err != nil {
		return ""
	}
	return lang
}

// SetUserLang stores the language preference for a user (upsert).
func (d *DB) SetUserLang(tg int64, lang string) error {
	_, err := d.db.Exec(`INSERT INTO user_settings (tg, lang) VALUES (?, ?) ON CONFLICT(tg) DO UPDATE SET lang = ?`, tg, lang, lang)
	return err
}
