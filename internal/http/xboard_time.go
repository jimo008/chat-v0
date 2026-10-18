package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// XBoard returns expiry as Unix seconds; integrations may send RFC3339 or milliseconds.
func (req *xboardLoginRequest) UnmarshalJSON(data []byte) error {
	type requestFields xboardLoginRequest
	wire := struct {
		*requestFields
		ExpireTime json.RawMessage `json:"expire_time"`
	}{requestFields: (*requestFields)(req)}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	value, err := parseXBoardExpiry(wire.ExpireTime)
	if err != nil {
		return fmt.Errorf("expire_time: %w", err)
	}
	req.ExpireTime = value
	return nil
}

func parseXBoardExpiry(raw json.RawMessage) (*time.Time, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	value := string(raw)
	if raw[0] == '"' {
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, nil
		}
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return &parsed, nil
		}
	}
	stamp, err := strconv.ParseInt(value, 10, 64)
	if err != nil || stamp < 0 {
		return nil, fmt.Errorf("expected Unix timestamp or RFC3339 date")
	}
	if stamp == 0 {
		return nil, nil
	}
	parsed := time.Unix(stamp, 0).UTC()
	if stamp >= 1_000_000_000_000 {
		parsed = time.UnixMilli(stamp).UTC()
	}
	if parsed.Year() < 1000 || parsed.Year() > 9999 {
		return nil, fmt.Errorf("date out of range")
	}
	return &parsed, nil
}
