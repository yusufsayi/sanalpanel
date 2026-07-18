-- 0035 - service_plans: MySQL Governor (CloudLinux DB-limit eşdeğeri, native MariaDB)
-- mysql_max_baglanti (MAX_USER_CONNECTIONS) ZATEN var — bağlantı limiti olarak korunur.
-- 3 yeni alan (0 = sınırsız):
--   db_max_queries_per_hour  → ALTER USER ... MAX_QUERIES_PER_HOUR
--   db_max_updates_per_hour  → ALTER USER ... MAX_UPDATES_PER_HOUR
--   db_max_query_seconds     → yavaş-sorgu watchdog KILL eşiği (0 = öldürme yok)
-- Idempotent: MariaDB 10.5+ ADD COLUMN IF NOT EXISTS destekler.

ALTER TABLE service_plans
  ADD COLUMN IF NOT EXISTS db_max_queries_per_hour  INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS db_max_updates_per_hour  INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS db_max_query_seconds     INT NOT NULL DEFAULT 0;
