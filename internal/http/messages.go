package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type sendMessageRequest struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

func (s *Server) customerSendMessage(w http.ResponseWriter, r *http.Request) {
	customer, ok := customerFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "customer_not_found"})
		return
	}
	if customer.Blocked {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "customer_blocked", "message": "当前无法使用在线客服，请通过其他联系方式联系我们。"})
		return
	}

	var req sendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	content := strings.TrimSpace(req.Content)
	if req.Type == "" {
		req.Type = "text"
	}
	if req.Type != "text" || content == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text_content_required"})
		return
	}

	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.logger.Error("begin customer message", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "message_failed"})
		return
	}
	defer tx.Rollback()

	message, err := s.insertTextMessage(r.Context(), tx, customer.SiteID, customer.ConversationID, "customer", customer.CustomerID, 0, content)
	if err != nil {
		s.logger.Error("insert customer message", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "message_insert_failed"})
		return
	}
	if _, _, err := createEvent(r.Context(), tx, customer.SiteID, "MESSAGE_CREATED", message); err != nil {
		s.logger.Error("customer message event", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "message_event_failed"})
		return
	}
	if err := tx.Commit(); err != nil {
		s.logger.Error("commit customer message", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "message_commit_failed"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"message": message})
}

func (s *Server) agentSendMessage(w http.ResponseWriter, r *http.Request) {
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
	var req sendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	content := strings.TrimSpace(req.Content)
	if req.Type == "" {
		req.Type = "text"
	}
	if req.Type != "text" || content == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text_content_required"})
		return
	}

	var siteID uint64
	if err := s.db.QueryRowContext(r.Context(), `SELECT site_id FROM conversations WHERE id = ? LIMIT 1`, conversationID).Scan(&siteID); err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation_not_found"})
		return
	} else if err != nil {
		s.logger.Error("agent conversation lookup", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "conversation_lookup_failed"})
		return
	}

	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.logger.Error("begin agent message", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "message_failed"})
		return
	}
	defer tx.Rollback()

	message, err := s.insertTextMessage(r.Context(), tx, siteID, conversationID, "agent", 0, agent.AgentID, content)
	if err != nil {
		s.logger.Error("insert agent message", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "message_insert_failed"})
		return
	}
	if _, _, err := createEvent(r.Context(), tx, siteID, "MESSAGE_CREATED", message); err != nil {
		s.logger.Error("agent message event", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "message_event_failed"})
		return
	}
	if err := s.ensureEmailBatch(r.Context(), tx, siteID, conversationID, message.ID); err != nil {
		s.logger.Error("ensure email batch", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "email_batch_failed"})
		return
	}
	if err := tx.Commit(); err != nil {
		s.logger.Error("commit agent message", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "message_commit_failed"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"message": message})
}

type messageDTO struct {
	ID               uint64    `json:"id"`
	SiteID           uint64    `json:"site_id"`
	ConversationID   uint64    `json:"conversation_id"`
	SenderType       string    `json:"sender_type"`
	SenderCustomerID any       `json:"sender_customer_id"`
	SenderAgentID    any       `json:"sender_agent_id"`
	Seq              uint64    `json:"seq"`
	Type             string    `json:"type"`
	Content          string    `json:"content"`
	CreatedAt        time.Time `json:"created_at"`
}

func (s *Server) insertTextMessage(ctx context.Context, tx *sql.Tx, siteID, conversationID uint64, senderType string, customerID, agentID uint64, content string) (messageDTO, error) {
	seq, err := nextSequence(ctx, tx, "messages")
	if err != nil {
		return messageDTO{}, err
	}
	var senderCustomerID sql.NullInt64
	var senderAgentID sql.NullInt64
	if customerID != 0 {
		senderCustomerID = sql.NullInt64{Int64: int64(customerID), Valid: true}
	}
	if agentID != 0 {
		senderAgentID = sql.NullInt64{Int64: int64(agentID), Valid: true}
	}
	result, err := tx.ExecContext(
		ctx,
		`INSERT INTO messages
		   (site_id, conversation_id, sender_type, sender_customer_id, sender_agent_id, seq, type, content)
		 VALUES (?, ?, ?, ?, ?, ?, 'text', ?)`,
		siteID,
		conversationID,
		senderType,
		senderCustomerID,
		senderAgentID,
		seq,
		content,
	)
	if err != nil {
		return messageDTO{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return messageDTO{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE conversations SET last_message_id = ?, last_message_at = NOW(3), updated_at = NOW(3) WHERE id = ?`, id, conversationID); err != nil {
		return messageDTO{}, err
	}
	return messageDTO{
		ID:               uint64(id),
		SiteID:           siteID,
		ConversationID:   conversationID,
		SenderType:       senderType,
		SenderCustomerID: nullableIntValue(senderCustomerID),
		SenderAgentID:    nullableIntValue(senderAgentID),
		Seq:              seq,
		Type:             "text",
		Content:          content,
		CreatedAt:        time.Now(),
	}, nil
}

func parsePathUint(r *http.Request, name string) (uint64, error) {
	return strconv.ParseUint(r.PathValue(name), 10, 64)
}
