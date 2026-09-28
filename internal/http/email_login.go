package http

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/smtp"
	"strings"

	"github.com/jimo008/chat-v0/internal/security"
)

type sendCodeRequest struct {
	SiteKey string `json:"site_key"`
	Email   string `json:"email"`
}

type verifyCodeRequest struct {
	SiteKey              string `json:"site_key"`
	Email                string `json:"email"`
	Code                 string `json:"code"`
	LastSupportEntryType string `json:"last_support_entry_type"`
	LastSupportEntryURL  string `json:"last_support_entry_url"`
	DeviceLabel          string `json:"device_label"`
}

func (s *Server) customerSendEmailCode(w http.ResponseWriter, r *http.Request) {
	var req sendCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	siteKey := strings.TrimSpace(req.SiteKey)
	emailOriginal := strings.TrimSpace(req.Email)
	normalizedEmail := normalizeEmail(emailOriginal)
	if siteKey == "" || normalizedEmail == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "site_email_required"})
		return
	}
	site, err := s.findSiteByKey(r.Context(), siteKey)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "site_not_found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "site_lookup_failed"})
		return
	}

	var recentID uint64
	err = s.db.QueryRowContext(
		r.Context(),
		`SELECT id FROM verification_codes
		 WHERE site_id = ? AND normalized_email = ? AND created_at >= DATE_SUB(NOW(3), INTERVAL 60 SECOND)
		 ORDER BY id DESC LIMIT 1`,
		site.ID,
		normalizedEmail,
	).Scan(&recentID)
	if err == nil {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "send_too_frequent"})
		return
	}
	if err != sql.ErrNoRows {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "rate_check_failed"})
		return
	}

	code, err := randomCode()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "code_failed"})
		return
	}
	_, err = s.db.ExecContext(
		r.Context(),
		`INSERT INTO verification_codes (site_id, normalized_email, code_hash, expires_at, created_ip)
		 VALUES (?, ?, ?, DATE_ADD(NOW(3), INTERVAL 10 MINUTE), ?)`,
		site.ID,
		normalizedEmail,
		security.TokenHash(code),
		clientIP(r),
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "code_store_failed"})
		return
	}
	if err := s.sendVerificationEmail(site.Name, emailOriginal, code); err != nil {
		s.logger.Error("send verification email", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "email_send_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": true, "resend_after_seconds": 60})
}

func (s *Server) customerVerifyEmailCode(w http.ResponseWriter, r *http.Request) {
	var req verifyCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	siteKey := strings.TrimSpace(req.SiteKey)
	emailOriginal := strings.TrimSpace(req.Email)
	normalizedEmail := normalizeEmail(emailOriginal)
	code := strings.TrimSpace(req.Code)
	if siteKey == "" || normalizedEmail == "" || code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "site_email_code_required"})
		return
	}
	site, err := s.findSiteByKey(r.Context(), siteKey)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "site_not_found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "site_lookup_failed"})
		return
	}

	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "verify_failed"})
		return
	}
	defer tx.Rollback()

	var codeID uint64
	var codeHash string
	var attempts uint64
	err = tx.QueryRowContext(
		r.Context(),
		`SELECT id, code_hash, attempt_count
		 FROM verification_codes
		 WHERE site_id = ? AND normalized_email = ? AND used_at IS NULL AND invalidated_at IS NULL AND expires_at > NOW(3)
		 ORDER BY id DESC LIMIT 1
		 FOR UPDATE`,
		site.ID,
		normalizedEmail,
	).Scan(&codeID, &codeHash, &attempts)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code_not_found_or_expired"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "code_lookup_failed"})
		return
	}
	if security.TokenHash(code) != codeHash {
		attempts++
		if attempts >= 5 {
			_, _ = tx.ExecContext(r.Context(), `UPDATE verification_codes SET attempt_count = ?, invalidated_at = NOW(3) WHERE id = ?`, attempts, codeID)
		} else {
			_, _ = tx.ExecContext(r.Context(), `UPDATE verification_codes SET attempt_count = ? WHERE id = ?`, attempts, codeID)
		}
		_ = tx.Commit()
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_code"})
		return
	}

	customerID, err := s.findOrCreateEmailCustomer(r.Context(), tx, site.ID, normalizedEmail, emailOriginal)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "customer_failed"})
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
	if _, err := tx.ExecContext(r.Context(), `UPDATE verification_codes SET used_at = NOW(3) WHERE id = ?`, codeID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "code_update_failed"})
		return
	}
	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "verify_commit_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"customer": map[string]any{
			"id":              customerID,
			"site_id":         site.ID,
			"site_key":        site.SiteKey,
			"email":           emailOriginal,
			"conversation_id": conversationID,
		},
		"token": token,
	})
}

func (s *Server) findOrCreateEmailCustomer(ctx context.Context, tx *sql.Tx, siteID uint64, normalizedEmail, emailOriginal string) (uint64, error) {
	var customerID uint64
	err := tx.QueryRowContext(ctx, `SELECT id FROM customers WHERE site_id = ? AND normalized_email = ? LIMIT 1`, siteID, normalizedEmail).Scan(&customerID)
	if err == nil {
		_, updateErr := tx.ExecContext(ctx, `UPDATE customers SET email_original = ?, last_active_at = NOW(3), updated_at = NOW(3) WHERE id = ?`, emailOriginal, customerID)
		return customerID, updateErr
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO customers (site_id, normalized_email, email_original, last_active_at) VALUES (?, ?, ?, NOW(3))`, siteID, normalizedEmail, emailOriginal)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return uint64(id), err
}

func (s *Server) sendVerificationEmail(siteName, to, code string) error {
	if s.cfg.PostalSMTPHost == "" || s.cfg.PostalSMTPUser == "" || s.cfg.PostalSMTPPassword == "" || s.cfg.PostalSMTPFromDomain == "" {
		return fmt.Errorf("smtp_not_configured")
	}
	fromAddress := "no-reply@" + s.cfg.PostalSMTPFromDomain
	fromName := siteName + "客服"
	subject := siteName + "客服验证码"
	body := fmt.Sprintf("您的验证码是：%s\\n\\n10 分钟内有效，请勿转发给他人。", code)
	msg := strings.Builder{}
	msg.WriteString("From: " + fromName + " <" + fromAddress + ">\\r\\n")
	msg.WriteString("To: " + to + "\\r\\n")
	msg.WriteString("Subject: " + subject + "\\r\\n")
	msg.WriteString("MIME-Version: 1.0\\r\\n")
	msg.WriteString("Content-Type: text/plain; charset=UTF-8\\r\\n\\r\\n")
	msg.WriteString(body)
	addr := fmt.Sprintf("%s:%d", s.cfg.PostalSMTPHost, s.cfg.PostalSMTPPort)
	auth := smtp.PlainAuth("", s.cfg.PostalSMTPUser, s.cfg.PostalSMTPPassword, s.cfg.PostalSMTPHost)
	return smtp.SendMail(addr, auth, fromAddress, []string{to}, []byte(msg.String()))
}

func randomCode() (string, error) {
	max := big.NewInt(1000000)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func clientIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}
	return r.RemoteAddr
}
