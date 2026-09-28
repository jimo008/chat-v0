package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jimo008/chat-v0/internal/config"
	httpserver "github.com/jimo008/chat-v0/internal/http"
	"github.com/jimo008/chat-v0/internal/migrations"
	"github.com/jimo008/chat-v0/internal/storage"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}

	db, err := storage.OpenMySQL(cfg.MySQLDSN)
	if err != nil {
		logger.Error("open mysql", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := migrations.Run(context.Background(), db, "migrations"); err != nil {
		logger.Error("run migrations", "error", err)
		os.Exit(1)
	}

	redisClient := storage.OpenRedis(cfg.Redis)
	defer redisClient.Close()

	app := httpserver.NewServer(cfg, db, redisClient, logger)
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           app.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("support api listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server failed", "error", err)
			os.Exit(1)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http shutdown failed", "error", err)
	}
}
