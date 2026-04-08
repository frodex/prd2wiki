package index

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

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
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS pages_fts USING fts5(
			id, title, body, tags
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

	return nil
}
