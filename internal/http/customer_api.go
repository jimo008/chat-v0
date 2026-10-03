package http

import (
	"database/sql"
	"net/http"
	"strconv"
)

func (s *Server) customerMe(w http.ResponseWriter, r *http.Request) {
	customer, ok := customerFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "customer_not_found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"customer": map[string]any{
			"id":              customer.CustomerID,
			"site_id":         customer.SiteID,
			"site_key":        customer.SiteKey,
			"email":           customer.Email,
			"xboard_user_id":  customer.XBoardUserID,
			"blocked":         customer.Blocked,
			"conversation_id": customer.ConversationID,
		},
	})
}

func (s *Server) customerConversation(w http.ResponseWriter, r *http.Request) {
	customer, ok := customerFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "customer_not_found"})
		return
	}

	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 200 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_limit"})
			return
		}
		limit = parsed
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
	beforeSeq := uint64(0)
	if raw := r.URL.Query().Get("before_seq"); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_before_seq"})
			return
		}
		beforeSeq = parsed
	}

	query := `SELECT id, sender_type, sender_customer_id, sender_agent_id, seq, type, content, attachment_id, customer_read_at, created_at
		 FROM messages
		 WHERE conversation_id = ?
		 ORDER BY created_at DESC, id DESC
		 LIMIT ?`
	args := []any{customer.ConversationID, limit}
	if beforeSeq > 0 {
		query = `SELECT id, sender_type, sender_customer_id, sender_agent_id, seq, type, content, attachment_id, customer_read_at, created_at
		 FROM messages
		 WHERE conversation_id = ? AND seq < ?
		 ORDER BY seq DESC
		 LIMIT ?`
		args = []any{customer.ConversationID, beforeSeq, limit}
	} else if afterSeq > 0 {
		query = `SELECT id, sender_type, sender_customer_id, sender_agent_id, seq, type, content, attachment_id, customer_read_at, created_at
		 FROM messages
		 WHERE conversation_id = ? AND seq > ?
		 ORDER BY seq ASC
		 LIMIT ?`
		args = []any{customer.ConversationID, afterSeq, limit}
	}

	rows, err := s.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		s.logger.Error("customer messages", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "messages_failed"})
		return
	}
	defer rows.Close()

	messages := make([]map[string]any, 0)
	for rows.Next() {
		var (
			id               uint64
			senderType       string
			senderCustomerID sql.NullInt64
			senderAgentID    sql.NullInt64
			seq              uint64
			messageType      string
			content          sql.NullString
			attachmentID     sql.NullInt64
			customerReadAt   sql.NullTime
			createdAt        sql.NullTime
		)
		if err := rows.Scan(&id, &senderType, &senderCustomerID, &senderAgentID, &seq, &messageType, &content, &attachmentID, &customerReadAt, &createdAt); err != nil {
			s.logger.Error("scan customer message", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "message_scan_failed"})
			return
		}
		messages = append(messages, map[string]any{
			"id":                 id,
			"sender_type":        senderType,
			"sender_customer_id": nullableIntValue(senderCustomerID),
			"sender_agent_id":    nullableIntValue(senderAgentID),
			"seq":                seq,
			"type":               messageType,
			"content":            nullableStringValue(content),
			"attachment_id":      nullableIntValue(attachmentID),
			"customer_read_at":   nullableTimeValue(customerReadAt),
			"created_at":         nullableTimeValue(createdAt),
		})
	}
	if err := rows.Err(); err != nil {
		s.logger.Error("iterate customer messages", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "message_iter_failed"})
		return
	}

	if afterSeq == 0 {
		reverseMessages(messages)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"conversation": map[string]any{
			"id": customer.ConversationID,
		},
		"messages": messages,
	})
}
