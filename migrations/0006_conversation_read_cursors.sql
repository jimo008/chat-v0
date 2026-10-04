ALTER TABLE conversations
  ADD COLUMN customer_last_seen_seq BIGINT UNSIGNED NOT NULL DEFAULT 0 AFTER last_message_at,
  ADD COLUMN customer_last_seen_at DATETIME(3) NULL AFTER customer_last_seen_seq,
  ADD COLUMN agent_last_seen_seq BIGINT UNSIGNED NOT NULL DEFAULT 0 AFTER customer_last_seen_at,
  ADD COLUMN agent_last_seen_at DATETIME(3) NULL AFTER agent_last_seen_seq;
