// Package kaynaklimit: domain başına cgroup + xfs_quota + MariaDB limitleri.
// Plan → domain eşleşmesinden alınan limitleri sistem seviyesinde uygular.
package kaynaklimit

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"girginospanel/internal/provisioner"
)

// Limitler: plan tablosundan okunan aktif değerler.
type Limitler struct {
	CPUYuzde         int
	RAMMB            int
	MaxProcess       int
	InodeKota        int
	IOAgirlik        int
	MySQLMaxBaglanti int
	DiskKotaMB       int
	PMMaxChildren    int // 0 = otomatik türet (max(4, ram_mb/64))
}

// planLimitleriGetir: domain'in bağlı olduğu plan'ın limitlerini döner.
// Plan atanmamışsa boş Limitler{0,...} — uygulama kaldırılır.
func PlanLimitleriGetir(ctx context.Context, db *sql.DB, domainID int64) (Limitler, error) {
	var l Limitler
	err := db.QueryRowContext(ctx, `
		SELECT COALESCE(p.cpu_yuzde,0), COALESCE(p.ram_mb,0),
		       COALESCE(p.max_process,0), COALESCE(p.inode_kota,0),
		       COALESCE(p.io_agirlik,100), COALESCE(p.mysql_max_baglanti,0),
		       COALESCE(p.disk_kota_mb,0), COALESCE(p.pm_max_children,0)
		FROM domains d LEFT JOIN service_plans p ON p.id=d.plan_id
		WHERE d.id=?`, domainID).
		Scan(&l.CPUYuzde, &l.RAMMB, &l.MaxProcess, &l.InodeKota,
			&l.IOAgirlik, &l.MySQLMaxBaglanti, &l.DiskKotaMB, &l.PMMaxChildren)
	return l, err
}

const sliceDir = "/etc/systemd/system"

func sliceName(sk string) string {
	// systemd slice — girginos-c_reg_kalici_test_local.slice
	return "girginos-" + sk + ".slice"
}

func slicePath(sk string) string {
	return filepath.Join(sliceDir, sliceName(sk))
}

// SystemdSliceYaz: /etc/systemd/system/girginos-<sk>.slice dosyasını yazar.
// CPUQuota, MemoryMax, TasksMax, IOWeight kural setini kullanır (cgroup v2).
func SystemdSliceYaz(sk string, l Limitler) error {
	content := fmt.Sprintf(`# GirginOSPanel per-domain resource slice — %s
[Unit]
Description=GirginOS panel slice for %s
Before=slices.target

[Slice]
CPUAccounting=yes
MemoryAccounting=yes
TasksAccounting=yes
IOAccounting=yes

CPUQuota=%d%%
MemoryMax=%dM
MemoryHigh=%dM
TasksMax=%d
IOWeight=%d
`, sk, sk,
		nonzero(l.CPUYuzde, 100),
		nonzero(l.RAMMB, 512),
		nonzero(l.RAMMB, 512)*90/100, // MemoryHigh = 90% of Max (soft throttle)
		nonzero(l.MaxProcess, 50),
		nonzero(l.IOAgirlik, 100))

	if err := os.WriteFile(slicePath(sk), []byte(content), 0644); err != nil {
		return fmt.Errorf("slice yaz: %w", err)
	}
	if out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("daemon-reload: %s: %w", strings.TrimSpace(string(out)), err)
	}
	// 🔴 CANLI GÜNCELLEME (süreç-öldürmez): slice zaten üye içeriyorsa (aktif),
	// yeni limitleri set-property --runtime ile ANINDA uygula. restart KULLANMA —
	// restart slice üyelerini (php-fpm worker'ları) öldürür. Dosya kalıcı kaynaktır;
	// slice inaktifse (ilk oluşturma) FPM servisi başlarken dosyadaki değerleri alır.
	if out, _ := exec.Command("systemctl", "is-active", sliceName(sk)).CombinedOutput(); strings.TrimSpace(string(out)) == "active" {
		cpu := nonzero(l.CPUYuzde, 100)
		mem := nonzero(l.RAMMB, 512)
		tasks := nonzero(l.MaxProcess, 50)
		io := nonzero(l.IOAgirlik, 100)
		if out, err := exec.Command("systemctl", "set-property", "--runtime", sliceName(sk),
			fmt.Sprintf("CPUQuota=%d%%", cpu),
			fmt.Sprintf("MemoryMax=%dM", mem),
			fmt.Sprintf("MemoryHigh=%dM", mem*90/100),
			fmt.Sprintf("TasksMax=%d", tasks),
			fmt.Sprintf("IOWeight=%d", io),
		).CombinedOutput(); err != nil {
			log.Printf("slice set-property %s: %s: %v", sk, strings.TrimSpace(string(out)), err)
		}
	}
	return nil
}

// SystemdSliceSil: kayıt varsa siler.
func SystemdSliceSil(sk string) error {
	p := slicePath(sk)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return nil
	}
	_ = os.Remove(p)
	_, _ = exec.Command("systemctl", "daemon-reload").CombinedOutput()
	return nil
}

// NOT (Batch5A): eski PHPFPMSlicePool kaldırıldı. FPM limit uygulaması artık TEK
// yazıcıda — provisioner.EnableTenantFPM (Seçenek A per-tenant php-fpm servisi +
// girginos-<sk>.slice cgroup üyeliği). kaynaklimit ARTIK pool dosyasına dokunmaz
// (yoksa php.go bir sonraki ayar kaydında yazdıklarımızı silerdi). Slice cgroup'u
// gerçek enforcement'ı sağlar; pool yalnız pm.* + php_admin_value taşır.

// hesaplaPMMaxChildren: plandan pm.max_children türetir. Plan değeri >0 ise onu,
// değilse ram_mb'den max(4, ram_mb/64) (RAM tavanı ile tutarlı → OOM-kill önler),
// hiç yoksa 8.
func hesaplaPMMaxChildren(l Limitler) int {
	if l.PMMaxChildren > 0 {
		return l.PMMaxChildren
	}
	if l.RAMMB > 0 {
		c := l.RAMMB / 64
		if c < 4 {
			c = 4
		}
		return c
	}
	return 8
}

// XFSKotaUygula: xfs_quota project quota (inode + blok) ile kullanıcı dizini kotalar.
// /home XFS ile mount olmalı ve pquota özelliği aktif.
func XFSKotaUygula(sk string, l Limitler) error {
	home := "/home/" + sk
	if _, err := os.Stat(home); os.IsNotExist(err) {
		return nil
	}
	// Project ID = uid (basit eşleme)
	uidOut, err := exec.Command("id", "-u", sk).Output()
	if err != nil {
		return fmt.Errorf("uid al: %w", err)
	}
	projID := strings.TrimSpace(string(uidOut))
	if projID == "" || projID == "0" {
		return fmt.Errorf("geçersiz uid: %s", projID)
	}

	// xfs_quota destekliyorsa dene (destek yoksa sessiz atla)
	// blok limit: KB cinsinden. disk_kota_mb * 1024 = KB.
	blokKB := l.DiskKotaMB * 1024
	inode := l.InodeKota
	if blokKB <= 0 && inode <= 0 {
		return nil
	}
	// Project mapping ekle (idempotent)
	line := fmt.Sprintf("%s:%s\n", projID, home)
	f, _ := os.OpenFile("/etc/projid", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if f != nil {
		defer f.Close()
		f.WriteString(line)
	}
	// project quota init (idempotent, hata yut)
	_ = exec.Command("xfs_quota", "-x", "-c",
		fmt.Sprintf("project -s -p %s %s", home, projID), "/home").Run()

	limit := fmt.Sprintf("limit -p bsoft=%dk bhard=%dk isoft=%d ihard=%d %s",
		blokKB, blokKB, inode, inode, projID)
	if out, err := exec.Command("xfs_quota", "-x", "-c", limit, "/home").CombinedOutput(); err != nil {
		// XFS quota özelliği yoksa (`pquota` mount opsiyonu eksikse) sessiz devam
		log.Printf("xfs_quota %s: %s (mount pquota aktif değil olabilir)", sk, strings.TrimSpace(string(out)))
	}
	return nil
}

// MySQLLimitUygula: DB kullanıcısına GRANT ... WITH MAX_USER_CONNECTIONS
func MySQLLimitUygula(sk string, l Limitler, mysqlDBUser string) error {
	if l.MySQLMaxBaglanti <= 0 {
		return nil
	}
	sqlCmd := fmt.Sprintf(
		"GRANT USAGE ON *.* TO '%s'@'localhost' WITH MAX_USER_CONNECTIONS %d;FLUSH PRIVILEGES;",
		mysqlDBUser, l.MySQLMaxBaglanti)
	cmd := exec.Command("mysql", "-uroot", "-e", sqlCmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mysql limit: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// UygulaHepsi: bir domain için plan'a göre TÜM limitleri uygular:
// slice (cgroup) + per-tenant FPM (Seçenek A, gerçek enforcement + CageFS izolasyon) +
// pm.max_children (plandan) + xfs + mysql.
func UygulaHepsi(ctx context.Context, db *sql.DB, domainID int64) error {
	var sk, dbUser, surum string
	if err := db.QueryRowContext(ctx,
		`SELECT sistem_kullanici, COALESCE(db_user,''), COALESCE(php_surum,'8.3') FROM domains WHERE id=?`, domainID).
		Scan(&sk, &dbUser, &surum); err != nil {
		return err
	}
	if sk == "" {
		return fmt.Errorf("sistem_kullanici boş")
	}
	l, err := PlanLimitleriGetir(ctx, db, domainID)
	if err != nil {
		return err
	}
	// Plan atanmamış? Per-tenant FPM'i geri al (paylaşılan düzene) + slice'ı sil.
	if l.CPUYuzde == 0 && l.RAMMB == 0 && l.MaxProcess == 0 {
		if provisioner.TenantFPMActive(sk) {
			if err := provisioner.RollbackToSharedFPM(db, domainID, sk, surum); err != nil {
				log.Printf("rollback shared fpm %s: %v", sk, err)
			}
		}
		_ = SystemdSliceSil(sk)
		return nil
	}
	// 1) slice (cgroup limitleri) — canlı, süreç-öldürmez.
	if err := SystemdSliceYaz(sk, l); err != nil {
		log.Printf("slice yaz %s: %v", sk, err)
	}
	// 2) pm.max_children'ı plandan php_settings'e sür (paylaşılan-mod tutarlılığı;
	//    per-tenant pool'u renderTenantPool zaten plandan hesaplar).
	pmc := hesaplaPMMaxChildren(l)
	if _, err := db.ExecContext(ctx,
		`UPDATE php_settings SET pm_max_children=? WHERE domain_id=?`, pmc, domainID); err != nil {
		log.Printf("php_settings pm_max_children %s: %v", sk, err)
	}
	// 3) 🔴 Seçenek A: per-tenant php-fpm servisi (slice üyeliği + CageFS sandbox).
	//    Limitler ancak böyle GERÇEKTEN enforce edilir. Başarısızlıkta EnableTenantFPM
	//    otomatik olarak paylaşılan düzene rollback eder → site asla düşmez.
	if _, err := provisioner.EnableTenantFPM(db, domainID, sk, surum); err != nil {
		log.Printf("per-tenant fpm %s: %v (paylaşılan düzende kalındı)", sk, err)
	}
	// 4) disk kotası (xfs) + MySQL bağlantı limiti.
	if err := XFSKotaUygula(sk, l); err != nil {
		log.Printf("xfs quota %s: %v", sk, err)
	}
	if dbUser != "" {
		if err := MySQLLimitUygula(sk, l, dbUser); err != nil {
			log.Printf("mysql limit %s: %v", sk, err)
		}
	}
	return nil
}

func nonzero(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

// planProbeHTTPS: domain'in nginx :443 üzerinden sağlık kodu (Host header, cert
// doğrulamasız). 0 = ulaşılamadı. Cutover öncesi/sonrası regresyon karşılaştırması için.
func planProbeHTTPS(alanAdi string) int {
	if alanAdi == "" {
		return 0
	}
	out, _ := exec.Command("curl", "-sk", "--max-time", "10",
		"-o", os.DevNull, "-w", "%{http_code}",
		"-H", "Host: "+alanAdi, "https://127.0.0.1/").Output()
	n, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return n
}

// servisAktif: systemd unit "active" mi.
func servisAktif(unit string) bool {
	out, _ := exec.Command("systemctl", "is-active", unit).CombinedOutput()
	return strings.TrimSpace(string(out)) == "active"
}

// HealTenantFPM: TÜM planlı domain'leri per-tenant FPM'e (Seçenek A) GÜVENLE + plan-driven
// migrate eder — mevcut (pre-Batch5A) müşterilerin cutover'ını otomatik tamamlar. Tenant
// başına BULLETPROOF:
//   - cutover ÖNCESİ baseline HTTP probe (nginx :443),
//   - UygulaHepsi (slice + per-tenant FPM + pm.max_children + xfs + mysql),
//   - cutover SONRASI probe,
//   - 🔴 REGRESYON: servis inaktif VEYA (baseline 2xx-4xx iken post 5xx) → otomatik
//     provisioner.RollbackToSharedFPM + slice sil → site paylaşılan düzende 200 kalır.
//
// İdempotent (migrate olanı atlar), SIRALI (thundering yok), ARKA PLANDA çağrılır
// (panel boot'unu bloklamaz). girginospanel-update her panel restart'ında tetikler →
// update için plan-driven cutover mekanizması. Plan atanmamış domain'e DOKUNMAZ.
func HealTenantFPM(ctx context.Context, db *sql.DB) {
	if db == nil {
		return
	}
	rows, err := db.QueryContext(ctx,
		`SELECT id, sistem_kullanici, COALESCE(php_surum,'8.3'), alan_adi
		 FROM domains WHERE plan_id IS NOT NULL ORDER BY id`)
	if err != nil {
		log.Printf("HealTenantFPM: domain listesi okunamadı: %v", err)
		return
	}
	type dom struct {
		id      int64
		sk      string
		php     string
		alanAdi string
	}
	var list []dom
	for rows.Next() {
		var d dom
		if e := rows.Scan(&d.id, &d.sk, &d.php, &d.alanAdi); e == nil {
			list = append(list, d)
		}
	}
	rows.Close()

	var migrated, zaten, rollback int
	for _, d := range list {
		select {
		case <-ctx.Done():
			log.Printf("HealTenantFPM: iptal (ctx) — %d migrate/%d zaten/%d rollback", migrated, zaten, rollback)
			return
		default:
		}
		if d.sk == "" || !strings.HasPrefix(d.sk, "c_") {
			continue
		}
		if provisioner.TenantFPMActive(d.sk) {
			zaten++ // zaten migrate; provisioner.EnsureTenantFPMOnStartup ayakta tutar
			continue
		}
		baseline := planProbeHTTPS(d.alanAdi)
		// Tam plan-driven uygulama: slice + per-tenant FPM + pm.max_children + xfs + mysql.
		if e := UygulaHepsi(ctx, db, d.id); e != nil {
			log.Printf("HealTenantFPM: %s UygulaHepsi hata: %v", d.sk, e)
		}
		// EnableTenantFPM başarısızlıkta kendi içinde rollback etmiş olabilir → unit yoksa
		// site zaten paylaşılan düzende (güvenli), bu tenant'ı atla.
		if !provisioner.TenantFPMActive(d.sk) {
			log.Printf("HealTenantFPM: %s cutover başarısız — paylaşılan düzende kaldı (güvenli)", d.sk)
			continue
		}
		time.Sleep(700 * time.Millisecond) // FPM ısınsın + nginx reload otursun
		aktif := servisAktif("php-fpm-" + d.sk + ".service")
		post := planProbeHTTPS(d.alanAdi)
		regresyon := baseline >= 200 && baseline < 500 && post >= 500
		if !aktif || regresyon {
			log.Printf("HealTenantFPM: %s REGRESYON (aktif=%v baseline=%d post=%d) → RollbackToSharedFPM",
				d.sk, aktif, baseline, post)
			if e := provisioner.RollbackToSharedFPM(db, d.id, d.sk, d.php); e != nil {
				log.Printf("HealTenantFPM: %s rollback HATA: %v", d.sk, e)
			}
			_ = SystemdSliceSil(d.sk)
			rollback++
			continue
		}
		log.Printf("HealTenantFPM: %s cutover OK (baseline=%d post=%d)", d.sk, baseline, post)
		migrated++
	}
	log.Printf("HealTenantFPM tamam: %d migrate / %d zaten-aktif / %d rollback (toplam %d planlı domain)",
		migrated, zaten, rollback, len(list))
}
