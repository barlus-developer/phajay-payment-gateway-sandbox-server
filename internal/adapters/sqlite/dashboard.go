package sqlite

import (
	"context"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/domain"
)

func (s *Store) Dashboard(ctx context.Context, project string, since int64) (domain.Dashboard, error) {
	var d domain.Dashboard
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(status='PAYMENT_COMPLETED'),0),COALESCE(SUM(status='WAITING'),0),COALESCE(SUM(status IN ('FAILED','CANCELED','EXPIRED')),0),COALESCE(SUM(CASE WHEN status='PAYMENT_COMPLETED' THEN amount_minor ELSE 0 END),0) FROM transactions WHERE project_id=?`, project).Scan(&d.Total, &d.Completed, &d.Pending, &d.Failed, &d.VolumeMinor)
	if err != nil {
		return d, err
	}
	err = s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(j.status='DELIVERED'),0),COALESCE(SUM(j.status='FAILED'),0) FROM webhook_jobs j JOIN webhook_events e ON e.id=j.event_id WHERE e.project_id=?`, project).Scan(&d.WebhookDelivered, &d.WebhookFailed)
	if err != nil {
		return d, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT strftime('%Y-%m-%d',created_at,'unixepoch'),COUNT(*),SUM(status='PAYMENT_COMPLETED') FROM transactions WHERE project_id=? AND created_at>=? GROUP BY 1 ORDER BY 1`, project, since)
	d.Activity, err = collect(rows, err, func(r scanner) (domain.Activity, error) {
		var a domain.Activity
		e := r.Scan(&a.Day, &a.Total, &a.Completed)
		return a, e
	})
	return d, err
}
