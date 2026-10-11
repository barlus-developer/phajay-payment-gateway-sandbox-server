package service

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/domain"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/ports"
	"log/slog"
	"time"
)

func (p *Platform) SaveEndpoint(ctx context.Context, owner, project, id string, x domain.Endpoint, validator ports.Sender) (domain.Endpoint, error) {
	if _, e := p.OwnProject(ctx, owner, project); e != nil {
		return x, e
	}
	if len(x.URL) > 2048 || len(x.Events) == 0 || len(x.Events) > 4 {
		return x, domain.ErrInvalid
	}
	allowed := map[string]bool{"payment.completed": true, "payment.failed": true, "payment.canceled": true, "payment.expired": true}
	seen := make(map[string]bool)
	for _, event := range x.Events {
		if !allowed[event] || seen[event] {
			return x, domain.ErrInvalid
		}
		seen[event] = true
	}
	if e := validator.Validate(ctx, x.URL); e != nil {
		return x, invalid("webhook destination is not permitted")
	}
	x.ProjectID = project
	x.CreatedAt = p.now().Unix()
	if id == "" {
		var e error
		x.ID, e = Random("wh_", 16)
		if e != nil {
			return x, e
		}
	} else {
		endpoints, e := p.repo.Endpoints(ctx, project)
		if e != nil {
			return x, e
		}
		found := false
		for _, current := range endpoints {
			if current.ID == id {
				x.ID = id
				x.CreatedAt = current.CreatedAt
				found = true
				break
			}
		}
		if !found {
			return x, domain.ErrNotFound
		}
	}
	return x, p.repo.SaveEndpoint(ctx, x)
}
func (p *Platform) DisableEndpoint(ctx context.Context, owner, project, id string) error {
	if _, e := p.OwnProject(ctx, owner, project); e != nil {
		return e
	}
	return p.repo.DeleteEndpoint(ctx, project, id)
}
func (p *Platform) TestWebhook(ctx context.Context, owner, project, endpoint string) (string, error) {
	if _, e := p.OwnProject(ctx, owner, project); e != nil {
		return "", e
	}
	items, e := p.repo.Endpoints(ctx, project)
	if e != nil {
		return "", e
	}
	found := false
	for _, item := range items {
		if item.ID == endpoint && item.Enabled {
			found = true
		}
	}
	if !found {
		return "", domain.ErrNotFound
	}
	id, e := Random("evt_", 16)
	if e != nil {
		return "", e
	}
	body, e := json.Marshal(map[string]any{"message": "SANDBOX TEST — NOT A REAL TRANSACTION", "paymentMethod": "JDB", "transactionId": "sandbox-test", "txnAmount": 1, "status": "PAYMENT_COMPLETED"})
	if e != nil {
		return "", e
	}
	e = p.repo.Enqueue(ctx, domain.OutboxEvent{ID: id, ProjectID: project, Type: "payment.test", Body: body, CreatedAt: p.now().Unix()}, endpoint)
	return id, e
}
func (p *Platform) RetryWebhook(ctx context.Context, owner, project, id string) error {
	if _, e := p.OwnProject(ctx, owner, project); e != nil {
		return e
	}
	return p.repo.RetryJob(ctx, project, id, p.now().Unix())
}

type Worker struct {
	platform *Platform
	sender   ports.Sender
	logger   *slog.Logger
}

func NewWorker(p *Platform, sender ports.Sender, logger *slog.Logger) *Worker {
	return &Worker{p, sender, logger}
}
func (w *Worker) Run(ctx context.Context) {
	expirationDone := make(chan struct{})
	go func() { defer close(expirationDone); w.runExpiration(ctx) }()
	defer func() { <-expirationDone }()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		for i := 0; i < 20; i++ {
			worked, err := w.ProcessOne(ctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				w.logger.Error("webhook worker operation failed", "error", err.Error())
			}
			if !worked || err != nil {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (w *Worker) runExpiration(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		w.expire(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (w *Worker) expire(ctx context.Context) {
	now := w.platform.now().Unix()
	items, err := w.platform.repo.Expiring(ctx, now)
	if err != nil {
		if ctx.Err() == nil {
			w.logger.Error("expiration query failed")
		}
		return
	}
	for _, item := range items {
		t, changed, e := w.platform.repo.Transition(ctx, item.TokenHash, domain.Expired, now)
		if e == nil && changed {
			w.platform.publish(t)
		}
	}
}

// ProcessOne claims a durable lease and records each attempt; the same event ID survives retries.
func (w *Worker) ProcessOne(ctx context.Context) (bool, error) {
	p := w.platform
	now := p.now().Unix()
	job, e := p.repo.ClaimJob(ctx, now, int64(p.cfg.WebhookTimeout/time.Second)+30)
	if errors.Is(e, domain.ErrNotFound) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	started := time.Now()
	code := 0
	response := ""
	message := ""
	key, e := p.repo.ActiveSigningKey(ctx, job.ProjectID)
	if e == nil {
		var secret string
		secret, e = p.vault.Open(key.Cipher, "signing:"+job.ProjectID)
		if e == nil {
			deliveryCtx, cancel := context.WithTimeout(ctx, p.cfg.WebhookTimeout)
			code, response, e = w.sender.Send(deliveryCtx, job.URL, job.Body, secret, job.EventID)
			cancel()
		}
	}
	if e != nil {
		message = "Webhook delivery failed; check signing configuration and destination availability"
	}
	if e == nil && (code < 200 || code >= 300) {
		message = "Callback returned a non-success HTTP status"
	}
	id, err := Random("delivery_", 16)
	if err != nil {
		return true, err
	}
	success := e == nil && code >= 200 && code < 300
	backoff := int64(1) << min(job.Attempts, 10)
	if backoff > 3600 {
		backoff = 3600
	}
	attempt := domain.Delivery{ID: id, HTTPStatus: code, Response: response, Error: message, DurationMS: time.Since(started).Milliseconds(), CreatedAt: p.now().Unix()}
	return true, p.repo.FinishJob(ctx, job, attempt, success, p.now().Unix()+backoff)
}
