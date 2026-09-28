package http

import "net/http"

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
	conversationID, err := parsePathUint(r, "id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_conversation_id"})
		return
	}
	result, err := s.db.ExecContext(
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
	affected, _ := result.RowsAffected()
	writeJSON(w, http.StatusOK, map[string]any{"updated": affected})
}
