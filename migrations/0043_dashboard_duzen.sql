-- Kullanıcıya özel anasayfa (dashboard) widget düzeni — sürükle-bırak sırası (JSON metni).
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS dashboard_duzen TEXT NULL;
