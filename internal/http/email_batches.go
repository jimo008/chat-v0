package http

import (
	"context"
	"database/sql"
	"errors"
)

func (s *Server) ensureEmailBatch(ctx context.Context, tx *sql.Tx, siteID, conversationID, messageID uint64) error {
	var customerID uint64
	var customerEmail string
	var entryType sql.NullString
	var entryURL sql.NullString
	err := tx.QueryRowContext(
		ctx,
		`SELECT c.id, c.email_original, c.last_support_entry_type, c.last_support_entry_url
		 FROM conversations conv
		 JOIN customers c ON c.id = conv.customer_id
		 WHERE conv.id = ? AND conv.site_id = ?
		 LIMIT 1`,
		conversationID,
		siteID,
	).Scan(&customerID, &customerEmail, &entryType, &entryURL)
	if err != nil {
		return err
	}
	if !looksLikeEmail(customerEmail) {
		return nil
	}

	var batchID uint64
	err = tx.QueryRowContext(
		ctx,
		`SELECT id
		 FROM email_batches
		 WHERE site_id = ? AND conversation_id = ? AND status = 'pending'
		 ORDER BY id DESC
		 LIMIT 1`,
		siteID,
		conversationID,
	).Scan(&batchID)
	if errors.Is(err, sql.ErrNoRows) {
		result, err := tx.ExecContext(
			ctx,
			`INSERT INTO email_batches
			   (site_id, customer_id, conversation_id, send_after, return_entry_type, return_url)
			 VALUES (?, ?, ?, DATE_ADD(NOW(3), INTERVAL 300 SECOND), ?, ?)`,
			siteID,
			customerID,
			conversationID,
			entryType,
			entryURL,
		)
		if err != nil {
			return err
		}
		id, err := result.LastInsertId()
		if err != nil {
			return err
		}
		batchID = uint64(id)
	} else if err != nil {
		return err
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT IGNORE INTO email_batch_messages (batch_id, message_id) VALUES (?, ?)`,
		batchID,
		messageID,
	)
	return err
}
