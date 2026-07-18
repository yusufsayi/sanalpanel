-- 0032_dns_dnssec.sql — DNSSEC opt-in bayrağı (varsayılan KAPALI).
-- dnssec_aktif=1 olan domain'lerde zone-include'a BIND "default" dnssec-policy eklenir
-- (inline-signing, CSK/ECDSAP256SHA256, CDS/CDNSKEY otomatik). Varsayılan 0 → mevcut
-- zone'lar deploy sonrası DEĞİŞMEZ; imzalama yalnız kullanıcı açıkça açınca başlar.
ALTER TABLE domains ADD COLUMN IF NOT EXISTS dnssec_aktif TINYINT(1) NOT NULL DEFAULT 0;
