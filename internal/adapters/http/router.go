// Package http adapts Gin requests to framework-independent sandbox services.
package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/config"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/domain"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/ports"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/service"
	"github.com/gin-gonic/gin"
)

type Adapter struct {
	p      *service.Platform
	sender ports.Sender
	cfg    config.Config
	logger *slog.Logger
	stop   context.Context
}
type bucket struct {
	start time.Time
	count int
}
type limiter struct {
	mu      sync.Mutex
	entries map[string]bucket
	limit   int
}

func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.entries) > 10000 {
		for k, b := range l.entries {
			if now.Sub(b.start) > time.Minute {
				delete(l.entries, k)
			}
		}
		if len(l.entries) > 10000 {
			return false
		}
	}
	b := l.entries[key]
	if now.Sub(b.start) > time.Minute {
		b = bucket{start: now}
	}
	b.count++
	l.entries[key] = b
	return b.count <= l.limit
}
func New(p *service.Platform, sender ports.Sender, cfg config.Config, logger *slog.Logger, stop context.Context) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	a := &Adapter{p, sender, cfg, logger, stop}
	_ = r.SetTrustedProxies(cfg.TrustedProxies)
	r.Use(gin.CustomRecovery(func(c *gin.Context, recovered any) {
		logger.Error("request panic recovered")
		c.AbortWithStatusJSON(500, gin.H{"error": gin.H{"code": "internal_error", "message": "Request failed"}})
	}), a.security)
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok", "mode": "sandbox"}) })
	r.GET("/readyz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if p.Ping(ctx) != nil {
			c.JSON(503, gin.H{"status": "unavailable"})
			return
		}
		c.JSON(200, gin.H{"status": "ready"})
	})
	api := r.Group("/api/internal/v1")
	api.GET("/openapi", func(c *gin.Context) { c.Header("Content-Type", "application/yaml"); c.File("docs/openapi.yaml") })
	authLimit := &limiter{entries: make(map[string]bucket), limit: 10}
	authRate := func(c *gin.Context) {
		if !authLimit.allow(c.ClientIP()) {
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(429, gin.H{"error": gin.H{"code": "rate_limited", "message": "Try again in a minute"}})
			return
		}
		c.Next()
	}
	api.POST("/auth/register", authRate, a.origin, a.register)
	api.POST("/auth/login", authRate, a.origin, a.login)
	private := api.Group("")
	private.Use(a.authenticate, a.csrf)
	private.GET("/auth/me", a.me)
	private.POST("/auth/logout", a.logout)
	private.GET("/sessions", func(c *gin.Context) { v, e := p.Sessions(c.Request.Context(), session(c)); respond(c, 200, v, e) })
	private.DELETE("/sessions/:id", func(c *gin.Context) {
		e := p.RevokeSession(c.Request.Context(), session(c), c.Param("id"))
		if c.Param("id") == session(c).ID && e == nil {
			a.clearCookies(c)
		}
		respond(c, 200, gin.H{"revoked": true}, e)
	})
	private.PATCH("/account", func(c *gin.Context) {
		var x struct {
			Name string `json:"name"`
		}
		if !bind(c, &x) {
			return
		}
		respond(c, 200, gin.H{"updated": true}, p.Account(c.Request.Context(), session(c), x.Name))
	})
	private.POST("/account/password", func(c *gin.Context) {
		var x struct {
			Current string `json:"currentPassword"`
			Next    string `json:"newPassword"`
		}
		if !bind(c, &x) {
			return
		}
		respond(c, 200, gin.H{"updated": true}, p.ChangePassword(c.Request.Context(), session(c), x.Current, x.Next))
	})
	private.GET("/projects", func(c *gin.Context) { v, e := p.Projects(c.Request.Context(), developer(c).ID); respond(c, 200, v, e) })
	private.POST("/projects", func(c *gin.Context) {
		var x domain.Project
		if !bind(c, &x) {
			return
		}
		v, e := p.CreateProject(c.Request.Context(), developer(c).ID, x)
		respond(c, 201, v, e)
	})
	private.PATCH("/projects/:project", func(c *gin.Context) {
		var x domain.Project
		if !bind(c, &x) {
			return
		}
		v, e := p.UpdateProject(c.Request.Context(), developer(c).ID, c.Param("project"), x)
		respond(c, 200, v, e)
	})
	for _, resource := range []string{"dashboard", "api-keys", "signing-keys", "webhooks", "transactions", "api-logs", "webhook-jobs"} {
		resource := resource
		private.GET("/projects/:project/"+resource, func(c *gin.Context) {
			f, e := filter(c)
			if e != nil {
				respond(c, 0, nil, e)
				return
			}
			v, e := p.ProjectView(c.Request.Context(), developer(c).ID, c.Param("project"), resource, "", f)
			respond(c, 200, v, e)
		})
	}
	private.GET("/projects/:project/transactions/:id", func(c *gin.Context) {
		v, e := p.ProjectView(c.Request.Context(), developer(c).ID, c.Param("project"), "transaction", c.Param("id"), domain.Filter{})
		respond(c, 200, v, e)
	})
	private.GET("/projects/:project/webhook-jobs/:id/deliveries", func(c *gin.Context) {
		v, e := p.ProjectView(c.Request.Context(), developer(c).ID, c.Param("project"), "deliveries", c.Param("id"), domain.Filter{})
		respond(c, 200, v, e)
	})
	private.POST("/projects/:project/api-keys", a.createKey)
	private.POST("/projects/:project/api-keys/:id/rotate", a.createKey)
	private.DELETE("/projects/:project/api-keys/:id", func(c *gin.Context) {
		respond(c, 200, gin.H{"revoked": true}, p.RevokeAPIKey(c.Request.Context(), developer(c).ID, c.Param("project"), c.Param("id")))
	})
	private.POST("/projects/:project/signing-keys", func(c *gin.Context) {
		v, secret, e := p.NewSigningKey(c.Request.Context(), developer(c).ID, c.Param("project"))
		respond(c, 201, gin.H{"key": v, "secret": secret}, e)
	})
	private.DELETE("/projects/:project/signing-keys/:id", func(c *gin.Context) {
		respond(c, 200, gin.H{"revoked": true}, p.RevokeSigningKey(c.Request.Context(), developer(c).ID, c.Param("project"), c.Param("id")))
	})
	private.POST("/projects/:project/webhooks", a.saveEndpoint)
	private.PATCH("/projects/:project/webhooks/:id", a.saveEndpoint)
	private.DELETE("/projects/:project/webhooks/:id", func(c *gin.Context) {
		respond(c, 200, gin.H{"disabled": true}, p.DisableEndpoint(c.Request.Context(), developer(c).ID, c.Param("project"), c.Param("id")))
	})
	private.POST("/projects/:project/webhooks/:id/test", func(c *gin.Context) {
		id, e := p.TestWebhook(c.Request.Context(), developer(c).ID, c.Param("project"), c.Param("id"))
		respond(c, 202, gin.H{"eventId": id}, e)
	})
	private.POST("/projects/:project/webhook-jobs/:id/retry", func(c *gin.Context) {
		respond(c, 202, gin.H{"scheduled": true}, p.RetryWebhook(c.Request.Context(), developer(c).ID, c.Param("project"), c.Param("id")))
	})
	sim := api.Group("/simulator")
	sim.GET("/payments/:token", a.payment)
	sim.POST("/resolve", a.origin, func(c *gin.Context) {
		var x struct {
			Reference string `json:"reference"`
			Project   string `json:"projectId"`
		}
		if !bind(c, &x) {
			return
		}
		owner := ""
		if token, e := c.Cookie("phajay_session"); e == nil {
			d, _, e := p.Authenticate(c.Request.Context(), token)
			if e == nil {
				owner = d.ID
			}
		}
		v, e := p.Resolve(c.Request.Context(), owner, x.Project, x.Reference)
		respond(c, 200, v, e)
	})
	sim.POST("/payments/:token/confirm", a.origin, a.confirm)
	sim.GET("/checkout/:id", a.checkout)
	sim.GET("/checkout/:id/events", a.events)
	gateway := r.Group("/v1/api/test/payment")
	gateway.Use(a.sdkAuth)
	gateway.POST("/get-payment-link", a.generate("LINK", "JDB"))
	for _, bank := range []string{"JDB", "LDB", "IB", "BCEL", "STB"} {
		gateway.POST("/generate-"+strings.ToLower(bank)+"-qr", a.generate("QR", bank))
	}
	r.NoRoute(a.spa)
	return r
}

func (a *Adapter) security(c *gin.Context) {
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Frame-Options", "DENY")
	c.Header("Permissions-Policy", "camera=(self), microphone=(), geolocation=()")
	c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; connect-src 'self'; worker-src 'self' blob:; media-src 'self' blob:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
	if a.cfg.SecureCookies {
		c.Header("Strict-Transport-Security", "max-age=31536000")
	}
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	if !strings.HasSuffix(c.Request.URL.Path, "/events") {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
	}
	c.Next()
	if failure, ok := c.Get("internal_error"); ok {
		a.logger.Error("request failed", "method", c.Request.Method, "route", c.FullPath(), "error", failure)
	}
}
func (a *Adapter) origin(c *gin.Context) {
	if c.GetHeader("Origin") != a.cfg.PublicURL || c.GetHeader("X-Phajay-Portal") != "1" {
		c.Abort()
		respond(c, 0, nil, domain.ErrUnauthorized)
		return
	}
	c.Next()
}
func (a *Adapter) authenticate(c *gin.Context) {
	token, e := c.Cookie("phajay_session")
	if e != nil {
		c.Abort()
		respond(c, 0, nil, domain.ErrUnauthorized)
		return
	}
	d, s, e := a.p.Authenticate(c.Request.Context(), token)
	if e != nil {
		c.Abort()
		respond(c, 0, nil, e)
		return
	}
	c.Set("developer", d)
	c.Set("session", s)
	c.Next()
}
func (a *Adapter) csrf(c *gin.Context) {
	if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
		c.Next()
		return
	}
	if c.GetHeader("Origin") != a.cfg.PublicURL || !a.p.ValidateCSRF(session(c), c.GetHeader("X-CSRF-Token")) {
		c.Abort()
		respond(c, 0, nil, domain.ErrUnauthorized)
		return
	}
	c.Next()
}
func developer(c *gin.Context) domain.Developer { return c.MustGet("developer").(domain.Developer) }
func session(c *gin.Context) domain.Session     { return c.MustGet("session").(domain.Session) }
func bind(c *gin.Context, v any) bool {
	d := json.NewDecoder(c.Request.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		respond(c, 0, nil, domain.ErrInvalid)
		return false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		respond(c, 0, nil, domain.ErrInvalid)
		return false
	}
	return true
}
func respond(c *gin.Context, code int, value any, err error) {
	if err == nil {
		c.JSON(code, gin.H{"data": value})
		return
	}
	status := 500
	name, message := "internal_error", "Request failed"
	switch {
	case errors.Is(err, domain.ErrInvalid):
		status, name, message = 400, "invalid_input", "Check the supplied values"
	case errors.Is(err, domain.ErrUnauthorized):
		status, name, message = 401, "unauthorized", "Authentication or request verification failed"
	case errors.Is(err, domain.ErrNotFound):
		status, name, message = 404, "not_found", "Resource not found"
	case errors.Is(err, domain.ErrExpired):
		status, name, message = 410, "expired", "Payment has expired"
	case errors.Is(err, domain.ErrConflict):
		status, name, message = 409, "conflict", "Operation conflicts with the current state"
	}
	if status == 500 {
		c.Set("internal_error", err.Error())
	}
	c.JSON(status, gin.H{"error": gin.H{"code": name, "message": message}})
}
func (a *Adapter) register(c *gin.Context) {
	var x struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if !bind(c, &x) {
		return
	}
	v, e := a.p.Register(c.Request.Context(), x.Email, x.Name, x.Password)
	respond(c, 201, v, e)
}
func (a *Adapter) login(c *gin.Context) {
	var x struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !bind(c, &x) {
		return
	}
	d, s, token, csrf, e := a.p.Login(c.Request.Context(), x.Email, x.Password)
	if e != nil {
		respond(c, 0, nil, e)
		return
	}
	for _, item := range []struct {
		name, value string
		httpOnly    bool
	}{{"phajay_session", token, true}, {"phajay_csrf", csrf, false}} {
		http.SetCookie(c.Writer, &http.Cookie{Name: item.name, Value: item.value, Path: "/", HttpOnly: item.httpOnly, Secure: a.cfg.SecureCookies, SameSite: http.SameSiteStrictMode, MaxAge: int(a.cfg.SessionTTL / time.Second)})
	}
	respond(c, 200, gin.H{"developer": d, "session": s, "csrfToken": csrf}, nil)
}
func (a *Adapter) clearCookies(c *gin.Context) {
	for _, name := range []string{"phajay_session", "phajay_csrf"} {
		http.SetCookie(c.Writer, &http.Cookie{Name: name, Value: "", Path: "/", HttpOnly: name == "phajay_session", Secure: a.cfg.SecureCookies, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	}
}
func (a *Adapter) logout(c *gin.Context) {
	e := a.p.Logout(c.Request.Context(), session(c))
	if e == nil {
		a.clearCookies(c)
	}
	respond(c, 200, gin.H{"loggedOut": true}, e)
}
func (a *Adapter) me(c *gin.Context) {
	csrf, _ := c.Cookie("phajay_csrf")
	if !a.p.ValidateCSRF(session(c), csrf) {
		csrf = ""
	}
	respond(c, 200, gin.H{"developer": developer(c), "session": session(c), "csrfToken": csrf}, nil)
}
func (a *Adapter) createKey(c *gin.Context) {
	var x struct {
		Name string `json:"name"`
	}
	if !bind(c, &x) {
		return
	}
	v, secret, e := a.p.NewAPIKey(c.Request.Context(), developer(c).ID, c.Param("project"), x.Name, c.Param("id"))
	respond(c, 201, gin.H{"key": v, "secret": secret}, e)
}
func (a *Adapter) saveEndpoint(c *gin.Context) {
	var x domain.Endpoint
	if !bind(c, &x) {
		return
	}
	v, e := a.p.SaveEndpoint(c.Request.Context(), developer(c).ID, c.Param("project"), c.Param("id"), x, a.sender)
	respond(c, 200, v, e)
}
func filter(c *gin.Context) (domain.Filter, error) {
	f := domain.Filter{Search: c.Query("search"), Status: c.Query("status"), Bank: c.Query("bank"), Limit: 25}
	if len(f.Search) > 100 {
		return f, domain.ErrInvalid
	}
	for _, v := range []struct {
		key    string
		target *int
	}{{"limit", &f.Limit}, {"offset", &f.Offset}} {
		if raw := c.Query(v.key); raw != "" {
			n, e := strconv.Atoi(raw)
			if e != nil {
				return f, domain.ErrInvalid
			}
			*v.target = n
		}
	}
	for _, v := range []struct {
		key    string
		target *int64
	}{{"from", &f.From}, {"to", &f.To}} {
		if raw := c.Query(v.key); raw != "" {
			n, e := strconv.ParseInt(raw, 10, 64)
			if e != nil || n < 0 {
				return f, domain.ErrInvalid
			}
			*v.target = n
		}
	}
	if f.Limit < 1 || f.Limit > 100 || f.Offset < 0 || f.Offset > 1000000 {
		return f, domain.ErrInvalid
	}
	if f.Bank != "" && !service.ValidBank(f.Bank) {
		return f, domain.ErrInvalid
	}
	if f.Status != "" && f.Status != string(domain.Waiting) && !domain.CanTransition(domain.Waiting, domain.State(f.Status)) {
		return f, domain.ErrInvalid
	}
	return f, nil
}
func (a *Adapter) sdkAuth(c *gin.Context) {
	key := ""
	if strings.HasSuffix(c.Request.URL.Path, "/get-payment-link") {
		header := c.GetHeader("Authorization")
		if strings.HasPrefix(header, "Basic ") && len(header) < 256 {
			decoded, e := base64.StdEncoding.DecodeString(strings.TrimPrefix(header, "Basic "))
			if e == nil {
				key = string(decoded)
			}
		}
	} else {
		key = c.GetHeader("secretKey")
	}
	p, e := a.p.APIKeyProject(c.Request.Context(), key)
	if e != nil {
		c.Abort()
		respond(c, 0, nil, e)
		return
	}
	c.Set("project", p)
	started := time.Now()
	c.Next()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if e := a.p.RecordRequest(ctx, p.ID, c.Request.Method, c.Request.URL.Path, c.Writer.Status(), time.Since(started)); e != nil {
		a.logger.Error("API audit log failed")
	}
}
func (a *Adapter) generate(kind, bank string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input domain.PaymentInput
		if !bind(c, &input) {
			return
		}
		t, e := a.p.CreatePayment(c.Request.Context(), c.MustGet("project").(domain.Project), bank, kind, input)
		if e != nil {
			respond(c, 0, nil, e)
			return
		}
		if kind == "LINK" {
			c.JSON(200, gin.H{"message": "Sandbox payment link created", "orderNo": t.OrderNo, "redirectURL": t.CheckoutURL})
			return
		}
		c.JSON(200, gin.H{"message": "Sandbox QR created", "transactionId": t.ID, "qrCode": t.PaymentURL, "link": t.PaymentURL})
	}
}
func (a *Adapter) payment(c *gin.Context) {
	v, e := a.p.Payment(c.Request.Context(), c.Param("token"))
	respond(c, 200, v, e)
}
func (a *Adapter) confirm(c *gin.Context) {
	var x struct {
		Status domain.State `json:"status"`
	}
	if !bind(c, &x) {
		return
	}
	v, e := a.p.Confirm(c.Request.Context(), c.Param("token"), x.Status)
	respond(c, 200, v, e)
}
func (a *Adapter) checkoutTransaction(c *gin.Context) (domain.Transaction, error) {
	v, e := a.p.Payment(c.Request.Context(), c.Query("token"))
	if e != nil {
		return v, e
	}
	if v.ID != c.Param("id") {
		return domain.Transaction{}, domain.ErrNotFound
	}
	return v, nil
}
func (a *Adapter) checkout(c *gin.Context) { v, e := a.checkoutTransaction(c); respond(c, 200, v, e) }
func (a *Adapter) events(c *gin.Context) {
	v, e := a.checkoutTransaction(c)
	if e != nil {
		respond(c, 0, nil, e)
		return
	}
	updates, unsubscribe := a.p.Subscribe(v.ID)
	defer unsubscribe()
	latest, e := a.checkoutTransaction(c)
	if e != nil {
		respond(c, 0, nil, e)
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache, no-store")
	c.Header("X-Accel-Buffering", "no")
	send := func(t domain.Transaction) bool {
		raw, e := json.Marshal(gin.H{"id": t.ID, "status": t.Status, "completedAt": t.CompletedAt})
		if e != nil {
			return false
		}
		_, e = fmt.Fprintf(c.Writer, "event: payment\ndata: %s\n\n", raw)
		c.Writer.Flush()
		return e == nil
	}
	if !send(latest) {
		return
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-a.stop.Done():
			return
		case <-c.Request.Context().Done():
			return
		case t := <-updates:
			if !send(t) {
				return
			}
		case <-ticker.C:
			latest, e := a.checkoutTransaction(c)
			if e != nil {
				return
			}
			if !send(latest) {
				return
			}
		}
	}
}
func (a *Adapter) spa(c *gin.Context) {
	path := c.Request.URL.Path
	if c.Request.Method != "GET" || path == "/api" || path == "/v1" || strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/v1/") {
		respond(c, 0, nil, domain.ErrNotFound)
		return
	}
	if strings.HasPrefix(path, "/portal") {
		token, _ := c.Cookie("phajay_session")
		if _, _, e := a.p.Authenticate(c.Request.Context(), token); e != nil {
			c.Redirect(303, "/login")
			return
		}
	}
	clean := filepath.Clean(strings.TrimPrefix(path, "/"))
	if clean != "." && !strings.HasPrefix(clean, "..") {
		asset := filepath.Join(a.cfg.WebDistDir, clean)
		if info, e := os.Stat(asset); e == nil && !info.IsDir() {
			c.File(asset)
			return
		}
	}
	index := filepath.Join(a.cfg.WebDistDir, "index.html")
	if _, e := os.Stat(index); e != nil {
		c.JSON(503, gin.H{"error": gin.H{"code": "frontend_unavailable", "message": "Frontend assets must be built with Bun"}})
		return
	}
	c.File(index)
}
