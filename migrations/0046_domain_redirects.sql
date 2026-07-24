-- 0046 - tüm-domain URL yönlendirme (redirect) editörü.
-- Domain başına en fazla bir yönlendirme (source column UNIQUE) — path bazlı
-- yönlendirme v1 kapsamında değil, tüm istekler hedef_url'e 301/302 ile gider.
-- provisioner.renderAndReload her render'da bu tabloyu okuyup vhost'u buna göre
-- render eder (bkz. redirectVhostTmpl) — ayrı bir "uygula" adımı gerekmez.
CREATE TABLE IF NOT EXISTS domain_redirects (
  id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  domain_id  BIGINT UNSIGNED NOT NULL UNIQUE,
  hedef_url  VARCHAR(2048) NOT NULL,
  kod        SMALLINT UNSIGNED NOT NULL DEFAULT 301,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT fk_redirect_domain FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE
) ENGINE=InnoDB;
