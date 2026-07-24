-- 0045 - addon/parked domain: domains tablosuna kendine-referans "ana domain" kolonu.
-- ana_domain_id NULL ise normal, bağımsız bir domain (mevcut davranış). Doluysa bu satır
-- ana_domain_id'nin sistem kullanıcısını (sk) paylaşan bir "ek alan adı"dır — kendi Linux
-- kullanıcısı/PHP havuzu YOKTUR, ana domainin hesabı altında barınır.
-- parked=1 ise ek domain, ana domain ile AYNI docroot'u paylaşır (klasik "parked domain");
-- parked=0 ise kendi docroot'u vardır (klasik "addon domain").
--
-- KASITLI OLARAK FK CASCADE YOK: ana domain silinirken, ek domainlerin kendi nginx conf +
-- DNS zone dosyalarının diskten temizlenmesi internal/domains Delete() içinde elle
-- yapılmalı — otomatik bir DB-seviyesi CASCADE bu adımı atlar ve dosyaları diskte öksüz
-- bırakır (bkz. cli_tokens/domain_trafik'teki "orphan temizliği" deseniyle aynı ders).
ALTER TABLE domains ADD COLUMN IF NOT EXISTS ana_domain_id BIGINT UNSIGNED NULL;
ALTER TABLE domains ADD COLUMN IF NOT EXISTS parked TINYINT(1) NOT NULL DEFAULT 0;
ALTER TABLE domains ADD INDEX IF NOT EXISTS ix_domains_ana (ana_domain_id);
