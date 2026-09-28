package http

import (
	"database/sql"
	"net/http"
)

func (s *Server) agentListCustomers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(
		r.Context(),
		`SELECT c.id, c.site_id, s.site_key, s.name, c.email_original, c.xboard_user_id, c.blocked,
		        conv.id, conv.last_message_at,
		        COALESCE(unread.unread_count, 0),
		        COALESCE(ringing.ringing_count, 0),
		        lm.type,
		        lm.content
		 FROM customers c
		 JOIN sites s ON s.id = c.site_id
		 JOIN conversations conv ON conv.customer_id = c.id AND conv.site_id = c.site_id
		 LEFT JOIN messages lm ON lm.id = conv.last_message_id
		 LEFT JOIN (
		   SELECT conversation_id, COUNT(*) AS unread_count
		   FROM messages
		   WHERE sender_type = 'customer' AND agent_read_at IS NULL
		   GROUP BY conversation_id
		 ) unread ON unread.conversation_id = conv.id
		 LEFT JOIN (
		   SELECT conversation_id, COUNT(*) AS ringing_count
		   FROM emergency_calls
		   WHERE status = 'RINGING'
		   GROUP BY conversation_id
		 ) ringing ON ringing.conversation_id = conv.id
		 ORDER BY conv.last_message_at DESC, c.last_active_at DESC, c.id DESC
		 LIMIT 200`,
	)
	if err != nil {
		s.logger.Error("agent list customers", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "customer_list_failed"})
		return
	}
	defer rows.Close()

	customers := make([]map[string]any, 0)
	for rows.Next() {
		var (
			customerID     uint64
			siteID         uint64
			siteKey        string
			siteName       string
			email          string
			xboardUserID   sql.NullString
			blocked        bool
			conversationID uint64
			lastMessageAt  sql.NullTime
			unreadCount    uint64
			ringingCount   uint64
			lastType       sql.NullString
			lastContent    sql.NullString
		)
		if err := rows.Scan(&customerID, &siteID, &siteKey, &siteName, &email, &xboardUserID, &blocked, &conversationID, &lastMessageAt, &unreadCount, &ringingCount, &lastType, &lastContent); err != nil {
			s.logger.Error("scan agent customer", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "customer_scan_failed"})
			return
		}
		lastMessage := nullableStringValue(lastContent)
		if lastType.Valid && lastType.String == "image" {
			lastMessage = "[图片消息]"
		}
		customers = append(customers, map[string]any{
			"id":              customerID,
			"site_id":         siteID,
			"site_key":        siteKey,
			"site_name":       siteName,
			"email":           email,
			"xboard_user_id":  nullableStringValue(xboardUserID),
			"blocked":         blocked,
			"conversation_id": conversationID,
			"last_message_at": nullableTimeValue(lastMessageAt),
			"last_message":    lastMessage,
			"unread_count":    unreadCount,
			"ringing_count":   ringingCount,
		})
	}
	if err := rows.Err(); err != nil {
		s.logger.Error("iterate agent customers", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "customer_iter_failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"customers": customers})
}

func (s *Server) agentGetConversation(w http.ResponseWriter, r *http.Request) {
	conversationID, err := parsePathUint(r, "id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_conversation_id"})
		return
	}

	var meta struct {
		ID           uint64
		SiteID       uint64
		SiteKey      string
		SiteName     string
		CustomerID   uint64
		Email        string
		XBoardUserID sql.NullString
		Blocked      bool
	}
	err = s.db.QueryRowContext(
		r.Context(),
		`SELECT conv.id, conv.site_id, s.site_key, s.name, c.id, c.email_original, c.xboard_user_id, c.blocked
		 FROM conversations conv
		 JOIN sites s ON s.id = conv.site_id
		 JOIN customers c ON c.id = conv.customer_id
		 WHERE conv.id = ?
		 LIMIT 1`,
		conversationID,
	).Scan(&meta.ID, &meta.SiteID, &meta.SiteKey, &meta.SiteName, &meta.CustomerID, &meta.Email, &meta.XBoardUserID, &meta.Blocked)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation_not_found"})
		return
	}
	if err != nil {
		s.logger.Error("agent conversation", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "conversation_failed"})
		return
	}

	rows, err := s.db.QueryContext(
		r.Context(),
		`SELECT id, sender_type, sender_customer_id, sender_agent_id, seq, type, content, attachment_id, customer_read_at, created_at
		 FROM messages
		 WHERE conversation_id = ?
		 ORDER BY created_at DESC, id DESC
		 LIMIT 100`,
		conversationID,
	)
	if err != nil {
		s.logger.Error("agent messages", "error", err)
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
			s.logger.Error("scan agent message", "error", err)
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
		s.logger.Error("iterate agent messages", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "message_iter_failed"})
		return
	}
	reverseMessages(messages)

	writeJSON(w, http.StatusOK, map[string]any{
		"conversation": map[string]any{
			"id": meta.ID,
			"site": map[string]any{
				"id":       meta.SiteID,
				"site_key": meta.SiteKey,
				"name":     meta.SiteName,
			},
			"customer": map[string]any{
				"id":             meta.CustomerID,
				"email":          meta.Email,
				"xboard_user_id": nullableStringValue(meta.XBoardUserID),
				"blocked":        meta.Blocked,
			},
		},
		"messages": messages,
	})
}
