package sqlite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/domain"
)

func TestMigrationPersistencePragmasAndBackup(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.db")
	s, e := Open(ctx, path, "../../../migrations")
	if e != nil {
		t.Fatal(e)
	}
	for _, check := range []struct{ query, want string }{{"PRAGMA journal_mode", "wal"}, {"PRAGMA foreign_keys", "1"}, {"PRAGMA busy_timeout", "5000"}} {
		var value string
		if e = s.db.QueryRowContext(ctx, check.query).Scan(&value); e != nil || value != check.want {
			t.Fatal(check, value, e)
		}
	}
	if e = s.CreateDeveloper(ctx, domain.Developer{ID: "dev", Email: "test@example.test", Name: "Name", PasswordHash: "hash", CreatedAt: 1}); e != nil {
		t.Fatal(e)
	}
	if e = s.CreateProject(ctx, domain.Project{ID: "project", DeveloperID: "missing", Name: "Bad", MerchantName: "Bad", PaymentTTL: 60, WebhookMaxAttempts: 3}); e == nil {
		t.Fatal("foreign key disabled")
	}
	backup := filepath.Join(dir, "backup.db")
	if e = s.Backup(ctx, backup); e != nil {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s, e = Open(ctx, path, "../../../migrations")
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if _, e = s.DeveloperByID(ctx, "dev"); e != nil {
		t.Fatal("restart lost developer", e)
	}
	snapshot, e := Open(ctx, backup, "../../../migrations")
	if e != nil {
		t.Fatal(e)
	}
	defer snapshot.Close()
	if _, e = snapshot.DeveloperByID(ctx, "dev"); e != nil {
		t.Fatal("backup incomplete", e)
	}
}

func TestAtomicOutboxRollbackAndDatabaseStateGuard(t *testing.T) {
	ctx := context.Background()
	s, e := Open(ctx, filepath.Join(t.TempDir(), "app.db"), "../../../migrations")
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.CreateDeveloper(ctx, domain.Developer{ID: "dev", Email: "dev@example.test", Name: "Developer", PasswordHash: "hash", CreatedAt: 1}); e != nil {
		t.Fatal(e)
	}
	if e = s.CreateProject(ctx, domain.Project{ID: "project", DeveloperID: "dev", Name: "Project", MerchantName: "Merchant", PaymentTTL: 60, WebhookMaxAttempts: 3, CreatedAt: 1}); e != nil {
		t.Fatal(e)
	}
	tx := domain.Transaction{ID: "txn", ProjectID: "project", OrderNo: "order", Kind: "QR", Bank: "JDB", MerchantName: "Merchant", AmountMinor: 100, Currency: "LAK", Description: "Test", Status: domain.Waiting, CreatedAt: 1, ExpiresAt: 100, TokenHash: "hash", TokenCipher: []byte{1}, ReceiptRef: "receipt"}
	if e = s.CreateTransaction(ctx, tx, domain.TransactionEvent{ID: "initial", TransactionID: tx.ID, Status: domain.Waiting, CreatedAt: 1}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.db.ExecContext(ctx, `CREATE TRIGGER reject_outbox BEFORE INSERT ON webhook_events BEGIN SELECT RAISE(ABORT,'test outbox failure'); END`); e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.Transition(ctx, tx.TokenHash, domain.Completed, 2); e == nil {
		t.Fatal("failed outbox committed")
	}
	current, e := s.Transaction(ctx, "project", "txn")
	if e != nil || current.Status != domain.Waiting || current.CompletedAt != nil {
		t.Fatal("state not rolled back", current, e)
	}
	history, e := s.TransactionEvents(ctx, tx.ID)
	if e != nil || len(history) != 1 {
		t.Fatal("history not rolled back", history, e)
	}
	if _, e = s.db.ExecContext(ctx, `DROP TRIGGER reject_outbox`); e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.Transition(ctx, tx.TokenHash, domain.Completed, 2); e != nil {
		t.Fatal(e)
	}
	if _, e = s.db.ExecContext(ctx, `UPDATE transactions SET status='FAILED',completed_at=NULL WHERE id=?`, tx.ID); !errors.Is(translate(e), domain.ErrConflict) {
		t.Fatal("database terminal guard failed", e)
	}
	// Recycling connections must retain connection-local safety pragmas.
	s.db.SetMaxIdleConns(0)
	for i := 0; i < 2; i++ {
		var enabled int
		if e = s.db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); e != nil || enabled != 1 {
			t.Fatal("reopened connection lost foreign keys", e)
		}
	}
}
func TestChangedMigrationFailsAndPendingMigrationRollsBack(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	migrations := filepath.Join(dir, "migrations")
	if e := os.Mkdir(migrations, 0700); e != nil {
		t.Fatal(e)
	}
	initial := filepath.Join(migrations, "001.sql")
	if e := os.WriteFile(initial, []byte("CREATE TABLE example(id TEXT PRIMARY KEY);"), 0600); e != nil {
		t.Fatal(e)
	}
	dbpath := filepath.Join(dir, "app.db")
	s, e := Open(ctx, dbpath, migrations)
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	if e := os.WriteFile(initial, []byte("CREATE TABLE changed(id TEXT PRIMARY KEY);"), 0600); e != nil {
		t.Fatal(e)
	}
	if s, e = Open(ctx, dbpath, migrations); e == nil {
		s.Close()
		t.Fatal("changed applied migration accepted")
	}
	os.WriteFile(initial, []byte("CREATE TABLE example(id TEXT PRIMARY KEY);"), 0600)
	bad := filepath.Join(migrations, "002.sql")
	os.WriteFile(bad, []byte("CREATE TABLE rollback_test(id TEXT); INVALID SQL;"), 0600)
	if s, e = Open(ctx, dbpath, migrations); e == nil {
		s.Close()
		t.Fatal("invalid migration applied")
	}
	os.Remove(bad)
	s, e = Open(ctx, dbpath, migrations)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	var count int
	if e = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE name='rollback_test'").Scan(&count); e != nil || count != 0 {
		t.Fatal("partial migration persisted", count, e)
	}
}
