package http

import (
	"net/http"
	"strconv"
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

	latestSeq, events, err := s.agentEvents(r.Context(), agent.AgentID, afterSeq, 200)
	if err != nil {
		s.logger.Error("query sync events", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "sync_events_failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"latest_seq": latestSeq,
		"events":     events,
	})
}
