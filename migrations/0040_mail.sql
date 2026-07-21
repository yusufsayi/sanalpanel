-- 0040 - native mail hosting (Postfix/Dovecot virtual mailbox tabloları).
-- Merkezi panel DB'de tutulur (per-tenant DB değil) — hem standart Postfix/Dovecot
-- "virtual mailbox in central DB" deseni budur, hem de panel DB zaten günlük
-- otomatik yedekleniyor (girginospanel-db-backup) — mail metadata'sı bedavaya biner.
-- Postfix/Dovecot bu tabloları CANLI MySQL sorgusuyla okur (mailro salt-okunur kullanıcı);
-- statik config yeniden üretimi gerekmez, bu yüzden değişiklikler anında etkilidir.

-- Domain başına mail opt-in + OS kimlik önbelleği (uid/gid mail-enable anında
-- os/user.Lookup ile bir kez çözülür — ftp_accounts.uid_n/gid_n ile aynı desen,
-- çünkü Postfix/Dovecot'un MySQL sorgu haritaları Go'nun user.Lookup'ını çağıramaz).
CREATE TABLE IF NOT EXISTS mail_domains (
  id               BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  domain_id        BIGINT UNSIGNED NOT NULL UNIQUE,
  alan_adi         VARCHAR(253) NOT NULL UNIQUE,
  sistem_kullanici VARCHAR(64) NOT NULL,
  uid_n            INT NOT NULL,
  gid_n            INT NOT NULL,
  maildir_root     VARCHAR(255) NOT NULL,
  dkim_selector    VARCHAR(32) NOT NULL DEFAULT 'default',
  durum            ENUM('active','suspended') NOT NULL DEFAULT 'active',
  created_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  KEY ix_mail_domains_durum (durum),
  CONSTRAINT fk_mail_domains_domain FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE
) ENGINE=InnoDB;

-- Tekil kutular (virtual users). Dovecot passdb/userdb bu tabloyu doğrudan sorgular.
CREATE TABLE IF NOT EXISTS mailboxes (
  id             BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  domain_id      BIGINT UNSIGNED NOT NULL,
  mail_domain_id BIGINT UNSIGNED NOT NULL,
  local_part     VARCHAR(64) NOT NULL,
  email          VARCHAR(320) NOT NULL UNIQUE,
  password_hash  VARCHAR(255) NOT NULL,
  maildir        VARCHAR(255) NOT NULL,
  quota_bytes    BIGINT UNSIGNED NOT NULL DEFAULT 0,
  status         ENUM('active','suspended') NOT NULL DEFAULT 'active',
  created_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uq_mailbox_domain_local (domain_id, local_part),
  KEY ix_mailbox_domain (domain_id),
  KEY ix_mailbox_status (status),
  CONSTRAINT fk_mailbox_domain FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE,
  CONSTRAINT fk_mailbox_maildomain FOREIGN KEY (mail_domain_id) REFERENCES mail_domains(id) ON DELETE CASCADE
) ENGINE=InnoDB;

-- Alias/forward (local_part@domain -> bir veya daha fazla hedef adres, virgülle ayrık).
CREATE TABLE IF NOT EXISTS mail_aliases (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  domain_id   BIGINT UNSIGNED NOT NULL,
  source      VARCHAR(320) NOT NULL UNIQUE,
  destination TEXT NOT NULL,
  status      ENUM('active','suspended') NOT NULL DEFAULT 'active',
  created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  KEY ix_alias_domain (domain_id),
  CONSTRAINT fk_alias_domain FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE
) ENGINE=InnoDB;

-- Kutu başına giden-posta izleme (kötüye kullanım tarayıcısı için — bkz. mail paketi).
CREATE TABLE IF NOT EXISTS mail_send_log (
  id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  mailbox_id BIGINT UNSIGNED NOT NULL,
  domain_id  BIGINT UNSIGNED NOT NULL,
  ts         TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  ok         TINYINT(1) NOT NULL DEFAULT 1,
  KEY ix_sendlog_mailbox_ts (mailbox_id, ts),
  CONSTRAINT fk_sendlog_domain FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE
) ENGINE=InnoDB;

-- Plan başına kutu-depolama kotası (disk_kota_mb deseniyle aynı). 0 = sınırsız / v1'de
-- kullanılmıyor — Maildir tenant home altında olduğu için mevcut XFS per-user kotasına
-- zaten biniyor; bu kolon ileride ayrı bir Dovecot quota-plugin geçişi için ayrılmıştır.
ALTER TABLE service_plans ADD COLUMN IF NOT EXISTS mailbox_quota_mb INT NOT NULL DEFAULT 0;
