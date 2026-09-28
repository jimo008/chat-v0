package http

import (
	"database/sql"
	"net/http"
	"path/filepath"
)

func (s *Server) customerAttachment(w http.ResponseWriter, r *http.Request) {
	customer, ok := customerFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "customer_not_found"})
		return
	}
	attachmentID, err := parsePathUint(r, "id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_attachment_id"})
		return
	}
	var storageKey, mimeType string
	err = s.db.QueryRowContext(r.Context(),
		`SELECT a.storage_key, a.mime_type
         FROM attachments a
         JOIN messages m ON m.attachment_id = a.id
         WHERE a.id = ? AND m.conversation_id = ? AND a.site_id = ?
         LIMIT 1`, attachmentID, customer.ConversationID, customer.SiteID).Scan(&storageKey, &mimeType)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "attachment_not_found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "attachment_lookup_failed"})
		return
	}
	serveAttachmentFile(w, r, s.cfg.UploadStoragePath, storageKey, mimeType)
}

func (s *Server) agentAttachment(w http.ResponseWriter, r *http.Request) {
	agent, ok := agentFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "agent_not_found"})
		return
	}
	attachmentID, err := parsePathUint(r, "id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_attachment_id"})
		return
	}
	var storageKey, mimeType string
	err = s.db.QueryRowContext(r.Context(),
		`SELECT a.storage_key, a.mime_type
         FROM attachments a
         JOIN agent_site_access asa ON asa.site_id = a.site_id
         WHERE a.id = ? AND asa.agent_id = ?
         LIMIT 1`, attachmentID, agent.AgentID).Scan(&storageKey, &mimeType)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "attachment_not_found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "attachment_lookup_failed"})
		return
	}
	serveAttachmentFile(w, r, s.cfg.UploadStoragePath, storageKey, mimeType)
}

func serveAttachmentFile(w http.ResponseWriter, r *http.Request, root, key, mimeType string) {
	clean := filepath.Clean(filepath.FromSlash(key))
	path := filepath.Join(root, clean)
	w.Header().Set("Content-Type", mimeType)
	http.ServeFile(w, r, path)
}
