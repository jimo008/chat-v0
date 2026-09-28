ALTER TABLE messages
  ADD COLUMN agent_read_at DATETIME(3) NULL AFTER customer_read_at,
  ADD KEY idx_messages_agent_unread (conversation_id, sender_type, agent_read_at);

