package provisioner

import (
	"database/sql"
	"regexp"
	"strings"
)

// reHotlinkDomain: hotlink_izinli'deki ekstra referrer domainleri icin savunma-derinligi
// dogrulamasi (API katmaninda zaten dogrulanir — bu, "nginx valid_referers direktifine
// dogrudan gomulen bir DB alanindan gelen deger" icin ikinci bir guvenlik katmani).
var reHotlinkDomain = regexp.MustCompile(`^\*?\.?[a-zA-Z0-9.-]+$`)

// buildIPRules: domain'in ip_erisim_modu + domain_ip_kurallari'ndan server-context
// allow/deny blogunu uretir (renderAndReload icinde her render'da cagirilir).
// mod='kapali' veya hic kural yoksa "" doner (vhost'a hic dokunmaz — "izin_ver" modunda
// kural yokken sadece "deny all;" yazmak siteyi tamamen kilitlerdi, bu yuzden kural
// sayisi 0 ise mod ne olursa olsun devre disi sayilir).
func buildIPRules(sk string) string {
	if pkgDB == nil {
		return ""
	}
	var domainID int64
	var mod string
	err := pkgDB.QueryRow(
		`SELECT id, ip_erisim_modu FROM domains WHERE sistem_kullanici=? AND ana_domain_id IS NULL`, sk).
		Scan(&domainID, &mod)
	if err != nil || mod == "kapali" {
		return ""
	}
	rows, err := pkgDB.Query(`SELECT ip_cidr FROM domain_ip_kurallari WHERE domain_id=? ORDER BY id`, domainID)
	if err != nil {
		return ""
	}
	defer rows.Close()
	directive := "deny"
	if mod == "izin_ver" {
		directive = "allow"
	}
	var b strings.Builder
	n := 0
	for rows.Next() {
		var ip string
		if rows.Scan(&ip) == nil {
			b.WriteString("    " + directive + " " + ip + ";\n")
			n++
		}
	}
	if n == 0 {
		return ""
	}
	out := "    # ---- IP erisim kurallari (panel'den yonetilir) ----\n" + b.String()
	if mod == "izin_ver" {
		out += "    deny all;\n"
	}
	return out
}

// buildHotlink: hotlink korumasi aktifse resim uzantilari icin valid_referers location'u
// doner; degilse "" (vhost'a hic dokunmaz). Bu location, backend-spesifik location'lardan
// (php-fpm/apache/static) ONCE render edilir — nginx ayni regex'e uyan istekleri "dosyada
// once gorunen regex location" kuralina gore eslestirir; bu sayede resim uzantilari her
// zaman burada, referrer kontrolunden gecerek islenir.
func buildHotlink(sk, alanAdi string) string {
	if pkgDB == nil {
		return ""
	}
	var aktif int
	var izinli sql.NullString
	err := pkgDB.QueryRow(
		`SELECT COALESCE(hotlink_aktif,0), hotlink_izinli FROM domains WHERE sistem_kullanici=? AND ana_domain_id IS NULL`, sk).
		Scan(&aktif, &izinli)
	if err != nil || aktif == 0 {
		return ""
	}
	extra := ""
	if izinli.Valid {
		for _, d := range strings.Split(izinli.String, ",") {
			d = strings.TrimSpace(d)
			if d != "" && reHotlinkDomain.MatchString(d) {
				extra += " " + d
			}
		}
	}
	return "    location ~* \\.(jpg|jpeg|png|gif|webp|svg|ico|bmp)$ {\n" +
		"        valid_referers none blocked " + alanAdi + " *." + alanAdi + extra + ";\n" +
		"        if ($invalid_referer) { return 403; }\n" +
		"        try_files $uri =404;\n" +
		"    }\n"
}
