package mail

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strconv"

	"girginospanel/internal/dns"
)

// MailUygula: bir domain için maili etkinleştirir (idempotent) — mail_domains satırını
// oluşturur/günceller, Maildir kök dizinini tenant kullanıcısına ait olarak hazırlar,
// DNS varsayılanlarını (MX/SPF/DMARC/DKIM) tohumlar. provisioner.WAFUygula ile aynı
// "küçük, tek-amaçlı, domain-create/plan-değişimi/açık-eylemden çağrılan" şeklini izler.
func MailUygula(ctx context.Context, db *sql.DB, domainID int64) error {
	var alanAdi, sk, ipv4 string
	if err := db.QueryRowContext(ctx,
		`SELECT alan_adi, sistem_kullanici, COALESCE(ipv4,'') FROM domains WHERE id=?`, domainID).
		Scan(&alanAdi, &sk, &ipv4); err != nil {
		return fmt.Errorf("domain bulunamadı: %w", err)
	}
	uid, gid, err := uidGid(sk)
	if err != nil {
		return fmt.Errorf("linux kullanıcı bulunamadı (%s): %w", sk, err)
	}
	maildirRoot := filepath.Join("/home", sk, "mail")
	if err := os.MkdirAll(maildirRoot, 0o750); err != nil {
		return fmt.Errorf("maildir kök dizini: %w", err)
	}
	_ = os.Chown(maildirRoot, uid, gid)

	if _, err := db.ExecContext(ctx,
		`INSERT INTO mail_domains(domain_id, alan_adi, sistem_kullanici, uid_n, gid_n, maildir_root)
		 VALUES(?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE sistem_kullanici=VALUES(sistem_kullanici), uid_n=VALUES(uid_n),
		   gid_n=VALUES(gid_n), maildir_root=VALUES(maildir_root), durum='active'`,
		domainID, alanAdi, sk, uid, gid, maildirRoot); err != nil {
		return fmt.Errorf("mail_domains kayıt: %w", err)
	}

	// MX/SPF/DMARC/DKIM domain oluşturulurken zaten SeedDefaults ile tohumlanmış olabilir
	// (idempotent COUNT(*) guard'lı, dns.go:338). Mail domain oluşturulduktan SONRA
	// etkinleştirilmişse burada tekrar çağırmak eksik kalan kayıtları/DKIM anahtarını tamamlar.
	if _, err := dns.SeedDefaults(ctx, db, domainID, alanAdi, ipv4); err != nil {
		log.Printf("mail: dns.SeedDefaults(%s): %v", alanAdi, err)
	}
	if err := dns.WriteZone(ctx, db, domainID); err != nil {
		log.Printf("mail: dns.WriteZone(%s): %v", alanAdi, err)
	}
	return nil
}

// MailKaldir: domain için maili DEVRE DIŞI bırakır (soft-disable) — mailboxes satırları
// SİLİNMEZ, sadece mail_domains.durum='suspended' olur. Postfix/Dovecot SQL sorguları
// zaten "durum/status='active'" filtrelediği için bu tek UPDATE anında hem gelen postayı
// reddeder hem SMTP AUTH'u keser — servis restart GEREKMEZ.
func MailKaldir(ctx context.Context, db *sql.DB, domainID int64) error {
	_, err := db.ExecContext(ctx, `UPDATE mail_domains SET durum='suspended' WHERE domain_id=?`, domainID)
	return err
}

// KapatDomain: domain SİLİNİRKEN çağrılır (domains.Delete, redis.KapatDomain ile aynı
// noktadan). mail_domains/mailboxes/mail_aliases hepsi domains(id)'e ON DELETE CASCADE
// FK ile bağlı, yani DB satırları zaten otomatik silinir — bu fonksiyon bugün no-op,
// yalnızca cascade-DIŞI bir yan etki (ör. ileride bir servis reload'u) gerekirse diye
// aynı çağrı noktasını (ve simetriyi) koruyan bir genişletme yeri.
func KapatDomain(db *sql.DB, domainID int64, sk string) {}

func uidGid(u string) (int, int, error) {
	uu, err := user.Lookup(u)
	if err != nil {
		return 0, 0, err
	}
	uid, err := strconv.Atoi(uu.Uid)
	if err != nil {
		return 0, 0, err
	}
	gid, err := strconv.Atoi(uu.Gid)
	if err != nil {
		return 0, 0, err
	}
	return uid, gid, nil
}
