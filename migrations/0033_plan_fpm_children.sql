-- 0033 - service_plans: PHP-FPM pm.max_children (plan-driven, per-tenant FPM ile enforce)
-- 0 = otomatik türet: max(4, ram_mb/64) — RAM tavanı (MemoryMax) ile tutarlı, OOM-kill önler.
-- Idempotent: MariaDB 10.5+ ADD COLUMN IF NOT EXISTS destekler.

ALTER TABLE service_plans
  ADD COLUMN IF NOT EXISTS pm_max_children INT NOT NULL DEFAULT 0;
