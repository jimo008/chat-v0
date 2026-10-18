package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jimo008/chat-v0/internal/security"
)

type requestSecurityPolicy struct {
	RequireSiteKey     bool       `json:"require_site_key"`
	RequireDeviceProof bool       `json:"require_device_proof"`
	SiteKeyEnforcedAt  *time.Time `json:"site_key_enforced_at,omitempty"`
}

const securityPolicyCacheKey = "chat-v0:security-policy"

func (s *Server) loadSecurityPolicy(ctx context.Context) (requestSecurityPolicy, error) {
	var policy requestSecurityPolicy
	if raw, err := s.redis.Get(ctx, securityPolicyCacheKey).Bytes(); err == nil && json.Unmarshal(raw, &policy) == nil {
		return policy, nil
	}
	var enforcedAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT require_site_key, require_device_proof, site_key_enforced_at FROM security_policy WHERE id = 1`).Scan(&policy.RequireSiteKey, &policy.RequireDeviceProof, &enforcedAt)
	if enforcedAt.Valid {
		policy.SiteKeyEnforcedAt = &enforcedAt.Time
	}
	if err == nil {
		raw, _ := json.Marshal(policy)
		_ = s.redis.Set(ctx, securityPolicyCacheKey, raw, 3*time.Second).Err()
	}
	return policy, err
}

func (s *Server) rejectSecurity(w http.ResponseWriter, r *http.Request, reason string) {
	s.logger.Warn("request security rejected", "reason", reason, "path", r.URL.Path, "site_key", requestSiteKey(r))
	w.Header().Set("X-Chat-Security-Reject", reason)
	writeJSON(w, http.StatusForbidden, map[string]string{"error": reason})
}

func requestSiteKey(r *http.Request) string {
	if value := strings.TrimSpace(r.Header.Get("X-Chat-Site-Key")); value != "" {
		return value
	}
	if value := strings.TrimSpace(r.URL.Query().Get("site_key")); value != "" {
		return value
	}
	return strings.TrimSpace(r.URL.Query().Get("site"))
}

func (s *Server) adminPairingCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"pairing_code"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	code := strings.TrimSpace(req.Code)
	if len(code) < 8 || len(code) > 64 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "pairing_code_length_8_to_64"})
		return
	}
	hash, err := security.HashPassword(code)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "pairing_code_hash_failed"})
		return
	}
	if _, err := s.db.ExecContext(r.Context(), `UPDATE security_policy SET pairing_code_hash = ? WHERE id = 1`, hash); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "pairing_code_store_failed"})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"pairing_code_configured": true})
}

func (s *Server) adminSecurityStatus(w http.ResponseWriter, r *http.Request) {
	policy, err := s.loadSecurityPolicy(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "security_policy_failed"})
		return
	}
	var paired, unpaired int
	if err := s.db.QueryRowContext(r.Context(), `SELECT COALESCE(SUM(public_key IS NOT NULL AND public_key <> ''), 0), COALESCE(SUM((public_key IS NULL OR public_key = '') AND last_sync_at > NOW(3) - INTERVAL 24 HOUR), 0) FROM agent_devices`).Scan(&paired, &unpaired); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "security_devices_failed"})
		return
	}
	var configured bool
	if err := s.db.QueryRowContext(r.Context(), `SELECT pairing_code_hash IS NOT NULL AND pairing_code_hash <> '' FROM security_policy WHERE id = 1`).Scan(&configured); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "security_policy_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"policy": policy, "paired_devices": paired, "recent_unpaired_devices": unpaired, "pairing_code_configured": configured})
}

func (s *Server) adminSetSecurity(w http.ResponseWriter, r *http.Request) {
	var policy requestSecurityPolicy
	if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if policy.RequireDeviceProof {
		var configured bool
		if err := s.db.QueryRowContext(r.Context(), `SELECT pairing_code_hash IS NOT NULL AND pairing_code_hash <> '' FROM security_policy WHERE id = 1`).Scan(&configured); err != nil || !configured {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "set_fixed_pairing_code_before_enforcing"})
			return
		}
		var paired int
		if err := s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM agent_devices WHERE public_key IS NOT NULL AND public_key <> ''`).Scan(&paired); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "security_devices_failed"})
			return
		}
		if paired == 0 {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "pair_a_device_before_enforcing"})
			return
		}
		var recentUnpaired int
		if err := s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM agent_devices WHERE (public_key IS NULL OR public_key = '') AND last_sync_at > NOW(3) - INTERVAL 24 HOUR`).Scan(&recentUnpaired); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "security_devices_failed"})
			return
		}
		if recentUnpaired > 0 {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "upgrade_recent_devices_before_enforcing", "recent_unpaired_devices": recentUnpaired})
			return
		}
	}
	if _, err := s.db.ExecContext(r.Context(), `UPDATE security_policy SET site_key_enforced_at = CASE WHEN require_site_key = FALSE AND ? = TRUE THEN NOW(3) ELSE site_key_enforced_at END, require_site_key = ?, require_device_proof = ? WHERE id = 1`, policy.RequireSiteKey, policy.RequireSiteKey, policy.RequireDeviceProof); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "security_policy_failed"})
		return
	}
	_ = s.redis.Del(r.Context(), securityPolicyCacheKey).Err()
	policy, err := s.loadSecurityPolicy(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "security_policy_failed"})
		return
	}
	s.logger.Info("request security policy updated", "require_site_key", policy.RequireSiteKey, "require_device_proof", policy.RequireDeviceProof)
	writeJSON(w, http.StatusOK, policy)
}
