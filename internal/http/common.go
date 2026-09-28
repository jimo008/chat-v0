package http

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

type siteRecord struct {
	ID      uint64
	SiteKey string
	Name    string
}

func (s *Server) findSiteByKey(ctx context.Context, siteKey string) (siteRecord, error) {
	var site siteRecord
	err := s.db.QueryRowContext(ctx, `SELECT id, site_key, name FROM sites WHERE site_key = ? LIMIT 1`, siteKey).Scan(&site.ID, &site.SiteKey, &site.Name)
	return site, err
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func nullableString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func nullableTime(value *time.Time) sql.NullTime {
	if value == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *value, Valid: true}
}

func nullableUint64(value *uint64) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*value), Valid: true}
}

func nullableStringValue(value sql.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}

func nullableIntValue(value sql.NullInt64) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
}

func nullableTimeValue(value sql.NullTime) any {
	if !value.Valid {
		return nil
	}
	return value.Time
}

func reverseMessages(messages []map[string]any) {
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
}
