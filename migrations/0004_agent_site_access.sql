CREATE TABLE agent_site_access (
  agent_id BIGINT UNSIGNED NOT NULL,
  site_id BIGINT UNSIGNED NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (agent_id, site_id),
  CONSTRAINT fk_agent_site_access_agent FOREIGN KEY (agent_id) REFERENCES agents(id),
  CONSTRAINT fk_agent_site_access_site FOREIGN KEY (site_id) REFERENCES sites(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

