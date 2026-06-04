package db

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/ncruces/go-sqlite3/driver"
)

// Open writes embedded DB bytes to a temp file and opens it read-only.
func Open(embeddedDB []byte) (*sql.DB, error) {
	tmp, err := os.CreateTemp("", "cisco-api-guide-*.db")
	if err != nil {
		return nil, fmt.Errorf("create temp db: %w", err)
	}
	if _, err := tmp.Write(embeddedDB); err != nil {
		tmp.Close()
		return nil, fmt.Errorf("write temp db: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("close temp db: %w", err)
	}
	db, err := sql.Open("sqlite3", "file:"+tmp.Name()+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	return db, nil
}

// OpenRW opens (or creates) a read-write SQLite DB at path. Used by ingest tool.
func OpenRW(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "file:"+path+"?_journal=WAL")
	if err != nil {
		return nil, fmt.Errorf("open sqlite rw: %w", err)
	}
	return db, nil
}
