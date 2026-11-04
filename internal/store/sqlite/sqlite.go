// Package sqlite implements the store on an embedded SQLite database.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"net/url"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

//go:embed migrations/*.sql
var migrations embed.FS

// DB is a transponder store backed by one SQLite file.
type DB struct {
	sql *sql.DB
}

// Open opens or creates the database at path and applies pending migrations.
func Open(ctx context.Context, path string) (*DB, error) {
	dsn := "file:" + url.PathEscape(path) + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite: %w", err)
	}
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrate(ctx, db, sub); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &DB{sql: db}, nil
}

// Ping checks that the database answers.
func (db *DB) Ping(ctx context.Context) error {
	return db.sql.PingContext(ctx)
}

// Close closes the database.
func (db *DB) Close() error {
	return db.sql.Close()
}
