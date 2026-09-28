package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jimo008/chat-v0/internal/storage"
	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv                      string
	AppBaseURL                  string
	HTTPAddr                    string
	AdminToken                  string
	MySQLDSN                    string
	Redis                       storage.RedisConfig
	UploadStoragePath           string
	PostalSMTPHost              string
	PostalSMTPPort              int
	PostalSMTPUser              string
	PostalSMTPPassword          string
	PostalSMTPFromDomain        string
	EmailUnreadDelaySeconds     int
	AgentSyncIntervalSeconds    int
	EmergencyDeviceAliveSeconds int
	EmergencyExpireSeconds      int
	RetentionCleanupEnabled     bool
	Timezone                    string
}

func Load() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		AppEnv:     env("APP_ENV", "development"),
		AppBaseURL: env("APP_BASE_URL", "http://localhost:8080"),
		HTTPAddr:   env("HTTP_ADDR", ":8080"),
		AdminToken: env("ADMIN_TOKEN", ""),
		MySQLDSN:   env("MYSQL_DSN", "support:support_password@tcp(localhost:3306)/support_chat?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci"),
		Redis: storage.RedisConfig{
			Addr:     env("REDIS_ADDR", "localhost:6379"),
			Password: env("REDIS_PASSWORD", ""),
			DB:       envInt("REDIS_DB", 0),
		},
		UploadStoragePath:           env("UPLOAD_STORAGE_PATH", "./storage/uploads"),
		PostalSMTPHost:              env("POSTAL_SMTP_HOST", ""),
		PostalSMTPPort:              envInt("POSTAL_SMTP_PORT", 587),
		PostalSMTPUser:              env("POSTAL_SMTP_USER", ""),
		PostalSMTPPassword:          env("POSTAL_SMTP_PASSWORD", ""),
		PostalSMTPFromDomain:        env("POSTAL_SMTP_FROM_DOMAIN", ""),
		EmailUnreadDelaySeconds:     envInt("EMAIL_UNREAD_DELAY_SECONDS", 300),
		AgentSyncIntervalSeconds:    envInt("AGENT_SYNC_INTERVAL_SECONDS", 3),
		EmergencyDeviceAliveSeconds: envInt("EMERGENCY_DEVICE_ALIVE_SECONDS", 60),
		EmergencyExpireSeconds:      envInt("EMERGENCY_EXPIRE_SECONDS", 180),
		RetentionCleanupEnabled:     envBool("RETENTION_CLEANUP_ENABLED", false),
		Timezone:                    env("TZ", "Asia/Shanghai"),
	}

	if cfg.AppBaseURL == "" {
		return Config{}, fmt.Errorf("APP_BASE_URL is required")
	}
	if cfg.MySQLDSN == "" {
		return Config{}, fmt.Errorf("MYSQL_DSN is required")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	value := strings.ToLower(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes" || value == "on"
}
