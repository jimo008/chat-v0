ALTER TABLE messages ADD COLUMN is_auto_reply BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE auto_reply_settings (
  id TINYINT UNSIGNED NOT NULL PRIMARY KEY,
  enabled BOOLEAN NOT NULL DEFAULT FALSE,
  content TEXT NOT NULL,
  generation BIGINT UNSIGNED NOT NULL DEFAULT 1,
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
INSERT INTO auto_reply_settings (id, content) VALUES (1, '');

CREATE TABLE auto_reply_jobs (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  site_id BIGINT UNSIGNED NOT NULL,
  conversation_id BIGINT UNSIGNED NOT NULL,
  trigger_message_seq BIGINT UNSIGNED NOT NULL UNIQUE,
  generation BIGINT UNSIGNED NOT NULL,
  triggered_at DATETIME(3) NOT NULL,
  due_at DATETIME(3) NOT NULL,
  KEY idx_auto_reply_due (due_at, id),
  CONSTRAINT fk_auto_reply_job_conversation FOREIGN KEY (conversation_id) REFERENCES conversations(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE auto_reply_state (
  conversation_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  last_sent_at DATETIME(3) NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
