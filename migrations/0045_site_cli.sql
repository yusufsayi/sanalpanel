-- 0045 - Site kullanicisi CLI komutlari: token tablosu + fastcgi cache-version sayaci
--
-- cli_tokens: her domain icin tek bir CLI bearer token'inin SHA-256 hash'ini tutar.
-- Ham token asla DB'ye yazilmaz — sadece /home/<sk>/.sanalpanel/token dosyasinda bulunur
-- (bkz. internal/cliapi paketi). domain_id PRIMARY KEY: domain basina tek token,
-- ON DUPLICATE KEY UPDATE ile rotasyon (cp_domain_redis ile ayni desen).
CREATE TABLE IF NOT EXISTS cli_tokens (
  domain_id  BIGINT NOT NULL PRIMARY KEY,
  token_hash CHAR(64) NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- cache_version: domain basina fastcgi cache-anahtarina gomulen sayac. "cache:purge"
-- CLI komutu bu sayaci artirip vhost'u yeniden render eder — diskte dosya SILMEZ,
-- eski cache girdileri sadece anahtar degistigi icin bir daha eslesmez (inactive=60m
-- ile kendiliginden temizlenir). Bkz. internal/provisioner/provisioner.go VhostOpts.CacheVersion.
ALTER TABLE domains
  ADD COLUMN IF NOT EXISTS cache_version INT NOT NULL DEFAULT 0;
