-- Per-domain SOA ayarları (düzenlenebilir). Yoksa kod tarafında default üretilir.
CREATE TABLE IF NOT EXISTS dns_soa (
  domain_id   BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  primary_ns  VARCHAR(255) NOT NULL DEFAULT '',
  hostmaster  VARCHAR(255) NOT NULL DEFAULT '',
  refresh     INT NOT NULL DEFAULT 3600,
  retry       INT NOT NULL DEFAULT 900,
  expire      INT NOT NULL DEFAULT 1209600,
  minimum     INT NOT NULL DEFAULT 3600,
  ttl         INT NOT NULL DEFAULT 3600,
  CONSTRAINT fk_dns_soa_domain FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
