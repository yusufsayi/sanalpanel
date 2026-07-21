-- 0041 - domain başına özel (ham) vhost modu. Admin panelden tam nginx server{}
-- gövdesini kaydedebilir; vhost_ozel=1 iken renderAndReload şablonu hiç render etmeden
-- vhost_ozel_icerik'i birebir dosyaya yazar (bkz. internal/provisioner/provisioner.go).
ALTER TABLE domains ADD COLUMN IF NOT EXISTS vhost_ozel TINYINT(1) NOT NULL DEFAULT 0;
ALTER TABLE domains ADD COLUMN IF NOT EXISTS vhost_ozel_icerik TEXT NULL;
