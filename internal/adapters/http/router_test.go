package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	phajay "github.com/barlus-developer/phajay-payment-gateway"
	adapter "github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/adapters/http"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/adapters/sqlite"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/adapters/webhook"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/config"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/domain"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/service"
)

type transport struct{ handler http.Handler }

type streamRecorder struct {
	*httptest.ResponseRecorder
	flushed chan struct{}
}

func (r *streamRecorder) Flush() { r.ResponseRecorder.Flush(); r.flushed <- struct{}{} }

func (r transport) RoundTrip(req *http.Request) (*http.Response, error) {
	rr := httptest.NewRecorder()
	r.handler.ServeHTTP(rr, req)
	return rr.Result(), nil
}

type allowed struct{}

func (allowed) Validate(context.Context, string) error { return nil }
func (allowed) Send(context.Context, string, []byte, string, string) (int, string, error) {
	return 200, "{}", nil
}

type fixture struct {
	handler http.Handler
	p       *service.Platform
	project domain.Project
	key     string
	csrf    string
	cookies []*http.Cookie
	store   *sqlite.Store
}

func setup(t *testing.T) fixture {
	t.Helper()
	cfg := config.Config{PublicURL: "http://sandbox.test", DatabasePath: filepath.Join(t.TempDir(), "sandbox.db"), MigrationsDir: "../../../migrations", SessionTTL: time.Hour, PaymentTTL: time.Minute, WebhookTimeout: time.Second, WebhookMaxAttempts: 3, WebDistDir: t.TempDir()}
	cfg.MasterKey[0] = 1
	ctx := context.Background()
	store, e := sqlite.Open(ctx, cfg.DatabasePath, cfg.MigrationsDir)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { store.Close() })
	p, e := service.New(store, cfg)
	if e != nil {
		t.Fatal(e)
	}
	d, e := p.Register(ctx, "developer@example.test", "Developer", "long-test-password")
	if e != nil {
		t.Fatal(e)
	}
	project, e := p.CreateProject(ctx, d.ID, domain.Project{Name: "Test", MerchantName: "Test Merchant"})
	if e != nil {
		t.Fatal(e)
	}
	_, key, e := p.NewAPIKey(ctx, d.ID, project.ID, "SDK", "")
	if e != nil {
		t.Fatal(e)
	}
	_, _, token, csrf, e := p.Login(ctx, d.Email, "long-test-password")
	if e != nil {
		t.Fatal(e)
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	handler := adapter.New(p, allowed{}, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), cancelCtx)
	return fixture{handler, p, project, key, csrf, []*http.Cookie{{Name: "phajay_session", Value: token}, {Name: "phajay_csrf", Value: csrf}}, store}
}
func request(f fixture, method, path, body string, authenticated, verified bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://sandbox.test"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if authenticated {
		for _, c := range f.cookies {
			r.AddCookie(c)
		}
	}
	if verified {
		r.Header.Set("Origin", "http://sandbox.test")
		r.Header.Set("X-CSRF-Token", f.csrf)
		r.Header.Set("X-Phajay-Portal", "1")
	}
	rr := httptest.NewRecorder()
	f.handler.ServeHTTP(rr, r)
	return rr
}
func TestActualSDKAllSandboxPayments(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	client := phajay.New(phajay.Config{APIKey: f.key, BaseURL: "http://sandbox.test"}, phajay.WithSandbox(), phajay.WithHTTPClient(&http.Client{Transport: transport{f.handler}}))
	link, e := client.CreatePaymentLink(ctx, phajay.PaymentLinkRequest{OrderNo: "order-link", Amount: 12.34, Description: "SDK link"})
	if e != nil || link.OrderNo != "order-link" || !strings.Contains(link.RedirectURL, "/sandbox/checkout/") {
		t.Fatalf("link %+v %v", link, e)
	}
	linkURL, _ := url.Parse(link.RedirectURL)
	linkToken := linkURL.Query().Get("token")
	linkTx, e := f.p.Confirm(ctx, linkToken, domain.Completed)
	if e != nil {
		t.Fatal(e)
	}
	linkBody, e := domain.CallbackBody(linkTx)
	if e != nil {
		t.Fatal(e)
	}
	linkHeaders := make(http.Header)
	stamp := time.Now().Unix()
	linkHeaders.Set(webhook.TimestampHeader, fmt.Sprint(stamp))
	linkHeaders.Set(webhook.SignatureHeader, webhook.Sign(stamp, linkBody, "link-signing-key"))
	linkHeaders.Set(webhook.EventHeader, "evt_link")
	var linkCallback phajay.SandboxPaymentLinkWebhookCallback
	if e := phajay.New(phajay.Config{WebhookSignatureKey: "link-signing-key"}, phajay.WithSandbox()).DecodeWebhook(linkBody, linkHeaders, &linkCallback); e != nil || linkCallback.OrderNo == nil || *linkCallback.OrderNo != link.OrderNo || linkCallback.Amount == nil || *linkCallback.Amount != 12.34 {
		t.Fatal(linkCallback, e)
	}
	for _, bank := range []phajay.Bank{phajay.BankJDB, phajay.BankLDB, phajay.BankIB, phajay.BankBCEL, phajay.BankSTB} {
		t.Run(fmt.Sprint(bank), func(t *testing.T) {
			qr, e := client.GenerateQR(ctx, bank, phajay.PaymentQRRequest{Amount: 99.25, Description: "SDK QR"})
			if e != nil || qr.TransactionID == "" || qr.QRCode != qr.Link {
				t.Fatalf("QR %+v %v", qr, e)
			}
			u, e := url.Parse(qr.Link)
			if e != nil {
				t.Fatal(e)
			}
			token := strings.TrimPrefix(u.Path, "/sandbox/pay/")
			waiting, e := f.p.Payment(ctx, token)
			if e != nil || waiting.Status != domain.Waiting {
				t.Fatalf("scan completed payment: %v %v", waiting, e)
			}
			resolve := request(f, "POST", "/api/internal/v1/simulator/resolve", fmt.Sprintf(`{"reference":%q}`, qr.QRCode), false, true)
			if resolve.Code != 200 {
				t.Fatalf("resolve %d %s", resolve.Code, resolve.Body)
			}
			paid, e := f.p.Confirm(ctx, token, domain.Completed)
			if e != nil || paid.Status != domain.Completed {
				t.Fatalf("confirm %v", e)
			}
			body, e := domain.CallbackBody(paid)
			if e != nil {
				t.Fatal(e)
			}
			h := make(http.Header)
			h.Set(webhook.TimestampHeader, fmt.Sprint(time.Now().Unix()))
			h.Set(webhook.SignatureHeader, webhook.Sign(time.Now().Unix(), body, "sdk-signing-key"))
			h.Set(webhook.EventHeader, "evt_unique")
			verified := phajay.New(phajay.Config{WebhookSignatureKey: "sdk-signing-key"}, phajay.WithSandbox())
			var callback phajay.PaymentQRWebhookCallback
			if e := verified.DecodeWebhook(body, h, &callback); e != nil || callback.TransactionID != paid.ID || callback.TxnAmount != 99.25 {
				t.Fatalf("callback %+v %v", callback, e)
			}
		})
	}
	bad := phajay.New(phajay.Config{APIKey: "invalid", BaseURL: "http://sandbox.test"}, phajay.WithSandbox(), phajay.WithHTTPClient(&http.Client{Transport: transport{f.handler}}))
	if _, e := bad.GenerateQR(ctx, phajay.BankJDB, phajay.PaymentQRRequest{Amount: 1, Description: "invalid key"}); e == nil {
		t.Fatal("invalid key accepted")
	}
	if _, e := client.GenerateQR(ctx, phajay.BankMMoneyX, phajay.PaymentQRRequest{Amount: 1, Description: "no route"}); e == nil {
		t.Fatal("unsupported bank accepted")
	}
	for _, path := range []string{"/v1/api/link/payment-link", "/v1/api/payment/generate-jdb-qr", "/api/internal/v1/unknown"} {
		rr := request(f, "POST", path, `{}`, false, false)
		if rr.Code != 404 {
			t.Fatalf("production/fallback %s %d", path, rr.Code)
		}
	}
}
func TestAuthCSRFAndProjectIsolation(t *testing.T) {
	f := setup(t)
	base := "/api/internal/v1/projects/" + f.project.ID
	for _, tt := range []struct {
		method, path, body string
		auth, csrf         bool
		code               int
	}{{"GET", base + "/api-keys", "", false, false, 401}, {"POST", base + "/api-keys", `{"name":"unsafe"}`, true, false, 401}, {"POST", base + "/api-keys", `{"name":"safe"}`, true, true, 201}, {"GET", "/api/internal/v1/projects/foreign/transactions", "", true, false, 404}, {"GET", "/api/internal/v1/simulator/checkout/unknown?token=invalid", "", false, false, 404}} {
		rr := request(f, tt.method, tt.path, tt.body, tt.auth, tt.csrf)
		if rr.Code != tt.code {
			t.Fatalf("%s %s %d %s", tt.method, tt.path, rr.Code, rr.Body)
		}
	}
	rr := request(f, "GET", base+"/api-keys", "", true, false)
	if strings.Contains(rr.Body.String(), f.key) || strings.Contains(rr.Body.String(), "hash") {
		t.Fatal("stored key leaked")
	}
	rr = request(f, "POST", "/api/internal/v1/auth/login", `{"email":"developer@example.test","password":"long-test-password"}`, false, true)
	if rr.Code != 200 {
		t.Fatal(rr.Body)
	}
	for _, c := range rr.Result().Cookies() {
		if c.Name == "phajay_session" && (!c.HttpOnly || c.SameSite != http.SameSiteStrictMode) {
			t.Fatal("unsafe session cookie")
		}
	}
	rr = request(f, "POST", "/api/internal/v1/auth/logout", `{}`, true, true)
	if rr.Code != 200 {
		t.Fatal(rr.Body)
	}
	if rr = request(f, "GET", "/api/internal/v1/auth/me", "", true, false); rr.Code != 401 {
		t.Fatal("logout retained session")
	}
}
func TestPortalManagementRoutesAndActualForeignOwner(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	base := "/api/internal/v1/projects/" + f.project.ID
	key := request(f, "POST", base+"/api-keys", `{"name":"Portal key"}`, true, true)
	if key.Code != 201 {
		t.Fatal(key.Body)
	}
	var keyResult struct {
		Data struct {
			Key    domain.APIKey
			Secret string
		}
	}
	if e := json.NewDecoder(key.Body).Decode(&keyResult); e != nil {
		t.Fatal(e)
	}
	rotated := request(f, "POST", base+"/api-keys/"+keyResult.Data.Key.ID+"/rotate", `{"name":"Replacement"}`, true, true)
	if rotated.Code != 201 {
		t.Fatal(rotated.Body)
	}
	if _, e := f.p.APIKeyProject(ctx, keyResult.Data.Secret); e == nil {
		t.Fatal("old portal key accepted")
	}
	signing := request(f, "POST", base+"/signing-keys", `{}`, true, true)
	if signing.Code != 201 {
		t.Fatal(signing.Body)
	}
	endpoint := request(f, "POST", base+"/webhooks", `{"url":"https://merchant.test/hook","enabled":true,"events":["payment.completed"]}`, true, true)
	if endpoint.Code != 200 {
		t.Fatal(endpoint.Body)
	}
	var endpointResult struct{ Data domain.Endpoint }
	if e := json.NewDecoder(endpoint.Body).Decode(&endpointResult); e != nil {
		t.Fatal(e)
	}
	if rr := request(f, "POST", base+"/webhooks/"+endpointResult.Data.ID+"/test", `{}`, true, true); rr.Code != 202 {
		t.Fatal(rr.Body)
	}
	for _, resource := range []string{"dashboard", "signing-keys", "webhooks", "webhook-jobs", "api-logs", "transactions"} {
		if rr := request(f, "GET", base+"/"+resource, "", true, false); rr.Code != 200 {
			t.Fatal(resource, rr.Body)
		}
	}
	other, e := f.p.Register(ctx, "other@example.test", "Other developer", "another-long-password")
	if e != nil {
		t.Fatal(e)
	}
	_, _, token, csrf, e := f.p.Login(ctx, other.Email, "another-long-password")
	if e != nil {
		t.Fatal(e)
	}
	foreign := f
	foreign.csrf = csrf
	foreign.cookies = []*http.Cookie{{Name: "phajay_session", Value: token}, {Name: "phajay_csrf", Value: csrf}}
	for _, tt := range []struct{ method, path, body string }{{"GET", base + "/api-keys", ""}, {"GET", base + "/transactions", ""}, {"POST", base + "/signing-keys", "{}"}, {"DELETE", base + "/webhooks/" + endpointResult.Data.ID, ""}, {"PATCH", base, `{"name":"stolen","merchantName":"bad","paymentTTL":60,"webhookMaxAttempts":2}`}} {
		rr := request(foreign, tt.method, tt.path, tt.body, true, true)
		if rr.Code != 404 {
			t.Fatal("foreign owner accessed", tt.path, rr.Code, rr.Body)
		}
	}
	for i := 0; i < 11; i++ {
		rr := request(f, "POST", "/api/internal/v1/auth/register", `{}`, false, true)
		if i == 10 && rr.Code != 429 {
			t.Fatal("auth rate limit missing", rr.Code)
		}
	}
}

func TestCheckoutSSELatestAndCancellation(t *testing.T) {
	f := setup(t)
	tx, e := f.p.CreatePayment(context.Background(), f.project, "JDB", "QR", domain.PaymentInput{Amount: "12.34", Description: "SSE"})
	if e != nil {
		t.Fatal(e)
	}
	u, _ := url.Parse(tx.CheckoutURL)
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest("GET", "http://sandbox.test/api/internal/v1/simulator/checkout/"+tx.ID+"/events?"+u.RawQuery, nil).WithContext(ctx)
	rr := &streamRecorder{ResponseRecorder: httptest.NewRecorder(), flushed: make(chan struct{}, 10)}
	done := make(chan struct{})
	go func() { f.handler.ServeHTTP(rr, r); close(done) }()
	select {
	case <-rr.flushed:
	case <-time.After(time.Second):
		t.Fatal("initial SSE snapshot not flushed")
	}
	token := u.Query().Get("token")
	if _, e := f.p.Confirm(context.Background(), token, domain.Completed); e != nil {
		t.Fatal(e)
	}
	select {
	case <-rr.flushed:
	case <-time.After(time.Second):
		t.Fatal("completion SSE event not flushed")
	}
	cancel()
	<-done
	if rr.Code != 200 || !bytes.Contains(rr.Body.Bytes(), []byte(`"status":"WAITING"`)) || !bytes.Contains(rr.Body.Bytes(), []byte(`"status":"PAYMENT_COMPLETED"`)) {
		t.Fatalf("snapshot %d %s", rr.Code, rr.Body)
	}
}
