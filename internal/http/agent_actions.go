package http

import (
	"database/sql"
	"net/http"
)

func (s *Server) agentBlockCustomer(w http.ResponseWriter, r *http.Request) {
	customerID, err := parsePathUint(r, "id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_customer_id"})
		return
	}
	if _, err := s.db.ExecContext(r.Context(), `UPDATE customers SET blocked = TRUE, updated_at = NOW(3) WHERE id = ?`, customerID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "block_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"blocked": true})
}

func (s *Server) agentUnblockCustomer(w http.ResponseWriter, r *http.Request) {
	customerID, err := parsePathUint(r, "id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_customer_id"})
		return
	}
	if _, err := s.db.ExecContext(r.Context(), `UPDATE customers SET blocked = FALSE, updated_at = NOW(3) WHERE id = ?`, customerID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unblock_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"blocked": false})
}

func (s *Server) agentMarkConversationRead(w http.ResponseWriter, r *http.Request) {
	agent, ok := agentFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "agent_not_found"})
		return
	}
	conversationID, err := parsePathUint(r, "id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_conversation_id"})
		return
	}
	var siteID uint64
	var maxCustomerSeq sql.NullInt64
	if err := s.db.QueryRowContext(
		r.Context(),
		`SELECT conv.site_id, MAX(m.seq)
		 FROM conversations conv
		 LEFT JOIN messages m ON m.conversation_id = conv.id AND m.sender_type = 'customer'
		 WHERE conv.id = ?
		 GROUP BY conv.site_id`,
		conversationID,
	).Scan(&siteID, &maxCustomerSeq); err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation_not_found"})
		return
	} else if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "conversation_lookup_failed"})
		return
	}
	upToSeq := uint64(0)
	if maxCustomerSeq.Valid && maxCustomerSeq.Int64 > 0 {
		upToSeq = uint64(maxCustomerSeq.Int64)
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "mark_read_failed"})
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(
		r.Context(),
		`UPDATE messages
		 SET agent_read_at = NOW(3)
		 WHERE conversation_id = ? AND sender_type = 'customer' AND agent_read_at IS NULL`,
		conversationID,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "mark_read_failed"})
		return
	}
	if upToSeq > 0 {
		if _, err := tx.ExecContext(
			r.Context(),
			`UPDATE conversations
			 SET agent_last_seen_seq = GREATEST(agent_last_seen_seq, ?),
			     agent_last_seen_at = NOW(3),
			     updated_at = NOW(3)
			 WHERE id = ?`,
			upToSeq,
			conversationID,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "mark_read_cursor_failed"})
			return
		}
		state, err := s.conversationReadState(r.Context(), tx, conversationID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read_state_failed"})
			return
		}
		if _, _, err := createEvent(r.Context(), tx, siteID, "AGENT_READ_UPDATED", map[string]any{
			"conversation_id": conversationID,
			"agent_id":        agent.AgentID,
			"up_to_seq":       upToSeq,
			"read_state":      state,
		}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read_event_failed"})
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "mark_read_commit_failed"})
		return
	}
	affected, _ := result.RowsAffected()
	writeJSON(w, http.StatusOK, map[string]any{"updated": affected})
}
