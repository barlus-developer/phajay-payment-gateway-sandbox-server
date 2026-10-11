package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/domain"
)

func (s *Store) CreateDeveloper(ctx context.Context, d domain.Developer) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO developers(id,email,name,password_hash,created_at) VALUES(?,?,?,?,?)`, d.ID, d.Email, d.Name, d.PasswordHash, d.CreatedAt)
	return translate(err)
}
func scanDeveloper(r scanner) (d domain.Developer, err error) {
	err = translate(r.Scan(&d.ID, &d.Email, &d.Name, &d.PasswordHash, &d.CreatedAt))
	return
}
func (s *Store) DeveloperByEmail(ctx context.Context, email string) (domain.Developer, error) {
	return scanDeveloper(s.db.QueryRowContext(ctx, `SELECT id,email,name,password_hash,created_at FROM developers WHERE email=?`, email))
}
func (s *Store) DeveloperByID(ctx context.Context, id string) (domain.Developer, error) {
	return scanDeveloper(s.db.QueryRowContext(ctx, `SELECT id,email,name,password_hash,created_at FROM developers WHERE id=?`, id))
}
func (s *Store) UpdateDeveloper(ctx context.Context, id, name string) error {
	return affected(s.db.ExecContext(ctx, `UPDATE developers SET name=? WHERE id=?`, name, id))
}
func (s *Store) ChangePassword(ctx context.Context, id, hash, keepSession string) error {
	return s.atomic(ctx, func(tx *sql.Tx) error {
		if err := affected(tx.ExecContext(ctx, `UPDATE developers SET password_hash=? WHERE id=?`, hash, id)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM developer_sessions WHERE developer_id=? AND id!=?`, id, keepSession)
		return err
	})
}
func (s *Store) CreateSession(ctx context.Context, x domain.Session) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO developer_sessions(id,developer_id,token_hash,csrf_hash,created_at,expires_at) VALUES(?,?,?,?,?,?)`, x.ID, x.DeveloperID, x.TokenHash, x.CSRFHash, x.CreatedAt, x.ExpiresAt)
	return err
}
func scanSession(r scanner) (x domain.Session, err error) {
	err = translate(r.Scan(&x.ID, &x.DeveloperID, &x.TokenHash, &x.CSRFHash, &x.CreatedAt, &x.ExpiresAt))
	return
}
func (s *Store) Session(ctx context.Context, hash string, now int64) (domain.Session, error) {
	return scanSession(s.db.QueryRowContext(ctx, `SELECT id,developer_id,token_hash,csrf_hash,created_at,expires_at FROM developer_sessions WHERE token_hash=? AND expires_at>?`, hash, now))
}
func (s *Store) Sessions(ctx context.Context, id string, now int64) ([]domain.Session, error) {
	r, e := s.db.QueryContext(ctx, `SELECT id,developer_id,token_hash,csrf_hash,created_at,expires_at FROM developer_sessions WHERE developer_id=? AND expires_at>? ORDER BY created_at DESC`, id, now)
	return collect(r, e, scanSession)
}
func (s *Store) RevokeSession(ctx context.Context, id, developer string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM developer_sessions WHERE id=? AND developer_id=?`, id, developer))
}
func scanProject(r scanner) (p domain.Project, err error) {
	err = translate(r.Scan(&p.ID, &p.DeveloperID, &p.Name, &p.MerchantName, &p.Description, &p.PaymentTTL, &p.WebhookMaxAttempts, &p.CreatedAt))
	return
}

const projectColumns = "id,developer_id,name,merchant_name,description,payment_ttl,webhook_max_attempts,created_at"

func (s *Store) CreateProject(ctx context.Context, p domain.Project) error {
	_, e := s.db.ExecContext(ctx, `INSERT INTO projects(id,developer_id,name,merchant_name,description,payment_ttl,webhook_max_attempts,created_at) VALUES(?,?,?,?,?,?,?,?)`, p.ID, p.DeveloperID, p.Name, p.MerchantName, p.Description, p.PaymentTTL, p.WebhookMaxAttempts, p.CreatedAt)
	return translate(e)
}
func (s *Store) Projects(ctx context.Context, id string) ([]domain.Project, error) {
	r, e := s.db.QueryContext(ctx, `SELECT `+projectColumns+` FROM projects WHERE developer_id=? ORDER BY created_at DESC`, id)
	return collect(r, e, scanProject)
}
func (s *Store) Project(ctx context.Context, id, owner string) (domain.Project, error) {
	return scanProject(s.db.QueryRowContext(ctx, `SELECT `+projectColumns+` FROM projects WHERE id=? AND developer_id=?`, id, owner))
}
func (s *Store) UpdateProject(ctx context.Context, p domain.Project) error {
	return affected(s.db.ExecContext(ctx, `UPDATE projects SET name=?,merchant_name=?,description=?,payment_ttl=?,webhook_max_attempts=? WHERE id=? AND developer_id=?`, p.Name, p.MerchantName, p.Description, p.PaymentTTL, p.WebhookMaxAttempts, p.ID, p.DeveloperID))
}
func scanAPIKey(r scanner) (k domain.APIKey, e error) {
	e = translate(r.Scan(&k.ID, &k.ProjectID, &k.Name, &k.Prefix, &k.Hash, &k.CreatedAt, &k.LastUsedAt, &k.RevokedAt))
	return
}
func (s *Store) CreateAPIKey(ctx context.Context, k domain.APIKey, rotate string) error {
	return s.atomic(ctx, func(tx *sql.Tx) error {
		if rotate != "" {
			if err := affected(tx.ExecContext(ctx, `UPDATE api_keys SET revoked_at=? WHERE id=? AND project_id=? AND revoked_at IS NULL`, k.CreatedAt, rotate, k.ProjectID)); err != nil {
				return err
			}
		}
		_, e := tx.ExecContext(ctx, `INSERT INTO api_keys(id,project_id,name,prefix,key_hash,created_at) VALUES(?,?,?,?,?,?)`, k.ID, k.ProjectID, k.Name, k.Prefix, k.Hash, k.CreatedAt)
		return e
	})
}
func (s *Store) APIKeys(ctx context.Context, id string) ([]domain.APIKey, error) {
	r, e := s.db.QueryContext(ctx, `SELECT id,project_id,name,prefix,key_hash,created_at,last_used_at,revoked_at FROM api_keys WHERE project_id=? ORDER BY created_at DESC`, id)
	return collect(r, e, scanAPIKey)
}
func (s *Store) AuthenticateKey(ctx context.Context, hash string, now int64) (p domain.Project, err error) {
	err = s.atomic(ctx, func(tx *sql.Tx) error {
		var project string
		if err := tx.QueryRowContext(ctx, `SELECT project_id FROM api_keys WHERE key_hash=? AND revoked_at IS NULL`, hash).Scan(&project); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE api_keys SET last_used_at=? WHERE key_hash=?`, now, hash); err != nil {
			return err
		}
		var err error
		p, err = scanProject(tx.QueryRowContext(ctx, `SELECT `+projectColumns+` FROM projects WHERE id=?`, project))
		return err
	})
	return
}
func (s *Store) RevokeAPIKey(ctx context.Context, project, id string, now int64) error {
	return affected(s.db.ExecContext(ctx, `UPDATE api_keys SET revoked_at=COALESCE(revoked_at,?) WHERE id=? AND project_id=?`, now, id, project))
}
func scanSigningKey(r scanner) (k domain.SigningKey, e error) {
	e = translate(r.Scan(&k.ID, &k.ProjectID, &k.Prefix, &k.Cipher, &k.Active, &k.CreatedAt, &k.ValidUntil, &k.RevokedAt))
	return
}
func (s *Store) RotateSigningKey(ctx context.Context, k domain.SigningKey, overlap int64) error {
	return s.atomic(ctx, func(tx *sql.Tx) error {
		if _, e := tx.ExecContext(ctx, `UPDATE webhook_signing_keys SET active=0,valid_until=? WHERE project_id=? AND active=1 AND revoked_at IS NULL`, k.CreatedAt+overlap, k.ProjectID); e != nil {
			return e
		}
		_, e := tx.ExecContext(ctx, `INSERT INTO webhook_signing_keys(id,project_id,prefix,cipher,active,created_at) VALUES(?,?,?,?,1,?)`, k.ID, k.ProjectID, k.Prefix, k.Cipher, k.CreatedAt)
		return e
	})
}
func (s *Store) SigningKeys(ctx context.Context, id string) ([]domain.SigningKey, error) {
	r, e := s.db.QueryContext(ctx, `SELECT id,project_id,prefix,cipher,active,created_at,valid_until,revoked_at FROM webhook_signing_keys WHERE project_id=? ORDER BY created_at DESC`, id)
	return collect(r, e, scanSigningKey)
}
func (s *Store) ActiveSigningKey(ctx context.Context, id string) (domain.SigningKey, error) {
	return scanSigningKey(s.db.QueryRowContext(ctx, `SELECT id,project_id,prefix,cipher,active,created_at,valid_until,revoked_at FROM webhook_signing_keys WHERE project_id=? AND active=1 AND revoked_at IS NULL`, id))
}
func (s *Store) RevokeSigningKey(ctx context.Context, project, id string, now int64) error {
	return affected(s.db.ExecContext(ctx, `UPDATE webhook_signing_keys SET active=0,revoked_at=COALESCE(revoked_at,?),valid_until=? WHERE project_id=? AND id=?`, now, now, project, id))
}
func scanEndpoint(r scanner) (x domain.Endpoint, err error) {
	var events string
	err = translate(r.Scan(&x.ID, &x.ProjectID, &x.URL, &x.Enabled, &events, &x.CreatedAt))
	if err == nil {
		err = json.Unmarshal([]byte(events), &x.Events)
	}
	return
}
func (s *Store) SaveEndpoint(ctx context.Context, x domain.Endpoint) error {
	events, e := json.Marshal(x.Events)
	if e != nil {
		return e
	}
	return affected(s.db.ExecContext(ctx, `INSERT INTO webhook_endpoints(id,project_id,url,enabled,events,created_at) VALUES(?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET url=excluded.url,enabled=excluded.enabled,events=excluded.events WHERE webhook_endpoints.project_id=excluded.project_id`, x.ID, x.ProjectID, x.URL, x.Enabled, string(events), x.CreatedAt))
}
func (s *Store) Endpoints(ctx context.Context, id string) ([]domain.Endpoint, error) {
	r, e := s.db.QueryContext(ctx, `SELECT id,project_id,url,enabled,events,created_at FROM webhook_endpoints WHERE project_id=? ORDER BY created_at DESC`, id)
	return collect(r, e, scanEndpoint)
}
func (s *Store) DeleteEndpoint(ctx context.Context, project, id string) error {
	return affected(s.db.ExecContext(ctx, `UPDATE webhook_endpoints SET enabled=0 WHERE project_id=? AND id=?`, project, id))
}
