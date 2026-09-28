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
	mux.HandleFunc("GET /widget.js", s.widgetJS)
	mux.HandleFunc("GET /widget/frame", s.widgetFrame)
	mux.HandleFunc("GET /api/v1/version", s.version)
	mux.Handle("POST /api/v1/admin/agents/init", s.requireAdmin(http.HandlerFunc(s.initAgent)))
	mux.Handle("POST /api/v1/admin/sites", s.requireAdmin(http.HandlerFunc(s.createSite)))
	mux.Handle("GET /api/v1/admin/sites", s.requireAdmin(http.HandlerFunc(s.listSites)))
	mux.HandleFunc("POST /api/v1/customer/xboard-login", s.customerXBoardLogin)
	mux.Handle("GET /api/v1/customer/me", s.requireCustomer(http.HandlerFunc(s.customerMe)))
	mux.Handle("GET /api/v1/customer/conversation", s.requireCustomer(http.HandlerFunc(s.customerConversation)))
	mux.Handle("POST /api/v1/customer/messages", s.requireCustomer(http.HandlerFunc(s.customerSendMessage)))
	mux.Handle("POST /api/v1/customer/images", s.requireCustomer(http.HandlerFunc(s.customerUploadImage)))
	mux.Handle("POST /api/v1/customer/read", s.requireCustomer(http.HandlerFunc(s.customerRead)))
	mux.Handle("GET /api/v1/customer/emergency/status", s.requireCustomer(http.HandlerFunc(s.customerEmergencyStatus)))
	mux.Handle("POST /api/v1/customer/emergency/start", s.requireCustomer(http.HandlerFunc(s.customerEmergencyStart)))
	mux.Handle("POST /api/v1/customer/emergency/cancel", s.requireCustomer(http.HandlerFunc(s.customerEmergencyCancel)))
	mux.HandleFunc("POST /api/v1/agent/login", s.agentLogin)
	mux.Handle("GET /api/v1/agent/me", s.requireAgent(http.HandlerFunc(s.agentMe)))
	mux.Handle("GET /api/v1/agent/sync", s.requireAgent(http.HandlerFunc(s.agentSync)))
	mux.Handle("POST /api/v1/agent/devices/foreground", s.requireAgent(http.HandlerFunc(s.agentDeviceForeground)))
	mux.Handle("POST /api/v1/agent/devices/emergency-duty", s.requireAgent(http.HandlerFunc(s.agentEmergencyDuty)))
	mux.Handle("POST /api/v1/agent/emergency/{id}/accept", s.requireAgent(http.HandlerFunc(s.agentEmergencyAccept)))
	mux.Handle("GET /api/v1/agent/customers", s.requireAgent(http.HandlerFunc(s.agentListCustomers)))
	mux.Handle("GET /api/v1/agent/conversations/{id}", s.requireAgent(http.HandlerFunc(s.agentGetConversation)))
	mux.Handle("POST /api/v1/agent/conversations/{id}/messages", s.requireAgent(http.HandlerFunc(s.agentSendMessage)))
	mux.Handle("POST /api/v1/agent/conversations/{id}/images", s.requireAgent(http.HandlerFunc(s.agentUploadImage)))
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
