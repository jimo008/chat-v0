package http

import (
	"database/sql"
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
			"id":        agent.DeviceID,
			"device_id": agent.DeviceKey,
		},
	})
}

func (s *Server) agentDevices(w http.ResponseWriter, r *http.Request) {
	agent, ok := agentFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "agent_not_found"})
		return
	}
	rows, err := s.db.QueryContext(
		r.Context(),
		`SELECT id, device_id, last_sync_at, ws_connected, app_foreground, accept_emergency, updated_at
		 FROM agent_devices
		 WHERE agent_id = ?
		 ORDER BY COALESCE(last_sync_at, updated_at, created_at) DESC, id DESC`,
		agent.AgentID,
	)
	if err != nil {
		s.logger.Error("agent devices", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "devices_failed"})
		return
	}
	defer rows.Close()

	devices := make([]map[string]any, 0)
	for rows.Next() {
		var (
			id              uint64
			deviceKey       string
			lastSyncAt      sql.NullTime
			wsConnected     bool
			appForeground   bool
			acceptEmergency bool
			updatedAt       sql.NullTime
		)
		if err := rows.Scan(&id, &deviceKey, &lastSyncAt, &wsConnected, &appForeground, &acceptEmergency, &updatedAt); err != nil {
			s.logger.Error("scan agent device", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "device_scan_failed"})
			return
		}
		devices = append(devices, map[string]any{
			"id":               id,
			"device_id":        deviceKey,
			"last_sync_at":     nullableTimeValue(lastSyncAt),
			"ws_connected":     wsConnected,
			"app_foreground":   appForeground,
			"accept_emergency": acceptEmergency,
			"updated_at":       nullableTimeValue(updatedAt),
			"is_current":       id == agent.DeviceID,
		})
	}
	if err := rows.Err(); err != nil {
		s.logger.Error("iterate agent devices", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "device_iter_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": devices})
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
