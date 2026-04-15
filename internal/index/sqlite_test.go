package index

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenDatabase(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := OpenDatabase(dbPath)
	if err != nil {
		t.Fatalf("OpenDatabase returned error: %v", err)
	}
	defer db.Close()

	// Verify WAL mode is active
	var journalMode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("failed to query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("expected journal_mode=wal, got %q", journalMode)
	}
}

func TestMigrations(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := OpenDatabase(dbPath)
	if err != nil {
		t.Fatalf("OpenDatabase returned error: %v", err)
	}
	defer db.Close()

	// Check that all expected tables exist
	expectedTables := []string{"pages", "pages_fts", "provenance_edges", "vocabulary"}
	for _, table := range expectedTables {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type IN ('table','shadow') AND name = ?",
			table,
		).Scan(&name)
		if err != nil {
			// pages_fts is a virtual table; it appears in sqlite_master as type='table'
			// but also check sqlite_schema for older SQLite versions
			err2 := db.QueryRow(
				"SELECT name FROM sqlite_master WHERE name = ?",
				table,
			).Scan(&name)
			if err2 != nil {
				t.Errorf("table %q not found in sqlite_master: %v", table, err2)
				continue
			}
		}
		if name != table {
			t.Errorf("expected table name %q, got %q", table, name)
		}
	}
}

func TestMigratePagesFTSUnindexedPreservesContent(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "mig.db")+"?_journal_mode=wal")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE VIRTUAL TABLE pages_fts USING fts5(id, title, body, tags)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO pages_fts(id,title,body,tags) VALUES ('x','hello','world body','t1')`)
	if err != nil {
		t.Fatal(err)
	}

	if err := migratePagesFTSUnindexed(db); err != nil {
		t.Fatal(err)
	}

	var title string
	if err := db.QueryRow(`SELECT title FROM pages_fts WHERE id = 'x'`).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "hello" {
		t.Fatalf("title = %q", title)
	}

	var def string
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='pages_fts'`).Scan(&def); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(def, "id UNINDEXED") {
		t.Fatalf("expected id UNINDEXED in %q", def)
	}

	if err := migratePagesFTSUnindexed(db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestOpenDatabaseIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Open twice to verify migrations are idempotent (CREATE TABLE IF NOT EXISTS)
	db1, err := OpenDatabase(dbPath)
	if err != nil {
		t.Fatalf("first OpenDatabase returned error: %v", err)
	}
	db1.Close()

	db2, err := OpenDatabase(dbPath)
	if err != nil {
		t.Fatalf("second OpenDatabase returned error: %v", err)
	}
	db2.Close()

	// If the file was created, clean up is handled by t.TempDir
	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("database file not found after open: %v", err)
	}
}
