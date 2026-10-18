package http

import (
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

type initAgentRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type agentLoginRequest struct {
	Login       string `json:"login"`
	Password    string `json:"password"`
	DeviceID    string `json:"device_id"`
	PublicKey   string `json:"device_public_key"`
	PairingCode string `json:"pairing_code"`
}

type agentDTO struct {
	ID        uint64    `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Server) initAgent(w http.ResponseWriter, r *http.Request) {
	var req initAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	username := strings.TrimSpace(req.Username)
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if username == "" || email == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username_email_password_required"})
		return
	}

	var count int
	if err := s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM agents`).Scan(&count); err != nil {
		s.logger.Error("count agents", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "agent_count_failed"})
		return
	}
	if count > 0 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "agent_already_initialized"})
		return
	}

	passwordHash, err := security.HashPassword(req.Password)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "weak_password"})
		return
	}
	result, err := s.db.ExecContext(r.Context(), `INSERT INTO agents (username, email, password_hash) VALUES (?, ?, ?)`, username, email, passwordHash)
	if err != nil {
		s.logger.Error("insert agent", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "agent_create_failed"})
		return
	}
	id, _ := result.LastInsertId()
	if _, err := s.db.ExecContext(r.Context(), `INSERT IGNORE INTO agent_site_access (agent_id, site_id) SELECT ?, id FROM sites`, id); err != nil {
		s.logger.Error("grant agent site access", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "agent_access_failed"})
		return
	}
	writeJSON(w, http.StatusCreated, agentDTO{ID: uint64(id), Username: username, Email: email, CreatedAt: time.Now()})
}

func (s *Server) agentLogin(w http.ResponseWriter, r *http.Request) {
	var req agentLoginRequest
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	body, err := readProofBody(r)
	if err != nil || json.Unmarshal(body, &req) != nil {
		s.rejectSecurity(w, r, "invalid_device_proof")
		return
	}
	login := strings.ToLower(strings.TrimSpace(req.Login))
	deviceID := strings.TrimSpace(req.DeviceID)
	if strings.TrimSpace(req.PairingCode) == "" {
		s.rejectSecurity(w, r, "pairing_code_required")
		return
	}
	if len(deviceID) > 128 || len(req.PublicKey) > 512 || len(req.PairingCode) > 64 {
		s.rejectSecurity(w, r, "invalid_device_proof")
		return
	}
	if login == "" || req.Password == "" || deviceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "login_password_device_required"})
		return
	}
	if !s.preflightDeviceLogin(w, r, req, body) {
		return
	}

	var agent struct {
		ID           uint64
		Username     string
		Email        string
		PasswordHash string
	}
	err = s.db.QueryRowContext(
		r.Context(),
		`SELECT id, username, email, password_hash FROM agents WHERE LOWER(username) = ? OR LOWER(email) = ? LIMIT 1`,
		login,
		login,
	).Scan(&agent.ID, &agent.Username, &agent.Email, &agent.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !security.CheckPassword(agent.PasswordHash, req.Password)) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_credentials"})
		return
	}
	if err != nil {
		s.logger.Error("agent login lookup", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "login_failed"})
		return
	}
	attempts, err := s.redis.Eval(r.Context(), `local n = redis.call('INCR', KEYS[1]); if n == 1 then redis.call('EXPIRE', KEYS[1], 60) end; return n`, []string{fmt.Sprintf("chat-v0:pairing-login:%d", agent.ID)}).Int64()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pairing_lookup_unavailable"})
		return
	}
	if attempts > 20 {
		w.Header().Set("Retry-After", "60")
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "pairing_login_rate_limited"})
		return
	}
	policy, err := s.loadSecurityPolicy(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "security_policy_unavailable"})
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "device_login_failed"})
		return
	}
	defer tx.Rollback()
	var pairingHash sql.NullString
	if err := tx.QueryRowContext(r.Context(), `SELECT pairing_code_hash FROM security_policy WHERE id = 1 FOR SHARE`).Scan(&pairingHash); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pairing_lookup_unavailable"})
		return
	}
	if !pairingHash.Valid || pairingHash.String == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pairing_code_not_configured"})
		return
	}
	if !security.CheckPassword(pairingHash.String, strings.TrimSpace(req.PairingCode)) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "pairing_code_invalid"})
		return
	}
	var storedKey sql.NullString
	err = tx.QueryRowContext(r.Context(), `SELECT public_key FROM agent_devices WHERE agent_id = ? AND device_id = ? FOR UPDATE`, agent.ID, deviceID).Scan(&storedKey)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "device_lookup_failed"})
		return
	}
	publicKey := strings.TrimSpace(req.PublicKey)
	if publicKey == "" {
		publicKey = storedKey.String
	}
	if publicKey == "" && policy.RequireDeviceProof {
		s.rejectSecurity(w, r, "device_pairing_required")
		return
	}
	if publicKey != "" {
		if err := s.verifyDeviceProof(r, body, publicKey, deviceID); err != nil {
			s.writeAgentAuthError(w, r, err)
			return
		}
	}

	token, err := security.NewToken(32)
	if err != nil {
		s.logger.Error("agent token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "token_failed"})
		return
	}
	tokenHash := security.TokenHash(token)

	_, err = tx.ExecContext(
		r.Context(),
		`INSERT INTO agent_devices (agent_id, device_id, token_hash, public_key, last_sync_at)
		 VALUES (?, ?, ?, ?, NOW(3))
		 ON DUPLICATE KEY UPDATE token_hash = VALUES(token_hash), public_key = VALUES(public_key), last_sync_at = NOW(3), updated_at = NOW(3)`,
		agent.ID,
		deviceID,
		tokenHash,
		nullableString(publicKey),
	)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) {
			s.logger.Error("agent device upsert mysql", "code", mysqlErr.Number, "error", mysqlErr.Message)
		} else {
			s.logger.Error("agent device upsert", "error", err)
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "device_login_failed"})
		return
	}
	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "device_login_commit_failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"agent":         agentDTO{ID: agent.ID, Username: agent.Username, Email: agent.Email},
		"token":         token,
		"device_paired": publicKey != "",
	})
}
