// bakim.go — KALICI bakım modu.
// Kök neden (Bug 4): `wp maintenance-mode activate` bir `.maintenance` dosyası yazar;
// WordPress çekirdeği bu dosyayı 10 DAKİKA sonra OTOMATİK yok sayar (wp-includes/load.php
// içinde sabit 600 sn kontrolü) → bakım modu "kendiliğinden kapanıyor".
// Çözüm: süresi dolmayan bir mu-plugin + bayrak dosyası. Bayrak varken tüm ön-yüz istekleri
// 503 bakım sayfası döner; wp-admin/wp-login erişilebilir kalır. Çekirdek onarımı
// (core download --skip-content) wp-content'e dokunmadığı için kalıcıdır.
package wordpress

import (
	"os"
	"os/exec"
	"path/filepath"
)

// muPluginPHP: süresi dolmayan bakım modu mu-plugin'i.
const muPluginPHP = `<?php
/*
 * Plugin Name: SanalPanel Bakım Modu
 * Description: Panel tarafından yönetilen kalıcı bakım modu (WP 10dk auto-expiry'sini bypass eder).
 */
if (php_sapi_name() === 'cli') { return; }
$sanal_flag = __DIR__ . '/../.sanal-bakim';
if (!file_exists($sanal_flag)) { return; }
$sanal_uri = isset($_SERVER['REQUEST_URI']) ? $_SERVER['REQUEST_URI'] : '';
if (strpos($sanal_uri, '/wp-admin') !== false || strpos($sanal_uri, '/wp-login.php') !== false || strpos($sanal_uri, '/wp-cron.php') !== false) { return; }
if (!headers_sent()) {
    header($_SERVER['SERVER_PROTOCOL'] . ' 503 Service Unavailable', true, 503);
    header('Retry-After: 3600');
    header('Content-Type: text/html; charset=utf-8');
}
$sanal_msg = @file_get_contents($sanal_flag);
if (!$sanal_msg) { $sanal_msg = 'Sitemiz kısa süreli bakımdadır. Lütfen daha sonra tekrar deneyin.'; }
echo '<!doctype html><html lang="tr"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Bakım Modu</title>';
echo '<style>body{font-family:system-ui,Segoe UI,sans-serif;background:#f8fafc;display:flex;min-height:100vh;align-items:center;justify-content:center;margin:0}.k{max-width:520px;background:#fff;border:1px solid #e2e8f0;border-radius:16px;padding:48px;text-align:center;box-shadow:0 10px 25px rgba(0,0,0,.05)}.l{width:52px;height:52px;background:#ea580c;border-radius:12px;margin:0 auto 20px;display:flex;align-items:center;justify-content:center;color:#fff;font-size:26px}h1{font-size:22px;color:#0f172a;margin:0 0 10px}p{color:#64748b;line-height:1.6;margin:0}</style></head>';
echo '<body><div class="k"><div class="l">&#9881;</div><h1>Bakım Modu</h1><p>' . htmlspecialchars($sanal_msg, ENT_QUOTES, 'UTF-8') . '</p></div></body></html>';
exit;
`

// bakimYollari: dizin için mu-plugin ve bayrak dosyası yollarını döner.
func bakimYollari(dir string) (muDir, muFile, flag string) {
	wpContent := filepath.Join(dir, "wp-content")
	muDir = filepath.Join(wpContent, "mu-plugins")
	muFile = filepath.Join(muDir, "sanal-bakim.php")
	flag = filepath.Join(wpContent, ".sanal-bakim")
	return
}

// bakimAktif: bakım modu açık mı (bayrak dosyası var mı).
func bakimAktif(dir string) bool {
	_, _, flag := bakimYollari(dir)
	_, err := os.Stat(flag)
	return err == nil
}

// bakimAc: mu-plugin'i (yoksa) yazar ve bayrak dosyasını oluşturur. Kalıcıdır — süre dolmaz.
func bakimAc(sk, dir string) error {
	muDir, muFile, flag := bakimYollari(dir)
	if err := os.MkdirAll(muDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(muFile, []byte(muPluginPHP), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(flag, []byte("Sitemiz kısa süreli bakımdadır. Lütfen daha sonra tekrar deneyin."), 0o644); err != nil {
		return err
	}
	// domain kullanıcısına devret + SELinux bağlamı düzelt (php-fpm okuyabilsin)
	_ = exec.Command("chown", "-R", sk+":"+sk, muDir, flag).Run()
	_ = exec.Command("restorecon", "-R", muDir, flag).Run()
	return nil
}

// bakimKapat: bayrak dosyasını kaldırır (mu-plugin kalır ama bayrak olmadan atıl).
func bakimKapat(dir string) error {
	_, _, flag := bakimYollari(dir)
	if err := os.Remove(flag); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
