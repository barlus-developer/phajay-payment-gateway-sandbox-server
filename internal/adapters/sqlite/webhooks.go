package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/domain"
)

func (s *Store) Enqueue(ctx context.Context, e domain.OutboxEvent, endpoint string) error {
	return s.atomic(ctx, func(tx *sql.Tx) error { return s.enqueueTx(ctx, tx, e, endpoint) })
}
func (s *Store) enqueueTx(ctx context.Context, tx *sql.Tx, e domain.OutboxEvent, endpoint string) error {
	var transaction any
	if e.TransactionID != "" {
		transaction = e.TransactionID
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO webhook_events(id,project_id,transaction_id,type,body,created_at) VALUES(?,?,?,?,?,?)`, e.ID, e.ProjectID, transaction, e.Type, e.Body, e.CreatedAt); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,events FROM webhook_endpoints WHERE project_id=? AND enabled=1 AND (?='' OR id=?)`, e.ProjectID, endpoint, endpoint)
	if err != nil {
		return err
	}
	var endpoints []string
	for rows.Next() {
		var id, raw string
		if err = rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return err
		}
		var types []string
		if err = json.Unmarshal([]byte(raw), &types); err != nil {
			rows.Close()
			return err
		}
		for _, t := range types {
			if t == e.Type || e.Type == "payment.test" {
				endpoints = append(endpoints, id)
				break
			}
		}
	}
	err = rows.Err()
	closeErr := rows.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	var max int
	if err = tx.QueryRowContext(ctx, `SELECT webhook_max_attempts FROM projects WHERE id=?`, e.ProjectID).Scan(&max); err != nil {
		return err
	}
	for _, endpoint := range endpoints {
		id, err := uid("job_")
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO webhook_jobs(id,event_id,endpoint_id,status,max_attempts,next_attempt_at,created_at) VALUES(?,?,?,'PENDING',?,?,?)`, id, e.ID, endpoint, max, e.CreatedAt, e.CreatedAt); err != nil {
			return err
		}
	}
	return nil
}

const jobColumns = `j.id,j.event_id,j.endpoint_id,e.project_id,w.url,e.body,j.status,j.attempts,j.max_attempts,j.next_attempt_at,j.lease_until,j.created_at`

func scanJob(r scanner) (j domain.Job, e error) {
	e = translate(r.Scan(&j.ID, &j.EventID, &j.EndpointID, &j.ProjectID, &j.URL, &j.Body, &j.Status, &j.Attempts, &j.MaxAttempts, &j.NextAttemptAt, &j.LeaseUntil, &j.CreatedAt))
	return
}
func (s *Store) ClaimJob(ctx context.Context, now, lease int64) (job domain.Job, err error) {
	err = s.atomic(ctx, func(tx *sql.Tx) error {
		// Disabled endpoints never deliver. Leases make interrupted jobs recoverable.
		if _, err := tx.ExecContext(ctx, `UPDATE webhook_jobs SET status='DISABLED' WHERE status IN('PENDING','DELIVERING') AND endpoint_id IN(SELECT id FROM webhook_endpoints WHERE enabled=0)`); err != nil {
			return err
		}
		var err error
		job, err = scanJob(tx.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM webhook_jobs j JOIN webhook_events e ON e.id=j.event_id JOIN webhook_endpoints w ON w.id=j.endpoint_id WHERE w.enabled=1 AND ((j.status='PENDING' AND j.next_attempt_at<=?) OR (j.status='DELIVERING' AND j.lease_until<=?)) ORDER BY j.next_attempt_at,j.id LIMIT 1`, now, now))
		if err != nil {
			return err
		}
		job.LeaseUntil = now + lease
		job.Attempts++
		job.Status = "DELIVERING"
		return affected(tx.ExecContext(ctx, `UPDATE webhook_jobs SET status='DELIVERING',attempts=?,lease_until=? WHERE id=?`, job.Attempts, job.LeaseUntil, job.ID))
	})
	return
}
func (s *Store) FinishJob(ctx context.Context, j domain.Job, d domain.Delivery, success bool, next int64) error {
	return s.atomic(ctx, func(tx *sql.Tx) error {
		// Fence stale workers after a job has been reclaimed.
		state := "PENDING"
		if success {
			state = "DELIVERED"
		} else if j.Attempts >= j.MaxAttempts {
			state = "FAILED"
		}
		result, err := tx.ExecContext(ctx, `UPDATE webhook_jobs SET status=?,next_attempt_at=?,lease_until=0 WHERE id=? AND status='DELIVERING' AND attempts=? AND lease_until=?`, state, next, j.ID, j.Attempts, j.LeaseUntil)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrConflict
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO webhook_deliveries(id,job_id,event_id,endpoint_id,attempt,http_status,response,error,duration_ms,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, d.ID, j.ID, j.EventID, j.EndpointID, j.Attempts, d.HTTPStatus, d.Response, d.Error, d.DurationMS, d.CreatedAt)
		return err
	})
}
func (s *Store) Jobs(ctx context.Context, project string, limit, offset int) ([]domain.Job, error) {
	r, e := s.db.QueryContext(ctx, `SELECT `+jobColumns+` FROM webhook_jobs j JOIN webhook_events e ON e.id=j.event_id JOIN webhook_endpoints w ON w.id=j.endpoint_id WHERE e.project_id=? ORDER BY j.created_at DESC LIMIT ? OFFSET ?`, project, limit, offset)
	return collect(r, e, scanJob)
}
func (s *Store) Deliveries(ctx context.Context, project, job string) ([]domain.Delivery, error) {
	r, e := s.db.QueryContext(ctx, `SELECT d.id,d.job_id,d.event_id,d.endpoint_id,d.attempt,d.http_status,d.response,d.error,d.duration_ms,d.created_at FROM webhook_deliveries d JOIN webhook_events e ON e.id=d.event_id WHERE e.project_id=? AND d.job_id=? ORDER BY d.attempt`, project, job)
	return collect(r, e, func(r scanner) (d domain.Delivery, e error) {
		e = r.Scan(&d.ID, &d.JobID, &d.EventID, &d.EndpointID, &d.Attempt, &d.HTTPStatus, &d.Response, &d.Error, &d.DurationMS, &d.CreatedAt)
		return
	})
}
func (s *Store) RetryJob(ctx context.Context, project, job string, now int64) error {
	return affected(s.db.ExecContext(ctx, `UPDATE webhook_jobs SET status='PENDING',next_attempt_at=?,max_attempts=attempts+3,lease_until=0 WHERE id=? AND status IN('FAILED','DELIVERED','DISABLED') AND event_id IN(SELECT id FROM webhook_events WHERE project_id=?) AND endpoint_id IN(SELECT id FROM webhook_endpoints WHERE enabled=1)`, now, job, project))
}
