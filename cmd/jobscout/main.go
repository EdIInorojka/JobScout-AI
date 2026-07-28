package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"jobscout.ai/internal/app"
	"jobscout.ai/internal/config"
	"jobscout.ai/internal/integrations/hh"
	tele "jobscout.ai/internal/integrations/telegram"
	"jobscout.ai/internal/migrate"
	"jobscout.ai/internal/store/postgres"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if len(args) > 0 && args[0] == "migrate" {
		return runMigrate(cfg, logger)
	}
	return runServe(cfg, logger)
}

func runMigrate(cfg config.Config, logger *slog.Logger) error {
	db, err := openDB(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := migrate.Up(ctx, db, filepath.Join(".", "migrations")); err != nil {
		return err
	}
	logger.Info("migrations applied")
	return nil
}

func runServe(cfg config.Config, logger *slog.Logger) error {
	db, err := openDB(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return err
	}

	storage := postgres.New(db)
	hhClient, err := hh.NewClient(cfg.HHBaseURL, cfg.HHUserAgent, cfg.HHAccessToken, "hh.ru", time.Duration(cfg.SearchTimeoutSec)*time.Second, time.Duration(cfg.HHMinDelayMS)*time.Millisecond)
	if err != nil {
		return err
	}
	var tgClient *tele.Client
	if cfg.EnableTelegram {
		tgClient = tele.NewClient(cfg.TelegramBotToken, 30*time.Second)
	}
	application := app.New(cfg, storage, hhClient, tgClient, logger)
	if err := application.SeedCoreSources(context.Background()); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           application.Router(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	if cfg.EnableTelegram {
		go func() {
			if err := application.StartTelegramPolling(ctx); err != nil && !errors.Is(err, context.Canceled) {
				errCh <- err
			}
		}()
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	go func() {
		logger.Info("server starting", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}

func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	return db, nil
}
