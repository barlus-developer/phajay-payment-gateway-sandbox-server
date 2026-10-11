// Package service implements sandbox business operations without HTTP dependencies.
package service

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/config"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/domain"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/ports"
	"net/mail"
	"strings"
	"sync"
	"time"
)

type Platform struct {
	repo          ports.Repository
	cfg           config.Config
	vault         *Vault
	dummyPassword string
	now           func() time.Time
	mu            sync.Mutex
	subscribers   map[string]map[chan domain.Transaction]struct{}
}

func New(repo ports.Repository, cfg config.Config) (*Platform, error) {
	v, e := NewVault(cfg.MasterKey)
	if e != nil {
		return nil, e
	}
	dummy, e := PasswordHash("constant-time-unregistered-account-placeholder")
	if e != nil {
		return nil, e
	}
	return &Platform{repo: repo, cfg: cfg, vault: v, dummyPassword: dummy, now: time.Now, subscribers: make(map[string]map[chan domain.Transaction]struct{})}, nil
}
func (p *Platform) Ping(ctx context.Context) error { return p.repo.Ping(ctx) }
func (p *Platform) Register(ctx context.Context, email, name, password string) (domain.Developer, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	address, e := mail.ParseAddress(email)
	if e != nil || address.Address != email || len(email) > 254 || strings.TrimSpace(name) == "" || len(name) > 100 || len(password) < 12 || len(password) > 128 {
		return domain.Developer{}, domain.ErrInvalid
	}
	hash, e := PasswordHash(password)
	if e != nil {
		return domain.Developer{}, e
	}
	id, e := Random("dev_", 16)
	if e != nil {
		return domain.Developer{}, e
	}
	d := domain.Developer{ID: id, Email: email, Name: strings.TrimSpace(name), PasswordHash: hash, CreatedAt: p.now().Unix()}
	e = p.repo.CreateDeveloper(ctx, d)
	if errors.Is(e, domain.ErrConflict) {
		e = domain.ErrInvalid
	}
	d.PasswordHash = ""
	return d, e
}
func (p *Platform) Login(ctx context.Context, email, password string) (domain.Developer, domain.Session, string, string, error) {
	if len(email) > 254 || len(password) > 128 {
		return domain.Developer{}, domain.Session{}, "", "", domain.ErrUnauthorized
	}
	d, err := p.repo.DeveloperByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	encoded := p.dummyPassword
	if err == nil {
		encoded = d.PasswordHash
	}
	match := PasswordMatches(encoded, password)
	if err != nil || !match {
		return domain.Developer{}, domain.Session{}, "", "", domain.ErrUnauthorized
	}
	token, e := Random("ses_", 32)
	if e != nil {
		return d, domain.Session{}, "", "", e
	}
	csrf, e := Random("csrf_", 32)
	if e != nil {
		return d, domain.Session{}, "", "", e
	}
	id, e := Random("sess_", 16)
	if e != nil {
		return d, domain.Session{}, "", "", e
	}
	now := p.now().Unix()
	session := domain.Session{ID: id, DeveloperID: d.ID, TokenHash: Hash(token), CSRFHash: Hash(csrf), CreatedAt: now, ExpiresAt: now + int64(p.cfg.SessionTTL/time.Second)}
	if e = p.repo.CreateSession(ctx, session); e != nil {
		return d, session, "", "", e
	}
	d.PasswordHash = ""
	return d, session, token, csrf, nil
}
func (p *Platform) Authenticate(ctx context.Context, token string) (domain.Developer, domain.Session, error) {
	s, e := p.repo.Session(ctx, Hash(token), p.now().Unix())
	if e != nil {
		return domain.Developer{}, s, domain.ErrUnauthorized
	}
	d, e := p.repo.DeveloperByID(ctx, s.DeveloperID)
	d.PasswordHash = ""
	return d, s, e
}
func (p *Platform) ValidateCSRF(s domain.Session, token string) bool {
	return token != "" && subtle.ConstantTimeCompare([]byte(s.CSRFHash), []byte(Hash(token))) == 1
}
func (p *Platform) Logout(ctx context.Context, s domain.Session) error {
	return p.repo.RevokeSession(ctx, s.ID, s.DeveloperID)
}
func (p *Platform) Sessions(ctx context.Context, s domain.Session) ([]domain.Session, error) {
	items, e := p.repo.Sessions(ctx, s.DeveloperID, p.now().Unix())
	for i := range items {
		items[i].Current = items[i].ID == s.ID
	}
	return items, e
}
func (p *Platform) RevokeSession(ctx context.Context, s domain.Session, id string) error {
	return p.repo.RevokeSession(ctx, id, s.DeveloperID)
}
func (p *Platform) Account(ctx context.Context, s domain.Session, name string) error {
	if strings.TrimSpace(name) == "" || len(name) > 100 {
		return domain.ErrInvalid
	}
	return p.repo.UpdateDeveloper(ctx, s.DeveloperID, strings.TrimSpace(name))
}
func (p *Platform) ChangePassword(ctx context.Context, s domain.Session, old, next string) error {
	d, e := p.repo.DeveloperByID(ctx, s.DeveloperID)
	if e != nil {
		return e
	}
	if !PasswordMatches(d.PasswordHash, old) {
		return domain.ErrUnauthorized
	}
	if len(next) < 12 || len(next) > 128 {
		return domain.ErrInvalid
	}
	hash, e := PasswordHash(next)
	if e != nil {
		return e
	}
	return p.repo.ChangePassword(ctx, s.DeveloperID, hash, s.ID)
}
func (p *Platform) Projects(ctx context.Context, owner string) ([]domain.Project, error) {
	return p.repo.Projects(ctx, owner)
}
func (p *Platform) OwnProject(ctx context.Context, owner, id string) (domain.Project, error) {
	return p.repo.Project(ctx, id, owner)
}
func validProject(x domain.Project) bool {
	return strings.TrimSpace(x.Name) != "" && len(x.Name) <= 100 && strings.TrimSpace(x.MerchantName) != "" && len(x.MerchantName) <= 150 && len(x.Description) <= 1000 && x.PaymentTTL >= 60 && x.PaymentTTL <= 86400 && x.WebhookMaxAttempts >= 1 && x.WebhookMaxAttempts <= 12
}
func (p *Platform) CreateProject(ctx context.Context, owner string, x domain.Project) (domain.Project, error) {
	x.DeveloperID = owner
	x.CreatedAt = p.now().Unix()
	x.PaymentTTL = int64(p.cfg.PaymentTTL / time.Second)
	x.WebhookMaxAttempts = p.cfg.WebhookMaxAttempts
	if !validProject(x) {
		return x, domain.ErrInvalid
	}
	id, e := Random("prj_", 16)
	if e != nil {
		return x, e
	}
	x.ID = id
	return x, p.repo.CreateProject(ctx, x)
}
func (p *Platform) UpdateProject(ctx context.Context, owner, id string, x domain.Project) (domain.Project, error) {
	existing, e := p.OwnProject(ctx, owner, id)
	if e != nil {
		return x, e
	}
	x.ID = id
	x.DeveloperID = owner
	x.CreatedAt = existing.CreatedAt
	if !validProject(x) {
		return x, domain.ErrInvalid
	}
	return x, p.repo.UpdateProject(ctx, x)
}
func (p *Platform) NewAPIKey(ctx context.Context, owner, project, name, rotate string) (domain.APIKey, string, error) {
	if _, e := p.OwnProject(ctx, owner, project); e != nil {
		return domain.APIKey{}, "", e
	}
	if strings.TrimSpace(name) == "" || len(name) > 100 {
		return domain.APIKey{}, "", domain.ErrInvalid
	}
	secret, e := Random("sk_sandbox_", 32)
	if e != nil {
		return domain.APIKey{}, "", e
	}
	id, e := Random("key_", 16)
	if e != nil {
		return domain.APIKey{}, "", e
	}
	key := domain.APIKey{ID: id, ProjectID: project, Name: strings.TrimSpace(name), Prefix: secret[:18], Hash: Hash(secret), CreatedAt: p.now().Unix()}
	e = p.repo.CreateAPIKey(ctx, key, rotate)
	key.Hash = ""
	if e != nil {
		return key, "", e
	}
	return key, secret, nil
}
func (p *Platform) APIKeyProject(ctx context.Context, key string) (domain.Project, error) {
	if !strings.HasPrefix(key, "sk_sandbox_") || len(key) > 128 {
		return domain.Project{}, domain.ErrUnauthorized
	}
	project, e := p.repo.AuthenticateKey(ctx, Hash(key), p.now().Unix())
	if e != nil {
		return domain.Project{}, domain.ErrUnauthorized
	}
	return project, nil
}
func (p *Platform) RevokeAPIKey(ctx context.Context, owner, project, id string) error {
	if _, e := p.OwnProject(ctx, owner, project); e != nil {
		return e
	}
	return p.repo.RevokeAPIKey(ctx, project, id, p.now().Unix())
}
func (p *Platform) NewSigningKey(ctx context.Context, owner, project string) (domain.SigningKey, string, error) {
	if _, e := p.OwnProject(ctx, owner, project); e != nil {
		return domain.SigningKey{}, "", e
	}
	secret, e := Random("whsec_sandbox_", 32)
	if e != nil {
		return domain.SigningKey{}, "", e
	}
	encrypted, e := p.vault.Seal(secret, "signing:"+project)
	if e != nil {
		return domain.SigningKey{}, "", e
	}
	id, e := Random("sig_", 16)
	if e != nil {
		return domain.SigningKey{}, "", e
	}
	key := domain.SigningKey{ID: id, ProjectID: project, Prefix: secret[:21], Cipher: encrypted, Active: true, CreatedAt: p.now().Unix()}
	e = p.repo.RotateSigningKey(ctx, key, 300)
	key.Cipher = nil
	if e != nil {
		return key, "", e
	}
	return key, secret, nil
}
func (p *Platform) RevokeSigningKey(ctx context.Context, owner, project, id string) error {
	if _, e := p.OwnProject(ctx, owner, project); e != nil {
		return e
	}
	return p.repo.RevokeSigningKey(ctx, project, id, p.now().Unix())
}
func (p *Platform) ProjectView(ctx context.Context, owner, project, resource, id string, filter domain.Filter) (any, error) {
	if _, e := p.OwnProject(ctx, owner, project); e != nil {
		return nil, e
	}
	switch resource {
	case "dashboard":
		return p.repo.Dashboard(ctx, project, p.now().Add(-7*24*time.Hour).Unix())
	case "api-keys":
		return p.repo.APIKeys(ctx, project)
	case "signing-keys":
		return p.repo.SigningKeys(ctx, project)
	case "webhooks":
		return p.repo.Endpoints(ctx, project)
	case "api-logs":
		return p.repo.Logs(ctx, project, filter.Limit, filter.Offset)
	case "deliveries":
		return p.repo.Deliveries(ctx, project, id)
	case "webhook-jobs":
		return p.repo.Jobs(ctx, project, filter.Limit, filter.Offset)
	case "transactions":
		items, total, e := p.repo.Transactions(ctx, project, filter)
		if e != nil {
			return nil, e
		}
		for i := range items {
			if e = p.decorate(&items[i]); e != nil {
				return nil, e
			}
		}
		return map[string]any{"items": items, "total": total, "limit": filter.Limit, "offset": filter.Offset}, nil
	case "transaction":
		t, e := p.repo.Transaction(ctx, project, id)
		if e != nil {
			return nil, e
		}
		if e = p.decorate(&t); e != nil {
			return nil, e
		}
		history, e := p.repo.TransactionEvents(ctx, t.ID)
		return map[string]any{"transaction": t, "history": history}, e
	}
	return nil, domain.ErrNotFound
}
func (p *Platform) Subscribe(id string) (<-chan domain.Transaction, func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	ch := make(chan domain.Transaction, 1)
	if p.subscribers[id] == nil {
		p.subscribers[id] = make(map[chan domain.Transaction]struct{})
	}
	p.subscribers[id][ch] = struct{}{}
	return ch, func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		delete(p.subscribers[id], ch)
		if len(p.subscribers[id]) == 0 {
			delete(p.subscribers, id)
		}
	}
}
func (p *Platform) publish(t domain.Transaction) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for ch := range p.subscribers[t.ID] {
		select {
		case ch <- t:
		default:
		}
	}
}
func invalid(message string) error { return fmt.Errorf("%w: %s", domain.ErrInvalid, message) }
