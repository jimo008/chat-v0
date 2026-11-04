package jobs

import (
	"context"
	"database/sql"
	"time"
)

const AutoReplyFooter = "此条消息由系统自动回复，有问题请留言，客服上线为您解决！"

// Runs independently of SMTP, so slow email delivery cannot delay chat replies.
func (r *Runner) RunAutoReplies(ctx context.Context) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n, err := r.ProcessAutoReplies(ctx, 100); err != nil {
				if ctx.Err() == nil {
					r.logger.Error("process auto replies", "error", err)
				}
			} else if n > 0 {
				r.logger.Info("processed auto reply jobs", "count", n)
			}
		}
	}
}

func (r *Runner) ProcessAutoReplies(ctx context.Context, limit int) (int, error) {
	for i := 0; i < limit; i++ {
		processed, err := r.processOneAutoReply(ctx)
		if err != nil {
			return i, err
		}
		if !processed {
			return i, nil
		}
	}
	return limit, nil
}

func (r *Runner) processOneAutoReply(ctx context.Context) (bool, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var enabled bool
	var content string
	var generation uint64
	var cooldownSeconds int
	// Serialize shutdown/content changes with dispatch, without blocking customer writes.
	if err := tx.QueryRowContext(ctx, `SELECT enabled, content, generation, cooldown_seconds FROM auto_reply_settings WHERE id = 1 FOR SHARE`).Scan(&enabled, &content, &generation, &cooldownSeconds); err != nil {
		return false, err
	}
	var job struct {
		id, siteID, conversationID, messageSeq, generation uint64
		triggeredAt                                        time.Time
	}
	err = tx.QueryRowContext(ctx, `SELECT id, site_id, conversation_id, trigger_message_seq, generation, triggered_at
        FROM auto_reply_jobs WHERE due_at <= NOW(3) OR generation <> ? OR ? = FALSE
        ORDER BY due_at, id LIMIT 1 FOR UPDATE SKIP LOCKED`, generation, enabled).Scan(&job.id, &job.siteID, &job.conversationID, &job.messageSeq, &job.generation, &job.triggeredAt)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	finish := func() (bool, error) {
		if _, err := tx.ExecContext(ctx, `DELETE FROM auto_reply_jobs WHERE id = ?`, job.id); err != nil {
			return false, err
		}
		return true, tx.Commit()
	}
	if !enabled || generation != job.generation || content == "" {
		return finish()
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO auto_reply_state (conversation_id) VALUES (?) ON DUPLICATE KEY UPDATE conversation_id = VALUES(conversation_id)`, job.conversationID); err != nil {
		return false, err
	}
	var lastSent sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT last_sent_at FROM auto_reply_state WHERE conversation_id = ? FOR UPDATE`, job.conversationID).Scan(&lastSent); err != nil {
		return false, err
	}
	if lastSent.Valid && job.triggeredAt.Before(lastSent.Time.Add(time.Duration(cooldownSeconds)*time.Second)) {
		return finish()
	}

	// Keep the same sequence -> message -> conversation lock order as human messages.
	messageSeq, err := workerNextSequence(ctx, tx, "messages")
	if err != nil {
		return false, err
	}
	var blocked bool
	var humanReplied bool
	err = tx.QueryRowContext(ctx, `SELECT c.blocked,
       EXISTS(SELECT 1 FROM messages m WHERE m.conversation_id = conv.id AND m.sender_type = 'agent'
         AND m.is_auto_reply = FALSE AND m.sender_agent_id IS NOT NULL AND m.seq > ?)
       FROM conversations conv JOIN customers c ON c.id = conv.customer_id WHERE conv.id = ? AND conv.site_id = ?`, job.messageSeq, job.conversationID, job.siteID).Scan(&blocked, &humanReplied)
	if err == sql.ErrNoRows {
		return finish()
	}
	if err != nil {
		return false, err
	}
	if blocked || humanReplied {
		return finish()
	}
	body := content + "\n\n" + AutoReplyFooter
	result, err := tx.ExecContext(ctx, `INSERT INTO messages (site_id, conversation_id, sender_type, seq, type, content, is_auto_reply)
       VALUES (?, ?, 'agent', ?, 'text', ?, TRUE)`, job.siteID, job.conversationID, messageSeq, body)
	if err != nil {
		return false, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE conversations SET last_message_id = ?, last_message_at = NOW(3), updated_at = NOW(3) WHERE id = ?`, id, job.conversationID); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE auto_reply_state SET last_sent_at = NOW(3) WHERE conversation_id = ?`, job.conversationID); err != nil {
		return false, err
	}
	var createdAt time.Time
	if err := tx.QueryRowContext(ctx, `SELECT created_at FROM messages WHERE id = ?`, id).Scan(&createdAt); err != nil {
		return false, err
	}
	createdAt = time.Date(createdAt.Year(), createdAt.Month(), createdAt.Day(), createdAt.Hour(), createdAt.Minute(), createdAt.Second(), createdAt.Nanosecond(), time.FixedZone("Asia/Shanghai", 8*60*60))
	if err := createWorkerEvent(ctx, tx, job.siteID, "MESSAGE_CREATED", map[string]any{
		"id": uint64(id), "site_id": job.siteID, "conversation_id": job.conversationID,
		"sender_type": "agent", "sender_customer_id": nil, "sender_agent_id": nil,
		"seq": messageSeq, "client_msg_id": nil, "type": "text", "content": body,
		"attachment_id": nil, "created_at": createdAt, "is_auto_reply": true,
	}); err != nil {
		return false, err
	}
	return finish()
}
