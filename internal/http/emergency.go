package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
)

func (s *Server) customerEmergencyStatus(w http.ResponseWriter, r *http.Request) {
	customer, ok := customerFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "customer_not_found"})
		return
	}
	available, err := s.emergencyAvailable(r.Context())
	if err != nil {
		s.logger.Error("emergency status", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "emergency_status_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"available": available && !customer.Blocked,
		"reason":    emergencyReason(available, customer.Blocked),
	})
}

func (s *Server) customerEmergencyStart(w http.ResponseWriter, r *http.Request) {
	customer, ok := customerFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "customer_not_found"})
		return
	}
	if customer.Blocked {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "customer_blocked", "message": "当前无法使用在线客服，请通过其他联系方式联系我们。"})
		return
	}
	available, err := s.emergencyAvailable(r.Context())
	if err != nil {
		s.logger.Error("emergency available", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "emergency_check_failed"})
		return
	}
	if !available {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "no_emergency_agent", "message": "当前暂无客服值班，您可以先发送消息。"})
		return
	}

	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.logger.Error("begin emergency", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "emergency_failed"})
		return
	}
	defer tx.Rollback()

	var callID uint64
	err = tx.QueryRowContext(
		r.Context(),
		`SELECT id FROM emergency_calls WHERE customer_id = ? AND status = 'RINGING' ORDER BY id DESC LIMIT 1 FOR UPDATE`,
		customer.CustomerID,
	).Scan(&callID)
	if err == sql.ErrNoRows {
		result, err := tx.ExecContext(
			r.Context(),
			`INSERT INTO emergency_calls (site_id, customer_id, conversation_id, status)
			 VALUES (?, ?, ?, 'RINGING')`,
			customer.SiteID,
			customer.CustomerID,
			customer.ConversationID,
		)
		if err != nil {
			s.logger.Error("insert emergency", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "emergency_create_failed"})
			return
		}
		id, _ := result.LastInsertId()
		callID = uint64(id)
	} else if err != nil {
		s.logger.Error("lookup emergency", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "emergency_lookup_failed"})
		return
	}

	if _, _, err := createEvent(r.Context(), tx, customer.SiteID, "EMERGENCY_STARTED", map[string]any{
		"call_id":         callID,
		"customer_id":     customer.CustomerID,
		"conversation_id": customer.ConversationID,
	}); err != nil {
		s.logger.Error("emergency event", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "emergency_event_failed"})
		return
	}
	if err := tx.Commit(); err != nil {
		s.logger.Error("commit emergency", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "emergency_commit_failed"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"call_id": callID, "status": "RINGING"})
}

func (s *Server) customerEmergencyCancel(w http.ResponseWriter, r *http.Request) {
	customer, ok := customerFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "customer_not_found"})
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cancel_failed"})
		return
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(
		r.Context(),
		`UPDATE emergency_calls
		 SET status = 'CANCELLED', cancelled_at = NOW(3)
		 WHERE customer_id = ? AND status = 'RINGING'`,
		customer.CustomerID,
	)
	if err != nil {
		s.logger.Error("cancel emergency", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cancel_update_failed"})
		return
	}
	affected, _ := result.RowsAffected()
	if affected > 0 {
		if _, _, err := createEvent(r.Context(), tx, customer.SiteID, "EMERGENCY_CANCELLED", map[string]any{
			"customer_id": customer.CustomerID,
		}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cancel_event_failed"})
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cancel_commit_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cancelled": affected})
}

func (s *Server) agentEmergencyAccept(w http.ResponseWriter, r *http.Request) {
	agent, ok := agentFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "agent_not_found"})
		return
	}
	callID, err := parsePathUint(r, "id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_call_id"})
		return
	}
	if err := s.acceptEmergencyCall(r.Context(), callID, agent.AgentID, agent.DeviceID); err != nil {
		s.logger.Error("accept emergency", "error", err)
		writeJSON(w, http.StatusConflict, map[string]string{"error": "accept_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"call_id": callID, "status": "ACCEPTED"})
}

type agentForegroundRequest struct {
	Foreground bool `json:"foreground"`
}

func (s *Server) agentDeviceForeground(w http.ResponseWriter, r *http.Request) {
	agent, ok := agentFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "agent_not_found"})
		return
	}
	var req agentForegroundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if _, err := s.db.ExecContext(r.Context(), `UPDATE agent_devices SET app_foreground = ?, updated_at = NOW(3) WHERE id = ?`, req.Foreground, agent.DeviceID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "foreground_failed"})
		return
	}
	accepted := uint64(0)
	if req.Foreground {
		count, err := s.acceptAllRinging(r.Context(), agent.AgentID, agent.DeviceID)
		if err != nil {
			s.logger.Error("foreground accept all", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "foreground_accept_failed"})
			return
		}
		accepted = count
	}
	writeJSON(w, http.StatusOK, map[string]any{"foreground": req.Foreground, "accepted_calls": accepted})
}

type agentDutyRequest struct {
	AcceptEmergency bool `json:"accept_emergency"`
}

func (s *Server) agentEmergencyDuty(w http.ResponseWriter, r *http.Request) {
	agent, ok := agentFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "agent_not_found"})
		return
	}
	var req agentDutyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if _, err := s.db.ExecContext(r.Context(), `UPDATE agent_devices SET accept_emergency = ?, updated_at = NOW(3) WHERE id = ?`, req.AcceptEmergency, agent.DeviceID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "duty_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"accept_emergency": req.AcceptEmergency})
}

func (s *Server) emergencyAvailable(ctx context.Context) (bool, error) {
	var count int
	err := s.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM agent_devices
		 WHERE accept_emergency = TRUE
		   AND last_sync_at >= DATE_SUB(NOW(3), INTERVAL ? SECOND)`,
		s.cfg.EmergencyDeviceAliveSeconds,
	).Scan(&count)
	return count > 0, err
}

func emergencyReason(available bool, blocked bool) string {
	if blocked {
		return "blocked"
	}
	if !available {
		return "no_emergency_agent"
	}
	return "available"
}

func (s *Server) acceptEmergencyCall(ctx context.Context, callID, agentID, deviceID uint64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var siteID uint64
	err = tx.QueryRowContext(ctx, `SELECT site_id FROM emergency_calls WHERE id = ? AND status = 'RINGING' FOR UPDATE`, callID).Scan(&siteID)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(
		ctx,
		`UPDATE emergency_calls
		 SET status = 'ACCEPTED', accepted_by_agent_id = ?, accepted_by_device_id = ?, accepted_at = NOW(3)
		 WHERE id = ? AND status = 'RINGING'`,
		agentID,
		deviceID,
		callID,
	)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	if _, _, err := createEvent(ctx, tx, siteID, "EMERGENCY_ACCEPTED", map[string]any{
		"call_id":   callID,
		"agent_id":  agentID,
		"device_id": deviceID,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Server) acceptAllRinging(ctx context.Context, agentID, deviceID uint64) (uint64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id, site_id FROM emergency_calls WHERE status = 'RINGING' FOR UPDATE`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	type call struct {
		ID     uint64
		SiteID uint64
	}
	calls := make([]call, 0)
	for rows.Next() {
		var c call
		if err := rows.Scan(&c.ID, &c.SiteID); err != nil {
			return 0, err
		}
		calls = append(calls, c)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, c := range calls {
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE emergency_calls
			 SET status = 'ACCEPTED', accepted_by_agent_id = ?, accepted_by_device_id = ?, accepted_at = NOW(3)
			 WHERE id = ? AND status = 'RINGING'`,
			agentID,
			deviceID,
			c.ID,
		); err != nil {
			return 0, err
		}
		if _, _, err := createEvent(ctx, tx, c.SiteID, "EMERGENCY_ACCEPTED", map[string]any{
			"call_id":   c.ID,
			"agent_id":  agentID,
			"device_id": deviceID,
		}); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return uint64(len(calls)), nil
}

func (s *Server) expireEmergencyCalls(ctx context.Context) (uint64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(
		ctx,
		`SELECT id, site_id FROM emergency_calls
		 WHERE status = 'RINGING'
		   AND created_at < DATE_SUB(NOW(3), INTERVAL ? SECOND)
		 FOR UPDATE`,
		s.cfg.EmergencyExpireSeconds,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	type call struct {
		ID     uint64
		SiteID uint64
	}
	calls := make([]call, 0)
	for rows.Next() {
		var c call
		if err := rows.Scan(&c.ID, &c.SiteID); err != nil {
			return 0, err
		}
		calls = append(calls, c)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, c := range calls {
		if _, err := tx.ExecContext(ctx, `UPDATE emergency_calls SET status = 'EXPIRED', expired_at = NOW(3) WHERE id = ? AND status = 'RINGING'`, c.ID); err != nil {
			return 0, err
		}
		if _, _, err := createEvent(ctx, tx, c.SiteID, "EMERGENCY_EXPIRED", map[string]any{"call_id": c.ID}); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return uint64(len(calls)), nil
}
