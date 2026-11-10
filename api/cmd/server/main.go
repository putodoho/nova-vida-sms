// Command server runs the Nova Vida SMS API.
package main

import (
	_ "time/tzdata" // embed zone data: the container image has none

	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/putodoho/nova-vida-sms/api/internal/auth"
	"github.com/putodoho/nova-vida-sms/api/internal/config"
	"github.com/putodoho/nova-vida-sms/api/internal/db"
	"github.com/putodoho/nova-vida-sms/api/internal/server"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if cfg.MigrateOnStart {
		if err := db.Migrate(ctx, pool, log); err != nil {
			return err
		}
	}

	handler := server.New(server.Deps{
		Pool: pool,
		Log:  log,
		Auth: &auth.Service{Pool: pool, TTL: cfg.SessionTTL, CookieSecure: cfg.CookieSecure},
	})
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.HTTPAddr, "env", cfg.Env)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
	return nil
}
