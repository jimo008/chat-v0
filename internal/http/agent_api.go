package http

import (
	"net/http"
	"strconv"
	"time"
)

func (s *Server) agentMe(w http.ResponseWriter, r *http.Request) {
	agent, ok := agentFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "agent_not_found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"agent": map[string]any{
			"id":       agent.AgentID,
			"username": agent.Username,
			"email":    agent.Email,
		},
		"device": map[string]any{
			"id": agent.DeviceID,
		},
	})
}

func (s *Server) agentSync(w http.ResponseWriter, r *http.Request) {
	agent, ok := agentFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "agent_not_found"})
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

	if _, err := s.db.ExecContext(r.Context(), `UPDATE agent_devices SET last_sync_at = NOW(3), updated_at = NOW(3) WHERE id = ?`, agent.DeviceID); err != nil {
		s.logger.Error("update agent sync", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "sync_failed"})
		return
	}

	rows, err := s.db.QueryContext(
		r.Context(),
		`SELECT event_id, seq, site_id, type, payload_json, created_at
		 FROM events
		 WHERE seq > ?
		 ORDER BY seq ASC
		 LIMIT 200`,
		afterSeq,
	)
	if err != nil {
		s.logger.Error("query sync events", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "sync_events_failed"})
		return
	}
	defer rows.Close()

	events := make([]map[string]any, 0)
	latestSeq := afterSeq
	for rows.Next() {
		var eventID, typ, payload string
		var seq, siteID uint64
		var createdAt time.Time
		if err := rows.Scan(&eventID, &seq, &siteID, &typ, &payload, &createdAt); err != nil {
			s.logger.Error("scan sync event", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "sync_event_scan_failed"})
			return
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
	if err := rows.Err(); err != nil {
		s.logger.Error("iterate sync events", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "sync_event_iter_failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"latest_seq": latestSeq,
		"events":     events,
	})
}

type jsonRaw string

func (r jsonRaw) MarshalJSON() ([]byte, error) {
	if r == "" {
		return []byte("null"), nil
	}
	return []byte(r), nil
}
