-- 0036 - db_accounts: bir DB-kullanicisinin BIRDEN COK veritabanina sahip olabilmesi icin
-- db_user UNIQUE kisitini kaldir. "Yeni Veritabani" modalinde musteri MEVCUT bir kullaniciyi
-- secip ona yeni bir DB baglayabilir (cPanel/Plesk modeli: 1 kullanici : N veritabani).
-- db_name UNIQUE korunur (her DB adi hala benzersiz). GRANT ve db_accounts satiri per-DB'dir.
-- Idempotent: MariaDB 10.5+ "DROP INDEX IF EXISTS" destekler; kisit yoksa sessiz gecer.
ALTER TABLE db_accounts DROP INDEX IF EXISTS db_user;
