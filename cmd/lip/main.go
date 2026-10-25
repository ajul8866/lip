package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lip-project/lip/internal/audit"
	"github.com/lip-project/lip/internal/config"
	"github.com/lip-project/lip/internal/db"
	"github.com/lip-project/lip/internal/egress"
	"github.com/lip-project/lip/internal/httpapi"
	"github.com/lip-project/lip/internal/i18n"
	"github.com/lip-project/lip/internal/llm"
	"github.com/lip-project/lip/internal/runner"
	"github.com/lip-project/lip/internal/store"
	"github.com/lip-project/lip/internal/webui"
)

var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "version":
		fmt.Printf("lip %s (commit %s, built %s)\n", version, commit, buildDate)
	case "serve":
		if err := serve(); err != nil {
			fmt.Fprintln(os.Stderr, "lip:", err)
			os.Exit(1)
		}
	case "migrate":
		if err := migrate(); err != nil {
			fmt.Fprintln(os.Stderr, "lip migrate:", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "usage: lip [serve|migrate|version]\n")
		os.Exit(2)
	}
}

func load() (config.Config, *slog.Logger, error) {
	cfg, err := config.Load(version)
	if err != nil {
		return cfg, nil, err
	}
	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	if cfg.LogFormat == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	logger := slog.New(handler)
	logger.Info("configuration loaded",
		"version", version,
		"addr", cfg.Addr,
		"safe_local_only", cfg.SafeLocalOnly,
		"safe_mode_source", cfg.SafeModeSource,
		"egress_rules", len(cfg.EgressAllowlist),
		"max_concurrent_tasks", cfg.MaxConcurrent,
		"autorun", cfg.AutoRun,
	)
	for _, warning := range cfg.Warnings {
		logger.Warn("configuration warning", "detail", warning)
	}
	return cfg, logger, nil
}

func migrate() error {
	cfg, logger, err := load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool, err := db.Open(ctx, cfg.DatabaseURL, cfg.DBMaxConns)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool, logger); err != nil {
		return err
	}
	logger.Info("migrations complete")
	return nil
}

func serve() error {
	cfg, logger, err := load()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.ImportRoot, 0o750); err != nil {
		return fmt.Errorf("create import root: %w", err)
	}

	guard, err := egress.New(egress.Options{Safe: cfg.SafeLocalOnly, Allowlist: cfg.EgressAllowlist})
	if err != nil {
		return fmt.Errorf("egress policy: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	pool, err := db.Open(ctx, cfg.DatabaseURL, cfg.DBMaxConns)
	cancel()
	if err != nil {
		return err
	}
	defer pool.Close()

	if cfg.AutoMigrate {
		migrateCtx, migrateCancel := context.WithTimeout(context.Background(), 90*time.Second)
		err = db.Migrate(migrateCtx, pool, logger)
		migrateCancel()
		if err != nil {
			return err
		}
	}

	bundle, err := i18n.New()
	if err != nil {
		return fmt.Errorf("load locales: %w", err)
	}
	if err := bundle.Validate(); err != nil {
		return fmt.Errorf("validate locales: %w", err)
	}
	st := store.New(pool)
	recorder := audit.NewRecorder(logger)
	registry := llm.NewRegistry(guard)
	taskRunner := runner.New(cfg, st, recorder, registry, logger)
	taskRunner.Start(context.Background())
	defer func() {
		taskRunner.Stop()
		done := make(chan struct{})
		go func() {
			taskRunner.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(cfg.ShutdownTimeout):
			logger.Warn("runner did not stop before shutdown deadline")
		}
	}()

	apiServer := httpapi.New(cfg, st, guard, registry, recorder, bundle, logger, webui.Handler())
	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           apiServer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      3 * time.Minute,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	logger.Info("LIP listening", "addr", listener.Addr().String())

	serveErr := make(chan error, 1)
	go func() { serveErr <- httpServer.Serve(listener) }()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sig)

	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case s := <-sig:
		logger.Info("shutdown requested", "signal", s.String())
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer shutdownCancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Warn("graceful shutdown failed", "error", err)
			_ = httpServer.Close()
		}
	}
	return nil
}
