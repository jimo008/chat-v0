package http

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"github.com/jimo008/chat-v0/internal/security"
)

type contextKey string

const agentContextKey contextKey = "agent"

type authenticatedAgent struct {
	AgentID  uint64
	DeviceID uint64
	Username string
	Email    string
}

func (s *Server) requireAgent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		token := strings.TrimPrefix(auth, "Bearer ")
		if token == "" || token == auth {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing_agent_token"})
			return
		}

		var agent authenticatedAgent
		err := s.db.QueryRowContext(
			r.Context(),
			`SELECT a.id, d.id, a.username, a.email
			 FROM agent_devices d
			 JOIN agents a ON a.id = d.agent_id
			 WHERE d.token_hash = ?
			 LIMIT 1`,
			security.TokenHash(token),
		).Scan(&agent.AgentID, &agent.DeviceID, &agent.Username, &agent.Email)
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_agent_token"})
			return
		}
		if err != nil {
			s.logger.Error("agent token lookup", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "agent_auth_failed"})
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
