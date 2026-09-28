package http

import (
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jimo008/chat-v0/internal/config"
)

type Server struct {
	cfg    config.Config
	db     *sql.DB
	redis  *redis.Client
	logger *slog.Logger
}

func NewServer(cfg config.Config, db *sql.DB, redisClient *redis.Client, logger *slog.Logger) *Server {
	return &Server{cfg: cfg, db: db, redis: redisClient, logger: logger}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /readyz", s.readyz)
	mux.HandleFunc("GET /api/v1/version", s.version)
	return s.withRequestLog(mux)
}

func (s *Server) withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.logger.Info("http request", "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(start).Milliseconds())
	})
}
