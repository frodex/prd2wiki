package index

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

func migratePagesFTSUnindexed(db *sql.DB) error {
	var sqlDef sql.NullString
	err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='pages_fts'`).Scan(&sqlDef)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read pages_fts schema: %w", err)
	}
	if !sqlDef.Valid || strings.Contains(sqlDef.String, "id UNINDEXED") {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin fts migration: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`CREATE TABLE _pages_fts_migrate (id TEXT, title TEXT, body TEXT, tags TEXT)`); err != nil {
		return fmt.Errorf("create fts backup table: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO _pages_fts_migrate SELECT id, title, body, tags FROM pages_fts`); err != nil {
		return fmt.Errorf("backup pages_fts: %w", err)
	}
	if _, err := tx.Exec(`DROP TABLE pages_fts`); err != nil {
		return fmt.Errorf("drop old pages_fts: %w", err)
	}
	if _, err := tx.Exec(`CREATE VIRTUAL TABLE pages_fts USING fts5(
			id UNINDEXED, title, body, tags
		)`); err != nil {
		return fmt.Errorf("create pages_fts (id UNINDEXED): %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO pages_fts (id, title, body, tags) SELECT id, title, body, tags FROM _pages_fts_migrate`); err != nil {
		return fmt.Errorf("restore pages_fts: %w", err)
	}
	if _, err := tx.Exec(`DROP TABLE _pages_fts_migrate`); err != nil {
		return fmt.Errorf("drop fts backup: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit fts migration: %w", err)
	}
	return nil
}

// OpenDatabase opens a SQLite database with WAL mode and runs migrations.
func OpenDatabase(path string) (*sql.DB, error) {
	dsn := path + "?_journal_mode=wal&_busy_timeout=5000"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Force WAL mode
	if _, err := db.Exec("PRAGMA journal_mode=wal"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set wal mode: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

func migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS pages (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			type TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'draft',
			path TEXT NOT NULL,
			project TEXT NOT NULL,
			branch TEXT NOT NULL DEFAULT 'truth',
			trust_level INTEGER DEFAULT 0,
			conformance TEXT DEFAULT 'pending',
			dc_creator TEXT,
			dc_created TEXT,
			dc_modified TEXT,
			supersedes TEXT,
			superseded_by TEXT,
			contested_by TEXT,
			tags TEXT,
			module TEXT,
			category TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS pages_fts USING fts5(
			id UNINDEXED, title, body, tags
		)`,
		`CREATE TABLE IF NOT EXISTS provenance_edges (
			source_page TEXT NOT NULL,
			target_ref TEXT NOT NULL,
			target_version INTEGER,
			target_checksum TEXT,
			status TEXT DEFAULT 'valid',
			PRIMARY KEY (source_page, target_ref)
		)`,
		`CREATE TABLE IF NOT EXISTS vocabulary (
			term TEXT PRIMARY KEY,
			category TEXT NOT NULL,
			usage_count INTEGER DEFAULT 1,
			canonical INTEGER DEFAULT 1,
			aliases TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_pages_project ON pages(project)`,
		`CREATE INDEX IF NOT EXISTS idx_pages_type ON pages(type)`,
		`CREATE INDEX IF NOT EXISTS idx_pages_status ON pages(status)`,
		`CREATE INDEX IF NOT EXISTS idx_provenance_target ON provenance_edges(target_ref)`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec migration %q: %w", stmt[:40], err)
		}
	}

	// Idempotent column additions for existing databases.
	// These are already in the CREATE TABLE for new databases.
	alterStmts := []string{
		`ALTER TABLE pages ADD COLUMN module TEXT`,
		`ALTER TABLE pages ADD COLUMN category TEXT`,
		`ALTER TABLE pages ADD COLUMN inlink_count INTEGER DEFAULT 0`,
		`ALTER TABLE pages ADD COLUMN outlink_count INTEGER DEFAULT 0`,
	}
	for _, stmt := range alterStmts {
		if _, err := db.Exec(stmt); err != nil {
			// Ignore "duplicate column" errors — column already exists.
			if !strings.Contains(err.Error(), "duplicate column") {
				return fmt.Errorf("exec migration %q: %w", stmt[:40], err)
			}
		}
	}

	// Index on module — must run after ALTER TABLE for existing databases.
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_pages_module ON pages(module)`); err != nil {
		return fmt.Errorf("create idx_pages_module: %w", err)
	}

	if err := migratePagesFTSUnindexed(db); err != nil {
		return err
	}

	return nil
}
