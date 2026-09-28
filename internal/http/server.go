package http

import (
	"crypto/subtle"
	"database/sql"
	"log/slog"
	"net/http"
	"strings"
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
	mux.Handle("POST /api/v1/admin/agents/init", s.requireAdmin(http.HandlerFunc(s.initAgent)))
	mux.Handle("POST /api/v1/admin/sites", s.requireAdmin(http.HandlerFunc(s.createSite)))
	mux.Handle("GET /api/v1/admin/sites", s.requireAdmin(http.HandlerFunc(s.listSites)))
	mux.HandleFunc("POST /api/v1/agent/login", s.agentLogin)
	return s.withRequestLog(mux)
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.AdminToken == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "admin_token_not_configured"})
			return
		}
		auth := r.Header.Get("Authorization")
		token := strings.TrimPrefix(auth, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.AdminToken)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.logger.Info("http request", "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(start).Milliseconds())
	})
}
