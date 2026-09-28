package http

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type siteDTO struct {
	ID             uint64    `json:"id"`
	SiteKey        string    `json:"site_key"`
	Name           string    `json:"name"`
	DeploymentCode string    `json:"deployment_code"`
	CreatedAt      time.Time `json:"created_at"`
}

type createSiteRequest struct {
	Name string `json:"name"`
}

func (s *Server) createSite(w http.ResponseWriter, r *http.Request) {
	var req createSiteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name_required"})
		return
	}
	if len([]rune(name)) > 128 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name_too_long"})
		return
	}

	siteKey, err := s.createUniqueSiteKey(r.Context())
	if err != nil {
		s.logger.Error("create site key", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "site_key_failed"})
		return
	}

	result, err := s.db.ExecContext(r.Context(), `INSERT INTO sites (site_key, name) VALUES (?, ?)`, siteKey, name)
	if err != nil {
		s.logger.Error("insert site", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "site_create_failed"})
		return
	}
	id, _ := result.LastInsertId()

	writeJSON(w, http.StatusCreated, siteDTO{
		ID:             uint64(id),
		SiteKey:        siteKey,
		Name:           name,
		DeploymentCode: s.deploymentCode(siteKey),
		CreatedAt:      time.Now(),
	})
}

func (s *Server) listSites(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id, site_key, name, created_at FROM sites ORDER BY id ASC`)
	if err != nil {
		s.logger.Error("list sites", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "site_list_failed"})
		return
	}
	defer rows.Close()

	sites := make([]siteDTO, 0)
	for rows.Next() {
		var site siteDTO
		if err := rows.Scan(&site.ID, &site.SiteKey, &site.Name, &site.CreatedAt); err != nil {
			s.logger.Error("scan site", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "site_scan_failed"})
			return
		}
		site.DeploymentCode = s.deploymentCode(site.SiteKey)
		sites = append(sites, site)
	}
	if err := rows.Err(); err != nil {
		s.logger.Error("iterate sites", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "site_iter_failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"sites": sites})
}

func (s *Server) createUniqueSiteKey(ctx context.Context) (string, error) {
	for attempts := 0; attempts < 8; attempts++ {
		key, err := randomSiteKey()
		if err != nil {
			return "", err
		}
		var id uint64
		err = s.db.QueryRowContext(ctx, `SELECT id FROM sites WHERE site_key = ? LIMIT 1`, key).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return key, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("site key collision")
}

func randomSiteKey() (string, error) {
	buf := make([]byte, 5)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "st_" + hex.EncodeToString(buf), nil
}

func (s *Server) deploymentCode(siteKey string) string {
	return fmt.Sprintf(`<script src="%s/widget.js" data-site="%s"></script>`, strings.TrimRight(s.cfg.AppBaseURL, "/"), siteKey)
}
