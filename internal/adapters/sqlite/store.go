// Package sqlite implements persistence with handwritten database/sql statements.
package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/domain"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/ports"
	modern "modernc.org/sqlite"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Store struct{ db *sql.DB }

var _ ports.Repository = (*Store)(nil)

func Open(ctx context.Context, path, migrations string) (*Store, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(absolute), 0750); err != nil {
		return nil, err
	}
	// Establish private permissions before SQLite creates its WAL/SHM files.
	file, err := os.OpenFile(absolute, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = file.Close(); err != nil {
		return nil, err
	}
	if err = os.Chmod(absolute, 0600); err != nil {
		return nil, err
	}
	dsn := (&url.URL{Scheme: "file", Path: absolute}).String() + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &Store{db: db}
	if err = db.PingContext(ctx); err == nil {
		err = s.migrate(ctx, migrations)
	}
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	if err = os.Chmod(absolute, 0600); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) Close() error                   { return s.db.Close() }
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }
func (s *Store) Backup(ctx context.Context, path string) error {
	_, err := s.db.ExecContext(ctx, "VACUUM INTO ?", path)
	if err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}
func (s *Store) migrate(ctx context.Context, dir string) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(name TEXT PRIMARY KEY,checksum TEXT NOT NULL,applied_at INTEGER NOT NULL)`); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return err
		}
		hash := sha256.Sum256(body)
		checksum := hex.EncodeToString(hash[:])
		var applied string
		err = s.db.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE name=?", entry.Name()).Scan(&applied)
		if err == nil {
			if applied != checksum {
				return fmt.Errorf("applied migration %s was changed", entry.Name())
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		err = s.atomic(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, string(body)); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations(name,checksum,applied_at) VALUES(?,?,?)", entry.Name(), checksum, time.Now().Unix())
			return err
		})
		if err != nil {
			return fmt.Errorf("migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}
func (s *Store) atomic(ctx context.Context, work func(*sql.Tx) error) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if rollback := tx.Rollback(); rollback != nil && !errors.Is(rollback, sql.ErrTxDone) {
			err = errors.Join(err, rollback)
		}
	}()
	if err = work(tx); err != nil {
		return translate(err)
	}
	return translate(tx.Commit())
}
func translate(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	var se *modern.Error
	if errors.As(err, &se) && se.Code()&255 == 19 {
		return domain.ErrConflict
	}
	return err
}

type scanner interface{ Scan(...any) error }

func collect[T any](rows *sql.Rows, err error, scan func(scanner) (T, error)) ([]T, error) {
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()
	out := make([]T, 0)
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
func affected(result sql.Result, err error) error {
	if err != nil {
		return translate(err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}
