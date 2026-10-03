ALTER TABLE messages
  ADD COLUMN client_msg_id VARCHAR(128) NULL AFTER seq,
  ADD UNIQUE KEY uk_messages_site_client_msg (site_id, client_msg_id);
