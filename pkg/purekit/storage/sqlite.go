package storage

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps a SQLite WAL connection inside a plugin's isolated directory.
type DB struct {
	sql  *sql.DB
	path string
}

// DBOption configures OpenSQLite.
type DBOption func(*dbOptions)

type dbOptions struct {
	schema string
}

// WithSchema executes ddl immediately after opening the database.
func WithSchema(ddl string) DBOption {
	return func(o *dbOptions) {
		o.schema = ddl
	}
}

// OpenSQLite opens (or creates) a WAL SQLite database at path with PureUtils's
// standard concurrency and durability pragmas.
func OpenSQLite(path string, opts ...DBOption) (*DB, error) {
	var cfg dbOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return nil, err
	}
	dsn := "file:" + filepath.ToSlash(abs) +
		"?_pragma=busy_timeout(2000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(0)"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(2)
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	db := &DB{sql: sqlDB, path: abs}
	if cfg.schema != "" {
		if err := db.ExecSchema(cfg.schema); err != nil {
			_ = sqlDB.Close()
			return nil, err
		}
	}
	return db, nil
}

// SQL returns the underlying *sql.DB.
func (d *DB) SQL() *sql.DB { return d.sql }

// Path returns the resolved file path.
func (d *DB) Path() string { return d.path }

// ExecSchema applies SQL schema statements.
func (d *DB) ExecSchema(ddl string) error {
	_, err := d.sql.Exec(ddl)
	return err
}

// Checkpoint flushes WAL frames back to the main database file.
func (d *DB) Checkpoint() error {
	if d == nil || d.sql == nil {
		return nil
	}
	_, err := d.sql.Exec(`PRAGMA wal_checkpoint(TRUNCATE);`)
	return err
}

// Close closes the database handle.
func (d *DB) Close() error {
	if d == nil || d.sql == nil {
		return nil
	}
	return d.sql.Close()
}
