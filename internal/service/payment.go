package service

import (
	"context"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/domain"
	"net/url"
	"strings"
	"time"
)

func ValidBank(bank string) bool {
	switch bank {
	case "JDB", "LDB", "IB", "BCEL", "STB":
		return true
	}
	return false
}
func (p *Platform) CreatePayment(ctx context.Context, project domain.Project, bank, kind string, input domain.PaymentInput) (domain.Transaction, error) {
	minor, e := domain.AmountMinor(string(input.Amount))
	if e != nil {
		return domain.Transaction{}, invalid("amount must be positive with at most two decimal places")
	}
	if !ValidBank(bank) || (kind != "QR" && kind != "LINK") || strings.TrimSpace(input.Description) == "" || len(input.Description) > 500 || len(input.OrderNo) > 100 || len(input.Tag1) > 500 || len(input.Tag2) > 500 || len(input.Tag3) > 500 {
		return domain.Transaction{}, domain.ErrInvalid
	}
	id, e := Random("txn_", 16)
	if e != nil {
		return domain.Transaction{}, e
	}
	token, e := Random("pay_", 32)
	if e != nil {
		return domain.Transaction{}, e
	}
	encrypted, e := p.vault.Seal(token, "payment:"+id)
	if e != nil {
		return domain.Transaction{}, e
	}
	receipt, e := Random("SIM_", 12)
	if e != nil {
		return domain.Transaction{}, e
	}
	history, e := Random("hist_", 16)
	if e != nil {
		return domain.Transaction{}, e
	}
	now := p.now().Unix()
	order := input.OrderNo
	if order == "" {
		order = id
	}
	t := domain.Transaction{ID: id, ProjectID: project.ID, OrderNo: order, Kind: kind, Bank: bank, MerchantName: project.MerchantName, AmountMinor: minor, Currency: "LAK", Description: strings.TrimSpace(input.Description), Status: domain.Waiting, CreatedAt: now, ExpiresAt: now + project.PaymentTTL, ReceiptRef: receipt, TokenHash: Hash(token), TokenCipher: encrypted, Tag1: input.Tag1, Tag2: input.Tag2, Tag3: input.Tag3}
	if e = p.repo.CreateTransaction(ctx, t, domain.TransactionEvent{ID: history, TransactionID: id, Status: domain.Waiting, CreatedAt: now}); e != nil {
		return domain.Transaction{}, e
	}
	if e = p.decorate(&t); e != nil {
		return domain.Transaction{}, e
	}
	return t, nil
}
func (p *Platform) decorate(t *domain.Transaction) error {
	token, e := p.vault.Open(t.TokenCipher, "payment:"+t.ID)
	if e != nil {
		return e
	}
	t.PaymentURL = p.cfg.PublicURL + "/sandbox/pay/" + token
	t.CheckoutURL = p.cfg.PublicURL + "/sandbox/checkout/" + t.ID + "?token=" + url.QueryEscape(token)
	return nil
}
func (p *Platform) Payment(ctx context.Context, token string) (domain.Transaction, error) {
	if !strings.HasPrefix(token, "pay_") || len(token) > 128 {
		return domain.Transaction{}, domain.ErrNotFound
	}
	t, e := p.repo.TransactionByToken(ctx, Hash(token))
	if e != nil {
		return t, e
	}
	if t.Status == domain.Waiting && t.ExpiresAt <= p.now().Unix() {
		t, changed, err := p.repo.Transition(ctx, t.TokenHash, domain.Expired, p.now().Unix())
		if err != nil {
			return t, err
		}
		if changed {
			p.publish(t)
		}
		if e = p.decorate(&t); e != nil {
			return t, e
		}
		return t, nil
	}
	if e = p.decorate(&t); e != nil {
		return t, e
	}
	return t, nil
}
func (p *Platform) Confirm(ctx context.Context, token string, status domain.State) (domain.Transaction, error) {
	if status != domain.Completed && status != domain.Failed && status != domain.Canceled {
		return domain.Transaction{}, domain.ErrInvalid
	}
	if !strings.HasPrefix(token, "pay_") || len(token) > 128 {
		return domain.Transaction{}, domain.ErrNotFound
	}
	t, changed, e := p.repo.Transition(ctx, Hash(token), status, p.now().Unix())
	if changed {
		p.publish(t)
	}
	if e != nil {
		return t, e
	}
	e = p.decorate(&t)
	return t, e
}
func (p *Platform) Resolve(ctx context.Context, owner, project, payload string) (domain.Transaction, error) {
	payload = strings.TrimSpace(payload)
	if len(payload) > 2048 {
		return domain.Transaction{}, domain.ErrInvalid
	}
	if strings.HasPrefix(payload, "pay_") {
		return p.scanPayment(ctx, payload)
	}
	if strings.HasPrefix(payload, "txn_") {
		if _, e := p.OwnProject(ctx, owner, project); e != nil {
			return domain.Transaction{}, e
		}
		t, e := p.repo.Transaction(ctx, project, payload)
		if e != nil {
			return t, e
		}
		token, e := p.vault.Open(t.TokenCipher, "payment:"+t.ID)
		if e != nil {
			return t, e
		}
		return p.scanPayment(ctx, token)
	}
	u, e := url.Parse(payload)
	base, _ := url.Parse(p.cfg.PublicURL)
	if e != nil || u.Scheme != base.Scheme || u.Host != base.Host || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.HasPrefix(u.Path, "/sandbox/pay/") {
		return domain.Transaction{}, invalid("QR must identify this sandbox")
	}
	return p.scanPayment(ctx, strings.TrimPrefix(u.Path, "/sandbox/pay/"))
}
func (p *Platform) scanPayment(ctx context.Context, token string) (domain.Transaction, error) {
	t, err := p.Payment(ctx, token)
	if err != nil {
		return t, err
	}
	if t.Status == domain.Expired || t.ExpiresAt <= p.now().Unix() {
		return t, domain.ErrExpired
	}
	if t.Status == domain.Failed || t.Status == domain.Canceled {
		return t, domain.ErrConflict
	}
	return t, nil
}
func (p *Platform) RecordRequest(ctx context.Context, project, method, path string, status int, duration time.Duration) error {
	id, e := Random("req_", 16)
	if e != nil {
		return e
	}
	message := ""
	if status >= 400 {
		message = "SDK request rejected"
	}
	return p.repo.WriteLog(ctx, domain.APILog{ID: id, ProjectID: project, Method: method, Path: path, Status: status, DurationMS: duration.Milliseconds(), Error: message, CreatedAt: p.now().Unix()})
}
