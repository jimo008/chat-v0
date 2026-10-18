package http

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

func conflictingSiteKey(r *http.Request, expected string) bool {
	for _, value := range []string{r.Header.Get("X-Chat-Site-Key"), r.URL.Query().Get("site_key"), r.URL.Query().Get("site")} {
		if value = strings.TrimSpace(value); value != "" && value != expected {
			return true
		}
	}
	return false
}

func (s *Server) validRequestSite(w http.ResponseWriter, r *http.Request, key string) bool {
	if key == "" || len(key) > 64 || conflictingSiteKey(r, key) {
		s.rejectSecurity(w, r, "invalid_site")
		return false
	}
	_, err := s.findSiteByKey(r.Context(), key)
	if err == sql.ErrNoRows {
		s.rejectSecurity(w, r, "invalid_site")
		return false
	}
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "site_lookup_unavailable"})
		return false
	}
	return true
}

func (s *Server) withCustomerSiteGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/customer/guest-login", "/api/v1/customer/xboard-login", "/api/v1/customer/email/send-code", "/api/v1/customer/email/verify":
			if r.Method == http.MethodPost {
				body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_customer_payload"})
					return
				}
				_ = r.Body.Close()
				r.Body = io.NopCloser(bytes.NewReader(body))
				var payload struct {
					SiteKey string `json:"site_key"`
				}
				if json.Unmarshal(body, &payload) != nil {
					s.rejectSecurity(w, r, "invalid_site")
					return
				}
				if !s.validRequestSite(w, r, strings.TrimSpace(payload.SiteKey)) {
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
