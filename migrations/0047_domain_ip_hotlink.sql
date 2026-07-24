-- 0047 - hotlink koruması + domain bazlı IP izin/engel listesi.
-- Hotlink: domains üzerinde iki skaler kolon (subdomain/vhost_ozel_icerik ile aynı
-- "domain satırına doğrudan kolon" deseni) — ayrı tabloya gerek yok, tek kayıt.
ALTER TABLE domains ADD COLUMN IF NOT EXISTS hotlink_aktif TINYINT(1) NOT NULL DEFAULT 0;
ALTER TABLE domains ADD COLUMN IF NOT EXISTS hotlink_izinli TEXT NULL; -- virgülle ayrık ekstra izinli referrer domainleri

-- IP kuralları: gerçek bir liste (1:N) olduğu için ayrı tablo. Mod, domains'te tek
-- bir kolon (ip_erisim_modu) — liste sadece IP/CIDR tutar, "izin mi engel mi" anlamı
-- moddan gelir (bkz. internal/domainek benzeri "ana_domain_id" deseni: anlam ayrı,
-- taşıyıcı tablo ayrı).
ALTER TABLE domains ADD COLUMN IF NOT EXISTS ip_erisim_modu ENUM('kapali','engelle','izin_ver') NOT NULL DEFAULT 'kapali';

CREATE TABLE IF NOT EXISTS domain_ip_kurallari (
  id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  domain_id  BIGINT UNSIGNED NOT NULL,
  ip_cidr    VARCHAR(43) NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_domain_ip (domain_id, ip_cidr),
  CONSTRAINT fk_ipkural_domain FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE
) ENGINE=InnoDB;
