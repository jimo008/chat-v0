package http

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jimo008/chat-v0/internal/config"
	"github.com/jimo008/chat-v0/internal/jobs"
	"github.com/jimo008/chat-v0/internal/migrations"
	"github.com/jimo008/chat-v0/internal/security"
)

func TestAutoReplyIntegration(t *testing.T) {
	dsn := os.Getenv("CHAT_V0_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("requires isolated MySQL test service")
	}
	if !strings.Contains(dsn, "/chat_v0_test?") {
		t.Fatal("dedicated test database required")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrations.Run(context.Background(), db, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE auto_reply_settings SET enabled = FALSE, content = '', generation = generation + 1, cooldown_seconds = 60 WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(`UPDATE auto_reply_settings SET enabled = FALSE, generation = generation + 1 WHERE id = 1`)
	suffix, _ := security.NewToken(5)
	execID := func(query string, args ...any) uint64 {
		t.Helper()
		result, err := db.Exec(query, args...)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := result.LastInsertId()
		return uint64(id)
	}
	siteID := execID(`INSERT INTO sites (site_key, name) VALUES (?, 'Auto reply test')`, "st_auto_"+suffix)
	customerID := execID(`INSERT INTO customers (site_id, normalized_email, email_original) VALUES (?, ?, ?)`, siteID, suffix+"@example.invalid", suffix+"@example.invalid")
	conversationID := execID(`INSERT INTO conversations (site_id, customer_id) VALUES (?, ?)`, siteID, customerID)
	agentID := execID(`INSERT INTO agents (username, email, password_hash) VALUES (?, ?, 'unused-test-hash')`, "auto-"+suffix, suffix+"@agent.invalid")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Config{AdminToken: "test-auto-admin", UploadStoragePath: t.TempDir()}
	s := NewServer(cfg, db, nil, logger)
	runner := jobs.NewRunner(cfg, db, logger)
	ctx := context.Background()
	scalar := func(query string, args ...any) int {
		t.Helper()
		var n int
		if err := db.QueryRow(query, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	countAuto := func() int {
		return scalar(`SELECT COUNT(*) FROM messages WHERE conversation_id = ? AND is_auto_reply = TRUE`, conversationID)
	}
	countPending := func() int {
		return scalar(`SELECT COUNT(*) FROM auto_reply_jobs WHERE conversation_id = ?`, conversationID)
	}
	update := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	due := func() {
		update(`UPDATE auto_reply_jobs SET due_at = NOW(3) - INTERVAL 1 SECOND WHERE conversation_id = ?`, conversationID)
	}
	expireCooldown := func() {
		update(`UPDATE auto_reply_state SET last_sent_at = NOW(3) - INTERVAL 61 SECOND WHERE conversation_id = ?`, conversationID)
	}
	process := func() {
		t.Helper()
		if _, err := runner.ProcessAutoReplies(ctx, 100); err != nil {
			t.Fatal(err)
		}
	}
	admin := func(method string, payload any, expected int) map[string]any {
		t.Helper()
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(method, "/api/v1/admin/auto-reply", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-auto-admin")
		rec := httptest.NewRecorder()
		s.Routes().ServeHTTP(rec, req)
		if rec.Code != expected {
			t.Fatalf("admin: %d %s", rec.Code, rec.Body.String())
		}
		var result map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &result)
		return result
	}
	customer := authenticatedCustomer{SiteID: siteID, CustomerID: customerID, ConversationID: conversationID, SiteKey: "st_auto_" + suffix}
	send := func(human bool, content, clientID string) {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"type": "text", "content": content, "client_msg_id": clientID})
		req := httptest.NewRequest("POST", "/messages", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		if human {
			req.SetPathValue("id", fmt.Sprint(conversationID))
			req = req.WithContext(context.WithValue(ctx, agentContextKey, authenticatedAgent{AgentID: agentID}))
			s.agentSendMessage(rec, req)
		} else {
			req = req.WithContext(context.WithValue(ctx, customerContextKey, customer))
			s.customerSendMessage(rec, req)
		}
		if rec.Code != 201 {
			t.Fatalf("send: %d %s", rec.Code, rec.Body.String())
		}
	}
	if result := admin("GET", nil, 200); result["enabled"] != false {
		t.Fatal("must default off")
	}
	admin("PATCH", map[string]any{"enabled": true}, 400)
	send(false, "while disabled", "")
	if countPending() != 0 {
		t.Fatal("disabled feature must not schedule")
	}
	body := "您好，已收到。\n引号 \" \\ <script> stays literal"
	admin("PATCH", map[string]any{"content": body}, 200)
	if result := admin("GET", nil, 200); result["enabled"] != false || result["content"] != body || result["footer"] != jobs.AutoReplyFooter {
		t.Fatal("setting body must not enable or alter it")
	}
	admin("PATCH", map[string]any{"enabled": true}, 200)
	send(false, "one", "idempotent-request")
	send(false, "one", "idempotent-request")
	send(false, "two", "")
	send(false, "three", "")
	if countPending() != 3 {
		t.Fatal("HTTP retry must not schedule twice")
	}
	process()
	if countAuto() != 0 {
		t.Fatal("must not reply before the three-second deadline")
	}
	due()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := runner.ProcessAutoReplies(ctx, 100); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	process()
	if countAuto() != 1 || countPending() != 0 {
		t.Fatalf("burst/multiple workers duplicated reply: %d pending %d", countAuto(), countPending())
	}
	var actual string
	if err := db.QueryRow(`SELECT content FROM messages WHERE conversation_id = ? AND is_auto_reply = TRUE`, conversationID).Scan(&actual); err != nil {
		t.Fatal(err)
	}
	if actual != body+"\n\n"+jobs.AutoReplyFooter {
		t.Fatalf("body/footer mismatch: %q", actual)
	}
	if scalar(`SELECT COUNT(*) FROM messages WHERE conversation_id = ? AND is_auto_reply = TRUE AND sender_agent_id IS NOT NULL`, conversationID) != 0 {
		t.Fatal("automatic message must not impersonate a human")
	}
	if scalar(`SELECT COUNT(*) FROM email_batches WHERE conversation_id = ?`, conversationID) != 0 {
		t.Fatal("automatic message created an email batch")
	}
	if scalar(`SELECT agent_last_seen_seq FROM conversations WHERE id = ?`, conversationID) != 0 || scalar(`SELECT COUNT(*) FROM messages WHERE conversation_id = ? AND sender_type = 'customer' AND agent_read_at IS NOT NULL`, conversationID) != 0 {
		t.Fatal("auto reply must not mark customers read")
	}
	send(false, "during cooldown", "")
	due()
	process()
	if countAuto() != 1 || countPending() != 0 {
		t.Fatal("cooldown must discard messages rather than postpone them")
	}
	expireCooldown()
	process()
	if countAuto() != 1 {
		t.Fatal("cooldown expiry alone must not trigger a reply")
	}
	send(false, "new after cooldown", "")
	admin("PATCH", map[string]any{"content": "修改后的正文"}, 200)
	due()
	process()
	if countAuto() != 2 {
		t.Fatal("fresh post-cooldown message should trigger")
	}
	if err := db.QueryRow(`SELECT content FROM messages WHERE conversation_id = ? AND is_auto_reply = TRUE ORDER BY id DESC LIMIT 1`, conversationID).Scan(&actual); err != nil {
		t.Fatal(err)
	}
	if actual != "修改后的正文\n\n"+jobs.AutoReplyFooter {
		t.Fatal("pending replies must use the current body")
	}
	expireCooldown()
	send(false, "pending at shutdown", "")
	admin("PATCH", map[string]any{"enabled": false}, 200)
	admin("PATCH", map[string]any{"enabled": true}, 200)
	due()
	process()
	if countAuto() != 2 || countPending() != 0 {
		t.Fatal("off/on must not revive pending jobs")
	}
	expireCooldown()
	send(false, "human will reply", "")
	send(true, "human reply", "")
	due()
	process()
	if countAuto() != 2 || countPending() != 0 {
		t.Fatal("human reply must cancel automatic reply")
	}
	expireCooldown()
	send(false, "read without a human reply", "")
	update(`UPDATE conversations SET agent_last_seen_seq = (SELECT MAX(seq) FROM messages WHERE conversation_id = ?) WHERE id = ?`, conversationID, conversationID)
	readBefore := scalar(`SELECT agent_last_seen_seq FROM conversations WHERE id = ?`, conversationID)
	callID := execID(`INSERT INTO emergency_calls (site_id, customer_id, conversation_id, status) VALUES (?, ?, ?, 'RINGING')`, siteID, customerID, conversationID)
	due()
	process()
	if countAuto() != 3 || scalar(`SELECT agent_last_seen_seq FROM conversations WHERE id = ?`, conversationID) != readBefore {
		t.Fatal("read-only activity must not cancel a reply or advance the read cursor")
	}
	var callStatus string
	if err := db.QueryRow(`SELECT status FROM emergency_calls WHERE id = ?`, callID).Scan(&callStatus); err != nil || callStatus != "RINGING" {
		t.Fatal("automatic replies must not accept emergency calls")
	}
	expireCooldown()
	var imageBody bytes.Buffer
	form := multipart.NewWriter(&imageBody)
	file, _ := form.CreateFormFile("file", "test.png")
	if err := png.Encode(file, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	_ = form.Close()
	req := httptest.NewRequest("POST", "/images", &imageBody).WithContext(context.WithValue(ctx, customerContextKey, customer))
	req.Header.Set("Content-Type", form.FormDataContentType())
	rec := httptest.NewRecorder()
	s.customerUploadImage(rec, req)
	if rec.Code != 201 || countPending() != 1 {
		t.Fatalf("image scheduling: %d %s", rec.Code, rec.Body.String())
	}
	due()
	process()
	if countAuto() != 4 {
		t.Fatal("customer images should trigger a reply")
	}
	result := admin("PATCH", map[string]any{"cooldown_minutes": 20}, 200)
	if result["cooldown_seconds"] != float64(1200) || result["cooldown_minutes"] != float64(20) || result["enabled"] != true || result["content"] != "修改后的正文" {
		t.Fatal("changing the cooldown must preserve content and enabled state")
	}
	for _, invalid := range []int{0, -1, 1441} {
		admin("PATCH", map[string]any{"cooldown_minutes": invalid}, 400)
	}
	if result := admin("GET", nil, 200); result["cooldown_seconds"] != float64(1200) {
		t.Fatal("invalid updates must leave the previous value intact")
	}
	update(`UPDATE auto_reply_state SET last_sent_at = NOW(3) - INTERVAL 19 MINUTE WHERE conversation_id = ?`, conversationID)
	send(false, "19 minutes is still in cooldown", "")
	due()
	process()
	if countAuto() != 4 || countPending() != 0 {
		t.Fatal("20 minute setting must suppress messages after 19 minutes")
	}
	update(`UPDATE auto_reply_state SET last_sent_at = NOW(3) - INTERVAL 21 MINUTE WHERE conversation_id = ?`, conversationID)
	process()
	if countAuto() != 4 {
		t.Fatal("20 minute expiry must not send without a new message")
	}
	send(false, "new message after 21 minutes", "")
	due()
	process()
	if countAuto() != 5 {
		t.Fatal("a new message after 20 minute cooldown must trigger")
	}
	// Even an accidentally linked automatic message must be excluded by the email worker.
	batchID := execID(`INSERT INTO email_batches (site_id, customer_id, conversation_id, send_after) VALUES (?, ?, ?, NOW(3))`, siteID, customerID, conversationID)
	update(`INSERT INTO email_batch_messages (batch_id, message_id) SELECT ?, id FROM messages WHERE conversation_id = ? AND is_auto_reply = TRUE`, batchID, conversationID)
	if _, err := runner.ProcessEmailBatches(ctx, 100); err != nil {
		t.Fatal(err)
	}
	var batchStatus string
	if err := db.QueryRow(`SELECT status FROM email_batches WHERE id = ?`, batchID).Scan(&batchStatus); err != nil || batchStatus != "skipped" {
		t.Fatalf("automatic message reached email pipeline: %s %v", batchStatus, err)
	}
	// No actual message contents or emergency status are changed by configuration.
	admin("PATCH", map[string]any{"content": ""}, 400)
	admin("PATCH", map[string]any{"enabled": false}, 200)
}
