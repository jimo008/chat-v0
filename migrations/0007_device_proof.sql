ALTER TABLE agent_devices ADD COLUMN public_key TEXT NULL;

CREATE TABLE agent_pairing_codes (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    code_hash CHAR(64) NOT NULL UNIQUE,
    expires_at DATETIME(3) NOT NULL,
    used_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE security_policy (
    id TINYINT UNSIGNED NOT NULL PRIMARY KEY,
    require_site_key BOOLEAN NOT NULL DEFAULT FALSE,
    require_device_proof BOOLEAN NOT NULL DEFAULT FALSE,
    site_key_enforced_at DATETIME(3) NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO security_policy (id) VALUES (1);
