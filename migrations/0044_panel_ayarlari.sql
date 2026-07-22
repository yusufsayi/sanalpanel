-- Panel geneli ayarlar (tek satır) — özel panel alan adı + otomatik Let's Encrypt durumu.
CREATE TABLE IF NOT EXISTS panel_ayarlari (
  id TINYINT UNSIGNED NOT NULL PRIMARY KEY DEFAULT 1,
  ozel_domain VARCHAR(255) NULL,
  ssl_durum ENUM('yok','aktif','basarisiz') NOT NULL DEFAULT 'yok',
  ssl_hata TEXT NULL,
  ssl_bitis DATE NULL,
  guncellenme TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
INSERT INTO panel_ayarlari (id) SELECT 1 FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM panel_ayarlari);
