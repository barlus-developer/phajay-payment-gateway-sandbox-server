package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/domain"
)

func uid(prefix string) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(raw[:]), nil
}

const transactionColumns = `id,project_id,order_no,kind,bank,merchant_name,amount_minor,currency,description,status,created_at,expires_at,completed_at,receipt_ref,token_hash,token_cipher,tag1,tag2,tag3`

func scanTransaction(r scanner) (t domain.Transaction, err error) {
	err = translate(r.Scan(&t.ID, &t.ProjectID, &t.OrderNo, &t.Kind, &t.Bank, &t.MerchantName, &t.AmountMinor, &t.Currency, &t.Description, &t.Status, &t.CreatedAt, &t.ExpiresAt, &t.CompletedAt, &t.ReceiptRef, &t.TokenHash, &t.TokenCipher, &t.Tag1, &t.Tag2, &t.Tag3))
	return
}
func (s *Store) CreateTransaction(ctx context.Context, t domain.Transaction, e domain.TransactionEvent) error {
	return s.atomic(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO transactions(id,project_id,order_no,kind,bank,merchant_name,amount_minor,currency,description,status,created_at,expires_at,completed_at,receipt_ref,token_hash,token_cipher,tag1,tag2,tag3) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, t.ID, t.ProjectID, t.OrderNo, t.Kind, t.Bank, t.MerchantName, t.AmountMinor, t.Currency, t.Description, t.Status, t.CreatedAt, t.ExpiresAt, t.CompletedAt, t.ReceiptRef, t.TokenHash, t.TokenCipher, t.Tag1, t.Tag2, t.Tag3)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO transaction_events(id,transaction_id,status,created_at) VALUES(?,?,?,?)`, e.ID, t.ID, e.Status, e.CreatedAt)
		return err
	})
}
func (s *Store) Transaction(ctx context.Context, project, id string) (domain.Transaction, error) {
	return scanTransaction(s.db.QueryRowContext(ctx, `SELECT `+transactionColumns+` FROM transactions WHERE project_id=? AND id=?`, project, id))
}
func (s *Store) TransactionByToken(ctx context.Context, hash string) (domain.Transaction, error) {
	return scanTransaction(s.db.QueryRowContext(ctx, `SELECT `+transactionColumns+` FROM transactions WHERE token_hash=?`, hash))
}
func (s *Store) Transactions(ctx context.Context, project string, f domain.Filter) ([]domain.Transaction, int, error) {
	var total int
	pattern := "%" + f.Search + "%"
	args := []any{project, f.Search, pattern, pattern, f.Status, f.Status, f.Bank, f.Bank, f.From, f.From, f.To, f.To}
	const where = ` WHERE project_id=? AND (?='' OR id LIKE ? OR order_no LIKE ?) AND (?='' OR status=?) AND (?='' OR bank=?) AND (?=0 OR created_at>=?) AND (?=0 OR created_at<=?)`
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM transactions`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.Limit, f.Offset)
	rows, err := s.db.QueryContext(ctx, `SELECT `+transactionColumns+` FROM transactions`+where+` ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?`, args...)
	items, err := collect(rows, err, scanTransaction)
	return items, total, err
}
func (s *Store) TransactionEvents(ctx context.Context, id string) ([]domain.TransactionEvent, error) {
	r, e := s.db.QueryContext(ctx, `SELECT id,transaction_id,status,created_at FROM transaction_events WHERE transaction_id=? ORDER BY created_at,id`, id)
	return collect(r, e, func(r scanner) (x domain.TransactionEvent, e error) {
		e = r.Scan(&x.ID, &x.TransactionID, &x.Status, &x.CreatedAt)
		return
	})
}
func (s *Store) Transition(ctx context.Context, hash string, desired domain.State, now int64) (t domain.Transaction, changed bool, err error) {
	err = s.atomic(ctx, func(tx *sql.Tx) error {
		var err error
		t, err = scanTransaction(tx.QueryRowContext(ctx, `SELECT `+transactionColumns+` FROM transactions WHERE token_hash=?`, hash))
		if err != nil {
			return err
		}
		if t.Status == domain.Completed && desired == domain.Completed {
			return nil
		}
		if t.Status != domain.Waiting {
			return domain.ErrConflict
		}
		target := desired
		if now >= t.ExpiresAt {
			target = domain.Expired
		}
		if !domain.CanTransition(t.Status, target) {
			return domain.ErrInvalid
		}
		var completed any
		if target == domain.Completed {
			completed = now
			t.CompletedAt = &now
		}
		result, err := tx.ExecContext(ctx, `UPDATE transactions SET status=?,completed_at=? WHERE id=? AND status='WAITING'`, target, completed, t.ID)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return domain.ErrConflict
		}
		t.Status = target
		history, err := uid("hist_")
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO transaction_events(id,transaction_id,status,created_at) VALUES(?,?,?,?)`, history, t.ID, target, now); err != nil {
			return err
		}
		body, err := domain.CallbackBodyAt(t, now)
		if err != nil {
			return err
		}
		eventID, err := uid("evt_")
		if err != nil {
			return err
		}
		if err = s.enqueueTx(ctx, tx, domain.OutboxEvent{ID: eventID, ProjectID: t.ProjectID, TransactionID: t.ID, Type: domain.EventType(target), Body: body, CreatedAt: now}, ""); err != nil {
			return err
		}
		changed = true
		return nil
	})
	if err == nil && t.Status == domain.Expired && desired != domain.Expired {
		err = domain.ErrExpired
	}
	return
}
func (s *Store) Expiring(ctx context.Context, now int64) ([]domain.Transaction, error) {
	r, e := s.db.QueryContext(ctx, `SELECT `+transactionColumns+` FROM transactions WHERE status='WAITING' AND expires_at<=? LIMIT 100`, now)
	return collect(r, e, scanTransaction)
}
func (s *Store) WriteLog(ctx context.Context, l domain.APILog) error {
	_, e := s.db.ExecContext(ctx, `INSERT INTO api_request_logs(id,project_id,method,path,status,duration_ms,error,created_at) VALUES(?,?,?,?,?,?,?,?)`, l.ID, l.ProjectID, l.Method, l.Path, l.Status, l.DurationMS, l.Error, l.CreatedAt)
	return e
}
func (s *Store) Logs(ctx context.Context, id string, limit, offset int) ([]domain.APILog, error) {
	r, e := s.db.QueryContext(ctx, `SELECT id,project_id,method,path,status,duration_ms,error,created_at FROM api_request_logs WHERE project_id=? ORDER BY created_at DESC LIMIT ? OFFSET ?`, id, limit, offset)
	return collect(r, e, func(r scanner) (l domain.APILog, e error) {
		e = r.Scan(&l.ID, &l.ProjectID, &l.Method, &l.Path, &l.Status, &l.DurationMS, &l.Error, &l.CreatedAt)
		return
	})
}
