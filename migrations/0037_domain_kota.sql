-- 0037 - per-tenant XFS user quota: domain-seviye disk + inode kota OVERRIDE
-- CloudLinux paritesi: disk kotası + inode kotası XFS *user* quota ile per-tenant (c_<sk>)
-- uygulanır (kaynaklimit.KotaUygula). Dosyalar zaten c_<sk>:c_<sk> sahipli → user quota tam
-- eşleşir + tenant chown yapamadığı için kaçış-korumalı.
--
-- service_plans ZATEN disk_kota_mb (0006) + inode_kota (0011) taşır (plan-seviye default).
-- Burada domains'e per-domain OVERRIDE eklenir: 0 = plandan miras (override yok).
-- Efektif kota = domain override (>0) > plan değeri > (plan yoksa) varsayılan 5120MB / 500000.
--
-- Idempotent: MariaDB 10.5+ ADD COLUMN IF NOT EXISTS destekler; migration her açılışta
-- tekrar-güvenli çalışır.

ALTER TABLE domains
  ADD COLUMN IF NOT EXISTS disk_kota_mb INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS inode_kota   INT NOT NULL DEFAULT 0;
