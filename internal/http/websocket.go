package http

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/jimo008/chat-v0/internal/security"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func (s *Server) agentWebSocket(w http.ResponseWriter, r *http.Request) {
	agent, err := s.authenticateAgentRequest(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	afterSeq := uint64(0)
	if raw := r.URL.Query().Get("after_seq"); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_after_seq"})
			return
		}
		afterSeq = parsed
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	_, _ = s.db.ExecContext(r.Context(), `UPDATE agent_devices SET ws_connected = TRUE, updated_at = NOW(3) WHERE id = ?`, agent.DeviceID)
	defer s.db.ExecContext(context.Background(), `UPDATE agent_devices SET ws_connected = FALSE, updated_at = NOW(3) WHERE id = ?`, agent.DeviceID)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			latestSeq, events, err := s.agentEvents(r.Context(), agent.AgentID, afterSeq, 100)
			if err != nil {
				_ = conn.WriteJSON(map[string]any{"type": "error", "error": "event_query_failed"})
				return
			}
			if len(events) == 0 {
				_ = conn.WriteJSON(map[string]any{"type": "heartbeat", "latest_seq": latestSeq})
				continue
			}
			if err := conn.WriteJSON(map[string]any{"type": "events", "latest_seq": latestSeq, "events": events}); err != nil {
				return
			}
			afterSeq = latestSeq
		}
	}
}

func (s *Server) authenticateAgentRequest(r *http.Request) (authenticatedAgent, error) {
	auth := r.Header.Get("Authorization")
	token := strings.TrimPrefix(auth, "Bearer ")
	if token == "" || token == auth {
		token = strings.TrimSpace(r.URL.Query().Get("token"))
	}
	if token == "" {
		return authenticatedAgent{}, sql.ErrNoRows
	}
	return s.agentByToken(r.Context(), token)
}

func (s *Server) agentByToken(ctx context.Context, token string) (authenticatedAgent, error) {
	var agent authenticatedAgent
	err := s.db.QueryRowContext(
		ctx,
		`SELECT a.id, d.id, d.device_id, a.username, a.email
		 FROM agent_devices d
		 JOIN agents a ON a.id = d.agent_id
		 WHERE d.token_hash = ?
		 LIMIT 1`,
		security.TokenHash(token),
	).Scan(&agent.AgentID, &agent.DeviceID, &agent.DeviceKey, &agent.Username, &agent.Email)
	return agent, err
}

func (s *Server) agentEvents(ctx context.Context, agentID, afterSeq uint64, limit int) (uint64, []map[string]any, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT e.event_id, e.seq, e.site_id, e.type, e.payload_json, e.created_at
		 FROM events e
		 JOIN agent_site_access asa ON asa.site_id = e.site_id
		 WHERE asa.agent_id = ? AND e.seq > ?
		 ORDER BY e.seq ASC
		 LIMIT ?`,
		agentID,
		afterSeq,
		limit,
	)
	if err != nil {
		return afterSeq, nil, err
	}
	defer rows.Close()

	latestSeq := afterSeq
	events := make([]map[string]any, 0)
	for rows.Next() {
		var eventID, typ, payload string
		var seq, siteID uint64
		var createdAt time.Time
		if err := rows.Scan(&eventID, &seq, &siteID, &typ, &payload, &createdAt); err != nil {
			return afterSeq, nil, err
		}
		if seq > latestSeq {
			latestSeq = seq
		}
		events = append(events, map[string]any{
			"event_id":   eventID,
			"seq":        seq,
			"site_id":    siteID,
			"type":       typ,
			"payload":    jsonRaw(payload),
			"created_at": createdAt,
		})
	}
	return latestSeq, events, rows.Err()
}
