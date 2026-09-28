package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jimo008/chat-v0/internal/config"
)

type Runner struct {
	cfg    config.Config
	db     *sql.DB
	logger *slog.Logger
}

func NewRunner(cfg config.Config, db *sql.DB, logger *slog.Logger) *Runner {
	return &Runner{cfg: cfg, db: db, logger: logger}
}

func (r *Runner) Tick(ctx context.Context) {
	if expired, err := r.ExpireEmergencyCalls(ctx); err != nil {
		r.logger.Error("expire emergency calls", "error", err)
	} else if expired > 0 {
		r.logger.Info("expired emergency calls", "count", expired)
	}

	if processed, err := r.ProcessEmailBatches(ctx, 10); err != nil {
		r.logger.Error("process email batches", "error", err)
	} else if processed > 0 {
		r.logger.Info("processed email batches", "count", processed)
	}

	if r.cfg.RetentionCleanupEnabled {
		if cleaned, err := r.CleanupExpiredMessages(ctx, 200); err != nil {
			r.logger.Error("cleanup expired messages", "error", err)
		} else if cleaned > 0 {
			r.logger.Info("cleaned expired messages", "count", cleaned)
		}
	}
}

func (r *Runner) ProcessEmailBatches(ctx context.Context, limit int) (int, error) {
	processed := 0
	for processed < limit {
		ok, err := r.processOneEmailBatch(ctx)
		if err != nil {
			return processed, err
		}
		if !ok {
			return processed, nil
		}
		processed++
	}
	return processed, nil
}

func (r *Runner) processOneEmailBatch(ctx context.Context) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	batch, err := r.lockNextEmailBatch(ctx, tx)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	messages, err := r.unreadBatchMessages(ctx, tx, batch.ID)
	if err != nil {
		return false, err
	}
	if len(messages) == 0 {
		_, err = tx.ExecContext(ctx, `UPDATE email_batches SET status = 'skipped', updated_at = NOW(3) WHERE id = ?`, batch.ID)
		if err != nil {
			return false, err
		}
		return true, tx.Commit()
	}

	if r.cfg.PostalSMTPHost == "" || r.cfg.PostalSMTPUser == "" || r.cfg.PostalSMTPPassword == "" || r.cfg.PostalSMTPFromDomain == "" {
		_, err = tx.ExecContext(ctx, `UPDATE email_batches SET status = 'failed', retry_count = retry_count + 1, last_error = 'smtp_not_configured', updated_at = NOW(3) WHERE id = ?`, batch.ID)
		if err != nil {
			return false, err
		}
		return true, tx.Commit()
	}

	if err := r.sendUnreadEmail(batch, messages); err != nil {
		_, updateErr := tx.ExecContext(ctx, `UPDATE email_batches SET status = 'failed', retry_count = retry_count + 1, last_error = ?, updated_at = NOW(3) WHERE id = ?`, err.Error(), batch.ID)
		if updateErr != nil {
			return false, updateErr
		}
		return true, tx.Commit()
	}

	for _, msg := range messages {
		if _, err := tx.ExecContext(ctx, `UPDATE messages SET email_notified_at = NOW(3) WHERE id = ?`, msg.ID); err != nil {
			return false, err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE email_batches SET status = 'sent', sent_at = NOW(3), updated_at = NOW(3) WHERE id = ?`, batch.ID)
	if err != nil {
		return false, err
	}
	return true, tx.Commit()
}

type emailBatch struct {
	ID              uint64
	SiteID          uint64
	SiteName        string
	CustomerEmail   string
	ReturnEntryType sql.NullString
	ReturnURL       sql.NullString
}

func (r *Runner) lockNextEmailBatch(ctx context.Context, tx *sql.Tx) (emailBatch, error) {
	var batch emailBatch
	err := tx.QueryRowContext(
		ctx,
		`SELECT b.id, b.site_id, s.name, c.email_original, b.return_entry_type, b.return_url
		 FROM email_batches b
		 JOIN sites s ON s.id = b.site_id
		 JOIN customers c ON c.id = b.customer_id
		 WHERE b.status IN ('pending','failed') AND b.send_after <= NOW(3) AND b.retry_count < 8
		 ORDER BY b.send_after ASC, b.id ASC
		 LIMIT 1
		 FOR UPDATE`,
	).Scan(&batch.ID, &batch.SiteID, &batch.SiteName, &batch.CustomerEmail, &batch.ReturnEntryType, &batch.ReturnURL)
	return batch, err
}

type emailMessage struct {
	ID      uint64
	Type    string
	Content sql.NullString
}

func (r *Runner) unreadBatchMessages(ctx context.Context, tx *sql.Tx, batchID uint64) ([]emailMessage, error) {
	rows, err := tx.QueryContext(
		ctx,
		`SELECT m.id, m.type, m.content
		 FROM email_batch_messages bm
		 JOIN messages m ON m.id = bm.message_id
		 WHERE bm.batch_id = ?
		   AND m.sender_type = 'agent'
		   AND m.customer_read_at IS NULL
		   AND m.email_notified_at IS NULL
		 ORDER BY m.created_at ASC, m.id ASC`,
		batchID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := make([]emailMessage, 0)
	for rows.Next() {
		var msg emailMessage
		if err := rows.Scan(&msg.ID, &msg.Type, &msg.Content); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

func (r *Runner) sendUnreadEmail(batch emailBatch, messages []emailMessage) error {
	fromAddress := "no-reply@" + r.cfg.PostalSMTPFromDomain
	fromName := batch.SiteName + "客服"
	subject := batch.SiteName + "客服回复了您的消息"

	var body strings.Builder
	body.WriteString("<p>您有新的客服消息：</p><blockquote>")
	for _, msg := range messages {
		if msg.Type == "image" {
			body.WriteString("<p>[图片消息]</p>")
			continue
		}
		body.WriteString("<p>")
		body.WriteString(html.EscapeString(msg.Content.String))
		body.WriteString("</p>")
	}
	body.WriteString("</blockquote>")
	if batch.ReturnEntryType.Valid && batch.ReturnEntryType.String == "web" && batch.ReturnURL.Valid && batch.ReturnURL.String != "" {
		body.WriteString(`<p><a href="`)
		body.WriteString(html.EscapeString(batch.ReturnURL.String))
		body.WriteString(`">查看客服消息</a></p>`)
	} else {
		body.WriteString("<p>请打开 App 查看客服消息。</p>")
	}
	body.WriteString("<p>请勿直接回复此邮件。</p>")

	msg := strings.Builder{}
	msg.WriteString("From: ")
	msg.WriteString(mimeHeader(fromName))
	msg.WriteString(" <")
	msg.WriteString(fromAddress)
	msg.WriteString(">\r\n")
	msg.WriteString("To: ")
	msg.WriteString(batch.CustomerEmail)
	msg.WriteString("\r\n")
	msg.WriteString("Subject: ")
	msg.WriteString(mimeHeader(subject))
	msg.WriteString("\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body.String())

	addr := fmt.Sprintf("%s:%d", r.cfg.PostalSMTPHost, r.cfg.PostalSMTPPort)
	auth := smtp.PlainAuth("", r.cfg.PostalSMTPUser, r.cfg.PostalSMTPPassword, r.cfg.PostalSMTPHost)
	return smtp.SendMail(addr, auth, fromAddress, []string{batch.CustomerEmail}, []byte(msg.String()))
}

func mimeHeader(value string) string {
	// Keep this small and dependency-free. UTF-8 subjects/display names are encoded by base64.
	return "=?UTF-8?B?" + base64Encode(value) + "?="
}

func base64Encode(value string) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	data := []byte(value)
	if len(data) == 0 {
		return ""
	}
	var out strings.Builder
	for i := 0; i < len(data); i += 3 {
		var b [3]byte
		n := copy(b[:], data[i:])
		triple := uint(b[0])<<16 | uint(b[1])<<8 | uint(b[2])
		out.WriteByte(alphabet[(triple>>18)&0x3f])
		out.WriteByte(alphabet[(triple>>12)&0x3f])
		if n > 1 {
			out.WriteByte(alphabet[(triple>>6)&0x3f])
		} else {
			out.WriteByte('=')
		}
		if n > 2 {
			out.WriteByte(alphabet[triple&0x3f])
		} else {
			out.WriteByte('=')
		}
	}
	return out.String()
}

func (r *Runner) ExpireEmergencyCalls(ctx context.Context) (uint64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
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
		r.cfg.EmergencyExpireSeconds,
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
		if err := createWorkerEvent(ctx, tx, c.SiteID, "EMERGENCY_EXPIRED", map[string]any{"call_id": c.ID}); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return uint64(len(calls)), nil
}

func createWorkerEvent(ctx context.Context, tx *sql.Tx, siteID uint64, eventType string, payload any) error {
	seq, err := workerNextSequence(ctx, tx, "events")
	if err != nil {
		return err
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	eventID := fmt.Sprintf("worker-%d-%d", siteID, time.Now().UnixNano())
	_, err = tx.ExecContext(ctx, `INSERT INTO events (event_id, seq, site_id, type, payload_json) VALUES (?, ?, ?, ?, ?)`, eventID, seq, siteID, eventType, string(payloadBytes))
	return err
}

func workerNextSequence(ctx context.Context, tx *sql.Tx, name string) (uint64, error) {
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO sequences (name, value)
		 VALUES (?, LAST_INSERT_ID(1))
		 ON DUPLICATE KEY UPDATE value = LAST_INSERT_ID(value + 1)`,
		name,
	)
	if err != nil {
		return 0, err
	}
	var seq uint64
	if err := tx.QueryRowContext(ctx, `SELECT LAST_INSERT_ID()`).Scan(&seq); err != nil {
		return 0, err
	}
	return seq, nil
}

func (r *Runner) CleanupExpiredMessages(ctx context.Context, limit int) (uint64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(
		ctx,
		`SELECT m.id, a.storage_key, a.thumbnail_key
		 FROM messages m
		 LEFT JOIN attachments a ON a.id = m.attachment_id
		 WHERE m.created_at < DATE_SUB(NOW(3), INTERVAL 3 MONTH)
		 ORDER BY m.created_at ASC
		 LIMIT ?
		 FOR UPDATE`,
		limit,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type expired struct {
		MessageID    uint64
		StorageKey   sql.NullString
		ThumbnailKey sql.NullString
	}
	expiredMessages := make([]expired, 0)
	for rows.Next() {
		var item expired
		if err := rows.Scan(&item.MessageID, &item.StorageKey, &item.ThumbnailKey); err != nil {
			return 0, err
		}
		expiredMessages = append(expiredMessages, item)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, item := range expiredMessages {
		if _, err := tx.ExecContext(ctx, `DELETE FROM email_batch_messages WHERE message_id = ?`, item.MessageID); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE conversations SET last_message_id = NULL WHERE last_message_id = ?`, item.MessageID); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM messages WHERE id = ?`, item.MessageID); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}

	// File deletion happens after DB commit. A later storage reconciliation job can retry leftovers.
	for _, item := range expiredMessages {
		if item.StorageKey.Valid {
			_ = os.Remove(filepath.Join(r.cfg.UploadStoragePath, filepath.FromSlash(item.StorageKey.String)))
		}
		if item.ThumbnailKey.Valid && item.ThumbnailKey.String != item.StorageKey.String {
			_ = os.Remove(filepath.Join(r.cfg.UploadStoragePath, filepath.FromSlash(item.ThumbnailKey.String)))
		}
	}
	return uint64(len(expiredMessages)), nil
}
