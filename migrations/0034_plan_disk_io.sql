-- 0034 - service_plans: mutlak disk G/Ç limitleri (CloudLinux LVE io+iops eşdeğeri)
-- cgroup v2 io controller → systemd IO{Read,Write}BandwidthMax / IO{Read,Write}IOPSMax.
-- io_agirlik (IOWeight) göreli önceliktir; bunlar MUTLAK throttle'dır. 0 = sınırsız.
-- Idempotent: MariaDB 10.5+ ADD COLUMN IF NOT EXISTS destekler.

ALTER TABLE service_plans
  ADD COLUMN IF NOT EXISTS io_read_mbps   INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS io_write_mbps  INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS io_read_iops   INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS io_write_iops  INT NOT NULL DEFAULT 0;
