package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jimo008/chat-v0/internal/config"
	"github.com/jimo008/chat-v0/internal/jobs"
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
	_ = redisClient

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	runner := jobs.NewRunner(cfg, db, logger)
	logger.Info("support worker started")
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("support worker stopped")
			return
		case <-ticker.C:
			runner.Tick(ctx)
		}
	}
}
