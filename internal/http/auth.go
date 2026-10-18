package http

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/jimo008/chat-v0/internal/security"
)

type contextKey string

const agentContextKey contextKey = "agent"
const customerContextKey contextKey = "customer"

type authenticatedAgent struct {
	AgentID   uint64
	DeviceID  uint64
	DeviceKey string
	Username  string
	Email     string
	PublicKey string
}

type authenticatedCustomer struct {
	SiteID         uint64
	SiteKey        string
	CustomerID     uint64
	ConversationID uint64
	Email          string
	XBoardUserID   any
	Blocked        bool
	TokenCreatedAt time.Time
}

func (s *Server) requireAgent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		token := strings.TrimPrefix(auth, "Bearer ")
		if token == "" || token == auth {
			if s.rejectUnknownAgent(w, r) {
				return
			}
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing_agent_token"})
			return
		}

		var agent authenticatedAgent
		var publicKey sql.NullString
		err := s.db.QueryRowContext(
			r.Context(),
			`SELECT a.id, d.id, d.device_id, a.username, a.email, d.public_key
			 FROM agent_devices d
			 JOIN agents a ON a.id = d.agent_id
			 WHERE d.token_hash = ?
			 LIMIT 1`,
			security.TokenHash(token),
		).Scan(&agent.AgentID, &agent.DeviceID, &agent.DeviceKey, &agent.Username, &agent.Email, &publicKey)
		if err == sql.ErrNoRows {
			if s.rejectUnknownAgent(w, r) {
				return
			}
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_agent_token"})
			return
		}
		if err != nil {
			s.logger.Error("agent token lookup", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "agent_auth_failed"})
			return
		}

		agent.PublicKey = publicKey.String
		if err := s.verifyAuthenticatedDevice(r, agent); err != nil {
			s.writeAgentAuthError(w, r, err)
			return
		}
		ctx := context.WithValue(r.Context(), agentContextKey, agent)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func agentFromContext(ctx context.Context) (authenticatedAgent, bool) {
	agent, ok := ctx.Value(agentContextKey).(authenticatedAgent)
	return agent, ok
}

func (s *Server) requireCustomer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		token := strings.TrimPrefix(auth, "Bearer ")
		if token == "" || token == auth {
			token = strings.TrimSpace(r.URL.Query().Get("token"))
		}
		if token == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing_customer_token"})
			return
		}

		var customer authenticatedCustomer
		var xboardUserID sql.NullString
		err := s.db.QueryRowContext(
			r.Context(),
			`SELECT s.id, s.site_key, c.id, conv.id, c.email_original, c.xboard_user_id, c.blocked, t.created_at
			 FROM customer_tokens t
			 JOIN customers c ON c.id = t.customer_id
			 JOIN sites s ON s.id = c.site_id
			 JOIN conversations conv ON conv.site_id = c.site_id AND conv.customer_id = c.id
			 WHERE t.token_hash = ? AND t.revoked_at IS NULL
			 LIMIT 1`,
			security.TokenHash(token),
		).Scan(&customer.SiteID, &customer.SiteKey, &customer.CustomerID, &customer.ConversationID, &customer.Email, &xboardUserID, &customer.Blocked, &customer.TokenCreatedAt)
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_customer_token"})
			return
		}
		if err != nil {
			s.logger.Error("customer token lookup", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "customer_auth_failed"})
			return
		}
		if xboardUserID.Valid {
			customer.XBoardUserID = xboardUserID.String
		}
		policy, err := s.loadSecurityPolicy(r.Context())
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "security_policy_unavailable"})
			return
		}
		siteKey := requestSiteKey(r)
		if siteKey == "" && policy.RequireSiteKey && policy.SiteKeyEnforcedAt != nil && customer.TokenCreatedAt.Before(*policy.SiteKeyEnforcedAt) {
			writeJSON(w, http.StatusUpgradeRequired, map[string]string{"error": "widget_reload_required"})
			return
		}
		if (siteKey == "" && policy.RequireSiteKey) || (siteKey != "" && siteKey != customer.SiteKey) || conflictingSiteKey(r, customer.SiteKey) {
			s.rejectSecurity(w, r, "invalid_site")
			return
		}

		_, _ = s.db.ExecContext(r.Context(), `UPDATE customer_tokens SET last_used_at = NOW(3) WHERE token_hash = ?`, security.TokenHash(token))
		ctx := context.WithValue(r.Context(), customerContextKey, customer)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func customerFromContext(ctx context.Context) (authenticatedCustomer, bool) {
	customer, ok := ctx.Value(customerContextKey).(authenticatedCustomer)
	return customer, ok
}
