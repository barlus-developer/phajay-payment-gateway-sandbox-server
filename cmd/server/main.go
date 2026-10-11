package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	httpadapter "github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/adapters/http"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/adapters/sqlite"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/adapters/webhook"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/config"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if e := run(logger); e != nil {
		logger.Error("server stopped", "error", e.Error())
		os.Exit(1)
	}
}
func run(logger *slog.Logger) error {
	health := flag.Bool("healthcheck", false, "check the local application health")
	backup := flag.String("backup", "", "write a consistent SQLite snapshot to a new path and exit")
	flag.Parse()
	if *health {
		addr := os.Getenv("HTTP_ADDR")
		if addr == "" {
			addr = ":8080"
		}
		if strings.HasPrefix(addr, ":") {
			addr = "127.0.0.1" + addr
		}
		client := http.Client{Timeout: 3 * time.Second}
		res, e := client.Get("http://" + addr + "/readyz")
		if e != nil {
			return e
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return fmt.Errorf("readiness returned %d", res.StatusCode)
		}
		return nil
	}
	cfg, e := config.Load()
	if e != nil {
		return e
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	store, e := sqlite.Open(ctx, cfg.DatabasePath, cfg.MigrationsDir)
	if e != nil {
		return e
	}
	defer store.Close()
	if *backup != "" {
		return store.Backup(ctx, *backup)
	}
	p, e := service.New(store, cfg)
	if e != nil {
		return e
	}
	sender := webhook.New(cfg.WebhookTimeout, cfg.AllowLocalWebhooks)
	defer sender.Close()
	worker := service.NewWorker(p, sender, logger)
	done := make(chan struct{})
	go func() { defer close(done); worker.Run(ctx) }()
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: httpadapter.New(p, sender, cfg, logger, ctx), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	server.ReadTimeout = 15 * time.Second
	errors := make(chan error, 1)
	go func() {
		logger.Info("Phajay sandbox started", "address", cfg.HTTPAddr, "publicURL", cfg.PublicURL)
		errors <- server.ListenAndServe()
	}()
	select {
	case e = <-errors:
		if e != nil && e != http.ErrServerClosed {
			stop()
			<-done
			return e
		}
	case <-ctx.Done():
	}
	stop()
	shutdown, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	e = server.Shutdown(shutdown)
	if e != nil {
		_ = server.Close()
	}
	select {
	case <-done:
	case <-shutdown.Done():
		return shutdown.Err()
	}
	return e
}
