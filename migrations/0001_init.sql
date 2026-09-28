CREATE TABLE sites (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  site_key VARCHAR(64) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE customers (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  site_id BIGINT UNSIGNED NOT NULL,
  normalized_email VARCHAR(320) NOT NULL,
  email_original VARCHAR(320) NOT NULL,
  xboard_user_id VARCHAR(128) NULL,
  canonical_customer_id BIGINT UNSIGNED NULL,
  blocked BOOLEAN NOT NULL DEFAULT FALSE,
  last_support_entry_type ENUM('web','app') NULL,
  last_support_entry_url TEXT NULL,
  last_active_at DATETIME(3) NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  CONSTRAINT fk_customers_site FOREIGN KEY (site_id) REFERENCES sites(id),
  CONSTRAINT fk_customers_canonical FOREIGN KEY (canonical_customer_id) REFERENCES customers(id),
  UNIQUE KEY uk_customers_site_email (site_id, normalized_email),
  UNIQUE KEY uk_customers_site_xboard (site_id, xboard_user_id),
  KEY idx_customers_site_active (site_id, last_active_at),
  KEY idx_customers_canonical (canonical_customer_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE customer_aliases (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  site_id BIGINT UNSIGNED NOT NULL,
  from_customer_id BIGINT UNSIGNED NOT NULL,
  to_customer_id BIGINT UNSIGNED NOT NULL,
  reason VARCHAR(64) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  CONSTRAINT fk_customer_aliases_site FOREIGN KEY (site_id) REFERENCES sites(id),
  CONSTRAINT fk_customer_aliases_from FOREIGN KEY (from_customer_id) REFERENCES customers(id),
  CONSTRAINT fk_customer_aliases_to FOREIGN KEY (to_customer_id) REFERENCES customers(id),
  UNIQUE KEY uk_customer_aliases_from (from_customer_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE customer_tokens (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  site_id BIGINT UNSIGNED NOT NULL,
  customer_id BIGINT UNSIGNED NOT NULL,
  token_hash CHAR(64) NOT NULL UNIQUE,
  device_label VARCHAR(128) NULL,
  revoked_at DATETIME(3) NULL,
  last_used_at DATETIME(3) NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  CONSTRAINT fk_customer_tokens_site FOREIGN KEY (site_id) REFERENCES sites(id),
  CONSTRAINT fk_customer_tokens_customer FOREIGN KEY (customer_id) REFERENCES customers(id),
  KEY idx_customer_tokens_customer (customer_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE xboard_profiles (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  site_id BIGINT UNSIGNED NOT NULL,
  customer_id BIGINT UNSIGNED NOT NULL,
  xboard_user_id VARCHAR(128) NOT NULL,
  email VARCHAR(320) NOT NULL,
  plan VARCHAR(128) NULL,
  expire_time DATETIME(3) NULL,
  used_traffic BIGINT UNSIGNED NULL,
  all_traffic BIGINT UNSIGNED NULL,
  raw_json JSON NULL,
  profile_updated_at DATETIME(3) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  CONSTRAINT fk_xboard_profiles_site FOREIGN KEY (site_id) REFERENCES sites(id),
  CONSTRAINT fk_xboard_profiles_customer FOREIGN KEY (customer_id) REFERENCES customers(id),
  UNIQUE KEY uk_xboard_profiles_customer (customer_id),
  UNIQUE KEY uk_xboard_profiles_site_user (site_id, xboard_user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE agents (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  username VARCHAR(64) NOT NULL UNIQUE,
  email VARCHAR(320) NOT NULL UNIQUE,
  password_hash VARCHAR(255) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE agent_devices (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  agent_id BIGINT UNSIGNED NOT NULL,
  device_id VARCHAR(128) NOT NULL,
  token_hash CHAR(64) NOT NULL UNIQUE,
  last_sync_at DATETIME(3) NULL,
  ws_connected BOOLEAN NOT NULL DEFAULT FALSE,
  app_foreground BOOLEAN NOT NULL DEFAULT FALSE,
  accept_emergency BOOLEAN NOT NULL DEFAULT FALSE,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  CONSTRAINT fk_agent_devices_agent FOREIGN KEY (agent_id) REFERENCES agents(id),
  UNIQUE KEY uk_agent_devices_device (agent_id, device_id),
  KEY idx_agent_devices_emergency (accept_emergency, last_sync_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE conversations (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  site_id BIGINT UNSIGNED NOT NULL,
  customer_id BIGINT UNSIGNED NOT NULL,
  last_message_id BIGINT UNSIGNED NULL,
  last_message_at DATETIME(3) NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  CONSTRAINT fk_conversations_site FOREIGN KEY (site_id) REFERENCES sites(id),
  CONSTRAINT fk_conversations_customer FOREIGN KEY (customer_id) REFERENCES customers(id),
  UNIQUE KEY uk_conversations_customer (site_id, customer_id),
  KEY idx_conversations_last_message (last_message_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE attachments (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  site_id BIGINT UNSIGNED NOT NULL,
  uploader_type ENUM('customer','agent') NOT NULL,
  uploader_id BIGINT UNSIGNED NOT NULL,
  mime_type VARCHAR(128) NOT NULL,
  size_bytes BIGINT UNSIGNED NOT NULL,
  storage_key VARCHAR(512) NOT NULL,
  thumbnail_key VARCHAR(512) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  CONSTRAINT fk_attachments_site FOREIGN KEY (site_id) REFERENCES sites(id),
  KEY idx_attachments_site_created (site_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE messages (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  site_id BIGINT UNSIGNED NOT NULL,
  conversation_id BIGINT UNSIGNED NOT NULL,
  sender_type ENUM('customer','agent') NOT NULL,
  sender_customer_id BIGINT UNSIGNED NULL,
  sender_agent_id BIGINT UNSIGNED NULL,
  seq BIGINT UNSIGNED NOT NULL UNIQUE,
  type ENUM('text','image') NOT NULL,
  content TEXT NULL,
  attachment_id BIGINT UNSIGNED NULL,
  customer_read_at DATETIME(3) NULL,
  email_notified_at DATETIME(3) NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  CONSTRAINT fk_messages_site FOREIGN KEY (site_id) REFERENCES sites(id),
  CONSTRAINT fk_messages_conversation FOREIGN KEY (conversation_id) REFERENCES conversations(id),
  CONSTRAINT fk_messages_customer_sender FOREIGN KEY (sender_customer_id) REFERENCES customers(id),
  CONSTRAINT fk_messages_agent_sender FOREIGN KEY (sender_agent_id) REFERENCES agents(id),
  CONSTRAINT fk_messages_attachment FOREIGN KEY (attachment_id) REFERENCES attachments(id),
  KEY idx_messages_conversation_created (conversation_id, created_at),
  KEY idx_messages_site_created (site_id, created_at),
  KEY idx_messages_email_pending (sender_type, customer_read_at, email_notified_at, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE conversations
  ADD CONSTRAINT fk_conversations_last_message FOREIGN KEY (last_message_id) REFERENCES messages(id);

CREATE TABLE events (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  event_id CHAR(36) NOT NULL UNIQUE,
  seq BIGINT UNSIGNED NOT NULL UNIQUE,
  site_id BIGINT UNSIGNED NOT NULL,
  type VARCHAR(64) NOT NULL,
  payload_json JSON NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  CONSTRAINT fk_events_site FOREIGN KEY (site_id) REFERENCES sites(id),
  KEY idx_events_site_seq (site_id, seq),
  KEY idx_events_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE emergency_calls (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  site_id BIGINT UNSIGNED NOT NULL,
  customer_id BIGINT UNSIGNED NOT NULL,
  conversation_id BIGINT UNSIGNED NOT NULL,
  status ENUM('RINGING','ACCEPTED','CANCELLED','EXPIRED') NOT NULL,
  accepted_by_agent_id BIGINT UNSIGNED NULL,
  accepted_by_device_id BIGINT UNSIGNED NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  accepted_at DATETIME(3) NULL,
  cancelled_at DATETIME(3) NULL,
  expired_at DATETIME(3) NULL,
  CONSTRAINT fk_emergency_calls_site FOREIGN KEY (site_id) REFERENCES sites(id),
  CONSTRAINT fk_emergency_calls_customer FOREIGN KEY (customer_id) REFERENCES customers(id),
  CONSTRAINT fk_emergency_calls_conversation FOREIGN KEY (conversation_id) REFERENCES conversations(id),
  CONSTRAINT fk_emergency_calls_agent FOREIGN KEY (accepted_by_agent_id) REFERENCES agents(id),
  CONSTRAINT fk_emergency_calls_device FOREIGN KEY (accepted_by_device_id) REFERENCES agent_devices(id),
  KEY idx_emergency_calls_status_created (status, created_at),
  KEY idx_emergency_calls_customer_status (customer_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE email_batches (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  site_id BIGINT UNSIGNED NOT NULL,
  customer_id BIGINT UNSIGNED NOT NULL,
  conversation_id BIGINT UNSIGNED NOT NULL,
  status ENUM('pending','sent','skipped','failed') NOT NULL DEFAULT 'pending',
  send_after DATETIME(3) NOT NULL,
  sent_at DATETIME(3) NULL,
  return_entry_type ENUM('web','app') NULL,
  return_url TEXT NULL,
  retry_count INT UNSIGNED NOT NULL DEFAULT 0,
  last_error TEXT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  CONSTRAINT fk_email_batches_site FOREIGN KEY (site_id) REFERENCES sites(id),
  CONSTRAINT fk_email_batches_customer FOREIGN KEY (customer_id) REFERENCES customers(id),
  CONSTRAINT fk_email_batches_conversation FOREIGN KEY (conversation_id) REFERENCES conversations(id),
  KEY idx_email_batches_pending (status, send_after)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE email_batch_messages (
  batch_id BIGINT UNSIGNED NOT NULL,
  message_id BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (batch_id, message_id),
  CONSTRAINT fk_email_batch_messages_batch FOREIGN KEY (batch_id) REFERENCES email_batches(id),
  CONSTRAINT fk_email_batch_messages_message FOREIGN KEY (message_id) REFERENCES messages(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE verification_codes (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  site_id BIGINT UNSIGNED NOT NULL,
  normalized_email VARCHAR(320) NOT NULL,
  code_hash CHAR(64) NOT NULL,
  attempt_count INT UNSIGNED NOT NULL DEFAULT 0,
  expires_at DATETIME(3) NOT NULL,
  used_at DATETIME(3) NULL,
  invalidated_at DATETIME(3) NULL,
  created_ip VARCHAR(64) NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  CONSTRAINT fk_verification_codes_site FOREIGN KEY (site_id) REFERENCES sites(id),
  KEY idx_verification_codes_lookup (site_id, normalized_email, created_at),
  KEY idx_verification_codes_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
