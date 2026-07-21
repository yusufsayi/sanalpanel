package mail

import (
	"context"
	"database/sql"
	"log"
	"os"
)

// HealMailOnStartup: her boot'ta çalışır. Postfix/Dovecot config dosyalarının varlığını
// doğrular (eksikse yalnız uyarı loglar — girginospanel-mail-setup henüz çalıştırılmamış
// olabilir, panel mail olmadan da ayakta kalmaya devam etmeli, diğer Heal* fonksiyonlarıyla
// aynı "asla fatal değil" üslup) ve aktif mail_domains satırlarının maildir_root'u diskte
// yoksa (ör. disk temizliği / yeni sunucuya taşıma) yeniden oluşturur.
func HealMailOnStartup(ctx context.Context, db *sql.DB) {
	must := []string{
		"/etc/postfix/mysql-virtual-domains.cf",
		"/etc/dovecot/dovecot-sql.conf.ext",
	}
	for _, p := range must {
		if _, err := os.Stat(p); err != nil {
			log.Printf("mail heal: %s eksik — girginospanel-mail-setup çalıştırılmamış olabilir", p)
		}
	}

	rows, err := db.QueryContext(ctx,
		`SELECT sistem_kullanici, uid_n, gid_n, maildir_root FROM mail_domains WHERE durum='active'`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var sk, root string
		var uid, gid int
		if err := rows.Scan(&sk, &uid, &gid, &root); err != nil {
			continue
		}
		if _, err := os.Stat(root); err != nil {
			_ = os.MkdirAll(root, 0o750)
			_ = os.Chown(root, uid, gid)
		}
	}
}

// EnsureInfra: boot'ta bir kez çağrılır (sshaccess.EnsureInfra ile aynı desen). v1'de
// HealMailOnStartup'ın yaptığı kontrolün ötesinde ek bir bootstrap gerekmiyor; ayrı
// tutulması ileride (ör. mailro DB kullanıcısının var olduğunu doğrulama) bir genişleme
// noktası bırakıyor.
func EnsureInfra() {}
