package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hwcopeland/iac/kai/internal/api"
	"github.com/hwcopeland/iac/kai/internal/auth"
	"github.com/hwcopeland/iac/kai/internal/config"
	"github.com/hwcopeland/iac/kai/internal/db"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "err", err)
		os.Exit(1)
	}

	if cfg.Dev {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	} else {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("db connect", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, cfg.DatabaseURL); err != nil {
		slog.Error("db migrate", "err", err)
		os.Exit(1)
	}

	oidcClient, err := auth.NewOIDCClient(
		cfg.AuthIssuerURL,
		cfg.AuthClientID,
		cfg.AuthClientSecret,
		cfg.AuthRedirectURL,
	)
	if err != nil {
		slog.Error("oidc init", "err", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      api.NewServer(oidcClient, pool).Router(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("kai-api listening", "addr", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutCtx); err != nil {
		slog.Error("graceful shutdown", "err", err)
	}

	slog.Info("kai shutdown complete")
}
