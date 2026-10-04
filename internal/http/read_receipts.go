package http

import (
	"encoding/json"
	"net/http"
)

type customerReadRequest struct {
	UpToSeq       uint64 `json:"up_to_seq"`
	Visible       bool   `json:"visible"`
	Open          bool   `json:"open"`
	OpenedSinceMS int64  `json:"opened_since_ms"`
}

func (s *Server) customerRead(w http.ResponseWriter, r *http.Request) {
	customer, ok := customerFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "customer_not_found"})
		return
	}
	var req customerReadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if req.UpToSeq == 0 || !req.Visible || !req.Open || req.OpenedSinceMS < 2000 {
		s.logger.Info(
			"customer read ignored",
			"conversation_id", customer.ConversationID,
			"customer_id", customer.CustomerID,
			"up_to_seq", req.UpToSeq,
			"visible", req.Visible,
			"open", req.Open,
			"opened_since_ms", req.OpenedSinceMS,
		)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "up_to_seq_required"})
		return
	}

	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.logger.Error("begin customer read", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read_failed"})
		return
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(
		r.Context(),
		`UPDATE messages
		 SET customer_read_at = NOW(3)
		 WHERE conversation_id = ?
		   AND sender_type = 'agent'
		   AND seq <= ?
		   AND customer_read_at IS NULL`,
		customer.ConversationID,
		req.UpToSeq,
	)
	if err != nil {
		s.logger.Error("update customer read", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read_update_failed"})
		return
	}
	affected, _ := result.RowsAffected()
	cursorResult, err := tx.ExecContext(
		r.Context(),
		`UPDATE conversations
		 SET customer_last_seen_seq = GREATEST(customer_last_seen_seq, ?),
		     customer_last_seen_at = NOW(3),
		     updated_at = NOW(3)
		 WHERE id = ? AND customer_last_seen_seq < ?`,
		req.UpToSeq,
		customer.ConversationID,
		req.UpToSeq,
	)
	if err != nil {
		s.logger.Error("update customer read cursor", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read_cursor_update_failed"})
		return
	}
	cursorAffected, _ := cursorResult.RowsAffected()
	if affected > 0 || cursorAffected > 0 {
		state, err := s.conversationReadState(r.Context(), tx, customer.ConversationID)
		if err != nil {
			s.logger.Error("load customer read state", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read_state_failed"})
			return
		}
		if _, _, err := createEvent(r.Context(), tx, customer.SiteID, "CUSTOMER_READ_UPDATED", map[string]any{
			"conversation_id": customer.ConversationID,
			"customer_id":     customer.CustomerID,
			"up_to_seq":       req.UpToSeq,
			"read_state":      state,
		}); err != nil {
			s.logger.Error("customer read event", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read_event_failed"})
			return
		}
	}
	if err := tx.Commit(); err != nil {
		s.logger.Error("commit customer read", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read_commit_failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"updated": affected})
}
