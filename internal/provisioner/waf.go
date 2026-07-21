// WAF (ModSecurity v3 + OWASP CRS) — per-domain / per-plan uygulama katmani.
//
// Tasarim:
//   - Modul yuklemesi GLOBAL fakat ZARARSIZ: bir vhost "modsecurity on" demedikce
//     hicbir sey degismez (per-domain opt-in). Bu yuzden WAF, mevcut siteleri hic
//     etkilemeden acilabilir.
//   - Efektif ayar = domain override (NULL/0 degilse) > plan varsayilani > kapali.
//   - Her vhost render'i (renderAndReload -> buildModSec) efektif ayardan:
//     (a) WAF aktif + modul yuklu ise per-domain modsec conf'unu TAZELER + server
//     bloguna "modsecurity on; modsecurity_rules_file <conf>;" enjekte eder,
//     (b) aktif ama modul YOK ise GRACEFUL atlar (loglar, vhost'u bozmaz),
//     (c) pasif ise per-domain conf'u temizler ve direktif enjekte etmez.
//     Boylece WAF her render'da kendini onarir; ayri "apply" cagrisi sart degil ama
//     WAFUygula(db, id) acik bir tetikleyici sunar.
package provisioner

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	wafModsecDir  = "/etc/nginx/modsec"
	wafDomainsDir = "/etc/nginx/modsec/domains"
	wafModulePath = "/usr/lib64/nginx/modules/ngx_http_modsecurity_module.so"
	wafNginxConf  = "/etc/nginx/nginx.conf"
)

// reWafSK: per-domain modsec conf dosya yolu path-injection guard'i. sk (sistem_kullanici)
// SlugFromDomain uretimi "c_" + [a-z0-9_]. Eslesmeyen sk icin WAF sessizce atlanir.
var reWafSK = regexp.MustCompile(`^c_[a-z0-9_]{1,60}$`)

// WAFModulYuklu: ModSecurity modulunun main-context'te YUKLU olup olmadigini bildirir.
// nginx -t'nin "modsecurity" direktifini taniyabilmesi icin (a) modul .so'su var VE
// (b) nginx.conf main-context'inde load_module edilmis olmali. Ikisi de saglanmadan
// vhost'a "modsecurity on" yazmak nginx -t'yi kirar → bu gate onu engeller.
func WAFModulYuklu() bool {
	if _, err := os.Stat(wafModulePath); err != nil {
		return false
	}
	b, err := os.ReadFile(wafNginxConf)
	if err != nil {
		return false
	}
	return strings.Contains(string(b), "ngx_http_modsecurity_module.so")
}

// WAFEfektif: domain'in efektif WAF ayarini (domain override + plan varsayilani) cozer.
// aktif=false ise WAF uygulanmaz. engine = "On" (engelle) | "DetectionOnly" (yalniz kaydet).
// paranoya 1..4 (clamp).
func WAFEfektif(db *sql.DB, sk string) (aktif bool, engine string, paranoya int) {
	paranoya = 1
	if db == nil {
		return false, "", paranoya
	}
	var dEn, dPL sql.NullInt64
	var dMode sql.NullString
	var pEn, pPL int
	var pMode string
	err := db.QueryRow(
		`SELECT d.waf_enabled, d.waf_mode, d.waf_paranoia,
		        COALESCE(p.waf_enabled,0), COALESCE(p.waf_mode,'on'), COALESCE(p.waf_paranoia,1)
		 FROM domains d LEFT JOIN service_plans p ON p.id = d.plan_id
		 WHERE d.sistem_kullanici = ?`, sk).
		Scan(&dEn, &dMode, &dPL, &pEn, &pMode, &pPL)
	if err != nil {
		return false, "", paranoya
	}
	enabled := pEn
	if dEn.Valid {
		enabled = int(dEn.Int64)
	}
	mode := strings.ToLower(strings.TrimSpace(pMode))
	if dMode.Valid && strings.TrimSpace(dMode.String) != "" {
		mode = strings.ToLower(strings.TrimSpace(dMode.String))
	}
	pl := pPL
	if dPL.Valid && dPL.Int64 > 0 {
		pl = int(dPL.Int64)
	}
	if pl < 1 {
		pl = 1
	}
	if pl > 4 {
		pl = 4
	}
	if enabled != 1 || mode == "off" || mode == "" {
		return false, "", pl
	}
	engine = "On"
	if mode == "detect" || mode == "detectiononly" {
		engine = "DetectionOnly"
	}
	return true, engine, pl
}

// wafDomainConfYaz: /etc/nginx/modsec/domains/<sk>.conf dosyasini uretir (engine + paranoya).
// Dosya, paylasilan modsecurity.conf + CRS crs-setup.conf + tum CRS kurallarini Include eder;
// SecRuleEngine ve paranoia seviyesini per-domain override eder. Bos bir <sk>.custom.conf da
// olusturulur → gelecekte per-domain ozel kurallar/haric-tutmalar icin hazir yapi (MVP: bos).
func wafDomainConfYaz(sk, engine string, paranoya int) error {
	if !reWafSK.MatchString(sk) {
		return fmt.Errorf("gecersiz sk: %q", sk)
	}
	if err := os.MkdirAll(wafDomainsDir, 0755); err != nil {
		return err
	}
	custom := filepath.Join(wafDomainsDir, sk+".custom.conf")
	if _, err := os.Stat(custom); err != nil {
		_ = os.WriteFile(custom,
			[]byte("# SanalPanel WAF — "+sk+" ozel kurallar / haric-tutmalar (opsiyonel, MVP: bos).\n"+
				"# Ornek CRS istisnasi: SecRuleRemoveById 942100\n"), 0644)
	}
	var b strings.Builder
	b.WriteString("# SanalPanel WAF — " + sk + " — OTOMATIK URETILDI, elle duzenlemeyin.\n")
	b.WriteString("# Mod + paranoya panelden yonetilir (domain override > plan varsayilani).\n")
	b.WriteString("Include " + wafModsecDir + "/modsecurity.conf\n")
	b.WriteString("SecRuleEngine " + engine + "\n")
	b.WriteString("Include " + wafModsecDir + "/crs/crs-setup.conf\n")
	// Paranoia seviyesini CRS kurallari (901-INITIALIZATION dahil) yuklenmeden ONCE ayarla.
	// crs-setup.conf.example'daki id:900000 paranoia SecAction'i VARSAYILAN OLARAK yorumdadir →
	// burada id:900000 kullanmak cakismaz ve CRS'in belgelenmis mekanizmasidir.
	b.WriteString(fmt.Sprintf(
		"SecAction \"id:900000,phase:1,pass,nolog,t:none,"+
			"setvar:tx.blocking_paranoia_level=%d,setvar:tx.detection_paranoia_level=%d\"\n",
		paranoya, paranoya))
	b.WriteString("Include " + wafModsecDir + "/crs/rules/*.conf\n")
	b.WriteString("Include " + custom + "\n")
	confPath := filepath.Join(wafDomainsDir, sk+".conf")
	return os.WriteFile(confPath, []byte(b.String()), 0644)
}

// buildModSec: vhost server-context'ine yazilacak WAF direktif blogunu doner (renderAndReload
// icinde her render'da cagirilir). Yan etki: WAF aktif+modul yuklu ise per-domain conf'u tazeler,
// pasif ise temizler. Vhost'u ASLA bozmaz — sorun varsa "" doner.
func buildModSec(sk string) string {
	if !reWafSK.MatchString(sk) {
		return "" // path-injection guard
	}
	confPath := filepath.Join(wafDomainsDir, sk+".conf")
	aktif, engine, paranoya := WAFEfektif(pkgDB, sk)
	if !aktif {
		_ = os.Remove(confPath) // pasifse per-domain conf'u temizle
		return ""
	}
	if !WAFModulYuklu() {
		log.Printf("waf: %s WAF-etkin ama ModSecurity modulu yuklu DEGIL — WAF ATLANDI (vhost saglam). 'sanalpanel-waf-setup' calistirin.", sk)
		return ""
	}
	if err := wafDomainConfYaz(sk, engine, paranoya); err != nil {
		log.Printf("waf: %s per-domain conf yazilamadi: %v — WAF ATLANDI (vhost saglam)", sk, err)
		return ""
	}
	return "    # ---- WAF (ModSecurity v3 + OWASP CRS) — panel'den yonetilir ----\n" +
		"    modsecurity on;\n" +
		"    modsecurity_rules_file " + confPath + ";\n"
}

// WAFUygula: bir domain'in WAF ayarini (domain override + plan varsayilani) yeniden uygular.
// per-domain modsec conf'u tazelenir + vhost yeniden render edilir (nginx -t gate + rollback).
// create + plan-degisimi + panel WAF ayar kaydi bu fonksiyonu cagirir.
func WAFUygula(db *sql.DB, domainID int64) error {
	return RerenderVhost(db, domainID)
}

// HealWAFOnStartup: acilista modul durumunu dogrular + WAF-etkin her (askida-olmayan) domain'in
// per-domain modsec conf'unu tazeler. Modul yuklu degilse UYARIR ve vhost'lara dokunmaz (graceful).
// Vhost'lar zaten HealVhostsOnStartup ile yeniden render edilir (buildModSec oradan da calisir);
// bu fonksiyon acik bir durum logu + conf tazeleme (belt-and-suspenders) saglar.
func HealWAFOnStartup() {
	if pkgDB == nil {
		return
	}
	_ = os.MkdirAll(wafDomainsDir, 0755)
	modul := WAFModulYuklu()
	rows, err := pkgDB.Query(`SELECT sistem_kullanici FROM domains WHERE COALESCE(askida,0)=0`)
	if err != nil {
		return
	}
	defer rows.Close()
	var aktifSayi int
	for rows.Next() {
		var sk string
		if rows.Scan(&sk) != nil {
			continue
		}
		aktif, engine, paranoya := WAFEfektif(pkgDB, sk)
		if !aktif {
			continue
		}
		aktifSayi++
		if !modul {
			continue // graceful: vhost render zaten WAF'i atlar
		}
		if err := wafDomainConfYaz(sk, engine, paranoya); err != nil {
			log.Printf("waf heal: %s conf yaz: %v", sk, err)
		}
	}
	if aktifSayi > 0 && !modul {
		log.Printf("waf heal: %d WAF-etkin domain var ama ModSecurity modulu YUKLU DEGIL — "+
			"'sanalpanel-waf-setup' calistirilmali (WAF su an PASIF, vhost'lar saglam)", aktifSayi)
	} else {
		log.Printf("waf heal: modul=%v, WAF-etkin domain=%d", modul, aktifSayi)
	}
}
