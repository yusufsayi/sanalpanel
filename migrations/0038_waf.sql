-- 0038 - WAF (ModSecurity v3 + OWASP CRS) plan + domain ayarlari
--
-- WAF, plan ve domain seviyesinde ac/kapa + ozellestirilebilir. Modul yuklemesi
-- global fakat ZARARSIZDIR: bir vhost "modsecurity on" demedikce davranis degismez
-- (per-domain opt-in). Bu migration yalnizca ayar kolonlarini ekler; modul + CRS
-- kurulumu ops script'i (girginospanel-waf-setup) ile yapilir.
--
-- SEMANTIK:
--   service_plans (plan varsayilani, NOT NULL):
--     waf_enabled  0/1   (0 = plan bu WAF'i kapali baslatir; guvenli varsayilan)
--     waf_mode     'on' | 'detect' | 'off'   ('on' = engelle, 'detect' = yalniz kaydet)
--     waf_paranoia 1..4  (CRS paranoia seviyesi; yuksek = daha siki + daha cok false-positive)
--   domains (per-domain OVERRIDE, NULL = plandan devral):
--     waf_enabled  NULL=devral / 0=kapali / 1=acik
--     waf_mode     NULL=devral / 'on' / 'detect' / 'off'
--     waf_paranoia NULL veya 0 = devral / 1..4 = override
--
-- Efektif deger = domain override (NULL/0 degilse) > plan degeri > (plan yoksa) kapali.
--
-- Idempotent: MariaDB 10.5+ ADD COLUMN IF NOT EXISTS destekler; migration her acilista
-- tekrar-guvenli calisir.

ALTER TABLE service_plans
  ADD COLUMN IF NOT EXISTS waf_enabled  TINYINT     NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS waf_mode     VARCHAR(10) NOT NULL DEFAULT 'on',
  ADD COLUMN IF NOT EXISTS waf_paranoia TINYINT     NOT NULL DEFAULT 1;

ALTER TABLE domains
  ADD COLUMN IF NOT EXISTS waf_enabled  TINYINT     NULL DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS waf_mode     VARCHAR(10) NULL DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS waf_paranoia TINYINT     NULL DEFAULT NULL;
