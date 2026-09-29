package http

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jimo008/chat-v0/internal/security"
)

const maxImageBytes = 30 << 20

func (s *Server) customerUploadImage(w http.ResponseWriter, r *http.Request) {
	customer, ok := customerFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "customer_not_found"})
		return
	}
	if customer.Blocked {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "customer_blocked", "message": "当前无法使用在线客服，请通过其他联系方式联系我们。"})
		return
	}
	attachment, err := s.storeUploadedImage(r, customer.SiteKey, "customer", customer.CustomerID)
	if err != nil {
		s.logger.Error("customer image upload", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "image_failed"})
		return
	}
	defer tx.Rollback()

	attachmentID, err := s.insertAttachment(r.Context(), tx, customer.SiteID, attachment)
	if err != nil {
		s.logger.Error("insert customer attachment", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "attachment_failed"})
		return
	}
	message, err := s.insertImageMessage(r.Context(), tx, customer.SiteID, customer.ConversationID, "customer", customer.CustomerID, 0, attachmentID)
	if err != nil {
		s.logger.Error("insert customer image message", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "image_message_failed"})
		return
	}
	if _, _, err := createEvent(r.Context(), tx, customer.SiteID, "MESSAGE_CREATED", message); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "image_event_failed"})
		return
	}
	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "image_commit_failed"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"attachment_id": attachmentID, "message": message})
}

func (s *Server) agentUploadImage(w http.ResponseWriter, r *http.Request) {
	agent, ok := agentFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "agent_not_found"})
		return
	}
	conversationID, err := parsePathUint(r, "id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_conversation_id"})
		return
	}
	var siteID uint64
	var siteKey string
	if err := s.db.QueryRowContext(r.Context(), `SELECT conv.site_id, s.site_key FROM conversations conv JOIN sites s ON s.id = conv.site_id WHERE conv.id = ?`, conversationID).Scan(&siteID, &siteKey); err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation_not_found"})
		return
	} else if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "conversation_lookup_failed"})
		return
	}
	attachment, err := s.storeUploadedImage(r, siteKey, "agent", agent.AgentID)
	if err != nil {
		s.logger.Error("agent image upload", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "image_failed"})
		return
	}
	defer tx.Rollback()
	attachmentID, err := s.insertAttachment(r.Context(), tx, siteID, attachment)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "attachment_failed"})
		return
	}
	message, err := s.insertImageMessage(r.Context(), tx, siteID, conversationID, "agent", 0, agent.AgentID, attachmentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "image_message_failed"})
		return
	}
	if _, _, err := createEvent(r.Context(), tx, siteID, "MESSAGE_CREATED", message); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "image_event_failed"})
		return
	}
	if err := s.ensureEmailBatch(r.Context(), tx, siteID, conversationID, message.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "email_batch_failed"})
		return
	}
	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "image_commit_failed"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"attachment_id": attachmentID, "message": message})
}

type storedAttachment struct {
	UploaderType string
	UploaderID   uint64
	MIMEType     string
	SizeBytes    uint64
	StorageKey   string
	ThumbnailKey string
}

func (s *Server) storeUploadedImage(r *http.Request, siteKey, uploaderType string, uploaderID uint64) (storedAttachment, error) {
	if err := r.ParseMultipartForm(maxImageBytes + 1024); err != nil {
		s.logger.Error("parse multipart image upload", "error", err, "content_length", r.ContentLength, "content_type", r.Header.Get("Content-Type"), "site_key", siteKey, "uploader_type", uploaderType)
		return storedAttachment{}, fmt.Errorf("invalid_upload")
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return storedAttachment{}, fmt.Errorf("file_required")
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxImageBytes+1))
	if err != nil {
		return storedAttachment{}, fmt.Errorf("file_read_failed")
	}
	if len(data) == 0 || len(data) > maxImageBytes {
		s.logger.Info("image upload size rejected", "size", len(data), "filename", header.Filename, "site_key", siteKey, "uploader_type", uploaderType)
		return storedAttachment{}, fmt.Errorf("file_too_large")
	}
	mimeType, ext, ok := allowedImage(data, header.Filename)
	if !ok {
		s.logger.Info("image upload type rejected", "filename", header.Filename, "size", len(data), "detected", http.DetectContentType(data), "site_key", siteKey, "uploader_type", uploaderType)
		return storedAttachment{}, fmt.Errorf("unsupported_image_type")
	}

	token, err := security.NewToken(16)
	if err != nil {
		return storedAttachment{}, fmt.Errorf("storage_key_failed")
	}
	day := time.Now().Format("2006/01/02")
	relative := filepath.ToSlash(filepath.Join(siteKey, day, token+ext))
	fullPath := filepath.Join(s.cfg.UploadStoragePath, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0750); err != nil {
		return storedAttachment{}, fmt.Errorf("storage_prepare_failed")
	}
	if err := os.WriteFile(fullPath, data, 0640); err != nil {
		return storedAttachment{}, fmt.Errorf("storage_write_failed")
	}

	return storedAttachment{
		UploaderType: uploaderType,
		UploaderID:   uploaderID,
		MIMEType:     mimeType,
		SizeBytes:    uint64(len(data)),
		StorageKey:   relative,
		ThumbnailKey: relative,
	}, nil
}

func allowedImage(data []byte, filename string) (string, string, bool) {
	_ = filename
	if len(data) >= 3 && bytes.Equal(data[:3], []byte{0xff, 0xd8, 0xff}) {
		return "image/jpeg", ".jpg", true
	}
	if len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		return "image/png", ".png", true
	}
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "image/webp", ".webp", true
	}
	detected := http.DetectContentType(data)
	if strings.HasPrefix(detected, "image/jpeg") {
		return "image/jpeg", ".jpg", true
	}
	if strings.HasPrefix(detected, "image/png") {
		return "image/png", ".png", true
	}
	return "", "", false
}

func (s *Server) insertAttachment(ctx context.Context, tx *sql.Tx, siteID uint64, attachment storedAttachment) (uint64, error) {
	result, err := tx.ExecContext(
		ctx,
		`INSERT INTO attachments
		   (site_id, uploader_type, uploader_id, mime_type, size_bytes, storage_key, thumbnail_key)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		siteID,
		attachment.UploaderType,
		attachment.UploaderID,
		attachment.MIMEType,
		attachment.SizeBytes,
		attachment.StorageKey,
		attachment.ThumbnailKey,
	)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return uint64(id), err
}
