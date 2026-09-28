package http

import (
	"encoding/json"
	"net/http"
)

type customerReadRequest struct {
	UpToSeq uint64 `json:"up_to_seq"`
	Visible bool   `json:"visible"`
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
	if req.UpToSeq == 0 || !req.Visible {
		s.logger.Info("customer read ignored", "conversation_id", customer.ConversationID, "customer_id", customer.CustomerID, "up_to_seq", req.UpToSeq, "visible", req.Visible)
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
	if affected > 0 {
		if _, _, err := createEvent(r.Context(), tx, customer.SiteID, "MESSAGE_READ", map[string]any{
			"conversation_id": customer.ConversationID,
			"customer_id":     customer.CustomerID,
			"up_to_seq":       req.UpToSeq,
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
