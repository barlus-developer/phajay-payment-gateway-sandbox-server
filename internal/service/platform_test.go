package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/adapters/sqlite"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/config"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/domain"
)

type fakeSender struct {
	code         int
	bodies       [][]byte
	keys, events []string
}

func (s *fakeSender) Validate(context.Context, string) error { return nil }
func (s *fakeSender) Send(_ context.Context, _ string, body []byte, key, event string) (int, string, error) {
	s.bodies = append(s.bodies, append([]byte{}, body...))
	s.keys = append(s.keys, key)
	s.events = append(s.events, event)
	return s.code, `{"accepted":true}`, nil
}

type fixture struct {
	p         *Platform
	store     *sqlite.Store
	project   domain.Project
	developer domain.Developer
	cfg       config.Config
}

func setup(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()
	cfg := config.Config{PublicURL: "http://sandbox.test", DatabasePath: filepath.Join(t.TempDir(), "sandbox.db"), MigrationsDir: "../../migrations", SessionTTL: time.Hour, PaymentTTL: time.Minute, WebhookTimeout: time.Second, WebhookMaxAttempts: 2}
	cfg.MasterKey[0] = 1
	s, e := sqlite.Open(ctx, cfg.DatabasePath, cfg.MigrationsDir)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	p, e := New(s, cfg)
	if e != nil {
		t.Fatal(e)
	}
	d, e := p.Register(ctx, "developer@example.test", "Developer", "long-test-password")
	if e != nil {
		t.Fatal(e)
	}
	project, e := p.CreateProject(ctx, d.ID, domain.Project{Name: "Project", MerchantName: "Merchant"})
	if e != nil {
		t.Fatal(e)
	}
	return fixture{p, s, project, d, cfg}
}
func payment(t *testing.T, f fixture) (domain.Transaction, string) {
	t.Helper()
	tx, e := f.p.CreatePayment(context.Background(), f.project, "JDB", "QR", domain.PaymentInput{Amount: "12.34", Description: "Coffee"})
	if e != nil {
		t.Fatal(e)
	}
	u, _ := url.Parse(tx.PaymentURL)
	return tx, strings.TrimPrefix(u.Path, "/sandbox/pay/")
}
func TestConcurrentConfirmCreatesOneHistoryAndOutbox(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	sender := &fakeSender{code: 200}
	if _, e := f.p.SaveEndpoint(ctx, f.developer.ID, f.project.ID, "", domain.Endpoint{URL: "https://merchant.test/hook", Enabled: true, Events: []string{"payment.completed"}}, sender); e != nil {
		t.Fatal(e)
	}
	if _, _, e := f.p.NewSigningKey(ctx, f.developer.ID, f.project.ID); e != nil {
		t.Fatal(e)
	}
	tx, token := payment(t, f)
	updates, unsub := f.p.Subscribe(tx.ID)
	defer unsub()
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			paid, e := f.p.Confirm(ctx, token, domain.Completed)
			if e == nil && paid.Status != domain.Completed {
				e = errors.New("wrong terminal state")
			}
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	history, e := f.store.TransactionEvents(ctx, tx.ID)
	if e != nil || len(history) != 2 {
		t.Fatalf("history %+v %v", history, e)
	}
	jobs, e := f.store.Jobs(ctx, f.project.ID, 100, 0)
	if e != nil || len(jobs) != 1 {
		t.Fatalf("outbox %+v %v", jobs, e)
	}
	select {
	case update := <-updates:
		if update.Status != domain.Completed {
			t.Fatal(update)
		}
	default:
		t.Fatal("missing status update")
	}
	if _, e := f.p.Confirm(ctx, token, domain.Failed); !errors.Is(e, domain.ErrConflict) {
		t.Fatalf("overwrote terminal %v", e)
	}
}
func TestPaymentExpirationAndAllTerminalStates(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	for _, state := range []domain.State{domain.Failed, domain.Canceled} {
		tx, token := payment(t, f)
		if _, e := f.p.Confirm(ctx, token, state); e != nil {
			t.Fatal(e)
		}
		if _, e := f.p.Confirm(ctx, token, domain.Completed); !errors.Is(e, domain.ErrConflict) {
			t.Fatal("terminal completed", tx.ID, e)
		}
	}
	tx, token := payment(t, f)
	f.p.now = func() time.Time { return time.Unix(tx.ExpiresAt, 0) }
	if _, e := f.p.Confirm(ctx, token, domain.Completed); !errors.Is(e, domain.ErrExpired) {
		t.Fatalf("expired %v", e)
	}
	latest, e := f.p.Payment(ctx, token)
	if e != nil || latest.Status != domain.Expired {
		t.Fatal(latest, e)
	}
	history, _ := f.store.TransactionEvents(ctx, tx.ID)
	if len(history) != 2 {
		t.Fatal(history)
	}
	if _, e := f.p.Resolve(ctx, "", "", "https://bank.example/real"); e == nil {
		t.Fatal("foreign QR accepted")
	}
}
func TestSecretsIsolationRotationAndSessionRevocation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_, key, e := f.p.NewAPIKey(ctx, f.developer.ID, f.project.ID, "SDK", "")
	if e != nil {
		t.Fatal(e)
	}
	items, _ := f.store.APIKeys(ctx, f.project.ID)
	if len(items) != 1 || items[0].Hash != Hash(key) {
		t.Fatal("key not hashed")
	}
	if _, e := f.p.APIKeyProject(ctx, key); e != nil {
		t.Fatal(e)
	}
	_, next, e := f.p.NewAPIKey(ctx, f.developer.ID, f.project.ID, "rotated", items[0].ID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := f.p.APIKeyProject(ctx, key); !errors.Is(e, domain.ErrUnauthorized) {
		t.Fatal("old key active")
	}
	if _, e := f.p.APIKeyProject(ctx, next); e != nil {
		t.Fatal(e)
	}
	if _, _, e := f.p.NewAPIKey(ctx, "other", f.project.ID, "bad", ""); !errors.Is(e, domain.ErrNotFound) {
		t.Fatal("ownership bypass")
	}
	first, secret, e := f.p.NewSigningKey(ctx, f.developer.ID, f.project.ID)
	if e != nil {
		t.Fatal(e)
	}
	stored, e := f.store.ActiveSigningKey(ctx, f.project.ID)
	if e != nil || strings.Contains(string(stored.Cipher), secret) {
		t.Fatal("plaintext signing secret")
	}
	rotatedKey, _, e := f.p.NewSigningKey(ctx, f.developer.ID, f.project.ID)
	if e != nil {
		t.Fatal(e)
	}
	keys, _ := f.store.SigningKeys(ctx, f.project.ID)
	for _, k := range keys {
		if k.ID == first.ID && (k.Active || k.ValidUntil == nil || *k.ValidUntil != rotatedKey.CreatedAt+300) {
			t.Fatal("rotation overlap", k)
		}
	}
	_, s, token, csrf, e := f.p.Login(ctx, f.developer.Email, "long-test-password")
	if e != nil {
		t.Fatal(e)
	}
	if !f.p.ValidateCSRF(s, csrf) || f.p.ValidateCSRF(s, "wrong") {
		t.Fatal("CSRF")
	}
	_, other, otherToken, _, _ := f.p.Login(ctx, f.developer.Email, "long-test-password")
	if other.ID == s.ID {
		t.Fatal("session randomness")
	}
	if e := f.p.ChangePassword(ctx, s, "long-test-password", "new-long-password"); e != nil {
		t.Fatal(e)
	}
	if _, _, e := f.p.Authenticate(ctx, otherToken); !errors.Is(e, domain.ErrUnauthorized) {
		t.Fatal("other session survived password change")
	}
	if _, _, e := f.p.Authenticate(ctx, token); e != nil {
		t.Fatal(e)
	}
	if e := f.p.Logout(ctx, s); e != nil {
		t.Fatal(e)
	}
	if _, _, e := f.p.Authenticate(ctx, token); !errors.Is(e, domain.ErrUnauthorized) {
		t.Fatal("logout failed")
	}
}
func TestWebhookRetriesRecoveryAndManualRetry(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	sender := &fakeSender{code: 500}
	endpoint, e := f.p.SaveEndpoint(ctx, f.developer.ID, f.project.ID, "", domain.Endpoint{URL: "https://merchant.test/hook", Enabled: true, Events: []string{"payment.completed"}}, sender)
	if e != nil {
		t.Fatal(e)
	}
	_, secret, e := f.p.NewSigningKey(ctx, f.developer.ID, f.project.ID)
	if e != nil {
		t.Fatal(e)
	}
	id, e := f.p.TestWebhook(ctx, f.developer.ID, f.project.ID, endpoint.ID)
	if e != nil {
		t.Fatal(e)
	}
	clock := time.Now()
	f.p.now = func() time.Time { return clock }
	worker := NewWorker(f.p, sender, slog.New(slog.NewTextHandler(io.Discard, nil)))
	worked, e := worker.ProcessOne(ctx)
	if !worked || e != nil {
		t.Fatal(worked, e)
	}
	jobs, _ := f.store.Jobs(ctx, f.project.ID, 100, 0)
	if jobs[0].Status != "PENDING" || jobs[0].Attempts != 1 || sender.keys[0] != secret || sender.events[0] != id {
		t.Fatal(jobs, sender)
	}
	if worked, e = worker.ProcessOne(ctx); worked || e != nil {
		t.Fatal("backoff ignored")
	}
	clock = clock.Add(3 * time.Second)
	if _, e = worker.ProcessOne(ctx); e != nil {
		t.Fatal(e)
	}
	jobs, _ = f.store.Jobs(ctx, f.project.ID, 100, 0)
	if jobs[0].Status != "FAILED" {
		t.Fatal(jobs)
	}
	if e = f.p.RetryWebhook(ctx, f.developer.ID, f.project.ID, jobs[0].ID); e != nil {
		t.Fatal(e)
	}
	sender.code = 200
	if _, e = worker.ProcessOne(ctx); e != nil {
		t.Fatal(e)
	}
	deliveries, e := f.store.Deliveries(ctx, f.project.ID, jobs[0].ID)
	if e != nil || len(deliveries) != 3 {
		t.Fatal(deliveries, e)
	}
	if sender.events[2] != id {
		t.Fatal("retry changed event ID")
	}
	_, e = f.p.TestWebhook(ctx, f.developer.ID, f.project.ID, endpoint.ID)
	if e != nil {
		t.Fatal(e)
	}
	claimed, e := f.store.ClaimJob(ctx, clock.Unix(), 1)
	if e != nil {
		t.Fatal(e)
	}
	if e = f.store.Close(); e != nil {
		t.Fatal(e)
	}
	reopened, e := sqlite.Open(ctx, f.cfg.DatabasePath, f.cfg.MigrationsDir)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	recovered, e := reopened.ClaimJob(ctx, clock.Unix()+2, 10)
	if e != nil || recovered.ID != claimed.ID || recovered.EventID != claimed.EventID || recovered.Attempts != claimed.Attempts+1 {
		t.Fatal("recovery", recovered, e)
	}
	if e = reopened.FinishJob(ctx, claimed, domain.Delivery{ID: "stale", CreatedAt: clock.Unix()}, true, 0); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("stale worker accepted", e)
	}
}
func TestPasswordAndVaultValidation(t *testing.T) {
	hash, e := PasswordHash("a-long-test-password")
	if e != nil || !PasswordMatches(hash, "a-long-test-password") || PasswordMatches(hash, "wrong") || PasswordMatches("$argon2id$v=19$m=999999999,t=3,p=2$xx$yy", "wrong") {
		t.Fatal("password validation", e)
	}
	var key [32]byte
	key[0] = 1
	v, _ := NewVault(key)
	cipher, e := v.Seal("secret", "scope:a")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = v.Open(cipher, "scope:b"); e == nil {
		t.Fatal("scope accepted")
	}
	cipher[len(cipher)-1] ^= 1
	if _, e = v.Open(cipher, "scope:a"); e == nil {
		t.Fatal("modified cipher accepted")
	}
}

type blockedSender struct {
	started chan struct{}
	once    sync.Once
}

func (s *blockedSender) Validate(context.Context, string) error { return nil }
func (s *blockedSender) Send(ctx context.Context, _ string, _ []byte, _ string, _ string) (int, string, error) {
	s.once.Do(func() { close(s.started) })
	<-ctx.Done()
	return 0, "", ctx.Err()
}
func TestExpirationContinuesDuringSlowDeliveryAndWorkersStop(t *testing.T) {
	f := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sender := &blockedSender{started: make(chan struct{})}
	if _, e := f.p.SaveEndpoint(ctx, f.developer.ID, f.project.ID, "", domain.Endpoint{URL: "https://merchant.test/hook", Enabled: true, Events: []string{"payment.completed"}}, sender); e != nil {
		t.Fatal(e)
	}
	if _, _, e := f.p.NewSigningKey(ctx, f.developer.ID, f.project.ID); e != nil {
		t.Fatal(e)
	}
	_, first := payment(t, f)
	if _, e := f.p.Confirm(ctx, first, domain.Completed); e != nil {
		t.Fatal(e)
	}
	pending, _ := payment(t, f)
	updates, unsubscribe := f.p.Subscribe(pending.ID)
	defer unsubscribe()
	var clock atomic.Int64
	clock.Store(time.Now().Unix())
	f.p.now = func() time.Time { return time.Unix(clock.Load(), 0) }
	f.p.cfg.WebhookTimeout = 5 * time.Second
	worker := NewWorker(f.p, sender, slog.New(slog.NewTextHandler(io.Discard, nil)))
	done := make(chan struct{})
	go func() { defer close(done); worker.Run(ctx) }()
	select {
	case <-sender.started:
	case <-time.After(time.Second):
		t.Fatal("delivery did not start")
	}
	clock.Store(pending.ExpiresAt)
	select {
	case update := <-updates:
		if update.Status != domain.Expired {
			t.Fatal(update)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("slow delivery blocked payment expiration")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("workers did not stop")
	}
}
