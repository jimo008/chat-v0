package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jimo008/chat-v0/internal/config"
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

	redisClient := storage.OpenRedis(cfg.Redis)
	defer redisClient.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("support worker started")
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("support worker stopped")
			return
		case <-ticker.C:
			// Job implementations are added in later phases: email batches,
			// emergency expiry, scheduled duty, and retention cleanup.
			logger.Debug("worker tick")
		}
	}
}
