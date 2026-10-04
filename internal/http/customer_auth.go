package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/jimo008/chat-v0/internal/security"
)

type xboardLoginRequest struct {
	SiteKey              string          `json:"site_key"`
	XBoardUserID         string          `json:"xboard_user_id"`
	Email                string          `json:"email"`
	Plan                 string          `json:"plan"`
	ExpireTime           *time.Time      `json:"expire_time"`
	UsedTraffic          *uint64         `json:"used_traffic"`
	AllTraffic           *uint64         `json:"all_traffic"`
	RawProfile           json.RawMessage `json:"raw_profile"`
	LastSupportEntryType string          `json:"last_support_entry_type"`
	LastSupportEntryURL  string          `json:"last_support_entry_url"`
	DeviceLabel          string          `json:"device_label"`
}

type guestLoginRequest struct {
	SiteKey              string `json:"site_key"`
	LastSupportEntryType string `json:"last_support_entry_type"`
	LastSupportEntryURL  string `json:"last_support_entry_url"`
	DeviceLabel          string `json:"device_label"`
}

func (s *Server) customerGuestLogin(w http.ResponseWriter, r *http.Request) {
	var req guestLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	siteKey := strings.TrimSpace(req.SiteKey)
	if siteKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "site_required"})
		return
	}
	site, err := s.findSiteByKey(r.Context(), siteKey)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "site_not_found"})
		return
	}
	if err != nil {
		s.logger.Error("find guest site", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "site_lookup_failed"})
		return
	}

	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "guest_login_failed"})
		return
	}
	defer tx.Rollback()

	customerID, displayName, err := s.createGuestCustomer(r.Context(), tx, site.ID)
	if err != nil {
		s.logger.Error("create guest customer", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "guest_customer_failed"})
		return
	}
	if err := s.updateLastSupportEntry(r.Context(), tx, customerID, req.LastSupportEntryType, req.LastSupportEntryURL); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "support_entry_failed"})
		return
	}
	conversationID, err := s.ensureConversation(r.Context(), tx, site.ID, customerID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "conversation_failed"})
		return
	}
	token, err := security.NewToken(32)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "token_failed"})
		return
	}
	if _, err := tx.ExecContext(
		r.Context(),
		`INSERT INTO customer_tokens (site_id, customer_id, token_hash, device_label, last_used_at)
		 VALUES (?, ?, ?, ?, NOW(3))`,
		site.ID,
		customerID,
		security.TokenHash(token),
		nullableString(strings.TrimSpace(req.DeviceLabel)),
	); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "token_store_failed"})
		return
	}
	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "guest_login_commit_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"customer": map[string]any{
			"id":              customerID,
			"site_id":         site.ID,
			"site_key":        site.SiteKey,
			"email":           displayName,
			"guest":           true,
			"conversation_id": conversationID,
		},
		"token": token,
	})
}

func (s *Server) createGuestCustomer(ctx context.Context, tx *sql.Tx, siteID uint64) (uint64, string, error) {
	for i := 0; i < 8; i++ {
		digits, err := randomDigits(8)
		if err != nil {
			return 0, "", err
		}
		displayName := "游客" + digits
		normalized := "guest:" + digits
		result, err := tx.ExecContext(
			ctx,
			`INSERT INTO customers (site_id, normalized_email, email_original, last_active_at)
			 VALUES (?, ?, ?, NOW(3))`,
			siteID,
			normalized,
			displayName,
		)
		if err == nil {
			id, err := result.LastInsertId()
			return uint64(id), displayName, err
		}
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			continue
		}
		return 0, "", err
	}
	return 0, "", fmt.Errorf("guest_id_exhausted")
}

func (s *Server) customerXBoardLogin(w http.ResponseWriter, r *http.Request) {
	var req xboardLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}

	siteKey := strings.TrimSpace(req.SiteKey)
	xboardUserID := strings.TrimSpace(req.XBoardUserID)
	emailOriginal := strings.TrimSpace(req.Email)
	normalizedEmail := normalizeEmail(emailOriginal)
	if siteKey == "" || xboardUserID == "" || normalizedEmail == "" {
		s.logger.Info("xboard login missing identity fields", "site_key", siteKey, "xboard_user_id", xboardUserID, "email", emailOriginal)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "site_xboard_user_email_required"})
		return
	}

	site, err := s.findSiteByKey(r.Context(), siteKey)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "site_not_found"})
		return
	}
	if err != nil {
		s.logger.Error("find site", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "site_lookup_failed"})
		return
	}

	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.logger.Error("begin xboard login", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "login_failed"})
		return
	}
	defer tx.Rollback()

	customerID, err := s.findOrCreateXBoardCustomer(r.Context(), tx, site.ID, xboardUserID, normalizedEmail, emailOriginal)
	if err != nil {
		s.logger.Error("xboard customer", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "customer_failed"})
		return
	}

	if err := s.updateLastSupportEntry(r.Context(), tx, customerID, req.LastSupportEntryType, req.LastSupportEntryURL); err != nil {
		s.logger.Error("update support entry", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "support_entry_failed"})
		return
	}

	rawProfile := string(req.RawProfile)
	if rawProfile == "" {
		rawProfile = "{}"
	}
	if err := s.upsertXBoardProfile(r.Context(), tx, site.ID, customerID, req, rawProfile); err != nil {
		s.logger.Error("upsert xboard profile", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "xboard_profile_failed"})
		return
	}

	conversationID, err := s.ensureConversation(r.Context(), tx, site.ID, customerID)
	if err != nil {
		s.logger.Error("ensure conversation", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "conversation_failed"})
		return
	}

	token, err := security.NewToken(32)
	if err != nil {
		s.logger.Error("customer token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "token_failed"})
		return
	}
	_, err = tx.ExecContext(
		r.Context(),
		`INSERT INTO customer_tokens (site_id, customer_id, token_hash, device_label, last_used_at)
		 VALUES (?, ?, ?, ?, NOW(3))`,
		site.ID,
		customerID,
		security.TokenHash(token),
		nullableString(strings.TrimSpace(req.DeviceLabel)),
	)
	if err != nil {
		s.logger.Error("insert customer token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "token_store_failed"})
		return
	}

	if err := tx.Commit(); err != nil {
		s.logger.Error("commit xboard login", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "login_commit_failed"})
		return
	}

	s.logger.Info("xboard login success", "site_key", site.SiteKey, "customer_id", customerID, "conversation_id", conversationID, "xboard_user_id", xboardUserID, "email", emailOriginal)
	writeJSON(w, http.StatusOK, map[string]any{
		"customer": map[string]any{
			"id":              customerID,
			"site_id":         site.ID,
			"site_key":        site.SiteKey,
			"email":           emailOriginal,
			"xboard_user_id":  xboardUserID,
			"conversation_id": conversationID,
		},
		"token": token,
	})
}

func (s *Server) findOrCreateXBoardCustomer(ctx context.Context, tx *sql.Tx, siteID uint64, xboardUserID, normalizedEmail, emailOriginal string) (uint64, error) {
	var customerID uint64
	err := tx.QueryRowContext(ctx, `SELECT id FROM customers WHERE site_id = ? AND xboard_user_id = ? LIMIT 1`, siteID, xboardUserID).Scan(&customerID)
	if err == nil {
		_, updateErr := tx.ExecContext(
			ctx,
			`UPDATE customers
			 SET normalized_email = ?, email_original = ?, last_active_at = NOW(3), updated_at = NOW(3)
			 WHERE id = ?`,
			normalizedEmail,
			emailOriginal,
			customerID,
		)
		return customerID, updateErr
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	err = tx.QueryRowContext(ctx, `SELECT id FROM customers WHERE site_id = ? AND normalized_email = ? LIMIT 1`, siteID, normalizedEmail).Scan(&customerID)
	if err == nil {
		_, updateErr := tx.ExecContext(
			ctx,
			`UPDATE customers
			 SET xboard_user_id = ?, email_original = ?, last_active_at = NOW(3), updated_at = NOW(3)
			 WHERE id = ?`,
			xboardUserID,
			emailOriginal,
			customerID,
		)
		return customerID, updateErr
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	result, err := tx.ExecContext(
		ctx,
		`INSERT INTO customers (site_id, normalized_email, email_original, xboard_user_id, last_active_at)
		 VALUES (?, ?, ?, ?, NOW(3))`,
		siteID,
		normalizedEmail,
		emailOriginal,
		xboardUserID,
	)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return uint64(id), err
}

func (s *Server) updateLastSupportEntry(ctx context.Context, tx *sql.Tx, customerID uint64, entryType, entryURL string) error {
	entryType = strings.TrimSpace(entryType)
	entryURL = strings.TrimSpace(entryURL)
	if entryType == "" {
		return nil
	}
	if entryType != "web" && entryType != "app" {
		return nil
	}
	if entryType == "web" && entryURL == "" {
		return nil
	}
	_, err := tx.ExecContext(
		ctx,
		`UPDATE customers
		 SET last_support_entry_type = ?, last_support_entry_url = ?, updated_at = NOW(3)
		 WHERE id = ?`,
		entryType,
		nullableString(entryURL),
		customerID,
	)
	return err
}

func (s *Server) upsertXBoardProfile(ctx context.Context, tx *sql.Tx, siteID, customerID uint64, req xboardLoginRequest, rawProfile string) error {
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO xboard_profiles
		   (site_id, customer_id, xboard_user_id, email, plan, expire_time, used_traffic, all_traffic, raw_json, profile_updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, CAST(? AS JSON), NOW(3))
		 ON DUPLICATE KEY UPDATE
		   customer_id = VALUES(customer_id),
		   email = VALUES(email),
		   plan = VALUES(plan),
		   expire_time = VALUES(expire_time),
		   used_traffic = VALUES(used_traffic),
		   all_traffic = VALUES(all_traffic),
		   raw_json = VALUES(raw_json),
		   profile_updated_at = NOW(3),
		   updated_at = NOW(3)`,
		siteID,
		customerID,
		strings.TrimSpace(req.XBoardUserID),
		strings.TrimSpace(req.Email),
		nullableString(strings.TrimSpace(req.Plan)),
		nullableTime(req.ExpireTime),
		nullableUint64(req.UsedTraffic),
		nullableUint64(req.AllTraffic),
		rawProfile,
	)
	return err
}

func (s *Server) ensureConversation(ctx context.Context, tx *sql.Tx, siteID, customerID uint64) (uint64, error) {
	var conversationID uint64
	err := tx.QueryRowContext(ctx, `SELECT id FROM conversations WHERE site_id = ? AND customer_id = ? LIMIT 1`, siteID, customerID).Scan(&conversationID)
	if err == nil {
		return conversationID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO conversations (site_id, customer_id) VALUES (?, ?)`, siteID, customerID)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			err = tx.QueryRowContext(ctx, `SELECT id FROM conversations WHERE site_id = ? AND customer_id = ? LIMIT 1`, siteID, customerID).Scan(&conversationID)
			return conversationID, err
		}
		return 0, err
	}
	id, err := result.LastInsertId()
	return uint64(id), err
}
