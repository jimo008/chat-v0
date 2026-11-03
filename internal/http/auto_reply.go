package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"
)

const autoReplyFooter = "此条消息由系统自动回复，有问题请留言，客服上线为您解决！"

func (s *Server) autoReplyStatus(w http.ResponseWriter, r *http.Request) {
	var enabled bool
	var content string
	if err := s.db.QueryRowContext(r.Context(), `SELECT enabled, content FROM auto_reply_settings WHERE id = 1`).Scan(&enabled, &content); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auto_reply_settings_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": enabled, "content": content, "delay_seconds": 3, "cooldown_seconds": 60, "footer": autoReplyFooter})
}

func (s *Server) setAutoReply(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled *bool   `json:"enabled"`
		Content *string `json:"content"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if json.NewDecoder(r.Body).Decode(&req) != nil || (req.Enabled == nil && req.Content == nil) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_auto_reply_settings"})
		return
	}
	if req.Content != nil {
		trimmed := strings.TrimSpace(*req.Content)
		if !utf8.ValidString(trimmed) || utf8.RuneCountInString(trimmed) > 2000 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "auto_reply_content_max_2000_characters"})
			return
		}
		req.Content = &trimmed
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auto_reply_settings_unavailable"})
		return
	}
	defer tx.Rollback()
	var enabled bool
	var content string
	var generation uint64
	if err := tx.QueryRowContext(r.Context(), `SELECT enabled, content, generation FROM auto_reply_settings WHERE id = 1 FOR UPDATE`).Scan(&enabled, &content, &generation); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auto_reply_settings_unavailable"})
		return
	}
	if req.Content != nil {
		content = *req.Content
	}
	if req.Enabled != nil && *req.Enabled != enabled {
		generation++
		enabled = *req.Enabled
	}
	if enabled && content == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "set_auto_reply_content_before_enabling"})
		return
	}
	if _, err := tx.ExecContext(r.Context(), `UPDATE auto_reply_settings SET enabled = ?, content = ?, generation = ? WHERE id = 1`, enabled, content, generation); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auto_reply_update_failed"})
		return
	}
	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auto_reply_update_failed"})
		return
	}
	s.autoReplyStatus(w, r)
}

func queueAutoReply(ctx context.Context, tx *sql.Tx, message messageDTO) error {
	var enabled bool
	var content string
	var generation uint64
	if err := tx.QueryRowContext(ctx, `SELECT enabled, content, generation FROM auto_reply_settings WHERE id = 1`).Scan(&enabled, &content, &generation); err != nil {
		return err
	}
	if !enabled || content == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx, `INSERT IGNORE INTO auto_reply_jobs (site_id, conversation_id, trigger_message_seq, generation, triggered_at, due_at)
      VALUES (?, ?, ?, ?, NOW(3), NOW(3) + INTERVAL 3 SECOND)`, message.SiteID, message.ConversationID, message.Seq, generation)
	return err
}
