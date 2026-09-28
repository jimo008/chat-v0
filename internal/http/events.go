package http

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
)

func nextSequence(ctx context.Context, tx *sql.Tx, name string) (uint64, error) {
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

func createEvent(ctx context.Context, tx *sql.Tx, siteID uint64, eventType string, payload any) (uint64, string, error) {
	seq, err := nextSequence(ctx, tx, "events")
	if err != nil {
		return 0, "", err
	}
	eventID, err := newEventID()
	if err != nil {
		return 0, "", err
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return 0, "", err
	}
	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO events (event_id, seq, site_id, type, payload_json)
		 VALUES (?, ?, ?, ?, ?)`,
		eventID,
		seq,
		siteID,
		eventType,
		string(payloadBytes),
	)
	return seq, eventID, err
}

func newEventID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
